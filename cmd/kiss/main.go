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
	if isVersionCommand(remaining[0]) {
		fmt.Println(kiss.Version)
		return nil
	}
	if isHelpCommand(remaining[0]) {
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
	case "import":
		return runImport(paths, remaining[1:])
	case "setup":
		return runSetup(remaining[1:])
	case "run":
		return runSkill(paths, remaining[1:])
	case "update":
		return runUpdate(paths, remaining[1:])
	case "upgrade":
		return runUpgrade(remaining[1:])
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

func runUpgrade(args []string) error {
	options, err := parseUpgradeArgs(args)
	if err != nil {
		return err
	}
	return kiss.UpgradeCLI(options, os.Stdout)
}

func parseUpgradeArgs(args []string) (kiss.UpgradeOptions, error) {
	options := kiss.UpgradeOptions{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--version":
			if i+1 >= len(args) {
				return kiss.UpgradeOptions{}, fmt.Errorf("%s requires a value", args[i])
			}
			options.Version = args[i+1]
			i++
		case "--force":
			options.Force = true
		default:
			return kiss.UpgradeOptions{}, fmt.Errorf("usage: kiss upgrade [--version <version>] [--force]")
		}
	}
	return options, nil
}

func isVersionCommand(arg string) bool {
	return arg == "--version" || arg == "-v" || arg == "version"
}

func isHelpCommand(arg string) bool {
	return arg == "--help" || arg == "-h" || arg == "help"
}

func runSetup(args []string) error {
	options, err := parseSetupArgs(args)
	if err != nil {
		return err
	}
	return kiss.SetupAgentSkill(options, os.Stdout)
}

func parseSetupArgs(args []string) (kiss.SetupOptions, error) {
	options := kiss.SetupOptions{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--agent":
			if i+1 >= len(args) {
				return kiss.SetupOptions{}, fmt.Errorf("%s requires a value", args[i])
			}
			options.Agent = args[i+1]
			i++
		default:
			return kiss.SetupOptions{}, fmt.Errorf("usage: kiss setup [--agent claude]")
		}
	}
	return options, nil
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
		return kiss.AddRemoteSkillWithOptions(paths, source, name, kiss.AddOptions{Force: request.force})
	}
	return kiss.AddLocalSkillWithOptions(paths, source, name, kiss.AddOptions{Force: request.force})
}

type addRequest struct {
	source string
	name   string
	skill  string
	force  bool
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
		case "--force":
			request.force = true
		default:
			if strings.HasPrefix(args[i], "-") {
				return addRequest{}, fmt.Errorf("unknown add flag %q", args[i])
			}
			if request.source != "" {
				return addRequest{}, fmt.Errorf("usage: kiss add <source> [--name <name>] [--skill <skill>] [--force]")
			}
			request.source = args[i]
		}
	}
	if request.source == "" {
		return addRequest{}, fmt.Errorf("usage: kiss add <source> [--name <name>] [--skill <skill>] [--force]")
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

	if request.skill != "" {
		return "", "", fmt.Errorf("--skill is only supported for GitHub sources")
	}
	if name != "" {
		return "", "", fmt.Errorf("--name is only supported for local paths, file:// URLs, and GitHub sources")
	}
	return "", "", fmt.Errorf("unsupported add source %q; use a local path, file:// URL, owner/repo[@ref], github.com/owner/repo[@ref], or github:owner/repo[/path][@ref]", source)
}

