package kiss

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUpdateSkillPlansThenAppliesRemoteUpdate(t *testing.T) {
	oldArchive := makeSkillArchive(t, "remote-root", "browser-test", "0.3.0", "Old remote instructions.")
	newArchive := makeSkillArchive(t, "remote-root", "browser-test", "0.3.1", "New remote instructions.")
	currentArchive := oldArchive
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/browser-test.tar.gz":
			_, _ = w.Write(currentArchive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	oldTransport := remoteHTTPTransport
	remoteHTTPTransport = server.Client().Transport
	defer func() { remoteHTTPTransport = oldTransport }()

	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := AddRemoteSkill(paths, server.URL+"/browser-test.tar.gz", "browser-test"); err != nil {
		t.Fatalf("add remote skill: %v", err)
	}
	currentArchive = newArchive

	var plan bytes.Buffer
	if err := UpdateSkill(paths, "browser-test", false, &plan); err != nil {
		t.Fatalf("plan update: %v", err)
	}
	if !strings.Contains(plan.String(), "Run `kiss update browser-test --yes` to apply.") {
		t.Fatalf("missing update apply hint:\n%s", plan.String())
	}
	for _, want := range []string{
		"- Current version: 0.3.0",
		"- Target version: 0.3.1",
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
