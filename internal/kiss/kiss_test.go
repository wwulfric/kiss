package kiss

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddRunListRemoveLocalSkill(t *testing.T) {
	home := t.TempDir()
	paths, err := NewPaths(home)
	if err != nil {
		t.Fatal(err)
	}
	source := makeSkill(t, "browser-test", "0.1.0", "Use this one.")
	other := makeSkill(t, "other-skill", "0.1.0", "Do not leak this text.")

	if err := AddLocalSkill(paths, source, "browser-test"); err != nil {
		t.Fatalf("add browser-test: %v", err)
	}
	if err := AddLocalSkill(paths, other, "other-skill"); err != nil {
		t.Fatalf("add other-skill: %v", err)
	}

	var listOut bytes.Buffer
	if err := ListSkills(paths, &listOut); err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(listOut.String(), "browser-test\t0.1.0\tlocal\tlocal:") {
		t.Fatalf("list output missing browser-test: %q", listOut.String())
	}

	var showOut bytes.Buffer
	if err := ShowSkill(paths, "browser-test", &showOut); err != nil {
		t.Fatalf("show: %v", err)
	}
	for _, want := range []string{"\"name\": \"browser-test\"", "\"full_name\": \"local:", "\"kind\": \"local\"", "\"installed_path\""} {
		if !strings.Contains(showOut.String(), want) {
			t.Fatalf("show output missing %q:\n%s", want, showOut.String())
		}
	}
	if _, err := os.Stat(paths.MetadataDB); err != nil {
		t.Fatalf("expected metadata db: %v", err)
	}

	var runOut bytes.Buffer
	if err := RunSkill(paths, "browser-test", []string{"url=https://example.com"}, &runOut); err != nil {
		t.Fatalf("run: %v", err)
	}
	got := runOut.String()
	for _, want := range []string{"# KISS loaded skill: browser-test", "url=https://example.com", "Use this one.", "Only this skill was loaded."} {
		if !strings.Contains(got, want) {
			t.Fatalf("run output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Do not leak this text.") || strings.Contains(got, "other-skill") {
		t.Fatalf("run leaked another skill:\n%s", got)
	}

	if err := RemoveSkill(paths, "browser-test"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(paths.SkillDir("browser-test")); !os.IsNotExist(err) {
		t.Fatalf("expected removed skill dir, got err=%v", err)
	}
	if _, ok, err := GetSkillMetadata(paths, "browser-test"); err != nil || ok {
		t.Fatalf("expected removed metadata, ok=%v err=%v", ok, err)
	}
}

func TestRunMissingSkillDoesNotInstall(t *testing.T) {
	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err = RunSkill(paths, "missing", nil, &out)
	if err == nil {
		t.Fatal("expected missing skill error")
	}
	if !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("expected not installed error, got %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("expected no output for missing skill, got %q", out.String())
	}
}

func TestRunSkillArgsAreSingleLine(t *testing.T) {
	home := t.TempDir()
	paths, err := NewPaths(home)
	if err != nil {
		t.Fatal(err)
	}
	source := makeSkill(t, "browser-test", "0.1.0", "Use this one.")
	if err := AddLocalSkill(paths, source, "browser-test"); err != nil {
		t.Fatalf("add browser-test: %v", err)
	}
	var out bytes.Buffer
	if err := RunSkill(paths, "browser-test", []string{"a\nb", "c\rd"}, &out); err != nil {
		t.Fatalf("run: %v", err)
	}
	if strings.Contains(out.String(), "- Args: a\n") || strings.Contains(out.String(), "- Args: c\r") {
		t.Fatalf("args should not contain newlines or carriage returns:\n%s", out.String())
	}
}

func TestAddRejectsSymlink(t *testing.T) {
	home := t.TempDir()
	paths, err := NewPaths(home)
	if err != nil {
		t.Fatal(err)
	}
	source := makeSkill(t, "unsafe", "0.1.0", "unsafe")
	if err := os.Symlink(filepath.Join(source, "SKILL.md"), filepath.Join(source, "link.md")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := AddLocalSkill(paths, source, "unsafe"); err == nil {
		t.Fatal("expected symlink rejection")
	}
}

func TestDoctorCreatesBaseDirectories(t *testing.T) {
	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Doctor(paths, &out); err != nil {
		t.Fatalf("doctor: %v", err)
	}
	for _, dir := range []string{paths.Home, paths.Skills, paths.Cache, paths.Logs} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("expected dir %s, info=%v err=%v", dir, info, err)
		}
	}
	if _, err := os.Stat(paths.MetadataDB); err != nil {
		t.Fatalf("expected metadata db: %v", err)
	}
	if !strings.Contains(out.String(), "Status: ok") {
		t.Fatalf("doctor output missing status: %q", out.String())
	}
}

func TestAdapterTemplatesAreThinCLIWrappers(t *testing.T) {
	var slash bytes.Buffer
	if err := PrintAdapter("slash", &slash); err != nil {
		t.Fatalf("slash adapter: %v", err)
	}
	slashText := slash.String()
	for _, want := range []string{"/kiss <skill> [args...]", "kiss run <skill> [args...]", "stdout", "Do not list or scan $KISS_HOME/skills"} {
		if !strings.Contains(slashText, want) {
			t.Fatalf("slash adapter missing %q:\n%s", want, slashText)
		}
	}
	for _, forbidden := range []string{"kiss add", "kiss update", "skills directory"} {
		if strings.Contains(slashText, forbidden) {
			t.Fatalf("slash adapter contains forbidden text %q:\n%s", forbidden, slashText)
		}
	}

	var skill bytes.Buffer
	if err := PrintAdapter("skill", &skill); err != nil {
		t.Fatalf("skill adapter: %v", err)
	}
	skillText := skill.String()
	for _, want := range []string{"name: kiss", "/kiss <skill> [args...]", "kiss run <skill> [args...]", "current-turn context", "normal model reasoning and tools"} {
		if !strings.Contains(skillText, want) {
			t.Fatalf("skill adapter missing %q:\n%s", want, skillText)
		}
	}
	for _, forbidden := range []string{"kiss add", "kiss update", "kiss remove", "scan, summarize"} {
		if strings.Contains(skillText, forbidden) {
			t.Fatalf("skill adapter contains forbidden text %q:\n%s", forbidden, skillText)
		}
	}

	var shell bytes.Buffer
	if err := PrintAdapter("shell", &shell); err != nil {
		t.Fatalf("shell adapter: %v", err)
	}
	shellText := shell.String()
	for _, want := range []string{"#!/bin/sh", `exec kiss run "$@"`} {
		if !strings.Contains(shellText, want) {
			t.Fatalf("shell adapter missing %q:\n%s", want, shellText)
		}
	}
	for _, forbidden := range []string{"kiss add", "kiss update", "$KISS_HOME/skills"} {
		if strings.Contains(shellText, forbidden) {
			t.Fatalf("shell adapter contains forbidden text %q:\n%s", forbidden, shellText)
		}
	}

	var powershell bytes.Buffer
	if err := PrintAdapter("powershell", &powershell); err != nil {
		t.Fatalf("powershell adapter: %v", err)
	}
	powershellText := powershell.String()
	for _, want := range []string{"# KISS PowerShell adapter", "ValueFromRemainingArguments", "& kiss run @KissArgs", "exit $LASTEXITCODE"} {
		if !strings.Contains(powershellText, want) {
			t.Fatalf("powershell adapter missing %q:\n%s", want, powershellText)
		}
	}
	for _, forbidden := range []string{"kiss add", "kiss update", "$KISS_HOME/skills"} {
		if strings.Contains(powershellText, forbidden) {
			t.Fatalf("powershell adapter contains forbidden text %q:\n%s", forbidden, powershellText)
		}
	}
	if err := PrintAdapter("unknown", &bytes.Buffer{}); err == nil {
		t.Fatal("expected unknown adapter error")
	}
}

func TestInstallAndUninstallAdapters(t *testing.T) {
	dir := t.TempDir()
	shellPath := filepath.Join(dir, "kiss-skill")
	if err := InstallAdapter("shell", shellPath, false); err != nil {
		t.Fatalf("install shell adapter: %v", err)
	}
	shellData, err := os.ReadFile(shellPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(shellData), `exec kiss run "$@"`) || strings.Contains(string(shellData), "kiss add") || strings.Contains(string(shellData), "kiss update") {
		t.Fatalf("unexpected shell adapter:\n%s", string(shellData))
	}
	info, err := os.Stat(shellPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("expected shell adapter to be executable, mode=%v", info.Mode().Perm())
	}
	if err := InstallAdapter("shell", shellPath, false); err == nil {
		t.Fatal("expected existing adapter install to require --force")
	}
	if err := InstallAdapter("shell", shellPath, true); err != nil {
		t.Fatalf("force install shell adapter: %v", err)
	}

	slashPath := filepath.Join(dir, "kiss.slash.md")
	if err := InstallAdapter("slash", slashPath, false); err != nil {
		t.Fatalf("install slash adapter: %v", err)
	}
	slashData, err := os.ReadFile(slashPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(slashData), "Usage: /kiss <skill> [args...]") || strings.Contains(string(slashData), "kiss add") || strings.Contains(string(slashData), "kiss update") {
		t.Fatalf("unexpected slash adapter:\n%s", string(slashData))
	}
	if err := UninstallAdapter(slashPath); err != nil {
		t.Fatalf("uninstall slash adapter: %v", err)
	}
	if _, err := os.Stat(slashPath); !os.IsNotExist(err) {
		t.Fatalf("expected slash adapter removed, err=%v", err)
	}

	skillPath := filepath.Join(dir, "KISS.md")
	if err := InstallAdapter("skill", skillPath, false); err != nil {
		t.Fatalf("install skill adapter: %v", err)
	}
	skillData, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(skillData), "name: kiss") || !strings.Contains(string(skillData), "kiss run <skill> [args...]") {
		t.Fatalf("unexpected skill adapter:\n%s", string(skillData))
	}
	if err := UninstallAdapter(skillPath); err != nil {
		t.Fatalf("uninstall skill adapter: %v", err)
	}
	if _, err := os.Stat(skillPath); !os.IsNotExist(err) {
		t.Fatalf("expected skill adapter removed, err=%v", err)
	}

	powershellPath := filepath.Join(dir, "kiss-skill.ps1")
	if err := InstallAdapter("powershell", powershellPath, false); err != nil {
		t.Fatalf("install powershell adapter: %v", err)
	}
	powershellData, err := os.ReadFile(powershellPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(powershellData), "& kiss run @KissArgs") || strings.Contains(string(powershellData), "kiss add") || strings.Contains(string(powershellData), "kiss update") {
		t.Fatalf("unexpected powershell adapter:\n%s", string(powershellData))
	}
	if err := UninstallAdapter(powershellPath); err != nil {
		t.Fatalf("uninstall powershell adapter: %v", err)
	}
	if _, err := os.Stat(powershellPath); !os.IsNotExist(err) {
		t.Fatalf("expected powershell adapter removed, err=%v", err)
	}
}

func TestUninstallAdapterRejectsNonKISSFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "user-file")
	if err := os.WriteFile(path, []byte("not generated by kiss\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := UninstallAdapter(path)
	if err == nil || !strings.Contains(err.Error(), "refusing to remove non-KISS adapter") {
		t.Fatalf("expected non-KISS uninstall rejection, got: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("non-KISS file should remain, got: %v", err)
	}
}

func TestCommandManifestIsRejected(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "command-skill")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "name = \"command-skill\"\nversion = \"0.18.0\"\nentry = \"SKILL.md\"\n\n[runner]\ntype = \"command\"\ncommand = \"scripts/run.sh\"\n"
	if err := os.WriteFile(filepath.Join(dir, "kiss.skill.toml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("command instructions\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	err = AddLocalSkill(paths, dir, "command-skill")
	if err == nil || !strings.Contains(err.Error(), `runner type "command" is not supported`) {
		t.Fatalf("expected unsupported runner rejection, got: %v", err)
	}
}

func makeSkill(t *testing.T, name, version, instructions string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "name = \"" + name + "\"\nversion = \"" + version + "\"\ndescription = \"test skill\"\nentry = \"SKILL.md\"\n\n[runner]\ntype = \"markdown\"\n"
	if err := os.WriteFile(filepath.Join(dir, "kiss.skill.toml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(instructions+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func tomlStringSlice(items []string) string {
	if len(items) == 0 {
		return "[]"
	}
	quoted := make([]string, len(items))
	for i, item := range items {
		quoted[i] = `"` + strings.ReplaceAll(strings.ReplaceAll(item, `\`, `\\`), `"`, `\"`) + `"`
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func signedRegistryFields(t *testing.T, name, sourceSpec, sha256Value string) (string, string, string) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	payload := registrySignaturePayload(name, sourceSpec, sha256Value)
	signature := ed25519.Sign(privateKey, payload)
	fingerprint := sha256.Sum256(publicKey)
	return base64.StdEncoding.EncodeToString(publicKey), base64.StdEncoding.EncodeToString(signature), hex.EncodeToString(fingerprint[:])
}

func serverURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func withGitHubTestServer(t *testing.T, sha string, archive []byte, archivePath *string) func() {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/repos/"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"sha":"` + sha + `"}`))
		case strings.Contains(r.URL.Path, "/tar.gz/"):
			if archivePath != nil {
				*archivePath = r.URL.Path
			}
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	oldTransport := remoteHTTPTransport
	oldAPIBase := githubAPIBaseURL
	oldCodeloadBase := githubCodeloadBaseURL
	remoteHTTPTransport = server.Client().Transport
	githubAPIBaseURL = server.URL
	githubCodeloadBaseURL = server.URL
	return func() {
		remoteHTTPTransport = oldTransport
		githubAPIBaseURL = oldAPIBase
		githubCodeloadBaseURL = oldCodeloadBase
		server.Close()
	}
}

func TestAddRemoteHTTPSkillWritesSHAAndFullName(t *testing.T) {
	archive := makeSkillArchive(t, "remote-root", "remote", "0.2.0", "Remote instructions.")
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer server.Close()
	oldTransport := remoteHTTPTransport
	remoteHTTPTransport = server.Client().Transport
	defer func() { remoteHTTPTransport = oldTransport }()

	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := AddRemoteSkill(paths, server.URL+"/remote.tar.gz", "remote"); err != nil {
		t.Fatalf("add remote: %v", err)
	}
	metadata, ok, err := GetSkillMetadata(paths, "remote")
	if err != nil || !ok {
		t.Fatalf("metadata missing ok=%v err=%v", ok, err)
	}
	if metadata.Source.Kind != "https" || metadata.Source.URI != server.URL+"/remote.tar.gz" {
		t.Fatalf("unexpected source: %+v", metadata.Source)
	}
	if metadata.Source.SHA256 == "" {
		t.Fatal("expected sha256")
	}
	if metadata.FullName != server.URL+"/remote.tar.gz" {
		t.Fatalf("unexpected full name %q", metadata.FullName)
	}
	var out bytes.Buffer
	if err := RunSkill(paths, "remote", nil, &out); err != nil {
		t.Fatalf("run remote: %v", err)
	}
	if !strings.Contains(out.String(), "Remote instructions.") {
		t.Fatalf("missing remote instructions: %s", out.String())
	}
}

func TestAddRegistrySkillWritesLockAndVerifiesSHA(t *testing.T) {
	archive := makeSkillArchive(t, "registry-root", "browser-test", "0.3.0", "Registry instructions.")
	sum := sha256Hex(archive)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/registry.toml":
			registry := "[skills.browser-test]\nsource = \"https\"\nurl = \"" + serverURL(r) + "/browser-test.tar.gz\"\nsha256 = \"" + sum + "\"\n"
			_, _ = w.Write([]byte(registry))
		case "/browser-test.tar.gz":
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	oldTransport := remoteHTTPTransport
	remoteHTTPTransport = server.Client().Transport
	defer func() { remoteHTTPTransport = oldTransport }()

	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := AddRegistry(paths, "personal", server.URL+"/registry.toml"); err != nil {
		t.Fatalf("add registry: %v", err)
	}
	var registries bytes.Buffer
	if err := ListRegistries(paths, &registries); err != nil {
		t.Fatalf("list registries: %v", err)
	}
	if !strings.Contains(registries.String(), "personal\t"+server.URL+"/registry.toml") {
		t.Fatalf("registry list missing personal registry: %q", registries.String())
	}
	if err := AddRegistrySkill(paths, "browser-test"); err != nil {
		t.Fatalf("add registry skill: %v", err)
	}
	metadata, ok, err := GetSkillMetadata(paths, "browser-test")
	if err != nil || !ok {
		t.Fatalf("metadata missing ok=%v err=%v", ok, err)
	}
	if metadata.Source.SHA256 != sum {
		t.Fatalf("expected sha %s, got %s", sum, metadata.Source.SHA256)
	}
	entry, ok, err := GetRegistryLockEntry(paths, "browser-test")
	if err != nil || !ok {
		t.Fatalf("lock missing ok=%v err=%v", ok, err)
	}
	if entry.Registry != "personal" || entry.SourceSpec != server.URL+"/browser-test.tar.gz" || entry.SHA256 != sum {
		t.Fatalf("unexpected lock entry: %+v", entry)
	}
}

func TestAddRegistrySkillRejectsSHA256Mismatch(t *testing.T) {
	archive := makeSkillArchive(t, "registry-root", "browser-test", "0.3.0", "Registry instructions.")
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/registry.toml":
			registry := "[skills.browser-test]\nsource = \"https\"\nurl = \"" + serverURL(r) + "/browser-test.tar.gz\"\nsha256 = \"deadbeef\"\n"
			_, _ = w.Write([]byte(registry))
		case "/browser-test.tar.gz":
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	oldTransport := remoteHTTPTransport
	remoteHTTPTransport = server.Client().Transport
	defer func() { remoteHTTPTransport = oldTransport }()

	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := AddRegistry(paths, "personal", server.URL+"/registry.toml"); err != nil {
		t.Fatalf("add registry: %v", err)
	}
	err = AddRegistrySkill(paths, "browser-test")
	if err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("expected sha mismatch, got: %v", err)
	}
	if _, ok, err := GetSkillMetadata(paths, "browser-test"); err != nil || ok {
		t.Fatalf("expected no installed metadata, ok=%v err=%v", ok, err)
	}
}

func TestRegistryRequireSignatureRejectsUnsignedEntry(t *testing.T) {
	archive := makeSkillArchive(t, "unsigned-root", "browser-test", "0.16.0", "Unsigned registry instructions.")
	sum := sha256Hex(archive)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/registry.toml":
			registry := "[skills.browser-test]\nsource = \"https\"\nurl = \"" + serverURL(r) + "/browser-test.tar.gz\"\nsha256 = \"" + sum + "\"\n"
			_, _ = w.Write([]byte(registry))
		case "/browser-test.tar.gz":
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	oldTransport := remoteHTTPTransport
	remoteHTTPTransport = server.Client().Transport
	defer func() { remoteHTTPTransport = oldTransport }()

	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := AddRegistry(paths, "team", server.URL+"/registry.toml"); err != nil {
		t.Fatalf("add registry: %v", err)
	}
	if err := RequireRegistrySignature(paths, "team"); err != nil {
		t.Fatalf("require signature: %v", err)
	}
	var registries bytes.Buffer
	if err := ListRegistries(paths, &registries); err != nil {
		t.Fatalf("list registries: %v", err)
	}
	if !strings.Contains(registries.String(), "require_signature=true") {
		t.Fatalf("registry list should show signature requirement:\n%s", registries.String())
	}
	err = AddRegistrySkill(paths, "browser-test")
	if err == nil || !strings.Contains(err.Error(), "requires signed entries") {
		t.Fatalf("expected require-signature rejection, got: %v", err)
	}
}

func TestAddSignedRegistrySkillWritesSignatureAudit(t *testing.T) {
	archive := makeSkillArchive(t, "signed-root", "browser-test", "0.14.0", "Signed registry instructions.")
	sum := sha256Hex(archive)
	registry := ""
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/registry.toml":
			_, _ = w.Write([]byte(registry))
		case "/browser-test.tar.gz":
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	sourceSpec := server.URL + "/browser-test.tar.gz"
	publicKey, signature, fingerprint := signedRegistryFields(t, "browser-test", sourceSpec, sum)
	registry = "[skills.browser-test]\nsource = \"https\"\nurl = \"" + sourceSpec + "\"\nsha256 = \"" + sum + "\"\npublic_key = \"" + publicKey + "\"\nsignature = \"" + signature + "\"\n"
	oldTransport := remoteHTTPTransport
	remoteHTTPTransport = server.Client().Transport
	defer func() { remoteHTTPTransport = oldTransport }()

	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := AddRegistry(paths, "signed", server.URL+"/registry.toml"); err != nil {
		t.Fatalf("add signed registry: %v", err)
	}
	if err := AddRegistrySkill(paths, "browser-test"); err != nil {
		t.Fatalf("add signed registry skill: %v", err)
	}
	entry, ok, err := GetRegistryLockEntry(paths, "browser-test")
	if err != nil || !ok {
		t.Fatalf("lock missing ok=%v err=%v", ok, err)
	}
	if !entry.SignatureVerified || entry.PublicKeySHA256 != fingerprint {
		t.Fatalf("expected signature audit in lock, got %+v", entry)
	}
}

func TestRegistryTrustedKeyAllowsSignedEntry(t *testing.T) {
	archive := makeSkillArchive(t, "trusted-root", "browser-test", "0.16.0", "Trusted registry instructions.")
	sum := sha256Hex(archive)
	registry := ""
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/registry.toml":
			_, _ = w.Write([]byte(registry))
		case "/browser-test.tar.gz":
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	sourceSpec := server.URL + "/browser-test.tar.gz"
	publicKey, signature, fingerprint := signedRegistryFields(t, "browser-test", sourceSpec, sum)
	registry = "[skills.browser-test]\nsource = \"https\"\nurl = \"" + sourceSpec + "\"\nsha256 = \"" + sum + "\"\npublic_key = \"" + publicKey + "\"\nsignature = \"" + signature + "\"\n"
	oldTransport := remoteHTTPTransport
	remoteHTTPTransport = server.Client().Transport
	defer func() { remoteHTTPTransport = oldTransport }()

	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := AddRegistry(paths, "team", server.URL+"/registry.toml"); err != nil {
		t.Fatalf("add registry: %v", err)
	}
	if err := TrustRegistryKey(paths, "team", publicKey); err != nil {
		t.Fatalf("trust registry key: %v", err)
	}
	var registries bytes.Buffer
	if err := ListRegistries(paths, &registries); err != nil {
		t.Fatalf("list registries: %v", err)
	}
	if !strings.Contains(registries.String(), "trusted_keys=1") {
		t.Fatalf("registry list should show trusted key count:\n%s", registries.String())
	}
	if err := AddRegistrySkill(paths, "browser-test"); err != nil {
		t.Fatalf("add trusted signed skill: %v", err)
	}
	entry, ok, err := GetRegistryLockEntry(paths, "browser-test")
	if err != nil || !ok {
		t.Fatalf("lock missing ok=%v err=%v", ok, err)
	}
	if !entry.SignatureVerified || entry.PublicKeySHA256 != fingerprint {
		t.Fatalf("expected trusted signature audit, got %+v", entry)
	}
}

func TestAddSignedRegistrySkillRejectsBadSignature(t *testing.T) {
	archive := makeSkillArchive(t, "signed-root", "browser-test", "0.14.0", "Signed registry instructions.")
	sum := sha256Hex(archive)
	registry := ""
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/registry.toml":
			_, _ = w.Write([]byte(registry))
		case "/browser-test.tar.gz":
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	sourceSpec := server.URL + "/browser-test.tar.gz"
	publicKey, signature, _ := signedRegistryFields(t, "other-skill", sourceSpec, sum)
	registry = "[skills.browser-test]\nsource = \"https\"\nurl = \"" + sourceSpec + "\"\nsha256 = \"" + sum + "\"\npublic_key = \"" + publicKey + "\"\nsignature = \"" + signature + "\"\n"
	oldTransport := remoteHTTPTransport
	remoteHTTPTransport = server.Client().Transport
	defer func() { remoteHTTPTransport = oldTransport }()

	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := AddRegistry(paths, "signed", server.URL+"/registry.toml"); err != nil {
		t.Fatalf("add signed registry: %v", err)
	}
	err = AddRegistrySkill(paths, "browser-test")
	if err == nil || !strings.Contains(err.Error(), "registry signature verification failed") {
		t.Fatalf("expected signature verification error, got: %v", err)
	}
	if _, ok, err := GetSkillMetadata(paths, "browser-test"); err != nil || ok {
		t.Fatalf("expected no installed metadata, ok=%v err=%v", ok, err)
	}
}

func TestAddSignedRegistrySkillRejectsMissingSignatureField(t *testing.T) {
	archive := makeSkillArchive(t, "signed-root", "browser-test", "0.14.0", "Signed registry instructions.")
	sum := sha256Hex(archive)
	registry := ""
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/registry.toml":
			_, _ = w.Write([]byte(registry))
		case "/browser-test.tar.gz":
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	sourceSpec := server.URL + "/browser-test.tar.gz"
	publicKey, _, _ := signedRegistryFields(t, "browser-test", sourceSpec, sum)
	registry = "[skills.browser-test]\nsource = \"https\"\nurl = \"" + sourceSpec + "\"\nsha256 = \"" + sum + "\"\npublic_key = \"" + publicKey + "\"\n"
	oldTransport := remoteHTTPTransport
	remoteHTTPTransport = server.Client().Transport
	defer func() { remoteHTTPTransport = oldTransport }()

	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := AddRegistry(paths, "signed", server.URL+"/registry.toml"); err != nil {
		t.Fatalf("add signed registry: %v", err)
	}
	err = AddRegistrySkill(paths, "browser-test")
	if err == nil || !strings.Contains(err.Error(), "requires both public_key and signature") {
		t.Fatalf("expected missing signature field error, got: %v", err)
	}
}

func TestRegistryTrustedKeyRejectsUntrustedAndFallsBack(t *testing.T) {
	archive := makeSkillArchive(t, "trusted-root", "browser-test", "0.16.0", "Trusted fallback instructions.")
	sum := sha256Hex(archive)
	badRegistry := ""
	goodRegistry := ""
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bad-registry.toml":
			_, _ = w.Write([]byte(badRegistry))
		case "/good-registry.toml":
			_, _ = w.Write([]byte(goodRegistry))
		case "/browser-test.tar.gz":
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	sourceSpec := server.URL + "/browser-test.tar.gz"
	badPublicKey, badSignature, _ := signedRegistryFields(t, "browser-test", sourceSpec, sum)
	otherPublicKey, _, _ := signedRegistryFields(t, "other-skill", sourceSpec, sum)
	goodPublicKey, goodSignature, goodFingerprint := signedRegistryFields(t, "browser-test", sourceSpec, sum)
	badRegistry = "[skills.browser-test]\nsource = \"https\"\nurl = \"" + sourceSpec + "\"\nsha256 = \"" + sum + "\"\npublic_key = \"" + badPublicKey + "\"\nsignature = \"" + badSignature + "\"\n"
	goodRegistry = "[skills.browser-test]\nsource = \"https\"\nurl = \"" + sourceSpec + "\"\nsha256 = \"" + sum + "\"\npublic_key = \"" + goodPublicKey + "\"\nsignature = \"" + goodSignature + "\"\n"
	oldTransport := remoteHTTPTransport
	remoteHTTPTransport = server.Client().Transport
	defer func() { remoteHTTPTransport = oldTransport }()

	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := AddRegistry(paths, "aaa-bad", server.URL+"/bad-registry.toml"); err != nil {
		t.Fatalf("add bad registry: %v", err)
	}
	if err := AddRegistry(paths, "zzz-good", server.URL+"/good-registry.toml"); err != nil {
		t.Fatalf("add good registry: %v", err)
	}
	if err := TrustRegistryKey(paths, "aaa-bad", otherPublicKey); err != nil {
		t.Fatalf("trust other key: %v", err)
	}
	if err := TrustRegistryKey(paths, "zzz-good", goodPublicKey); err != nil {
		t.Fatalf("trust good key: %v", err)
	}
	if err := AddRegistrySkill(paths, "browser-test"); err != nil {
		t.Fatalf("expected fallback to trusted key registry: %v", err)
	}
	entry, ok, err := GetRegistryLockEntry(paths, "browser-test")
	if err != nil || !ok {
		t.Fatalf("lock missing ok=%v err=%v", ok, err)
	}
	if entry.Registry != "zzz-good" || entry.PublicKeySHA256 != goodFingerprint {
		t.Fatalf("expected trusted fallback lock, got %+v", entry)
	}
}

func TestAddRegistrySkillFallsBackAfterBadSignature(t *testing.T) {
	archive := makeSkillArchive(t, "signed-root", "browser-test", "0.14.0", "Fallback after bad signature.")
	sum := sha256Hex(archive)
	badRegistry := ""
	goodRegistry := ""
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bad-registry.toml":
			_, _ = w.Write([]byte(badRegistry))
		case "/good-registry.toml":
			_, _ = w.Write([]byte(goodRegistry))
		case "/browser-test.tar.gz":
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	sourceSpec := server.URL + "/browser-test.tar.gz"
	badPublicKey, badSignature, _ := signedRegistryFields(t, "browser-test", sourceSpec, "deadbeef")
	goodPublicKey, goodSignature, goodFingerprint := signedRegistryFields(t, "browser-test", sourceSpec, sum)
	badRegistry = "[skills.browser-test]\nsource = \"https\"\nurl = \"" + sourceSpec + "\"\nsha256 = \"" + sum + "\"\npublic_key = \"" + badPublicKey + "\"\nsignature = \"" + badSignature + "\"\n"
	goodRegistry = "[skills.browser-test]\nsource = \"https\"\nurl = \"" + sourceSpec + "\"\nsha256 = \"" + sum + "\"\npublic_key = \"" + goodPublicKey + "\"\nsignature = \"" + goodSignature + "\"\n"
	oldTransport := remoteHTTPTransport
	remoteHTTPTransport = server.Client().Transport
	defer func() { remoteHTTPTransport = oldTransport }()

	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := AddRegistry(paths, "aaa-bad", server.URL+"/bad-registry.toml"); err != nil {
		t.Fatalf("add bad registry: %v", err)
	}
	if err := AddRegistry(paths, "zzz-good", server.URL+"/good-registry.toml"); err != nil {
		t.Fatalf("add good registry: %v", err)
	}
	if err := AddRegistrySkill(paths, "browser-test"); err != nil {
		t.Fatalf("expected fallback after bad signature: %v", err)
	}
	entry, ok, err := GetRegistryLockEntry(paths, "browser-test")
	if err != nil || !ok {
		t.Fatalf("lock missing ok=%v err=%v", ok, err)
	}
	if entry.Registry != "zzz-good" || !entry.SignatureVerified || entry.PublicKeySHA256 != goodFingerprint {
		t.Fatalf("expected signed fallback lock, got %+v", entry)
	}
}

func TestAddRegistrySkillFallsBackAfterRegistryLoadError(t *testing.T) {
	archive := makeSkillArchive(t, "fallback-root", "browser-test", "0.11.0", "Fallback registry instructions.")
	sum := sha256Hex(archive)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/broken-registry.toml":
			http.Error(w, "broken", http.StatusInternalServerError)
		case "/fallback-registry.toml":
			registry := "[skills.browser-test]\nsource = \"https\"\nurl = \"" + serverURL(r) + "/browser-test.tar.gz\"\nsha256 = \"" + sum + "\"\n"
			_, _ = w.Write([]byte(registry))
		case "/browser-test.tar.gz":
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	oldTransport := remoteHTTPTransport
	remoteHTTPTransport = server.Client().Transport
	defer func() { remoteHTTPTransport = oldTransport }()

	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := AddRegistry(paths, "aaa-broken", server.URL+"/broken-registry.toml"); err != nil {
		t.Fatalf("add broken registry: %v", err)
	}
	if err := AddRegistry(paths, "zzz-fallback", server.URL+"/fallback-registry.toml"); err != nil {
		t.Fatalf("add fallback registry: %v", err)
	}
	if err := AddRegistrySkill(paths, "browser-test"); err != nil {
		t.Fatalf("fallback registry install: %v", err)
	}
	entry, ok, err := GetRegistryLockEntry(paths, "browser-test")
	if err != nil || !ok {
		t.Fatalf("lock missing ok=%v err=%v", ok, err)
	}
	if entry.Registry != "zzz-fallback" {
		t.Fatalf("expected fallback registry in lock, got %+v", entry)
	}
	var out bytes.Buffer
	if err := RunSkill(paths, "browser-test", nil, &out); err != nil {
		t.Fatalf("run fallback skill: %v", err)
	}
	if !strings.Contains(out.String(), "Fallback registry instructions.") {
		t.Fatalf("missing fallback instructions:\n%s", out.String())
	}
}

func TestResolveRegistrySkillReportsAllRegistryErrors(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/broken-registry.toml":
			http.Error(w, "broken", http.StatusInternalServerError)
		case "/bad-entry-registry.toml":
			_, _ = w.Write([]byte("[skills.browser-test]\nsource = \"https\"\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	oldTransport := remoteHTTPTransport
	remoteHTTPTransport = server.Client().Transport
	defer func() { remoteHTTPTransport = oldTransport }()

	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := AddRegistry(paths, "aaa-broken", server.URL+"/broken-registry.toml"); err != nil {
		t.Fatalf("add broken registry: %v", err)
	}
	if err := AddRegistry(paths, "bbb-bad-entry", server.URL+"/bad-entry-registry.toml"); err != nil {
		t.Fatalf("add bad registry: %v", err)
	}
	_, err = ResolveRegistrySkill(paths, "browser-test")
	if err == nil {
		t.Fatal("expected registry resolution error")
	}
	for _, want := range []string{"browser-test", "aaa-broken", "bbb-bad-entry", "registry errors", "https registry entry requires url"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("registry error missing %q: %v", want, err)
		}
	}
}

func TestUpdateSkillPlansThenAppliesRegistryUpdate(t *testing.T) {
	oldArchive := makeSkillArchive(t, "registry-root", "browser-test", "0.3.0", "Old registry instructions.")
	newArchive := makeSkillArchive(t, "registry-root", "browser-test", "0.3.1", "New registry instructions.")
	currentArchive := oldArchive
	currentSHA := sha256Hex(oldArchive)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/registry.toml":
			registry := "[skills.browser-test]\nsource = \"https\"\nurl = \"" + serverURL(r) + "/browser-test.tar.gz\"\nsha256 = \"" + currentSHA + "\"\n"
			_, _ = w.Write([]byte(registry))
		case "/browser-test.tar.gz":
			_, _ = w.Write(currentArchive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	oldTransport := remoteHTTPTransport
	remoteHTTPTransport = server.Client().Transport
	defer func() { remoteHTTPTransport = oldTransport }()

	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := AddRegistry(paths, "personal", server.URL+"/registry.toml"); err != nil {
		t.Fatalf("add registry: %v", err)
	}
	if err := AddRegistrySkill(paths, "browser-test"); err != nil {
		t.Fatalf("add registry skill: %v", err)
	}
	currentArchive = newArchive
	currentSHA = sha256Hex(newArchive)

	var plan bytes.Buffer
	if err := UpdateSkill(paths, "browser-test", false, &plan); err != nil {
		t.Fatalf("plan update: %v", err)
	}
	if !strings.Contains(plan.String(), "Run `kiss update browser-test --yes` to apply.") {
		t.Fatalf("missing update apply hint:\n%s", plan.String())
	}
	for _, want := range []string{
		"- Current version: 0.3.0",
		"- Target version: 0.3.1",
		"- Entry content: changed",
		"## Entry diff",
		"```diff",
		"--- current/SKILL.md",
		"+++ target/SKILL.md",
		"-Old registry instructions.",
		"+New registry instructions.",
	} {
		if !strings.Contains(plan.String(), want) {
			t.Fatalf("update plan missing %q:\n%s", want, plan.String())
		}
	}
	if strings.Index(plan.String(), "-Old registry instructions.") > strings.Index(plan.String(), "+New registry instructions.") {
		t.Fatalf("expected deletion to appear before addition:\n%s", plan.String())
	}
	var out bytes.Buffer
	if err := RunSkill(paths, "browser-test", nil, &out); err != nil {
		t.Fatalf("run after plan: %v", err)
	}
	if !strings.Contains(out.String(), "Old registry instructions.") || strings.Contains(out.String(), "New registry instructions.") {
		t.Fatalf("plan should not apply update:\n%s", out.String())
	}

	var applyOut bytes.Buffer
	if err := UpdateSkill(paths, "browser-test", true, &applyOut); err != nil {
		t.Fatalf("apply update: %v", err)
	}
	out.Reset()
	if err := RunSkill(paths, "browser-test", nil, &out); err != nil {
		t.Fatalf("run after apply: %v", err)
	}
	if !strings.Contains(out.String(), "New registry instructions.") {
		t.Fatalf("expected updated instructions:\n%s", out.String())
	}
	entry, ok, err := GetRegistryLockEntry(paths, "browser-test")
	if err != nil || !ok {
		t.Fatalf("lock missing ok=%v err=%v", ok, err)
	}
	if entry.SHA256 != currentSHA {
		t.Fatalf("expected updated lock sha %s, got %s", currentSHA, entry.SHA256)
	}
}

func TestUpdateSkillPlanShowsNoDiffWhenEntryUnchanged(t *testing.T) {
	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source := makeSkill(t, "browser-test", "0.10.0", "Stable instructions.")
	if err := AddLocalSkill(paths, source, "browser-test"); err != nil {
		t.Fatalf("add local skill: %v", err)
	}

	var plan bytes.Buffer
	if err := UpdateSkill(paths, "browser-test", false, &plan); err != nil {
		t.Fatalf("plan update: %v", err)
	}
	got := plan.String()
	if !strings.Contains(got, "## Entry diff") || !strings.Contains(got, "Entry content unchanged; no diff.") {
		t.Fatalf("expected unchanged diff note:\n%s", got)
	}
	if strings.Contains(got, "```diff") {
		t.Fatalf("unchanged plan should not emit a diff block:\n%s", got)
	}
}

func TestEntryDiffTruncatesLargeOutput(t *testing.T) {
	var current strings.Builder
	var target strings.Builder
	for i := 0; i < 200; i++ {
		current.WriteString("old line\n")
		target.WriteString("new line\n")
	}
	diff := buildEntryDiff("SKILL.md", "SKILL.md", []byte(current.String()), []byte(target.String()))
	if !diff.Truncated {
		t.Fatalf("expected large diff to be truncated")
	}
	if got := len(strings.Split(diff.Text, "\n")); got > updateDiffMaxLines {
		t.Fatalf("expected at most %d diff lines, got %d", updateDiffMaxLines, got)
	}
}

func TestEntryDiffMarkdownFenceExceedsContentBackticks(t *testing.T) {
	diff := buildEntryDiff("SKILL.md", "SKILL.md", []byte("old\n"), []byte("```go\nnew\n```\n"))
	fence := markdownFenceFor(diff.Text)
	if fence != "````" {
		t.Fatalf("expected four-backtick fence, got %q for diff:\n%s", fence, diff.Text)
	}
}

func TestUpdateSkillPreviewRejectsSHA256Mismatch(t *testing.T) {
	oldArchive := makeSkillArchive(t, "registry-root", "browser-test", "0.3.0", "Old registry instructions.")
	newArchive := makeSkillArchive(t, "registry-root", "browser-test", "0.3.1", "New registry instructions.")
	currentArchive := oldArchive
	currentSHA := sha256Hex(oldArchive)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/registry.toml":
			registry := "[skills.browser-test]\nsource = \"https\"\nurl = \"" + serverURL(r) + "/browser-test.tar.gz\"\nsha256 = \"" + currentSHA + "\"\n"
			_, _ = w.Write([]byte(registry))
		case "/browser-test.tar.gz":
			_, _ = w.Write(currentArchive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	oldTransport := remoteHTTPTransport
	remoteHTTPTransport = server.Client().Transport
	defer func() { remoteHTTPTransport = oldTransport }()

	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := AddRegistry(paths, "personal", server.URL+"/registry.toml"); err != nil {
		t.Fatalf("add registry: %v", err)
	}
	if err := AddRegistrySkill(paths, "browser-test"); err != nil {
		t.Fatalf("add registry skill: %v", err)
	}
	currentArchive = newArchive
	currentSHA = "deadbeef"

	var plan bytes.Buffer
	err = UpdateSkill(paths, "browser-test", false, &plan)
	if err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("expected preview sha mismatch, got: %v", err)
	}
	var out bytes.Buffer
	if err := RunSkill(paths, "browser-test", nil, &out); err != nil {
		t.Fatalf("run after failed preview: %v", err)
	}
	if !strings.Contains(out.String(), "Old registry instructions.") || strings.Contains(out.String(), "New registry instructions.") {
		t.Fatalf("failed preview should not update skill:\n%s", out.String())
	}
}

func TestParseGitHubSource(t *testing.T) {
	got, err := ParseGitHubSource("github:vercel-labs/agent-skills/skills/browser-test#v1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if got.Owner != "vercel-labs" || got.Repo != "agent-skills" || got.Path != "skills/browser-test" || got.Ref != "v1.2.3" {
		t.Fatalf("unexpected parse result: %+v", got)
	}
	sha := "0123456789abcdef0123456789abcdef01234567"
	defer withGitHubTestServer(t, sha, nil, nil)()
	resolved, archiveURL, subdir, err := ResolveRemoteSource("github:vercel-labs/agent-skills/skills/browser-test#v1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.FullName != "github:vercel-labs/agent-skills/skills/browser-test" || resolved.Ref != "v1.2.3" || resolved.Resolved != sha || subdir != "skills/browser-test" {
		t.Fatalf("unexpected resolved=%+v subdir=%q", resolved, subdir)
	}
	if !strings.Contains(archiveURL, "/tar.gz/"+sha) {
		t.Fatalf("archive URL should use resolved SHA, got %s", archiveURL)
	}
}

func TestParseGitHubSourcePathValidation(t *testing.T) {
	got, err := ParseGitHubSource("github:owner/repo/skills/v1..0/browser")
	if err != nil {
		t.Fatalf("expected '..' within segment to be allowed, got: %v", err)
	}
	if got.Path != "skills/v1..0/browser" {
		t.Fatalf("unexpected path: %q", got.Path)
	}
	for _, source := range []string{
		"github:owner/repo/skills/../browser",
		`github:owner/repo/skills\browser`,
	} {
		if _, err := ParseGitHubSource(source); err == nil {
			t.Fatalf("expected invalid path for %q", source)
		}
	}
}

func TestResolveRemoteSourceGitHubRootURI(t *testing.T) {
	defer withGitHubTestServer(t, "abcdef0123456789abcdef0123456789abcdef01", nil, nil)()
	resolved, _, _, err := ResolveRemoteSource("github:owner/repo")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.URI != "owner/repo" {
		t.Fatalf("unexpected URI: %q", resolved.URI)
	}
}

func TestAddRemoteGitHubSkillWritesResolvedCommit(t *testing.T) {
	sha := "1111111111111111111111111111111111111111"
	archive := makeNestedSkillArchive(t, "repo-main", "skills/browser-test", "browser-test", "0.7.0", "GitHub instructions.")
	archivePath := ""
	defer withGitHubTestServer(t, sha, archive, &archivePath)()

	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := AddRemoteSkill(paths, "github:owner/repo/skills/browser-test#main", "browser-test"); err != nil {
		t.Fatalf("add github remote: %v", err)
	}
	metadata, ok, err := GetSkillMetadata(paths, "browser-test")
	if err != nil || !ok {
		t.Fatalf("metadata missing ok=%v err=%v", ok, err)
	}
	if metadata.Source.Kind != "github" || metadata.Source.Ref != "main" || metadata.Source.Resolved != sha {
		t.Fatalf("unexpected github source metadata: %+v", metadata.Source)
	}
	if archivePath != "/owner/repo/tar.gz/"+sha {
		t.Fatalf("expected archive request to use resolved SHA, got %q", archivePath)
	}
	var out bytes.Buffer
	if err := RunSkill(paths, "browser-test", nil, &out); err != nil {
		t.Fatalf("run github skill: %v", err)
	}
	if !strings.Contains(out.String(), "GitHub instructions.") {
		t.Fatalf("missing github instructions:\n%s", out.String())
	}
}

func TestDownloadArchiveRejectsOversizedResponse(t *testing.T) {
	oldMax := maxRemoteArchiveBytes
	maxRemoteArchiveBytes = 16
	defer func() { maxRemoteArchiveBytes = oldMax }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("01234567890123456789"))
	}))
	defer server.Close()

	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := downloadArchive(paths, server.URL, "too-large"); err == nil {
		t.Fatal("expected download size limit error")
	}
}

func TestExtractTarGzRejectsLinkEntriesWithSpecificError(t *testing.T) {
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{
		Name:     "skill/link",
		Typeflag: tar.TypeSymlink,
		Linkname: "SKILL.md",
	}); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}

	archivePath := filepath.Join(t.TempDir(), "skill.tar.gz")
	if err := os.WriteFile(archivePath, archive.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	err := extractTarGz(archivePath, filepath.Join(t.TempDir(), "out"))
	if err == nil || !strings.Contains(err.Error(), "link entry") {
		t.Fatalf("expected specific link-entry error, got: %v", err)
	}
}

func TestExtractTarGzAllowsPAXGlobalHeader(t *testing.T) {
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "pax_global_header", Typeflag: tar.TypeXGlobalHeader, Size: 0}); err != nil {
		t.Fatal(err)
	}
	if err := tw.WriteHeader(&tar.Header{Name: "skill/", Mode: 0o755, Typeflag: tar.TypeDir}); err != nil {
		t.Fatal(err)
	}
	body := "hello\n"
	if err := tw.WriteHeader(&tar.Header{Name: "skill/SKILL.md", Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(t.TempDir(), "skill.tar.gz")
	if err := os.WriteFile(archivePath, archive.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "out")
	if err := extractTarGz(archivePath, dest); err != nil {
		t.Fatalf("expected PAX global header to be accepted, got: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "skill", "SKILL.md")); err != nil {
		t.Fatalf("expected extracted file, got: %v", err)
	}
}

func TestExtractTarGzRejectsOversizedExtractedContent(t *testing.T) {
	oldMax := maxExtractedArchiveBytes
	maxExtractedArchiveBytes = 4
	defer func() { maxExtractedArchiveBytes = oldMax }()

	archive := makeSkillArchive(t, "skill", "remote", "0.2.0", "Remote instructions.")
	archivePath := filepath.Join(t.TempDir(), "skill.tar.gz")
	if err := os.WriteFile(archivePath, archive, 0o644); err != nil {
		t.Fatal(err)
	}
	err := extractTarGz(archivePath, filepath.Join(t.TempDir(), "out"))
	if err == nil || !strings.Contains(err.Error(), "exceeds maximum allowed size") {
		t.Fatalf("expected extracted size limit error, got: %v", err)
	}
}

func TestSafeArchiveTarget(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "extract")
	if _, err := safeArchiveTarget(dest, "skill/SKILL.md"); err != nil {
		t.Fatalf("expected safe path, got: %v", err)
	}
	for _, name := range []string{"../evil", "root/../../evil", "a//b", `a\..\b`} {
		if _, err := safeArchiveTarget(dest, name); err == nil {
			t.Fatalf("expected traversal path to be rejected: %q", name)
		}
	}
}

func TestManifestEntryValidation(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "skill")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes..md"), []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "kiss.skill.toml"), []byte("entry = \"notes..md\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadManifest(dir); err != nil {
		t.Fatalf("expected notes..md to be allowed, got: %v", err)
	}
	for _, entry := range []string{`"..\\notes.md"`, `"../notes.md"`} {
		if err := os.WriteFile(filepath.Join(dir, "kiss.skill.toml"), []byte("entry = "+entry+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadManifest(dir); err == nil {
			t.Fatalf("expected unsafe entry to be rejected: %s", entry)
		}
	}
}

func TestManifestEntryValidationErrorIncludesEntryAndCause(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "skill")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "kiss.skill.toml"), []byte("entry = \"../notes.md\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadManifest(dir)
	if err == nil {
		t.Fatal("expected unsafe entry error")
	}
	if !strings.Contains(err.Error(), `entry "../notes.md"`) || !strings.Contains(err.Error(), "safe relative path") {
		t.Fatalf("expected detailed entry validation error, got: %v", err)
	}
}

func TestManifestParsesQuotedHashAndComment(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "skill")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := "description = \"foo # bar\" # trailing comment\n"
	if err := os.WriteFile(filepath.Join(dir, "kiss.skill.toml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := LoadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Description != "foo # bar" {
		t.Fatalf("expected description to preserve # inside quoted string, got %q", got.Description)
	}
}

func TestAddRemoteSkillRejectsInvalidNameBeforeDownload(t *testing.T) {
	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := AddRemoteSkill(paths, "https://example.com/skill.tar.gz", "../bad"); err == nil {
		t.Fatal("expected invalid skill name error")
	}
}

func TestParseGitHubSourceIncludesInvalidPathInError(t *testing.T) {
	_, err := ParseGitHubSource("github:owner/repo/skills/../browser")
	if err == nil {
		t.Fatal("expected invalid github sub-path error")
	}
	if !strings.Contains(err.Error(), "skills/../browser") || !strings.Contains(err.Error(), "safe relative path") {
		t.Fatalf("expected path and cause in error, got: %v", err)
	}
}

func TestDownloadArchiveRejectsRedirectToHTTP(t *testing.T) {
	insecureServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("unexpected"))
	}))
	defer insecureServer.Close()

	redirectServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, insecureServer.URL, http.StatusFound)
	}))
	defer redirectServer.Close()

	oldTransport := remoteHTTPTransport
	remoteHTTPTransport = redirectServer.Client().Transport
	defer func() { remoteHTTPTransport = oldTransport }()

	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := downloadArchive(paths, redirectServer.URL+"/skill.tar.gz", "redirect"); err == nil || !strings.Contains(err.Error(), "non-https") {
		t.Fatalf("expected redirect rejection, got: %v", err)
	}
}

