package kiss

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

const RegistryLockSchemaVersion = 1

type ConfigFile struct {
	Registries map[string]RegistryConfig `toml:"registries"`
}

type RegistryConfig struct {
	URL              string   `toml:"url"`
	RequireSignature bool     `toml:"require_signature"`
	TrustedKeys      []string `toml:"trusted_keys"`
}

type RegistryIndex struct {
	Skills map[string]RegistrySkill `toml:"skills"`
}

type RegistrySkill struct {
	Source string `toml:"source"`
	Repo   string `toml:"repo"`
	Path   string `toml:"path"`
	Ref    string `toml:"ref"`
	URL    string `toml:"url"`
	SHA256 string `toml:"sha256"`

	PublicKey string `toml:"public_key"`
	Signature string `toml:"signature"`
}

type RegistryLock struct {
	SchemaVersion int                          `json:"schema_version"`
	Skills        map[string]RegistryLockEntry `json:"skills"`
}

type RegistryLockEntry struct {
	Name       string `json:"name"`
	Registry   string `json:"registry"`
	SourceSpec string `json:"source_spec"`
	SHA256     string `json:"sha256"`
	ResolvedAt string `json:"resolved_at"`

	SignatureVerified bool   `json:"signature_verified"`
	PublicKeySHA256   string `json:"public_key_sha256,omitempty"`
}

func AddRegistry(paths Paths, name, registryURL string) error {
	if err := ValidateSkillName(name); err != nil {
		return err
	}
	normalizedURL, err := normalizeRegistryURL(registryURL)
	if err != nil {
		return err
	}
	if err := paths.EnsureBase(); err != nil {
		return err
	}
	config, err := LoadConfig(paths)
	if err != nil {
		return err
	}
	registry := config.Registries[name]
	registry.URL = normalizedURL
	config.Registries[name] = registry
	return SaveConfig(paths, config)
}

func RequireRegistrySignature(paths Paths, name string) error {
	if err := ValidateSkillName(name); err != nil {
		return err
	}
	if err := paths.EnsureBase(); err != nil {
		return err
	}
	config, err := LoadConfig(paths)
	if err != nil {
		return err
	}
	registry, ok := config.Registries[name]
	if !ok {
		return fmt.Errorf("registry %q is not configured", name)
	}
	registry.RequireSignature = true
	config.Registries[name] = registry
	return SaveConfig(paths, config)
}

func TrustRegistryKey(paths Paths, name, publicKeyBase64 string) error {
	if err := ValidateSkillName(name); err != nil {
		return err
	}
	fingerprint, err := registryPublicKeyFingerprint(publicKeyBase64)
	if err != nil {
		return err
	}
	if err := paths.EnsureBase(); err != nil {
		return err
	}
	config, err := LoadConfig(paths)
	if err != nil {
		return err
	}
	registry, ok := config.Registries[name]
	if !ok {
		return fmt.Errorf("registry %q is not configured", name)
	}
	if !containsString(registry.TrustedKeys, fingerprint) {
		registry.TrustedKeys = append(registry.TrustedKeys, fingerprint)
		sort.Strings(registry.TrustedKeys)
	}
	config.Registries[name] = registry
	return SaveConfig(paths, config)
}

func ListRegistries(paths Paths, out io.Writer) error {
	if err := paths.EnsureBase(); err != nil {
		return err
	}
	config, err := LoadConfig(paths)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(config.Registries))
	for name := range config.Registries {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		registry := config.Registries[name]
		fmt.Fprintf(out, "%s\t%s\trequire_signature=%v\ttrusted_keys=%d\n", name, registry.URL, registry.RequireSignature, len(registry.TrustedKeys))
	}
	return nil
}

func AddRegistrySkill(paths Paths, name string) error {
	entry, err := ResolveRegistrySkill(paths, name)
	if err != nil {
		return err
	}
	if err := installSourceSpec(paths, entry.SourceSpec, name, entry.SHA256); err != nil {
		return err
	}
	return UpsertRegistryLockEntry(paths, entry)
}

