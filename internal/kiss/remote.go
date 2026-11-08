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

var githubAPITimeout = 30 * time.Second
var archiveDownloadTimeout = 5 * time.Minute

var maxRemoteArchiveBytes int64 = 100 << 20    // 100 MiB
var maxExtractedArchiveBytes int64 = 200 << 20 // 200 MiB
var maxGitHubAPIResponseBytes int64 = 1 << 20  // 1 MiB

// ResolvedSource 是规范化后的 remote source，以及写入 metadata 的稳定身份信息。
type ResolvedSource struct {
	Kind     string
	URI      string
	Ref      string
	Resolved string
	FullName string
}

// GitHubSource 是 github:owner/repo[/path][@ref] 的解析结果。
type GitHubSource struct {
	Owner string
	Repo  string
	Path  string
	Ref   string
}

// AddRemoteSkill 从 GitHub source 安装 skill。
func AddRemoteSkill(paths Paths, sourceSpec, name string) error {
	return AddRemoteSkillWithOptions(paths, sourceSpec, name, AddOptions{})
}

// AddRemoteSkillWithOptions 从 GitHub source 安装 skill；skill 选择和替换规则见 AddSkills。
func AddRemoteSkillWithOptions(paths Paths, sourceSpec, name string, options AddOptions) error {
	if !strings.HasPrefix(sourceSpec, "github:") {
		return fmt.Errorf("unsupported remote source %q", sourceSpec)
	}
	_, err := AddSkills(paths, sourceSpec, name, options)
	return err
}

// openRemoteSkillSource 下载并解包 GitHub source，返回可供发现的仓库目录。
func openRemoteSkillSource(paths Paths, sourceSpec string) (*skillSource, error) {
	gh, err := ParseGitHubSource(sourceSpec)
	if err != nil {
		return nil, err
	}
	resolved, downloadURL, _, err := ResolveRemoteSource(sourceSpec)
	if err != nil {
		return nil, err
	}
	if err := paths.EnsureBase(); err != nil {
		return nil, err
	}
	archivePath, sum, err := downloadArchive(paths, downloadURL, gh.Repo)
	if err != nil {
		return nil, err
	}
	tmpParent := filepath.Join(paths.Home, ".tmp")
	if err := os.MkdirAll(tmpParent, 0o755); err != nil {
		return nil, err
	}
	extractDir, err := os.MkdirTemp(tmpParent, "remote-")
	if err != nil {
		return nil, err
	}
	cleanup := func() { _ = os.RemoveAll(extractDir) }
	skippedLinks, err := extractSkillSourceTarGz(archivePath, extractDir)
	if err != nil {
		cleanup()
		return nil, err
	}
	repoRoot, err := archiveRepoRoot(extractDir)
	if err != nil {
		cleanup()
		return nil, err
	}
	return &skillSource{
		root:    repoRoot,
		subpath: gh.Path,
		cleanup: cleanup,
		check: func(skill discoveredSkill) error {
			return rejectLinksInSkill(extractDir, skill.Dir, skippedLinks)
		},
		metadata: func(skill discoveredSkill) (SourceMetadata, string) {
			uri := gh.Owner + "/" + gh.Repo
			if skill.RelPath != "" {
				uri += "/" + skill.RelPath
			}
			return SourceMetadata{
				Kind:     resolved.Kind,
				URI:      uri,
				Ref:      resolved.Ref,
				Resolved: resolved.Resolved,
				SHA256:   sum,
			}, "github:" + uri
		},
	}, nil
}

// rejectLinksInSkill 拒绝落在选定 skill 目录内的 archive link 条目，与本地安装拒绝 symlink 的规则一致。
func rejectLinksInSkill(extractDir, skillDir string, links []string) error {
	for _, link := range links {
		if isContainedPath(filepath.Join(extractDir, filepath.FromSlash(link)), skillDir) {
			return fmt.Errorf("archive contains link entry (not allowed): %s", link)
		}
	}
	return nil
}

// archiveRepoRoot 返回解包后的仓库根目录；GitHub codeload tarball 会多包一层 <repo>-<sha>/ 目录。
func archiveRepoRoot(extractDir string) (string, error) {
	if hasSkillMd(extractDir) {
		return extractDir, nil
	}
	entries, err := os.ReadDir(extractDir)
	if err != nil {
		return "", err
	}
	if len(entries) == 1 && entries[0].IsDir() {
		return filepath.Join(extractDir, entries[0].Name()), nil
	}
	return extractDir, nil
}

// ResolveRemoteSource 把 remote source 解析成 metadata、下载 URL 和可选 archive 子目录。
func ResolveRemoteSource(sourceSpec string) (ResolvedSource, string, string, error) {
	if strings.HasPrefix(sourceSpec, "github:") {
		gh, err := ParseGitHubSource(sourceSpec)
		if err != nil {
			return ResolvedSource{}, "", "", err
		}
		resolvedCommit, resolvedRef, err := ResolveGitHubCommit(gh)
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
		return ResolvedSource{Kind: "github", URI: uri, Ref: resolvedRef, Resolved: resolvedCommit, FullName: fullName}, archiveURL, gh.Path, nil
	}
	return ResolvedSource{}, "", "", fmt.Errorf("unsupported remote source %q", sourceSpec)
}