func TestDownloadArchiveRemovesPartialFileOnCopyError(t *testing.T) {
	oldTransport := remoteHTTPTransport
	remoteHTTPTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       &failingReadCloser{data: []byte("partial"), failErr: errors.New("boom")},
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})
	defer func() { remoteHTTPTransport = oldTransport }()

	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := downloadArchive(paths, "https://example.com/skill.tar.gz", "partial"); err == nil {
		t.Fatal("expected copy error")
	}
	entries, err := os.ReadDir(filepath.Join(paths.Cache, "downloads"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no leftover partial files, found %d", len(entries))
	}
}

func TestInstallSkillRollsBackOnMetadataFailure(t *testing.T) {
	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	original := makeSkill(t, "rollback", "0.1.0", "old instructions")
	if err := AddLocalSkill(paths, original, "rollback"); err != nil {
		t.Fatalf("add original: %v", err)
	}
	updated := makeSkill(t, "rollback", "0.2.0", "new instructions")
	badMetadataPath := filepath.Join(paths.Home, "bad-metadata")
	if err := os.MkdirAll(badMetadataPath, 0o755); err != nil {
		t.Fatal(err)
	}
	paths.MetadataDB = badMetadataPath

	err = AddLocalSkill(paths, updated, "rollback")
	if err == nil {
		t.Fatal("expected metadata write failure")
	}
	content, readErr := os.ReadFile(filepath.Join(paths.SkillDir("rollback"), "SKILL.md"))
	if readErr != nil {
		t.Fatalf("expected rollback skill dir to exist, read err: %v", readErr)
	}
	if !strings.Contains(string(content), "old instructions") {
		t.Fatalf("expected original content restored, got: %q", string(content))
	}
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type failingReadCloser struct {
	data    []byte
	read    bool
	failErr error
}

func (f *failingReadCloser) Read(p []byte) (int, error) {
	if !f.read {
		f.read = true
		n := copy(p, f.data)
		return n, nil
	}
	return 0, f.failErr
}

func (f *failingReadCloser) Close() error {
	return nil
}

func makeSkillArchive(t *testing.T, root, name, version, instructions string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	addDir := func(path string) {
		t.Helper()
		if err := tw.WriteHeader(&tar.Header{Name: path, Mode: 0o755, Typeflag: tar.TypeDir}); err != nil {
			t.Fatal(err)
		}
	}
	addFile := func(path, body string) {
		t.Helper()
		if err := tw.WriteHeader(&tar.Header{Name: path, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	addDir(root + "/")
	manifest := "name = \"" + name + "\"\nversion = \"" + version + "\"\ndescription = \"remote skill\"\nentry = \"SKILL.md\"\n\n[runner]\ntype = \"markdown\"\n"
	addFile(root+"/kiss.skill.toml", manifest)
	addFile(root+"/SKILL.md", instructions+"\n")
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func makeNestedSkillArchive(t *testing.T, root, subdir, name, version, instructions string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	addDir := func(path string) {
		t.Helper()
		if err := tw.WriteHeader(&tar.Header{Name: path, Mode: 0o755, Typeflag: tar.TypeDir}); err != nil {
			t.Fatal(err)
		}
	}
	addFile := func(path, body string) {
		t.Helper()
		if err := tw.WriteHeader(&tar.Header{Name: path, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	addDir(root + "/")
	current := root
	for _, segment := range strings.Split(subdir, "/") {
		current += "/" + segment
		addDir(current + "/")
	}
	manifest := "name = \"" + name + "\"\nversion = \"" + version + "\"\ndescription = \"github skill\"\nentry = \"SKILL.md\"\n\n[runner]\ntype = \"markdown\"\n"
	addFile(current+"/kiss.skill.toml", manifest)
	addFile(current+"/SKILL.md", instructions+"\n")
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
