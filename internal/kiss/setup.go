package kiss

import (
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	setupAgentClaude = "claude"
)

//go:embed setup_assets/kiss/SKILL.md
var kissBridgeSkillMarkdown string

// SetupOptions 描述要把 KISS agent 入口安装到哪个 agent 目录。
type SetupOptions struct {
	// Agent 为空时安装到 .agents；目前只接受空值或 "claude"。
	Agent string
}

// SetupAgentSkill 把内置 KISS skill 安装到 agent 的原生 skill 目录。
func SetupAgentSkill(options SetupOptions, out io.Writer) error {
	agent := strings.ToLower(strings.TrimSpace(options.Agent))
	target, err := setupSkillTarget(agent)
	if err != nil {
		return err
	}
	action, err := writeSetupSkill(target)
	if err != nil {
		return err
	}
	writer := errWriter{out: out}
	label := setupAgentLabel(agent)
	switch action {
	case "unchanged":
		writer.printf("KISS skill already installed for %s: %s\n", label, target)
	case "updated":
		writer.printf("Updated KISS skill for %s: %s\n", label, target)
	default:
		writer.printf("Installed KISS skill for %s: %s\n", label, target)
	}
	return writer.err
}

func setupAgentLabel(agent string) string {
	if agent == "" {
		return ".agents"
	}
	return agent
}

func setupSkillTarget(agent string) (string, error) {
	agent = strings.ToLower(strings.TrimSpace(agent))
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch agent {
	case "":
		return filepath.Join(home, ".agents", "skills", "kiss", "SKILL.md"), nil
	case setupAgentClaude:
		return filepath.Join(home, ".claude", "skills", "kiss", "SKILL.md"), nil
	default:
		return "", fmt.Errorf("unsupported setup agent %q; omit --agent for .agents or use --agent claude", agent)
	}
}

func writeSetupSkill(target string) (string, error) {
	action := "installed"
	existing, err := os.ReadFile(target)
	if err == nil {
		if string(existing) == kissBridgeSkillMarkdown {
			return "unchanged", nil
		}
		action = "updated"
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(target, []byte(kissBridgeSkillMarkdown), 0o644); err != nil {
		return "", err
	}
	return action, nil
}