func isRemoteSource(source string) bool {
	return strings.HasPrefix(source, "github:")
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
		path, ref, _ := splitGitRefSuffixForCLI(parsed.Path)
		parts := splitPathSegments(path)
		if len(parts) == 2 {
			repo, repoRef, _ := splitGitRefSuffixForCLI(strings.TrimSuffix(parts[1], ".git"))
			if ref == "" {
				ref = repoRef
			}
			return appendGitRef("github:"+parts[0]+"/"+repo, ref), true, nil
		}
		if len(parts) >= 5 && parts[2] == "tree" {
			repo := strings.TrimSuffix(parts[1], ".git")
			ref := parts[3]
			path := strings.Join(parts[4:], "/")
			return appendGitRef("github:"+parts[0]+"/"+repo+"/"+path, ref), true, nil
		}
		return "", false, nil
	}
	if strings.HasPrefix(source, "github.com/") {
		path, ref, _ := splitGitRefSuffixForCLI(strings.TrimPrefix(source, "github.com/"))
		parts := splitPathSegments(path)
		if len(parts) >= 2 {
			repo := strings.TrimSuffix(parts[1], ".git")
			spec := "github:" + parts[0] + "/" + repo
			if len(parts) > 2 {
				spec += "/" + strings.Join(parts[2:], "/")
			}
			return appendGitRef(spec, ref), true, nil
		}
		return "", false, nil
	}
	if isGitHubShorthand(source) {
		path, ref, _ := splitGitRefSuffixForCLI(source)
		parts := splitPathSegments(path)
		repo := strings.TrimSuffix(parts[1], ".git")
		return appendGitRef("github:"+parts[0]+"/"+repo, ref), true, nil
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
		source = appendGitRef(source, gh.Ref)
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
	path, _, _ := splitGitRefSuffixForCLI(source)
	parts := splitPathSegments(path)
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

func appendGitRef(source, ref string) string {
	if ref == "" {
		return source
	}
	return source + "@" + ref
}

func splitGitRefSuffixForCLI(source string) (string, string, bool) {
	at := strings.LastIndex(source, "@")
	if at <= 0 || at == len(source)-1 {
		return source, "", false
	}
	return source[:at], source[at+1:], true
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

func runImport(paths kiss.Paths, args []string) error {
	options, err := parseImportArgs(args)
	if err != nil {
		return err
	}
	return kiss.ImportSkills(paths, options, os.Stdout)
}

func parseImportArgs(args []string) (kiss.ImportSkillsOptions, error) {
	options := kiss.ImportSkillsOptions{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--skill":
			if i+1 >= len(args) {
				return kiss.ImportSkillsOptions{}, fmt.Errorf("%s requires a value", args[i])
			}
			options.Names = append(options.Names, args[i+1])
			i++
		case "--all":
			options.All = true
		case "--dry-run":
			options.DryRun = true
		case "--global", "-g":
			return kiss.ImportSkillsOptions{}, fmt.Errorf("import does not support %s; KISS imports into KISS_HOME, not an agent global skill directory", args[i])
		default:
			return kiss.ImportSkillsOptions{}, fmt.Errorf("unknown import flag %q", args[i])
		}
	}
	if options.All == (len(options.Names) > 0) {
		return kiss.ImportSkillsOptions{}, fmt.Errorf("usage: kiss import (--skill <name> | --all) [--dry-run]")
	}
	return options, nil
}

func runUpdate(paths kiss.Paths, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: kiss update <name>[@ref] [--yes]")
	}
	name := ""
	ref := ""
	yes := false
	for _, arg := range args {
		switch arg {
		case "--yes", "-y":
			yes = true
		default:
			if name != "" {
				return fmt.Errorf("usage: kiss update <name>[@ref] [--yes]")
			}
			name, ref, _ = splitGitRefSuffixForCLI(arg)
		}
	}
	if name == "" {
		return fmt.Errorf("usage: kiss update <name>[@ref] [--yes]")
	}
	return kiss.UpdateSkillWithOptions(paths, name, kiss.UpdateOptions{Yes: yes, Ref: ref}, os.Stdout)
}

func printHelp() {
	fmt.Printf(`kiss %s

Usage:
  kiss [--kiss-home <path>] add <source> [--name <name>] [--skill <skill>] [--force]
  kiss setup [--agent claude]
  kiss [--kiss-home <path>] import (--skill <name> | --all) [--dry-run]
  kiss [--kiss-home <path>] run <name> [args...]
  kiss [--kiss-home <path>] update <name>[@ref] [--yes]
  kiss upgrade [--version <version>] [--force]
  kiss [--kiss-home <path>] list
  kiss [--kiss-home <path>] show <name>
  kiss [--kiss-home <path>] remove <name>
  kiss [--kiss-home <path>] doctor
  kiss --version | -v
  kiss --help | -h
`, kiss.Version)
}
