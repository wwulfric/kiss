package kiss

import (
	"bytes"
	"strings"
	"testing"
)

func TestUpdateSkillPlansThenAppliesRemoteUpdate(t *testing.T) {
	oldArchive := makeNestedSkillArchive(t, "repo-main", "skills/browser-test", "browser-test", "0.3.0", "Old remote instructions.")
	newArchive := makeNestedSkillArchive(t, "repo-main", "skills/browser-test", "browser-test", "0.3.1", "New remote instructions.")
	refs := map[string]githubRefArchive{
		"main": {sha: "1111111111111111111111111111111111111111", archive: oldArchive},
	}
	defer withGitHubArchiveByRefTestServer(t, refs, nil)()

	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := AddRemoteSkill(paths, "github:owner/repo/skills/browser-test@main", "browser-test"); err != nil {
		t.Fatalf("add remote skill: %v", err)
	}
	metadata, ok, err := GetSkillMetadata(paths, "browser-test")
	if err != nil || !ok {
		t.Fatalf("metadata missing ok=%v err=%v", ok, err)
	}
	refs["main"] = githubRefArchive{sha: "2222222222222222222222222222222222222222", archive: newArchive}

	var plan bytes.Buffer
	if err := UpdateSkill(paths, "browser-test", false, &plan); err != nil {
		t.Fatalf("plan update: %v", err)
	}
	if !strings.Contains(plan.String(), "Run `kiss update browser-test --yes` to apply.") {
		t.Fatalf("missing update apply hint:\n%s", plan.String())
	}
	for _, want := range []string{
		"- Current version: " + metadata.Version,
		"- Skill folder: changed",
		"- Entry content: changed",
		"## Entry diff",
		"```diff",
		"--- current/SKILL.md",
		"+++ target/SKILL.md",
		"-Old remote instructions.",
		"+New remote instructions.",
	} {
		if !strings.Contains(plan.String(), want) {
			t.Fatalf("update plan missing %q:\n%s", want, plan.String())
		}
	}
	if strings.Index(plan.String(), "-Old remote instructions.") > strings.Index(plan.String(), "+New remote instructions.") {
		t.Fatalf("expected deletion to appear before addition:\n%s", plan.String())
	}
	var out bytes.Buffer
	if err := RunSkill(paths, "browser-test", nil, &out); err != nil {
		t.Fatalf("run after plan: %v", err)
	}
	if !strings.Contains(out.String(), "Old remote instructions.") || strings.Contains(out.String(), "New remote instructions.") {
		t.Fatalf("plan should not apply update:\n%s", out.String())
	}

	var applyOut bytes.Buffer
	if err := UpdateSkill(paths, "browser-test", true, &applyOut); err != nil {
		t.Fatalf("apply update: %v", err)
	}
	if !strings.Contains(applyOut.String(), "Updated browser-test") {
		t.Fatalf("unexpected apply output: %s", applyOut.String())
	}
	out.Reset()
	if err := RunSkill(paths, "browser-test", nil, &out); err != nil {
		t.Fatalf("run after apply: %v", err)
	}
	if !strings.Contains(out.String(), "New remote instructions.") {
		t.Fatalf("expected updated instructions:\n%s", out.String())
	}
}

func TestUpdateSkillPlanShowsNoDiffWhenEntryUnchanged(t *testing.T) {
	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source := makeSkill(t, "browser-test", "0.10.0", "Stable instructions.")
	if err := AddLocalSkill(paths, source, "browser-test"); err != nil {
		t.Fatalf("add local skill: %v", err)
	}

	var plan bytes.Buffer
	if err := UpdateSkill(paths, "browser-test", false, &plan); err != nil {
		t.Fatalf("plan update: %v", err)
	}
	got := plan.String()
	if !strings.Contains(got, "## Entry diff") || !strings.Contains(got, "Entry content unchanged; no diff.") {
		t.Fatalf("expected unchanged diff note:\n%s", got)
	}
	if strings.Contains(got, "```diff") {
		t.Fatalf("unchanged plan should not emit a diff block:\n%s", got)
	}
}

func TestUpdateSkillYesSkipsWhenFolderHashUnchanged(t *testing.T) {
	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source := makeSkill(t, "browser-test", "0.10.0", "Stable instructions.")
	if err := AddLocalSkill(paths, source, "browser-test"); err != nil {
		t.Fatalf("add local skill: %v", err)
	}
	before, ok, err := GetSkillMetadata(paths, "browser-test")
	if err != nil || !ok {
		t.Fatalf("metadata missing ok=%v err=%v", ok, err)
	}

	var out bytes.Buffer
	if err := UpdateSkill(paths, "browser-test", true, &out); err != nil {
		t.Fatalf("apply unchanged update: %v", err)
	}
	after, ok, err := GetSkillMetadata(paths, "browser-test")
	if err != nil || !ok {
		t.Fatalf("metadata missing ok=%v err=%v", ok, err)
	}
	if before.Version != after.Version {
		t.Fatalf("expected unchanged version, before=%q after=%q", before.Version, after.Version)
	}
	if !strings.Contains(out.String(), "already up to date") {
		t.Fatalf("expected up-to-date output, got %s", out.String())
	}
}