func ResolveRegistrySkill(paths Paths, name string) (RegistryLockEntry, error) {
	if err := ValidateSkillName(name); err != nil {
		return RegistryLockEntry{}, err
	}
	if err := paths.EnsureBase(); err != nil {
		return RegistryLockEntry{}, err
	}
	config, err := LoadConfig(paths)
	if err != nil {
		return RegistryLockEntry{}, err
	}
	if len(config.Registries) == 0 {
		return RegistryLockEntry{}, fmt.Errorf("no registries configured; run kiss registry add <name> <url-or-path> first")
	}
	registryNames := make([]string, 0, len(config.Registries))
	for registryName := range config.Registries {
		registryNames = append(registryNames, registryName)
	}
	sort.Strings(registryNames)
	var registryErrors []string
	for _, registryName := range registryNames {
		registryConfig := config.Registries[registryName]
		index, err := LoadRegistryIndex(registryConfig.URL)
		if err != nil {
			registryErrors = append(registryErrors, fmt.Sprintf("%s: %v", registryName, err))
			continue
		}
		skill, ok := index.Skills[name]
		if !ok {
			continue
		}
		sourceSpec, err := skill.SourceSpec()
		if err != nil {
			registryErrors = append(registryErrors, fmt.Sprintf("%s: skill %q: %v", registryName, name, err))
			continue
		}
		signatureVerified, publicKeySHA256, err := skill.VerifySignature(name, sourceSpec)
		if err != nil {
			registryErrors = append(registryErrors, fmt.Sprintf("%s: skill %q: %v", registryName, name, err))
			continue
		}
		if err := checkRegistryTrustPolicy(registryName, registryConfig, signatureVerified, publicKeySHA256); err != nil {
			registryErrors = append(registryErrors, fmt.Sprintf("%s: skill %q: %v", registryName, name, err))
			continue
		}
		return RegistryLockEntry{
			Name:              name,
			Registry:          registryName,
			SourceSpec:        sourceSpec,
			SHA256:            skill.SHA256,
			ResolvedAt:        time.Now().UTC().Format(time.RFC3339),
			SignatureVerified: signatureVerified,
			PublicKeySHA256:   publicKeySHA256,
		}, nil
	}
	if len(registryErrors) > 0 {
		return RegistryLockEntry{}, fmt.Errorf("skill %q not found in configured registries; registry errors: %s", name, strings.Join(registryErrors, "; "))
	}
	return RegistryLockEntry{}, fmt.Errorf("skill %q not found in configured registries", name)
}

func LoadConfig(paths Paths) (ConfigFile, error) {
	config := ConfigFile{Registries: map[string]RegistryConfig{}}
	if _, err := os.Stat(paths.Config); err != nil {
		if os.IsNotExist(err) {
			return config, nil
		}
		return ConfigFile{}, err
	}
	if _, err := toml.DecodeFile(paths.Config, &config); err != nil {
		return ConfigFile{}, err
	}
	if config.Registries == nil {
		config.Registries = map[string]RegistryConfig{}
	}
	return config, nil
}

func SaveConfig(paths Paths, config ConfigFile) error {
	if config.Registries == nil {
		config.Registries = map[string]RegistryConfig{}
	}
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(config); err != nil {
		return err
	}
	return writeFileAtomic(paths.Home, paths.Config, buf.Bytes(), "config-*.toml")
}

func LoadRegistryIndex(registryURL string) (RegistryIndex, error) {
	data, err := readRegistryBytes(registryURL)
	if err != nil {
		return RegistryIndex{}, err
	}
	var index RegistryIndex
	if _, err := toml.Decode(string(data), &index); err != nil {
		return RegistryIndex{}, err
	}
	if index.Skills == nil {
		index.Skills = map[string]RegistrySkill{}
	}
	return index, nil
}

func checkRegistryTrustPolicy(registryName string, config RegistryConfig, signatureVerified bool, publicKeySHA256 string) error {
	if config.RequireSignature && !signatureVerified {
		return fmt.Errorf("registry %q requires signed entries", registryName)
	}
	if len(config.TrustedKeys) == 0 {
		return nil
	}
	if !signatureVerified {
		return fmt.Errorf("registry %q requires a signature from a trusted key", registryName)
	}
	if !containsTrustedKey(config.TrustedKeys, publicKeySHA256) {
		return fmt.Errorf("registry %q signed by untrusted key %s", registryName, publicKeySHA256)
	}
	return nil
}

