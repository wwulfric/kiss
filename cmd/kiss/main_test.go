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
