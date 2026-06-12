package kiss

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunMissingSkillDoesNotInstall(t *testing.T) {
	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err = RunSkill(paths, "missing", nil, &out)
	if err == nil {
		t.Fatal("expected missing skill error")
	}
	if !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("expected not installed error, got %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("expected no output for missing skill, got %q", out.String())
	}
}

func TestRunSkillArgsAreSingleLine(t *testing.T) {
	home := t.TempDir()
	paths, err := NewPaths(home)
	if err != nil {
		t.Fatal(err)
	}
	source := makeSkill(t, "browser-test", "0.1.0", "Use this one.")
	if err := AddLocalSkill(paths, source, "browser-test"); err != nil {
		t.Fatalf("add browser-test: %v", err)
	}
	var out bytes.Buffer
	if err := RunSkill(paths, "browser-test", []string{"a\nb", "c\rd"}, &out); err != nil {
		t.Fatalf("run: %v", err)
	}
	if strings.Contains(out.String(), "- Args: a\n") || strings.Contains(out.String(), "- Args: c\r") {
		t.Fatalf("args should not contain newlines or carriage returns:\n%s", out.String())
	}
}
