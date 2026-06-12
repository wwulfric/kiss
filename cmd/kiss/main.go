package main

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/wwulfric/kiss/internal/kiss"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "kiss: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	kissHome, remaining, err := extractGlobalFlags(args)
	if err != nil {
		return err
	}
	if len(remaining) == 0 {
		printHelp()
		return nil
	}
	if remaining[0] == "--version" || remaining[0] == "version" {
		fmt.Println(kiss.Version)
		return nil
	}
	if remaining[0] == "--help" || remaining[0] == "help" {
		printHelp()
		return nil
	}
	paths, err := kiss.NewPaths(kissHome)
	if err != nil {
		return err
	}

	switch remaining[0] {
	case "add":
		return runAdd(paths, remaining[1:])
	case "run":
		return runSkill(paths, remaining[1:])
	case "update":
		return runUpdate(paths, remaining[1:])
	case "list":
		return kiss.ListSkills(paths, os.Stdout)
	case "show":
		if len(remaining) != 2 {
			return fmt.Errorf("usage: kiss show <name>")
		}
		return kiss.ShowSkill(paths, remaining[1], os.Stdout)
	case "remove":
		if len(remaining) != 2 {
			return fmt.Errorf("usage: kiss remove <name>")
		}
		return kiss.RemoveSkill(paths, remaining[1])
	case "doctor":
		return kiss.Doctor(paths, os.Stdout)
	default:
		return fmt.Errorf("unknown command %q", remaining[0])
	}
}

func extractGlobalFlags(args []string) (string, []string, error) {
	remaining := make([]string, 0, len(args))
	kissHome := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--kiss-home":
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("--kiss-home requires a value")
			}
			kissHome = args[i+1]
			i++
		default:
			remaining = append(remaining, arg)
		}
	}
	return kissHome, remaining, nil
}

func runAdd(paths kiss.Paths, args []string) error {
	request, err := parseAddArgs(args)
	if err != nil {
		return err
	}
	source, name, err := resolveAddRequest(request)
	if err != nil {
		return err
	}
	if isRemoteSource(source) {
		return kiss.AddRemoteSkill(paths, source, name)
	}
	return kiss.AddLocalSkill(paths, source, name)
}

type addRequest struct {
	source string
	name   string
	skill  string
}

func parseAddArgs(args []string) (addRequest, error) {
	request := addRequest{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--name":
			if i+1 >= len(args) {
				return addRequest{}, fmt.Errorf("%s requires a value", args[i])
			}
			request.name = args[i+1]
			i++
		case "--skill":
			if i+1 >= len(args) {
				return addRequest{}, fmt.Errorf("%s requires a value", args[i])
			}
			request.skill = args[i+1]
			i++
		default:
			if strings.HasPrefix(args[i], "-") {
				return addRequest{}, fmt.Errorf("unknown add flag %q", args[i])
			}
			if request.source != "" {
				return addRequest{}, fmt.Errorf("usage: kiss add <source> [--name <name>] [--skill <skill>]")
			}
			request.source = args[i]
		}
	}
	if request.source == "" {
		return addRequest{}, fmt.Errorf("usage: kiss add <source> [--name <name>] [--skill <skill>]")
	}
	return request, nil
}

func resolveAddRequest(request addRequest) (string, string, error) {
	source := request.source
	name := request.name
	if isLocalSource(source) {
		if request.skill != "" {
			return "", "", fmt.Errorf("--skill is only supported for GitHub sources")
		}
		if name == "" {
			inferred, err := kiss.InferLocalSkillName(source)
			if err != nil {
				return "", "", err
			}
			name = inferred
		}
		return source, name, nil
	}

	githubSource, isGitHub, err := normalizeGitHubAddSource(source)
	if err != nil {
		return "", "", err
	}
	if isGitHub {
		source, name, err = applyGitHubSkillSelection(githubSource, request.skill, name)
		if err != nil {
			return "", "", err
		}
		return source, name, nil
	}

	if isRemoteSource(source) {
		if request.skill != "" {
			return "", "", fmt.Errorf("--skill is only supported for GitHub sources")
		}
		if name == "" {
			return "", "", fmt.Errorf("--name is required for generic remote sources")
		}
		return source, name, nil
	}

	if request.skill != "" {
		return "", "", fmt.Errorf("--skill is only supported for GitHub sources")
	}
	if name != "" {
		return "", "", fmt.Errorf("--name is only supported for local paths, file:// URLs, GitHub sources, and https tarballs")
	}
	return "", "", fmt.Errorf("unsupported add source %q; use a local path, file:// URL, owner/repo, github:owner/repo[/path][#ref], or https://...tar.gz with --name", source)
}

func isRemoteSource(source string) bool {
	return strings.HasPrefix(source, "https://") || strings.HasPrefix(source, "github:")
}