func containsTrustedKey(items []string, fingerprint string) bool {
	normalized := strings.ToLower(fingerprint)
	for _, item := range items {
		if strings.ToLower(item) == normalized {
			return true
		}
	}
	return false
}

func (skill RegistrySkill) SourceSpec() (string, error) {
	switch {
	case strings.HasPrefix(skill.Source, "github:"):
		return skill.Source, nil
	case strings.HasPrefix(skill.Source, "https://"):
		return skill.Source, nil
	case skill.Source == "github":
		if skill.Repo == "" {
			return "", fmt.Errorf("github registry entry requires repo")
		}
		spec := "github:" + skill.Repo
		if skill.Path != "" {
			if err := validateSafeRelativePath(skill.Path); err != nil {
				return "", fmt.Errorf("path %q must be safe relative path: %w", skill.Path, err)
			}
			spec += "/" + skill.Path
		}
		if skill.Ref != "" {
			spec += "#" + skill.Ref
		}
		return spec, nil
	case skill.Source == "https" || skill.Source == "":
		if skill.URL == "" {
			return "", fmt.Errorf("https registry entry requires url")
		}
		if _, err := url.ParseRequestURI(skill.URL); err != nil {
			return "", err
		}
		if !strings.HasPrefix(skill.URL, "https://") {
			return "", fmt.Errorf("registry URL must use https: %s", skill.URL)
		}
		return skill.URL, nil
	default:
		return "", fmt.Errorf("unsupported registry source %q", skill.Source)
	}
}

func (skill RegistrySkill) VerifySignature(name, sourceSpec string) (bool, string, error) {
	if skill.PublicKey == "" && skill.Signature == "" {
		return false, "", nil
	}
	if skill.PublicKey == "" || skill.Signature == "" {
		return false, "", fmt.Errorf("registry signature requires both public_key and signature")
	}
	if skill.SHA256 == "" {
		return false, "", fmt.Errorf("signed registry entry requires sha256")
	}
	publicKey, err := base64.StdEncoding.DecodeString(skill.PublicKey)
	if err != nil {
		return false, "", fmt.Errorf("decode public_key: %w", err)
	}
	if len(publicKey) != ed25519.PublicKeySize {
		return false, "", fmt.Errorf("public_key must decode to %d bytes", ed25519.PublicKeySize)
	}
	signature, err := base64.StdEncoding.DecodeString(skill.Signature)
	if err != nil {
		return false, "", fmt.Errorf("decode signature: %w", err)
	}
	if len(signature) != ed25519.SignatureSize {
		return false, "", fmt.Errorf("signature must decode to %d bytes", ed25519.SignatureSize)
	}
	payload := registrySignaturePayload(name, sourceSpec, skill.SHA256)
	if !ed25519.Verify(ed25519.PublicKey(publicKey), payload, signature) {
		return false, "", fmt.Errorf("registry signature verification failed")
	}
	fingerprint := sha256.Sum256(publicKey)
	return true, hex.EncodeToString(fingerprint[:]), nil
}

func registryPublicKeyFingerprint(publicKeyBase64 string) (string, error) {
	publicKey, err := base64.StdEncoding.DecodeString(publicKeyBase64)
	if err != nil {
		return "", fmt.Errorf("decode public key: %w", err)
	}
	if len(publicKey) != ed25519.PublicKeySize {
		return "", fmt.Errorf("public key must decode to %d bytes", ed25519.PublicKeySize)
	}
	fingerprint := sha256.Sum256(publicKey)
	return hex.EncodeToString(fingerprint[:]), nil
}

func registrySignaturePayload(name, sourceSpec, sha256Value string) []byte {
	payload := "kiss-registry-entry-v1\n"
	payload += "name=" + name + "\n"
	payload += "source=" + sourceSpec + "\n"
	payload += "sha256=" + strings.ToLower(sha256Value) + "\n"
	return []byte(payload)
}

