package kiss

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

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
		archiveURL := fmt.Sprintf("https://codeload.github.com/%s/%s/tar.gz/%s", gh.Owner, gh.Repo, url.PathEscape(gh.Ref))
		fullName := fmt.Sprintf("github:%s/%s", gh.Owner, gh.Repo)
		if gh.Path != "" {
			fullName += "/" + gh.Path
		}
		return ResolvedSource{Kind: "github", URI: gh.Owner + "/" + gh.Repo + "/" + gh.Path, Ref: gh.Ref, Resolved: gh.Ref, FullName: fullName}, archiveURL, gh.Path, nil
	}
	if strings.HasPrefix(sourceSpec, "https://") {
		if _, err := url.ParseRequestURI(sourceSpec); err != nil {
			return ResolvedSource{}, "", "", err
		}
		return ResolvedSource{Kind: "https", URI: sourceSpec, FullName: sourceSpec}, sourceSpec, "", nil
	}
	return ResolvedSource{}, "", "", fmt.Errorf("unsupported remote source %q", sourceSpec)
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
	if strings.Contains(filepath.Clean(path), "..") || strings.HasPrefix(path, "/") {
		return GitHubSource{}, fmt.Errorf("github skill path must be safe relative path")
	}
	return GitHubSource{Owner: parts[0], Repo: parts[1], Path: path, Ref: ref}, nil
}

func downloadArchive(paths Paths, downloadURL, name string) (string, string, error) {
	resp, err := http.Get(downloadURL)
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
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(file, hash), resp.Body); err != nil {
		return "", "", err
	}
	return file.Name(), hex.EncodeToString(hash.Sum(nil)), nil
}

func extractTarGz(archivePath, dest string) error {
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
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		clean := filepath.Clean(header.Name)
		if clean == "." || filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") {
			return fmt.Errorf("unsafe path in archive: %s", header.Name)
		}
		target := filepath.Join(dest, clean)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode).Perm())
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(out, tr)
			closeErr := out.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		default:
			return fmt.Errorf("unsupported archive entry type for %s", header.Name)
		}
	}
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