// ResolveGitHubCommit 把 GitHub ref 解析成 commit SHA，用于记录可复现的 metadata。
func ResolveGitHubCommit(source GitHubSource) (string, string, error) {
	if source.Ref != "" {
		sha, err := resolveGitHubCommitRef(source, source.Ref)
		return sha, source.Ref, err
	}
	var lastErr error
	for _, ref := range []string{"main", "master"} {
		sha, err := resolveGitHubCommitRef(source, ref)
		if err == nil {
			return sha, ref, nil
		}
		lastErr = err
	}
	return "", "", fmt.Errorf("resolve GitHub commit failed for default refs main/master: %w", lastErr)
}

func resolveGitHubCommitRef(source GitHubSource, ref string) (string, error) {
	commitURL := fmt.Sprintf("%s/repos/%s/%s/commits/%s", strings.TrimRight(githubAPIBaseURL, "/"), source.Owner, source.Repo, url.PathEscape(ref))
	client := remoteHTTPClient(configuredRemoteTimeout("KISS_HTTP_TIMEOUT", githubAPITimeout))
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
	defer func() { _ = resp.Body.Close() }()
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

// ParseGitHubSource 解析 github:owner/repo[/path][@ref] 语法，并兼容旧的 #ref。
func ParseGitHubSource(sourceSpec string) (GitHubSource, error) {
	spec := strings.TrimPrefix(sourceSpec, "github:")
	ref := ""
	if before, after, ok := strings.Cut(spec, "#"); ok {
		spec = before
		if after != "" {
			ref = after
		}
	} else if before, after, ok := splitGitRefSuffix(spec); ok {
		spec = before
		ref = after
	}
	parts := strings.Split(spec, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return GitHubSource{}, fmt.Errorf("github source must be github:owner/repo[/path][@ref]")
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

func splitGitRefSuffix(spec string) (string, string, bool) {
	at := strings.LastIndex(spec, "@")
	if at <= 0 || at == len(spec)-1 {
		return spec, "", false
	}
	return spec[:at], spec[at+1:], true
}

func downloadArchive(paths Paths, downloadURL, name string) (string, string, error) {
	client := remoteHTTPClient(configuredRemoteTimeout("KISS_DOWNLOAD_TIMEOUT", archiveDownloadTimeout))
	resp, err := client.Get(downloadURL)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = resp.Body.Close() }()
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

func remoteHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: remoteHTTPTransport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL.Scheme != "https" {
				return fmt.Errorf("redirect to non-https URL is not allowed: %s", req.URL.String())
			}
			return nil
		},
	}
}

func configuredRemoteTimeout(envName string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(envName))
	if value == "" {
		return fallback
	}
	timeout, err := time.ParseDuration(value)
	if err != nil || timeout <= 0 {
		return fallback
	}
	return timeout
}

func extractTarGz(archivePath, dest string) error {
	_, err := extractTarGzWithLinkPolicy(archivePath, dest, false)
	return err
}

// extractSkillSourceTarGz 解包 skill source archive；link 条目不会写到磁盘，而是返回其 slash 路径，
// 由调用方在选定 skill 后判断：只有落在被安装 skill 目录内的 link 才拒绝，仓库其他位置的 link 不影响安装。
func extractSkillSourceTarGz(archivePath, dest string) ([]string, error) {
	return extractTarGzWithLinkPolicy(archivePath, dest, true)
}

func extractTarGzWithLinkPolicy(archivePath, dest string, skipLinks bool) ([]string, error) {
	// 远程 archive 只允许写出普通文件和目录；link 要么拒绝、要么跳过，越界路径一律拒绝，避免解包写出目标目录。
	cleanDest := filepath.Clean(dest)
	var skippedLinks []string
	file, err := os.Open(archivePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	var totalExtracted int64
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return skippedLinks, nil
		}
		if err != nil {
			return nil, err
		}
		target, err := safeArchiveTarget(cleanDest, header.Name)
		if err != nil {
			return nil, err
		}
		switch header.Typeflag {
		case tar.TypeXHeader, tar.TypeXGlobalHeader, tar.TypeGNULongName, tar.TypeGNULongLink:
			continue
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return nil, err
			}
		case tar.TypeReg:
			if header.Size < 0 {
				return nil, fmt.Errorf("invalid archive entry size for %s", header.Name)
			}
			if totalExtracted+header.Size > maxExtractedArchiveBytes {
				return nil, fmt.Errorf("extracted archive exceeds maximum allowed size of %d bytes", maxExtractedArchiveBytes)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return nil, err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode).Perm())
			if err != nil {
				return nil, err
			}
			n, copyErr := io.Copy(out, io.LimitReader(tr, header.Size+1))
			closeErr := out.Close()
			if copyErr != nil {
				return nil, copyErr
			}
			if n > header.Size {
				return nil, fmt.Errorf("archive entry %s exceeds declared size", header.Name)
			}
			if n != header.Size {
				return nil, fmt.Errorf("archive entry %s size mismatch: %w", header.Name, io.ErrUnexpectedEOF)
			}
			if closeErr != nil {
				return nil, closeErr
			}
			totalExtracted += n
		case tar.TypeSymlink, tar.TypeLink:
			if !skipLinks {
				return nil, fmt.Errorf("archive contains link entry (not allowed): %s", header.Name)
			}
			rel, err := filepath.Rel(cleanDest, target)
			if err != nil {
				return nil, err
			}
			skippedLinks = append(skippedLinks, filepath.ToSlash(rel))
		default:
			return nil, fmt.Errorf("unsupported archive entry type for %s", header.Name)
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
