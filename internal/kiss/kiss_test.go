package kiss

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
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
	oldTransport := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	defer func() { http.DefaultTransport = oldTransport }()

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