func TestUpdateSkillWithRefOverrideUpdatesSourceAndContent(t *testing.T) {
	v1Archive := makeNestedSkillArchive(t, "repo-main", "skills/browser-test", "browser-test", "0.1.0", "Version one.")
	v2Archive := makeNestedSkillArchive(t, "repo-main", "skills/browser-test", "browser-test", "0.2.0", "Version two.")
	defer withGitHubArchiveByRefTestServer(t, map[string]githubRefArchive{
		"v1": {sha: "1111111111111111111111111111111111111111", archive: v1Archive},
		"v2": {sha: "2222222222222222222222222222222222222222", archive: v2Archive},
	}, nil)()

	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := AddRemoteSkill(paths, "github:owner/repo/skills/browser-test@v1", "browser-test"); err != nil {
		t.Fatalf("add v1: %v", err)
	}

	var plan bytes.Buffer
	if err := UpdateSkillWithOptions(paths, "browser-test", UpdateOptions{Ref: "v2"}, &plan); err != nil {
		t.Fatalf("plan update with ref override: %v", err)
	}
	if !strings.Contains(plan.String(), "Run `kiss update browser-test@v2 --yes` to apply.") {
		t.Fatalf("expected ref-preserving apply hint:\n%s", plan.String())
	}

	var out bytes.Buffer
	if err := UpdateSkillWithOptions(paths, "browser-test", UpdateOptions{Yes: true, Ref: "v2"}, &out); err != nil {
		t.Fatalf("update with ref override: %v", err)
	}
	metadata, ok, err := GetSkillMetadata(paths, "browser-test")
	if err != nil || !ok {
		t.Fatalf("metadata missing ok=%v err=%v", ok, err)
	}
	if metadata.Source.Ref != "v2" || metadata.Source.Resolved != "2222222222222222222222222222222222222222" {
		t.Fatalf("unexpected updated source: %+v", metadata.Source)
	}
	var runOut bytes.Buffer
	if err := RunSkill(paths, "browser-test", nil, &runOut); err != nil {
		t.Fatalf("run updated skill: %v", err)
	}
	if !strings.Contains(runOut.String(), "Version two.") {
		t.Fatalf("expected v2 instructions:\n%s", runOut.String())
	}
}

func TestUpdateSkillWithRefOverrideRefreshesSourceWhenFolderUnchanged(t *testing.T) {
	archive := makeNestedSkillArchive(t, "repo-main", "skills/browser-test", "browser-test", "0.1.0", "Same content.")
	defer withGitHubArchiveByRefTestServer(t, map[string]githubRefArchive{
		"v1": {sha: "1111111111111111111111111111111111111111", archive: archive},
		"v2": {sha: "2222222222222222222222222222222222222222", archive: archive},
	}, nil)()

	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := AddRemoteSkill(paths, "github:owner/repo/skills/browser-test@v1", "browser-test"); err != nil {
		t.Fatalf("add v1: %v", err)
	}
	before, ok, err := GetSkillMetadata(paths, "browser-test")
	if err != nil || !ok {
		t.Fatalf("metadata missing ok=%v err=%v", ok, err)
	}

	var out bytes.Buffer
	if err := UpdateSkillWithOptions(paths, "browser-test", UpdateOptions{Yes: true, Ref: "v2"}, &out); err != nil {
		t.Fatalf("update ref with unchanged folder: %v", err)
	}
	after, ok, err := GetSkillMetadata(paths, "browser-test")
	if err != nil || !ok {
		t.Fatalf("metadata missing ok=%v err=%v", ok, err)
	}
	if after.Source.Ref != "v2" || after.Source.Resolved != "2222222222222222222222222222222222222222" {
		t.Fatalf("expected source ref metadata refresh, got %+v", after.Source)
	}
	if after.Version != before.Version {
		t.Fatalf("expected same skill folder hash, before=%q after=%q", before.Version, after.Version)
	}
	if !strings.Contains(out.String(), "skill folder unchanged") {
		t.Fatalf("expected unchanged folder output, got %s", out.String())
	}
}

func TestUpdateSkillRejectsSkillsImportSnapshot(t *testing.T) {
	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source := makeSkill(t, "story-helper", "0.1.0", "Imported snapshot.")
	if err := AddLocalSkill(paths, source, "story-helper"); err != nil {
		t.Fatalf("add local skill: %v", err)
	}
	metadata, ok, err := GetSkillMetadata(paths, "story-helper")
	if err != nil || !ok {
		t.Fatalf("metadata missing ok=%v err=%v", ok, err)
	}
	metadata.Source = SourceMetadata{Kind: "skills-import", URI: source}
	if err := UpsertSkillMetadata(paths, metadata); err != nil {
		t.Fatalf("upsert metadata: %v", err)
	}

	err = UpdateSkill(paths, "story-helper", false, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "re-run kiss import") {
		t.Fatalf("expected skills-import update hint, got: %v", err)
	}
}
