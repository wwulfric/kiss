package kiss

import (
	"bytes"
	"os"
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
