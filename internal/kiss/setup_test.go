package kiss

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmbeddedKISSSkillMatchesPublicSkill(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "skills", "kiss", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != kissBridgeSkillMarkdown {
		t.Fatal("embedded KISS skill must match skills/kiss/SKILL.md")
	}
}

func TestSetupAgentSkillInstallsDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	var out bytes.Buffer
	if err := SetupAgentSkill(SetupOptions{}, &out); err != nil {
		t.Fatalf("setup default: %v", err)
	}
	target := filepath.Join(home, ".agents", "skills", "kiss", "SKILL.md")
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != kissBridgeSkillMarkdown {
		t.Fatal("installed default skill content mismatch")
	}
	if !strings.Contains(out.String(), "Installed KISS skill for .agents: "+target) {
		t.Fatalf("unexpected setup output: %s", out.String())
	}

	out.Reset()
	if err := SetupAgentSkill(SetupOptions{}, &out); err != nil {
		t.Fatalf("repeat setup default: %v", err)
	}
	if !strings.Contains(out.String(), "already installed") {
		t.Fatalf("expected idempotent setup output, got %s", out.String())
	}
}

func TestSetupAgentSkillInstallsClaude(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	var out bytes.Buffer
	if err := SetupAgentSkill(SetupOptions{Agent: "claude"}, &out); err != nil {
		t.Fatalf("setup claude: %v", err)
	}
	target := filepath.Join(home, ".claude", "skills", "kiss", "SKILL.md")
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != kissBridgeSkillMarkdown {
		t.Fatal("installed claude skill content mismatch")
	}
	if !strings.Contains(out.String(), "Installed KISS skill for claude: "+target) {
		t.Fatalf("unexpected setup output: %s", out.String())
	}
}

func TestSetupAgentSkillUpdatesDifferentContent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	target := filepath.Join(home, ".agents", "skills", "kiss", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := SetupAgentSkill(SetupOptions{}, &out); err != nil {
		t.Fatalf("setup update: %v", err)
	}
	if !strings.Contains(out.String(), "Updated KISS skill") {
		t.Fatalf("expected update output, got %s", out.String())
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != kissBridgeSkillMarkdown {
		t.Fatal("expected setup to replace stale skill content")
	}
}

func TestSetupAgentSkillRejectsUnsupportedOptions(t *testing.T) {
	for _, tc := range []SetupOptions{
		{Agent: "cursor"},
		{Agent: "codex"},
	} {
		if err := SetupAgentSkill(tc, &bytes.Buffer{}); err == nil {
			t.Fatalf("expected setup rejection for %+v", tc)
		}
	}
}
