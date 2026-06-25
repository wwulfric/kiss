package kiss

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportSkillsImportsNamedGlobalSkillSnapshot(t *testing.T) {
	sha := "3333333333333333333333333333333333333333"
	defer withGitHubTestServer(t, sha, nil, nil)()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	writeSkillsGlobalSkill(t, home, "story-helper", "Use this skill for fiction outlines.")
	writeSkillsGlobalLock(t, filepath.Join(home, ".agents", ".skill-lock.json"), map[string]skillsLockEntry{
		"story-helper": {
			Source:          "owner/repo",
			SourceType:      "github",
			SourceURL:       "https://github.com/owner/repo.git",
			Ref:             "main",
			SkillPath:       "skills/story-helper/SKILL.md",
			SkillFolderHash: "abcdef",
			InstalledAt:     "2026-06-15T00:00:00Z",
			UpdatedAt:       "2026-06-15T00:00:00Z",
		},
	})
	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := ImportSkills(paths, ImportSkillsOptions{Names: []string{"story-helper"}}, &out); err != nil {
		t.Fatalf("import: %v", err)
	}

	metadata, ok, err := GetSkillMetadata(paths, "story-helper")
	if err != nil || !ok {
		t.Fatalf("metadata missing ok=%v err=%v", ok, err)
	}
	if metadata.Source.Kind != "github" {
		t.Fatalf("unexpected source kind: %+v", metadata.Source)
	}
	if metadata.Source.URI != "owner/repo/skills/story-helper" || metadata.Source.Ref != "main" || metadata.Source.Resolved != "" {
		t.Fatalf("unexpected github source metadata: %+v", metadata.Source)
	}
	if metadata.FullName != "github:owner/repo/skills/story-helper" {
		t.Fatalf("unexpected full name: %q", metadata.FullName)
	}
	if metadata.Source.SkillsImport == nil || metadata.Source.SkillsImport.SkillFolderHash != "abcdef" {
		t.Fatalf("missing import metadata: %+v", metadata.Source.SkillsImport)
	}
	var runOut bytes.Buffer
	if err := RunSkill(paths, "story-helper", nil, &runOut); err != nil {
		t.Fatalf("run imported skill: %v", err)
	}
	if !strings.Contains(runOut.String(), "fiction outlines") {
		t.Fatalf("missing imported instructions: %s", runOut.String())
	}
	if !strings.Contains(out.String(), "Imported story-helper") {
		t.Fatalf("unexpected output: %s", out.String())
	}
}

func TestImportSkillsFallsBackWhenSourceMetadataUnavailable(t *testing.T) {
	oldTransport := remoteHTTPTransport
	remoteHTTPTransport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, os.ErrNotExist
	})
	defer func() { remoteHTTPTransport = oldTransport }()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	writeSkillsGlobalSkill(t, home, "fallback", "Use fallback.")
	writeSkillsGlobalLock(t, filepath.Join(home, ".agents", ".skill-lock.json"), map[string]skillsLockEntry{
		"fallback": {
			Source:          "owner/repo",
			SourceType:      "github",
			SkillPath:       "skills/fallback/SKILL.md",
			SkillFolderHash: "hash",
		},
	})
	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	if err := ImportSkills(paths, ImportSkillsOptions{Names: []string{"fallback"}}, &bytes.Buffer{}); err != nil {
		t.Fatalf("import fallback: %v", err)
	}
	metadata, ok, err := GetSkillMetadata(paths, "fallback")
	if err != nil || !ok {
		t.Fatalf("metadata missing ok=%v err=%v", ok, err)
	}
	if metadata.Source.Kind != skillsImportSourceKind {
		t.Fatalf("expected skills-import fallback, got %+v", metadata.Source)
	}
	if metadata.Source.SkillsImport == nil || metadata.Source.SkillsImport.SkillFolderHash != "hash" {
		t.Fatalf("missing import audit metadata: %+v", metadata.Source.SkillsImport)
	}
}

