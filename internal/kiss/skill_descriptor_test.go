package kiss

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSkillDescriptorUsesRootSkillMarkdown(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "markdown-only")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("Use this skill.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := LoadSkillDescriptor(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "markdown-only" || len(got.Version) != 64 || got.Entry != "SKILL.md" {
		t.Fatalf("unexpected descriptor: %+v", got)
	}
}

func TestLoadSkillDescriptorRequiresRootSkillMarkdown(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadSkillDescriptor(dir); err == nil {
		t.Fatal("expected missing SKILL.md error")
	}
}

func TestLoadSkillDescriptorIgnoresKissSkillToml(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ignored-toml")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("Use root markdown.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "kiss.skill.toml"), []byte("name = \"toml-name\"\nversion = \"9.9.9\"\nentry = \"../escape.md\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := LoadSkillDescriptor(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "ignored-toml" || len(got.Version) != 64 || got.Entry != "SKILL.md" {
		t.Fatalf("expected TOML to be ignored, got %+v", got)
	}
}
