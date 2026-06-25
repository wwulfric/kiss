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

func TestAddLocalSkillRefreshesSameSourceWithoutForce(t *testing.T) {
	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source := makeSkill(t, "refresh", "0.1.0", "old instructions")
	if err := AddLocalSkill(paths, source, "refresh"); err != nil {
		t.Fatalf("add original: %v", err)
	}
	originalMetadata, ok, err := GetSkillMetadata(paths, "refresh")
	if err != nil || !ok {
		t.Fatalf("metadata missing ok=%v err=%v", ok, err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("new instructions\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := AddLocalSkill(paths, source, "refresh"); err != nil {
		t.Fatalf("refresh same source: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(paths.SkillDir("refresh"), "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "new instructions") {
		t.Fatalf("expected refreshed content, got: %q", string(content))
	}
	metadata, ok, err := GetSkillMetadata(paths, "refresh")
	if err != nil || !ok {
		t.Fatalf("metadata missing ok=%v err=%v", ok, err)
	}
	if metadata.InstalledAt != originalMetadata.InstalledAt {
		t.Fatalf("expected installed_at to be preserved, before=%q after=%q", originalMetadata.InstalledAt, metadata.InstalledAt)
	}
}

func TestAddLocalSkillRejectsDifferentSourceWithoutForce(t *testing.T) {
	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	original := makeSkill(t, "replace", "0.1.0", "original instructions")
	replacement := makeSkill(t, "replace", "0.2.0", "replacement instructions")
	if err := AddLocalSkill(paths, original, "replace"); err != nil {
		t.Fatalf("add original: %v", err)
	}

	err = AddLocalSkill(paths, replacement, "replace")
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected force hint for different source, got %v", err)
	}
	content, readErr := os.ReadFile(filepath.Join(paths.SkillDir("replace"), "SKILL.md"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !strings.Contains(string(content), "original instructions") {
		t.Fatalf("expected original content to remain, got: %q", string(content))
	}

	if err := AddLocalSkillWithOptions(paths, replacement, "replace", AddOptions{Force: true}); err != nil {
		t.Fatalf("force replace: %v", err)
	}
	content, readErr = os.ReadFile(filepath.Join(paths.SkillDir("replace"), "SKILL.md"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !strings.Contains(string(content), "replacement instructions") {
		t.Fatalf("expected replacement content, got: %q", string(content))
	}
}

func TestAddLocalSkillSkipsIgnoredPackageEntries(t *testing.T) {
	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source := makeSkill(t, "skip-entries", "0.1.0", "Use this skill.")
	if err := os.WriteFile(filepath.Join(source, installRecordFileName), []byte("source metadata\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(source, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, ".git", "config"), []byte("secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(source, "node_modules", "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "node_modules", "pkg", "index.js"), []byte("ignored\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := AddLocalSkill(paths, source, "skip-entries"); err != nil {
		t.Fatalf("add local skill: %v", err)
	}

	for _, skipped := range []string{
		filepath.Join(paths.SkillDir("skip-entries"), ".git"),
		filepath.Join(paths.SkillDir("skip-entries"), "node_modules"),
	} {
		if _, err := os.Stat(skipped); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be skipped, err=%v", skipped, err)
		}
	}
	data, err := os.ReadFile(filepath.Join(paths.SkillDir("skip-entries"), installRecordFileName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "source metadata") {
		t.Fatalf("expected source install record to be replaced, got %s", string(data))
	}
	metadata, ok, err := GetSkillMetadata(paths, "skip-entries")
	if err != nil || !ok {
		t.Fatalf("metadata missing ok=%v err=%v", ok, err)
	}
	installedHash, err := computeSkillFolderHash(paths.SkillDir("skip-entries"))
	if err != nil {
		t.Fatal(err)
	}
	if installedHash != metadata.Version {
		t.Fatalf("expected installed hash to match metadata version, installed=%s metadata=%s", installedHash, metadata.Version)
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
	if err := os.WriteFile(filepath.Join(original, "SKILL.md"), []byte("new instructions\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	badMetadataPath := filepath.Join(paths.Home, "bad-metadata")
	if err := os.MkdirAll(badMetadataPath, 0o755); err != nil {
		t.Fatal(err)
	}
	paths.MetadataDB = badMetadataPath

	err = AddLocalSkill(paths, original, "rollback")
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
