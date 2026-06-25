package main

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wwulfric/kiss/internal/kiss"
)

func TestRunAddLocalFileURLInfersName(t *testing.T) {
	paths, err := kiss.NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "file-skill")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("Use file URL skill.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fileURL := (&url.URL{Scheme: "file", Path: source}).String()

	if err := runAdd(paths, []string{fileURL}); err != nil {
		t.Fatalf("add file URL: %v", err)
	}
	metadata, ok, err := kiss.GetSkillMetadata(paths, "file-skill")
	if err != nil || !ok {
		t.Fatalf("metadata missing ok=%v err=%v", ok, err)
	}
	if metadata.Source.Kind != "local" || metadata.Source.URI != source {
		t.Fatalf("unexpected source metadata: %+v", metadata.Source)
	}
}

func TestResolveAddRequestGitHubShorthandWithSkill(t *testing.T) {
	source, name, err := resolveAddRequest(addRequest{
		source: "vercel-labs/agent-skills",
		skill:  "frontend-design",
	})
	if err != nil {
		t.Fatal(err)
	}
	if source != "github:vercel-labs/agent-skills/skills/frontend-design" {
		t.Fatalf("unexpected source: %q", source)
	}
	if name != "frontend-design" {
		t.Fatalf("unexpected name: %q", name)
	}
}

func TestResolveAddRequestGitHubAtRefWithSkill(t *testing.T) {
	source, name, err := resolveAddRequest(addRequest{
		source: "github.com/owner/repo@v1.0.0",
		skill:  "browser-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if source != "github:owner/repo/skills/browser-test@v1.0.0" || name != "browser-test" {
		t.Fatalf("unexpected source/name: %q %q", source, name)
	}
}

func TestResolveAddRequestOwnerRepoAtRef(t *testing.T) {
	source, name, err := resolveAddRequest(addRequest{
		source: "owner/repo@feature/test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if source != "github:owner/repo@feature/test" || name != "repo" {
		t.Fatalf("unexpected source/name: %q %q", source, name)
	}
}

func TestParseAddArgsForce(t *testing.T) {
	got, err := parseAddArgs([]string{"owner/repo", "--skill", "browser-test", "--force"})
	if err != nil {
		t.Fatal(err)
	}
	if !got.force || got.source != "owner/repo" || got.skill != "browser-test" {
		t.Fatalf("unexpected add request: %+v", got)
	}
}

func TestResolveAddRequestGitHubURL(t *testing.T) {
	source, name, err := resolveAddRequest(addRequest{
		source: "https://github.com/owner/repo",
	})
	if err != nil {
		t.Fatal(err)
	}
	if source != "github:owner/repo" || name != "repo" {
		t.Fatalf("unexpected source/name: %q %q", source, name)
	}
}

func TestResolveAddRequestRejectsGenericHTTPS(t *testing.T) {
	_, _, err := resolveAddRequest(addRequest{
		source: "https://example.com/browser-test.tar.gz",
		name:   "browser-test",
	})
	if err == nil || !strings.Contains(err.Error(), "--name is only supported") {
		t.Fatalf("expected generic https rejection, got %v", err)
	}
	if strings.Contains(err.Error(), "tarball") {
		t.Fatalf("generic https rejection should not mention tarball support: %v", err)
	}
}

func TestResolveAddRequestRejectsBareName(t *testing.T) {
	_, _, err := resolveAddRequest(addRequest{source: "browser-test"})
	if err == nil || !strings.Contains(err.Error(), "unsupported add source") {
		t.Fatalf("expected bare source rejection, got %v", err)
	}
}

func TestParseAddRejectsOldSkillAliasForLocalPaths(t *testing.T) {
	source := filepath.Join(t.TempDir(), "local-skill")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("local\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := resolveAddRequest(addRequest{source: source, skill: "alias"})
	if err == nil || !strings.Contains(err.Error(), "GitHub sources") {
		t.Fatalf("expected --skill local path rejection, got %v", err)
	}
}

func TestParseImportArgs(t *testing.T) {
	got, err := parseImportArgs([]string{"--skill", "find-skills", "--dry-run"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Names) != 1 || got.Names[0] != "find-skills" || !got.DryRun || got.All {
		t.Fatalf("unexpected options: %+v", got)
	}
}

func TestParseImportRejectsGlobalFlag(t *testing.T) {
	_, err := parseImportArgs([]string{"--global", "--skill", "find-skills"})
	if err == nil || !strings.Contains(err.Error(), "does not support") {
		t.Fatalf("expected --global rejection, got %v", err)
	}
}

func TestParseImportRequiresExplicitScope(t *testing.T) {
	_, err := parseImportArgs(nil)
	if err == nil || !strings.Contains(err.Error(), "usage: kiss import") {
		t.Fatalf("expected usage error, got %v", err)
	}
	_, err = parseImportArgs([]string{"--all", "--skill", "find-skills"})
	if err == nil || !strings.Contains(err.Error(), "usage: kiss import") {
		t.Fatalf("expected mutually-exclusive usage error, got %v", err)
	}
}

func TestParseSetupArgs(t *testing.T) {
	got, err := parseSetupArgs([]string{"--agent", "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Agent != "claude" {
		t.Fatalf("unexpected setup options: %+v", got)
	}

	got, err = parseSetupArgs(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Agent != "" {
		t.Fatalf("unexpected default setup options: %+v", got)
	}
}

func TestParseSetupRejectsUnknownArg(t *testing.T) {
	for _, args := range [][]string{
		{"--dir", "."},
	} {
		_, err := parseSetupArgs(args)
		if err == nil || !strings.Contains(err.Error(), "usage: kiss setup") {
			t.Fatalf("expected setup usage error for %v, got %v", args, err)
		}
	}
}

func TestRunSetupInstallsDefaultSkill(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if err := run([]string{"setup"}); err != nil {
		t.Fatalf("run setup: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "kiss", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}

func TestRunSetupInstallsClaudeSkill(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if err := run([]string{"setup", "--agent", "claude"}); err != nil {
		t.Fatalf("run setup: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "kiss", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}

func TestRunUpdateParsesAtRef(t *testing.T) {
	paths, err := kiss.NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "local-skill")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("local\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := kiss.AddLocalSkill(paths, source, "local-skill"); err != nil {
		t.Fatal(err)
	}

	err = runUpdate(paths, []string{"local-skill@main"})
	if err == nil || !strings.Contains(err.Error(), "@ref can only be used with GitHub-backed skills") {
		t.Fatalf("expected non-github @ref rejection, got %v", err)
	}
}
