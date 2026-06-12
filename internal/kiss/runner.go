package kiss

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// RunSkill 把被点名 skill 的 instructions 包装成 Markdown envelope 输出给调用方 agent。
func RunSkill(paths Paths, name string, args []string, out io.Writer) error {
	if err := ValidateSkillName(name); err != nil {
		return err
	}
	if err := paths.EnsureBase(); err != nil {
		return err
	}
	if _, ok, err := GetSkillMetadata(paths, name); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("skill %q is not installed; run kiss add <source> --name %s first", name, name)
	}
	skillDir := paths.SkillDir(name)
	manifest, err := LoadManifest(skillDir)
	if err != nil {
		return err
	}
	entryPath := filepath.Join(skillDir, manifest.Entry)
	content, err := os.ReadFile(entryPath)
	if err != nil {
		return err
	}

	// run 只加载被点名的 skill 并输出上下文，不自动安装、不执行脚本、不调用模型。
	writer := errWriter{out: out}
	writer.printf("# KISS loaded skill: %s\n\n", name)
	writer.println("## User invocation")
	writer.println()
	writer.printf("- Skill: %s\n", name)
	writer.printf("- Args: %s\n\n", formatArgs(args))
	writer.println("## Skill metadata")
	writer.println()
	writer.printf("- Manifest name: %s\n", manifest.Name)
	writer.printf("- Version: %s\n", manifest.Version)
	if manifest.Description != "" {
		writer.printf("- Description: %s\n", manifest.Description)
	}
	writer.println()
	writer.println("## Skill instructions")
	writer.println()
	writer.println(strings.TrimRight(string(content), "\n"))
	writer.println()
	writer.println("## KISS runtime notes")
	writer.println()
	writer.println("- Only this skill was loaded.")
	writer.println("- Do not inspect $KISS_HOME unless the user explicitly asks.")
	writer.println("- KISS did not call any model provider; the invoking agent decides how to use these instructions.")
	writer.println("- KISS does not execute skill scripts; the invoking agent decides what work to perform.")
	return writer.err
}

func formatArgs(args []string) string {
	if len(args) == 0 {
		return "(none)"
	}
	escaped := make([]string, len(args))
	for i, arg := range args {
		// Args 放在单行 Markdown 中，避免用户参数破坏 envelope 结构。
		singleLine := strings.NewReplacer("\r", " ", "\n", " ").Replace(arg)
		escaped[i] = strconv.Quote(singleLine)
	}
	return strings.Join(escaped, " ")
}