func TestImportSkillsDryRunDoesNotWriteStore(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	writeSkillsGlobalSkill(t, home, "drafting", "Draft safely.")
	writeSkillsGlobalLock(t, filepath.Join(home, ".agents", ".skill-lock.json"), map[string]skillsLockEntry{
		"drafting": {Source: "owner/repo", SourceType: "github", SkillPath: "skills/drafting/SKILL.md"},
	})
	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := ImportSkills(paths, ImportSkillsOptions{Names: []string{"drafting"}, DryRun: true}, &out); err != nil {
		t.Fatalf("dry run import: %v", err)
	}
	if _, err := os.Stat(paths.SkillDir("drafting")); !os.IsNotExist(err) {
		t.Fatalf("expected no imported skill dir, got err=%v", err)
	}
	if !strings.Contains(out.String(), "Would import drafting") {
		t.Fatalf("unexpected dry-run output: %s", out.String())
	}
}

func TestImportSkillsAllImportsEveryLockedSkill(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	writeSkillsGlobalSkill(t, home, "alpha", "Alpha.")
	writeSkillsGlobalSkill(t, home, "beta", "Beta.")
	writeSkillsGlobalLock(t, filepath.Join(home, ".agents", ".skill-lock.json"), map[string]skillsLockEntry{
		"beta":  {Source: "owner/repo", SkillPath: "skills/beta/SKILL.md"},
		"alpha": {Source: "owner/repo", SkillPath: "skills/alpha/SKILL.md"},
	})
	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	if err := ImportSkills(paths, ImportSkillsOptions{All: true}, &bytes.Buffer{}); err != nil {
		t.Fatalf("import all: %v", err)
	}
	for _, name := range []string{"alpha", "beta"} {
		if _, ok, err := GetSkillMetadata(paths, name); err != nil || !ok {
			t.Fatalf("metadata missing for %s ok=%v err=%v", name, ok, err)
		}
	}
}

func TestImportSkillsUsesXDGStateLockPath(t *testing.T) {
	home := t.TempDir()
	xdgState := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", xdgState)
	writeSkillsGlobalSkill(t, home, "xdg-skill", "XDG lock.")
	writeSkillsGlobalLock(t, filepath.Join(xdgState, "skills", ".skill-lock.json"), map[string]skillsLockEntry{
		"xdg-skill": {Source: "owner/repo", SkillPath: "skills/xdg-skill/SKILL.md"},
	})
	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	if err := ImportSkills(paths, ImportSkillsOptions{Names: []string{"xdg-skill"}}, &bytes.Buffer{}); err != nil {
		t.Fatalf("import with XDG_STATE_HOME lock: %v", err)
	}
	metadata, ok, err := GetSkillMetadata(paths, "xdg-skill")
	if err != nil || !ok {
		t.Fatalf("metadata missing ok=%v err=%v", ok, err)
	}
	if metadata.Source.SkillsImport == nil || !strings.Contains(metadata.Source.SkillsImport.LockPath, xdgState) {
		t.Fatalf("expected XDG lock path, got %+v", metadata.Source.SkillsImport)
	}
}

func TestImportSkillsRejectsMissingSkill(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	writeSkillsGlobalLock(t, filepath.Join(home, ".agents", ".skill-lock.json"), map[string]skillsLockEntry{})
	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	err = ImportSkills(paths, ImportSkillsOptions{Names: []string{"missing"}}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected missing skill error, got: %v", err)
	}
}

func writeSkillsGlobalSkill(t *testing.T, home, name, instructions string) {
	t.Helper()
	dir := filepath.Join(home, ".agents", "skills", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(instructions+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeSkillsGlobalLock(t *testing.T, lockPath string, skills map[string]skillsLockEntry) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(skillsLockFile{Version: 3, Skills: skills}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(lockPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
