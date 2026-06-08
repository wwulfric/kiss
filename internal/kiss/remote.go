package kiss

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var remoteHTTPTransport http.RoundTripper = http.DefaultTransport
var githubAPIBaseURL = "https://api.github.com"
var githubCodeloadBaseURL = "https://codeload.github.com"

var maxRemoteArchiveBytes int64 = 100 << 20    // 100 MiB
var maxExtractedArchiveBytes int64 = 200 << 20 // 200 MiB
var maxGitHubAPIResponseBytes int64 = 1 << 20  // 1 MiB

type ResolvedSource struct {
	Kind     string
	URI      string
	Ref      string
	Resolved string
	FullName string
}

type GitHubSource struct {
	Owner string
	Repo  string
	Path  string
	Ref   string
}

func AddRemoteSkill(paths Paths, sourceSpec, name string) error {
	return AddRemoteSkillWithExpectedSHA(paths, sourceSpec, name, "")
}

func AddRemoteSkillWithExpectedSHA(paths Paths, sourceSpec, name, expectedSHA256 string) error {
	if err := ValidateSkillName(name); err != nil {
		return err
	}
	resolved, downloadURL, subdir, err := ResolveRemoteSource(sourceSpec)
	if err != nil {
		return err
	}
	if err := paths.EnsureBase(); err != nil {
		return err
	}
	archivePath, sum, err := downloadArchive(paths, downloadURL, name)
	if err != nil {
		return err
	}
	if expectedSHA256 != "" && !strings.EqualFold(sum, expectedSHA256) {
		_ = os.Remove(archivePath)
		return fmt.Errorf("sha256 mismatch for %s: expected %s, got %s", sourceSpec, expectedSHA256, sum)
	}
	tmpParent := filepath.Join(paths.Home, ".tmp")
	if err := os.MkdirAll(tmpParent, 0o755); err != nil {
		return err
	}
	extractDir, err := os.MkdirTemp(tmpParent, name+"-remote-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(extractDir)
	if err := extractTarGz(archivePath, extractDir); err != nil {
		return err
	}
	skillDir := extractDir
	if subdir != "" {
		found, err := findExtractedSubdir(extractDir, subdir)
		if err != nil {
			return err
		}
		skillDir = found
	} else {
		found, err := findArchiveSkillRoot(extractDir)
		if err != nil {
			return err
		}
		skillDir = found
	}
	resolvedSource := SourceMetadata{
		Kind:     resolved.Kind,
		URI:      resolved.URI,
		Ref:      resolved.Ref,
		Resolved: resolved.Resolved,
		SHA256:   sum,
	}
	return installSkillFromDir(paths, skillDir, name, resolved.FullName, resolvedSource)
}

func ResolveRemoteSource(sourceSpec string) (ResolvedSource, string, string, error) {
	if strings.HasPrefix(sourceSpec, "github:") {
		gh, err := ParseGitHubSource(sourceSpec)
		if err != nil {
			return ResolvedSource{}, "", "", err
		}
		resolvedCommit, err := ResolveGitHubCommit(gh)
		if err != nil {
			return ResolvedSource{}, "", "", err
		}
		archiveURL := fmt.Sprintf("%s/%s/%s/tar.gz/%s", strings.TrimRight(githubCodeloadBaseURL, "/"), gh.Owner, gh.Repo, url.PathEscape(resolvedCommit))
		fullName := fmt.Sprintf("github:%s/%s", gh.Owner, gh.Repo)
		if gh.Path != "" {
			fullName += "/" + gh.Path
		}
		uri := gh.Owner + "/" + gh.Repo
		if gh.Path != "" {
			uri += "/" + gh.Path
		}
		return ResolvedSource{Kind: "github", URI: uri, Ref: gh.Ref, Resolved: resolvedCommit, FullName: fullName}, archiveURL, gh.Path, nil
	}
	if strings.HasPrefix(sourceSpec, "https://") {
		if _, err := url.ParseRequestURI(sourceSpec); err != nil {
			return ResolvedSource{}, "", "", err
		}
		return ResolvedSource{Kind: "https", URI: sourceSpec, FullName: sourceSpec}, sourceSpec, "", nil
	}
	return ResolvedSource{}, "", "", fmt.Errorf("unsupported remote source %q", sourceSpec)
}

func ResolveGitHubCommit(source GitHubSource) (string, error) {
	commitURL := fmt.Sprintf("%s/repos/%s/%s/commits/%s", strings.TrimRight(githubAPIBaseURL, "/"), source.Owner, source.Repo, url.PathEscape(source.Ref))
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
	req, err := http.NewRequest(http.MethodGet, commitURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "kiss-skill-store")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("resolve GitHub commit failed: %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxGitHubAPIResponseBytes+1))
	if err != nil {
		return "", err
	}
	if int64(len(data)) > maxGitHubAPIResponseBytes {
		return "", fmt.Errorf("GitHub commit response exceeds maximum allowed size of %d bytes", maxGitHubAPIResponseBytes)
	}
	var body struct {
		SHA string `json:"sha"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		return "", err
	}
	if body.SHA == "" {
		return "", fmt.Errorf("resolve GitHub commit failed: response did not contain sha")
	}
	return body.SHA, nil
}

func ParseGitHubSource(sourceSpec string) (GitHubSource, error) {
	spec := strings.TrimPrefix(sourceSpec, "github:")
	ref := "main"
	if before, after, ok := strings.Cut(spec, "#"); ok {
		spec = before
		if after != "" {
			ref = after
		}
	}
	parts := strings.Split(spec, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return GitHubSource{}, fmt.Errorf("github source must be github:owner/repo[/path]#ref")
	}
	path := ""
	if len(parts) > 2 {
		path = strings.Join(parts[2:], "/")
	}
	if err := validateOptionalSafeRelativePath(path); err != nil {
		return GitHubSource{}, fmt.Errorf("github skill path %q must be safe relative path: %w", path, err)
	}
	return GitHubSource{Owner: parts[0], Repo: parts[1], Path: path, Ref: ref}, nil
}

func downloadArchive(paths Paths, downloadURL, name string) (string, string, error) {
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
	resp, err := client.Get(downloadURL)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("download failed: %s", resp.Status)
	}
	if err := os.MkdirAll(filepath.Join(paths.Cache, "downloads"), 0o755); err != nil {
		return "", "", err
	}
	file, err := os.CreateTemp(filepath.Join(paths.Cache, "downloads"), name+"-*.tar.gz")
	if err != nil {
		return "", "", err
	}
	hash := sha256.New()
	limitedBody := io.LimitReader(resp.Body, maxRemoteArchiveBytes+1)
	n, err := io.Copy(io.MultiWriter(file, hash), limitedBody)
	if err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return "", "", err
	}
	if n > maxRemoteArchiveBytes {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return "", "", fmt.Errorf("download exceeds maximum allowed size of %d bytes", maxRemoteArchiveBytes)
	}
	if err := file.Close(); err != nil {
		return "", "", err
	}
	return file.Name(), hex.EncodeToString(hash.Sum(nil)), nil
}

func extractTarGz(archivePath, dest string) error {
	cleanDest := filepath.Clean(dest)
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var totalExtracted int64
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		target, err := safeArchiveTarget(cleanDest, header.Name)
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeXHeader, tar.TypeXGlobalHeader, tar.TypeGNULongName, tar.TypeGNULongLink:
			continue
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if header.Size < 0 {
				return fmt.Errorf("invalid archive entry size for %s", header.Name)
			}
			if totalExtracted+header.Size > maxExtractedArchiveBytes {
				return fmt.Errorf("extracted archive exceeds maximum allowed size of %d bytes", maxExtractedArchiveBytes)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode).Perm())
			if err != nil {
				return err
			}
			n, copyErr := io.Copy(out, io.LimitReader(tr, header.Size+1))
			closeErr := out.Close()
			if copyErr != nil {
				return copyErr
			}
			if n > header.Size {
				return fmt.Errorf("archive entry %s exceeds declared size", header.Name)
			}
			if n != header.Size {
				return fmt.Errorf("archive entry %s size mismatch: %w", header.Name, io.ErrUnexpectedEOF)
			}
			if closeErr != nil {
				return closeErr
			}
			totalExtracted += n
		case tar.TypeSymlink, tar.TypeLink:
			return fmt.Errorf("archive contains link entry (not allowed): %s", header.Name)
		default:
			return fmt.Errorf("unsupported archive entry type for %s", header.Name)
		}
	}
}

func safeArchiveTarget(dest, name string) (string, error) {
	if strings.Contains(name, `\`) {
		return "", fmt.Errorf("unsafe path in archive: %s", name)
	}
	trimmed := strings.Trim(name, "/")
	if strings.Contains(trimmed, "//") {
		return "", fmt.Errorf("unsafe path in archive: %s", name)
	}
	clean := filepath.Clean(name)
	if clean == "." || filepath.IsAbs(clean) {
		return "", fmt.Errorf("unsafe path in archive: %s", name)
	}
	target := filepath.Join(dest, clean)
	rel, err := filepath.Rel(dest, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("unsafe path in archive: %s", name)
	}
	return target, nil
}

func findExtractedSubdir(root, subdir string) (string, error) {
	cleanSubdir := filepath.Clean(subdir)
	if cleanSubdir == "." || filepath.IsAbs(cleanSubdir) || strings.HasPrefix(cleanSubdir, "..") {
		return "", fmt.Errorf("unsafe subdir %q", subdir)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		candidate := filepath.Join(root, entry.Name(), cleanSubdir)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate, nil
		}
	}
	candidate := filepath.Join(root, cleanSubdir)
	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		return candidate, nil
	}
	return "", fmt.Errorf("skill subdir %q not found in archive", subdir)
}

func findArchiveSkillRoot(root string) (string, error) {
	if _, err := os.Stat(filepath.Join(root, "SKILL.md")); err == nil {
		return root, nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	var matches []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		candidate := filepath.Join(root, entry.Name())
		if _, err := os.Stat(filepath.Join(candidate, "SKILL.md")); err == nil {
			matches = append(matches, candidate)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("archive does not contain SKILL.md at root or single top-level directory")
	}
	return "", fmt.Errorf("archive contains multiple top-level skill directories; use a source with explicit path")
}
