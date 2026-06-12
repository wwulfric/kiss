package kiss

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommandManifestIsRejected(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "command-skill")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "name = \"command-skill\"\nversion = \"0.18.0\"\nentry = \"SKILL.md\"\n\n[runner]\ntype = \"command\"\ncommand = \"scripts/run.sh\"\n"
	if err := os.WriteFile(filepath.Join(dir, "kiss.skill.toml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("command instructions\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	err = AddLocalSkill(paths, dir, "command-skill")
	if err == nil || !strings.Contains(err.Error(), `runner type "command" is not supported`) {
		t.Fatalf("expected unsupported runner rejection, got: %v", err)
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
