package kiss

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

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

	fmt.Fprintf(out, "# KISS loaded skill: %s\n\n", name)
	fmt.Fprintln(out, "## User invocation")
	fmt.Fprintln(out)
	fmt.Fprintf(out, "- Skill: %s\n", name)
	fmt.Fprintf(out, "- Args: %s\n\n", formatArgs(args))
	fmt.Fprintln(out, "## Skill metadata")
	fmt.Fprintln(out)
	fmt.Fprintf(out, "- Manifest name: %s\n", manifest.Name)
	fmt.Fprintf(out, "- Version: %s\n", manifest.Version)
	if manifest.Description != "" {
		fmt.Fprintf(out, "- Description: %s\n", manifest.Description)
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "## Skill instructions")
	fmt.Fprintln(out)
	fmt.Fprintln(out, strings.TrimRight(string(content), "\n"))
	fmt.Fprintln(out)
	fmt.Fprintln(out, "## KISS runtime notes")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "- Only this skill was loaded.")
	fmt.Fprintln(out, "- Do not inspect $KISS_HOME unless the user explicitly asks.")
	fmt.Fprintln(out, "- Command runner is disabled in this iteration; follow Markdown instructions only.")
	return nil
}

func formatArgs(args []string) string {
	if len(args) == 0 {
		return "(none)"
	}
	escaped := make([]string, len(args))
	for i, arg := range args {
		singleLine := strings.NewReplacer("\r", " ", "\n", " ").Replace(arg)
		escaped[i] = strconv.Quote(singleLine)
	}
	return strings.Join(escaped, " ")
}
