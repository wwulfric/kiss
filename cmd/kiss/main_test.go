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

func TestTopLevelCommandAliases(t *testing.T) {
	for _, arg := range []string{"--version", "-v", "version"} {
		if !isVersionCommand(arg) {
			t.Fatalf("expected %q to be a version command", arg)
		}
	}
	for _, arg := range []string{"--help", "-h", "help"} {
		if !isHelpCommand(arg) {
			t.Fatalf("expected %q to be a help command", arg)
		}
	}
}

func TestResolveAddSourceFollowsNpxSkillsGrammar(t *testing.T) {
	tests := []struct {
		input      string
		wantSource string
		wantSkill  string
	}{
		{"owner/repo", "github:owner/repo", ""},
		{"owner/repo@tdd", "github:owner/repo", "tdd"},
		{"owner/repo/.claude/skills", "github:owner/repo/.claude/skills", ""},
		{"owner/repo/sub@x", "github:owner/repo/sub@x#", ""},
		{"owner/repo#v1", "github:owner/repo#v1", ""},
		{"owner/repo#v1@tdd", "github:owner/repo#v1", "tdd"},
		{"owner/repo#@tdd", "github:owner/repo", "tdd"},
		{"github:owner/repo@tdd", "github:owner/repo", "tdd"},
		{"github:owner/repo/skills#main", "github:owner/repo/skills#main", ""},
		{"https://github.com/owner/repo", "github:owner/repo", ""},
		{"https://github.com/owner/repo.git", "github:owner/repo", ""},
		{"github.com/owner/repo", "github:owner/repo", ""},
		{"https://github.com/owner/repo/tree/main", "github:owner/repo#main", ""},
		{"https://github.com/owner/repo/tree/main/skills/tdd", "github:owner/repo/skills/tdd#main", ""},
		{"https://github.com/owner/repo#feature%2Fx@tdd", "github:owner/repo#feature/x", "tdd"},
		{"vercel-labs/vercel-skills", "github:vercel-labs/agent-skills", ""},
	}
	for _, tt := range tests {
		got, err := resolveAddSource(tt.input)
		if err != nil {
			t.Fatalf("%s: %v", tt.input, err)
		}
		if got.source != tt.wantSource || got.skill != tt.wantSkill {
			t.Fatalf("%s: got source=%q skill=%q, want %q %q", tt.input, got.source, got.skill, tt.wantSource, tt.wantSkill)
		}
	}
}

func TestResolveAddSourceLocalPaths(t *testing.T) {
	for _, input := range []string{"./skills/tdd", "../x", ".", "/abs/path", "~/skills/x", "file:///abs/x"} {
		got, err := resolveAddSource(input)
		if err != nil || got.source != input {
			t.Fatalf("%s: expected local source, got %+v err=%v", input, got, err)
		}
	}
}

func TestResolveAddSourceRejectsUnsupported(t *testing.T) {
	for _, input := range []string{"browser-test", "https://example.com/browser-test.tar.gz", "gitlab:group/repo", "owner/repo/../x"} {
		if _, err := resolveAddSource(input); err == nil {
			t.Fatalf("%s: expected error", input)
		}
	}
	_, err := resolveAddSource("github.com/owner/repo@v1.0.0")
	if err == nil || !strings.Contains(err.Error(), "#v1.0.0") {
		t.Fatalf("expected #ref hint for old URL @ref syntax, got %v", err)
	}
}

func TestParseAddArgsFollowsNpxSkillsFlags(t *testing.T) {
	got, err := parseAddArgs([]string{"owner/repo", "-s", "tdd", "why", "--skill", "teach", "-y", "-g", "-a", "claude-code", "codex", "--copy", "--full-depth", "--force"})
	if err != nil {
		t.Fatal(err)
	}
	if got.source != "owner/repo" || strings.Join(got.skills, ",") != "tdd,why,teach" || !got.force || !got.fullDepth {
		t.Fatalf("unexpected add request: %+v", got)
	}
	if strings.Join(got.ignored, ",") != "-a,--copy" {
		t.Fatalf("unexpected ignored flags: %v", got.ignored)
	}
	got, err = parseAddArgs([]string{"owner/repo", "--all", "-l"})
	if err != nil || !got.all || !got.list {
		t.Fatalf("unexpected add request: %+v err=%v", got, err)
	}
	if _, err := parseAddArgs([]string{"owner/repo", "--skill", "skills/tdd"}); err == nil || !strings.Contains(err.Error(), "skill name") {
		t.Fatalf("expected --skill path rejection, got %v", err)
	}
	if _, err := parseAddArgs([]string{"owner/repo", "--skill"}); err == nil {
		t.Fatal("expected --skill without value error")
	}
	if _, err := parseAddArgs([]string{"owner/repo", "--bogus"}); err == nil {
		t.Fatal("expected unknown flag error")
	}
}

func writeLocalSkillRepo(t *testing.T, names ...string) string {
	t.Helper()
	repo := t.TempDir()
	for _, name := range names {
		dir := filepath.Join(repo, "skills", name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		content := "---\nname: " + name + "\ndescription: " + name + " skill\n---\n"
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return repo
}

func TestRunAddLocalMultiSkillRepo(t *testing.T) {
	paths, err := kiss.NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := writeLocalSkillRepo(t, "tdd", "why", "teach")
	err = runAdd(paths, []string{repo})
	if err == nil || !strings.Contains(err.Error(), "use --skill") || !strings.Contains(err.Error(), "tdd, teach, why") {
		t.Fatalf("expected multi-skill hint, got %v", err)
	}
	if err := runAdd(paths, []string{repo, "-s", "why", "tdd"}); err != nil {
		t.Fatalf("add -s why tdd: %v", err)
	}
	for _, name := range []string{"why", "tdd"} {
		metadata, ok, err := kiss.GetSkillMetadata(paths, name)
		if err != nil || !ok {
			t.Fatalf("%s metadata missing ok=%v err=%v", name, ok, err)
		}
		if metadata.Source.URI != filepath.Join(repo, "skills", name) {
			t.Fatalf("unexpected source metadata: %+v", metadata.Source)
		}
	}
	if err := runAdd(paths, []string{repo, "--all"}); err != nil {
		t.Fatalf("add --all: %v", err)
	}
	if _, ok, _ := kiss.GetSkillMetadata(paths, "teach"); !ok {
		t.Fatal("expected --all to install teach")
	}
}

func TestPrintSourceSkills(t *testing.T) {
	var out strings.Builder
	err := printSourceSkills(&out, []kiss.SourceSkill{{Name: "root", Description: "d"}, {Name: "tdd", Path: "skills/tdd", Description: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	if out.String() != "root\t.\td\ntdd\tskills/tdd\tt\n" {
		t.Fatalf("unexpected list output: %q", out.String())
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

func TestParseUpgradeArgs(t *testing.T) {
	got, err := parseUpgradeArgs([]string{"--version", "0.1.2", "--force"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "0.1.2" || !got.Force {
		t.Fatalf("unexpected upgrade options: %+v", got)
	}

	got, err = parseUpgradeArgs(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "" || got.Force {
		t.Fatalf("unexpected default upgrade options: %+v", got)
	}
}

func TestParseUpgradeRejectsUnknownArg(t *testing.T) {
	_, err := parseUpgradeArgs([]string{"--apply"})
	if err == nil || !strings.Contains(err.Error(), "usage: kiss upgrade") {
		t.Fatalf("expected upgrade usage error, got %v", err)
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