func isLocalSource(source string) bool {
	if strings.HasPrefix(strings.ToLower(source), "file://") {
		return true
	}
	if strings.HasPrefix(source, ".") || strings.HasPrefix(source, "~") || strings.HasPrefix(source, "/") || strings.Contains(source, `\`) {
		return true
	}
	if _, err := os.Stat(source); err == nil {
		return true
	}
	return false
}

func normalizeGitHubAddSource(source string) (string, bool, error) {
	if strings.HasPrefix(source, "github:") {
		return source, true, nil
	}
	if strings.HasPrefix(source, "https://") {
		parsed, err := url.Parse(source)
		if err != nil {
			return "", false, err
		}
		if !strings.EqualFold(parsed.Host, "github.com") {
			return "", false, nil
		}
		parts := splitPathSegments(parsed.Path)
		if len(parts) == 2 {
			repo := strings.TrimSuffix(parts[1], ".git")
			return "github:" + parts[0] + "/" + repo, true, nil
		}
		if len(parts) >= 5 && parts[2] == "tree" {
			repo := strings.TrimSuffix(parts[1], ".git")
			ref := parts[3]
			path := strings.Join(parts[4:], "/")
			return "github:" + parts[0] + "/" + repo + "/" + path + "#" + ref, true, nil
		}
		return "", false, nil
	}
	if isGitHubShorthand(source) {
		return "github:" + source, true, nil
	}
	return "", false, nil
}

func applyGitHubSkillSelection(source, skill, name string) (string, string, error) {
	gh, err := kiss.ParseGitHubSource(source)
	if err != nil {
		return "", "", err
	}
	if skill != "" {
		if gh.Path != "" {
			return "", "", fmt.Errorf("--skill cannot be combined with a GitHub source that already includes a path")
		}
		if strings.Contains(skill, `\`) || strings.HasPrefix(skill, "/") {
			return "", "", fmt.Errorf("invalid --skill value %q", skill)
		}
		skillPath := skill
		if !strings.Contains(skill, "/") {
			skillPath = "skills/" + skill
		}
		source = "github:" + gh.Owner + "/" + gh.Repo + "/" + skillPath
		if gh.Ref != "" && gh.Ref != "main" {
			source += "#" + gh.Ref
		}
		gh, err = kiss.ParseGitHubSource(source)
		if err != nil {
			return "", "", err
		}
		if name == "" {
			name = lastPathSegment(skill)
		}
	}
	if name == "" {
		if gh.Path != "" {
			name = lastPathSegment(gh.Path)
		} else {
			name = strings.TrimSuffix(gh.Repo, ".git")
		}
	}
	if err := kiss.ValidateSkillName(name); err != nil {
		return "", "", err
	}
	return source, name, nil
}

func isGitHubShorthand(source string) bool {
	if strings.Contains(source, "://") || strings.ContainsAny(source, `\`) {
		return false
	}
	parts := splitPathSegments(source)
	if len(parts) != 2 {
		return false
	}
	for _, part := range parts {
		if part == "." || part == ".." || strings.HasPrefix(part, ".") {
			return false
		}
	}
	return true
}

func splitPathSegments(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	raw := strings.Split(trimmed, "/")
	parts := make([]string, 0, len(raw))
	for _, part := range raw {
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}

func lastPathSegment(path string) string {
	parts := splitPathSegments(path)
	if len(parts) == 0 {
		return path
	}
	return parts[len(parts)-1]
}

func runSkill(paths kiss.Paths, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: kiss run <name> [args...]")
	}
	runArgs := args[1:]
	if len(runArgs) > 0 && runArgs[0] == "--" {
		runArgs = runArgs[1:]
	}
	return kiss.RunSkill(paths, args[0], runArgs, os.Stdout)
}

func runUpdate(paths kiss.Paths, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: kiss update <name> [--yes]")
	}
	name := ""
	yes := false
	for _, arg := range args {
		switch arg {
		case "--yes", "-y":
			yes = true
		default:
			if name != "" {
				return fmt.Errorf("usage: kiss update <name> [--yes]")
			}
			name = arg
		}
	}
	if name == "" {
		return fmt.Errorf("usage: kiss update <name> [--yes]")
	}
	return kiss.UpdateSkill(paths, name, yes, os.Stdout)
}

func printHelp() {
	fmt.Printf(`kiss %s

Usage:
  kiss [--kiss-home <path>] add <source> [--name <name>] [--skill <skill>]
  kiss [--kiss-home <path>] run <name> [args...]
  kiss [--kiss-home <path>] update <name> [--yes]
  kiss [--kiss-home <path>] list
  kiss [--kiss-home <path>] show <name>
  kiss [--kiss-home <path>] remove <name>
  kiss [--kiss-home <path>] doctor
  kiss --version
`, kiss.Version)
}
