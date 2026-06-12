package kiss

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
