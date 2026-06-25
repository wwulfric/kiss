package kiss

import (
	"os"
	"path/filepath"
	"testing"
)

func TestComputeSkillFolderHashTracksWholeFolder(t *testing.T) {
	dir := makeSkill(t, "hash-skill", "0.1.0", "Use hash skill.")
	first, err := computeSkillFolderHash(dir)
	if err != nil {
		t.Fatal(err)
	}
	if first == "" || len(first) != 64 {
		t.Fatalf("expected sha256 hash, got %q", first)
	}

	if err := os.MkdirAll(filepath.Join(dir, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "references", "guide.md"), []byte("Guide.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := computeSkillFolderHash(dir)
	if err != nil {
		t.Fatal(err)
	}
	if second == first {
		t.Fatal("expected nested file to change skill folder hash")
	}
}

func TestComputeSkillFolderHashFramesPathAndContent(t *testing.T) {
	left := filepath.Join(t.TempDir(), "left")
	if err := os.MkdirAll(left, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(left, "SKILL.md"), []byte("same\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(left, "a"), []byte("bc"), 0o644); err != nil {
		t.Fatal(err)
	}

	right := filepath.Join(t.TempDir(), "right")
	if err := os.MkdirAll(right, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(right, "SKILL.md"), []byte("same\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(right, "ab"), []byte("c"), 0o644); err != nil {
		t.Fatal(err)
	}

	leftHash, err := computeSkillFolderHash(left)
	if err != nil {
		t.Fatal(err)
	}
	rightHash, err := computeSkillFolderHash(right)
	if err != nil {
		t.Fatal(err)
	}
	if leftHash == rightHash {
		t.Fatalf("expected framed path/content hash to differ, got %s", leftHash)
	}
}

func TestComputeSkillFolderHashSkipsInstallRecordAndNodeModules(t *testing.T) {
	dir := makeSkill(t, "hash-skip", "0.1.0", "Use hash skill.")
	first, err := computeSkillFolderHash(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, installRecordFileName), []byte("metadata\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "config"), []byte("ignored\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "node_modules", "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "node_modules", "pkg", "index.js"), []byte("ignored\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := computeSkillFolderHash(dir)
	if err != nil {
		t.Fatal(err)
	}
	if second != first {
		t.Fatalf("expected install record, .git, and node_modules to be ignored, before=%s after=%s", first, second)
	}
}
