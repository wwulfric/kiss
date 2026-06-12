package kiss

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestAddLocalSkillAcceptsFileURL(t *testing.T) {
	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source := makeSkill(t, "file-url-skill", "0.1.0", "file URL instructions")
	fileURL := (&url.URL{Scheme: "file", Path: source}).String()
	if err := AddLocalSkill(paths, fileURL, "file-url-skill"); err != nil {
		t.Fatalf("add file URL: %v", err)
	}
	metadata, ok, err := GetSkillMetadata(paths, "file-url-skill")
	if err != nil || !ok {
		t.Fatalf("metadata missing ok=%v err=%v", ok, err)
	}
	if metadata.Source.URI != source || metadata.FullName != "local:"+source {
		t.Fatalf("unexpected metadata source: %+v full=%q", metadata.Source, metadata.FullName)
	}
}

func TestAddLocalSkillRejectsRemoteFileURLHost(t *testing.T) {
	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	err = AddLocalSkill(paths, "file://example.com/tmp/skill", "bad-file-url")
	if err == nil || !strings.Contains(err.Error(), "host") {
		t.Fatalf("expected file URL host rejection, got %v", err)
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
