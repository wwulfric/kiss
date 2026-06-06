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
	resolved, _, subdir, err := ResolveRemoteSource("github:vercel-labs/agent-skills/skills/browser-test#v1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.FullName != "github:vercel-labs/agent-skills/skills/browser-test" || resolved.Ref != "v1.2.3" || subdir != "skills/browser-test" {
		t.Fatalf("unexpected resolved=%+v subdir=%q", resolved, subdir)
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
	resolved, _, _, err := ResolveRemoteSource("github:owner/repo")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.URI != "owner/repo" {
		t.Fatalf("unexpected URI: %q", resolved.URI)
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