func LoadRegistryLock(paths Paths) (RegistryLock, error) {
	lock := RegistryLock{SchemaVersion: RegistryLockSchemaVersion, Skills: map[string]RegistryLockEntry{}}
	if _, err := os.Stat(paths.RegistryLock); err != nil {
		if os.IsNotExist(err) {
			return lock, nil
		}
		return RegistryLock{}, err
	}
	data, err := os.ReadFile(paths.RegistryLock)
	if err != nil {
		return RegistryLock{}, err
	}
	if err := json.Unmarshal(data, &lock); err != nil {
		return RegistryLock{}, err
	}
	if lock.SchemaVersion == 0 {
		lock.SchemaVersion = RegistryLockSchemaVersion
	}
	if lock.SchemaVersion != RegistryLockSchemaVersion {
		return RegistryLock{}, fmt.Errorf("unsupported registry lock schema version %d", lock.SchemaVersion)
	}
	if lock.Skills == nil {
		lock.Skills = map[string]RegistryLockEntry{}
	}
	return lock, nil
}

func SaveRegistryLock(paths Paths, lock RegistryLock) error {
	if lock.SchemaVersion == 0 {
		lock.SchemaVersion = RegistryLockSchemaVersion
	}
	if lock.Skills == nil {
		lock.Skills = map[string]RegistryLockEntry{}
	}
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeFileAtomic(paths.Home, paths.RegistryLock, data, "registry-lock-*.json")
}

func UpsertRegistryLockEntry(paths Paths, entry RegistryLockEntry) error {
	lock, err := LoadRegistryLock(paths)
	if err != nil {
		return err
	}
	lock.Skills[entry.Name] = entry
	return SaveRegistryLock(paths, lock)
}

func GetRegistryLockEntry(paths Paths, name string) (RegistryLockEntry, bool, error) {
	lock, err := LoadRegistryLock(paths)
	if err != nil {
		return RegistryLockEntry{}, false, err
	}
	entry, ok := lock.Skills[name]
	return entry, ok, nil
}

func DeleteRegistryLockEntry(paths Paths, name string) error {
	if _, err := os.Stat(paths.RegistryLock); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	lock, err := LoadRegistryLock(paths)
	if err != nil {
		return err
	}
	delete(lock.Skills, name)
	return SaveRegistryLock(paths, lock)
}

func normalizeRegistryURL(registryURL string) (string, error) {
	if strings.HasPrefix(registryURL, "https://") {
		if _, err := url.ParseRequestURI(registryURL); err != nil {
			return "", err
		}
		return registryURL, nil
	}
	abs, err := filepath.Abs(registryURL)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(abs); err != nil {
		return "", fmt.Errorf("registry must be an https URL or existing local file: %w", err)
	}
	return abs, nil
}

func readRegistryBytes(registryURL string) ([]byte, error) {
	if strings.HasPrefix(registryURL, "https://") {
		client := &http.Client{
			Timeout:   30 * time.Second,
			Transport: remoteHTTPTransport,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if req.URL.Scheme != "https" {
					return fmt.Errorf("redirect to non-https URL is not allowed: %s", req.URL.String())
				}
				return nil
			},
		}
		resp, err := client.Get(registryURL)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("registry download failed: %s", resp.Status)
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, maxRemoteArchiveBytes+1))
		if err != nil {
			return nil, err
		}
		if int64(len(data)) > maxRemoteArchiveBytes {
			return nil, fmt.Errorf("registry download exceeds maximum allowed size of %d bytes", maxRemoteArchiveBytes)
		}
		return data, nil
	}
	return os.ReadFile(registryURL)
}

func installSourceSpec(paths Paths, sourceSpec, name, expectedSHA256 string) error {
	if strings.HasPrefix(sourceSpec, "https://") || strings.HasPrefix(sourceSpec, "github:") {
		return AddRemoteSkillWithExpectedSHA(paths, sourceSpec, name, expectedSHA256)
	}
	if expectedSHA256 != "" {
		return fmt.Errorf("sha256 verification is only supported for remote sources")
	}
	return AddLocalSkill(paths, sourceSpec, name)
}

func writeFileAtomic(dir, dest string, data []byte, pattern string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, dest)
}
