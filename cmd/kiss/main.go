package main

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
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

const addUsage = "usage: kiss add <source> [-s|--skill <skills...>] [--all] [-l|--list] [--full-depth] [--name <name>] [--force]"

func runAdd(paths kiss.Paths, args []string) error {
	request, err := parseAddArgs(args)
	if err != nil {
		return err
	}
	for _, flag := range request.ignored {
		fmt.Fprintf(os.Stderr, "kiss: ignoring %s: KISS installs skills into its own store, not into agent directories\n", flag)
	}
	resolved, err := resolveAddSource(request.source)
	if err != nil {
		return err
	}
	if request.list {
		skills, err := kiss.ListSourceSkills(paths, resolved.source, request.fullDepth)
		if err != nil {
			return err
		}
		return printSourceSkills(os.Stdout, skills)
	}
	skills := append([]string{}, request.skills...)
	if resolved.skill != "" && !containsString(skills, resolved.skill) {
		skills = append(skills, resolved.skill)
	}
	installed, err := kiss.AddSkills(paths, resolved.source, request.name, kiss.AddOptions{
		Skills:    skills,
		All:       request.all,
		FullDepth: request.fullDepth,
		Force:     request.force,
	})
	if err != nil {
		if resolved.atSkill && strings.Contains(err.Error(), "no matching skills found") {
			return fmt.Errorf("%w (owner/repo@%s selects a skill, as in npx skills; to pin a git ref use owner/repo#%s)", err, resolved.skill, resolved.skill)
		}
		return err
	}
	if len(installed) > 0 {
		fmt.Printf("Installed %s\n", strings.Join(installed, ", "))
	}
	return nil
}

func printSourceSkills(out io.Writer, skills []kiss.SourceSkill) error {
	for _, skill := range skills {
		path := skill.Path
		if path == "" {
			path = "."
		}
		if _, err := fmt.Fprintf(out, "%s\t%s\t%s\n", skill.Name, path, skill.Description); err != nil {
			return err
		}
	}
	return nil
}

// addRequest 是 kiss add 的参数，flag 语法对齐 npx skills add。
type addRequest struct {
	source    string
	name      string
	skills    []string
	all       bool
	list      bool
	fullDepth bool
	force     bool
	// ignored 记录 npx skills 中只影响 agent 目录安装、对 KISS 无意义的 flag。
	ignored []string
}

func parseAddArgs(args []string) (addRequest, error) {
	request := addRequest{}
	// collectValues 与 npx skills 一致：吞掉后续所有不以 - 开头的参数。
	collectValues := func(i int) ([]string, int) {
		var values []string
		for i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			i++
			values = append(values, args[i])
		}
		return values, i
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-s", "--skill":
			var values []string
			values, i = collectValues(i)
			if len(values) == 0 {
				return addRequest{}, fmt.Errorf("%s requires at least one skill name", arg)
			}
			request.skills = append(request.skills, values...)
		case "--all":
			request.all = true
		case "-l", "--list":
			request.list = true
		case "--full-depth":
			request.fullDepth = true
		case "--force":
			request.force = true
		case "--name":
			if i+1 >= len(args) {
				return addRequest{}, fmt.Errorf("%s requires a value", arg)
			}
			request.name = args[i+1]
			i++
		case "-y", "--yes", "-g", "--global":
			// KISS 不交互、store 总在用户目录，这两个 flag 天然满足，静默接受。
		case "-a", "--agent", "--subagent":
			_, i = collectValues(i)
			request.ignored = append(request.ignored, arg)
		case "--copy":
			request.ignored = append(request.ignored, arg)
		default:
			if strings.HasPrefix(arg, "-") {
				return addRequest{}, fmt.Errorf("unknown add flag %q", arg)
			}
			if request.source != "" {
				return addRequest{}, fmt.Errorf("%s", addUsage)
			}
			request.source = arg
		}
	}
	if request.source == "" {
		return addRequest{}, fmt.Errorf("%s", addUsage)
	}
	for _, skill := range request.skills {
		if strings.Contains(skill, `\`) || strings.Contains(skill, "/") {
			return addRequest{}, fmt.Errorf("invalid --skill value %q: --skill takes a skill name; put repository paths in the source (owner/repo/path)", skill)
		}
	}
	return request, nil
}

// resolvedAddSource 是按 npx skills 语法解析后的 source。
type resolvedAddSource struct {
	// source 是交给核心包的 source：本地路径，或 github:owner/repo[/path][#ref]。
	source string
	// skill 来自 owner/repo@skill 或 #ref@skill。
	skill string
	// atSkill 表示 skill 来自 owner/repo@skill 简写，用于给旧的 @ref 写法提示。
	atSkill bool
}

// githubSourceAliases 与 npx skills 的 SOURCE_ALIASES 保持一致。
var githubSourceAliases = map[string]string{
	"coinbase/agentWallet":      "coinbase/agentic-wallet-skills",
	"vercel-labs/vercel-skills": "vercel-labs/agent-skills",
}

var (
	githubTreeWithPathPattern = regexp.MustCompile(`github\.com/([^/]+)/([^/]+)/tree/([^/]+)/(.+)`)
	githubTreePattern         = regexp.MustCompile(`github\.com/([^/]+)/([^/]+)/tree/([^/]+)$`)
	githubRepoPattern         = regexp.MustCompile(`github\.com/([^/]+)/([^/]+)`)
	atSkillPattern            = regexp.MustCompile(`^([^/]+)/([^/@]+)@(.+)$`)
	shorthandPattern          = regexp.MustCompile(`^([^/]+)/([^/]+)(?:/(.+?))?/?$`)
	windowsDrivePattern       = regexp.MustCompile(`^[a-zA-Z]:[/\\]`)
)

// resolveAddSource 按 npx skills 的 parseSource 规则解析 source；KISS 只支持其中的本地路径和 GitHub。
func resolveAddSource(input string) (resolvedAddSource, error) {
	if isLocalSource(input) {
		return resolvedAddSource{source: input}, nil
	}
	base, ref, fragmentSkill, err := splitSourceFragment(input)
	if err != nil {
		return resolvedAddSource{}, err
	}
	if alias, ok := githubSourceAliases[base]; ok {
		base = alias
	}
	base = strings.TrimPrefix(base, "github:")

	result := resolvedAddSource{skill: fragmentSkill}
	var owner, repo, subpath string
	switch {
	case githubTreeWithPathPattern.MatchString(base):
		m := githubTreeWithPathPattern.FindStringSubmatch(base)
		owner, repo, ref, subpath = m[1], m[2], m[3], m[4]
	case githubTreePattern.MatchString(base):
		m := githubTreePattern.FindStringSubmatch(base)
		owner, repo, ref = m[1], m[2], m[3]
	case githubRepoPattern.MatchString(base):
		m := githubRepoPattern.FindStringSubmatch(base)
		owner, repo = m[1], strings.TrimSuffix(m[2], ".git")
		if strings.Contains(repo, "@") {
			return resolvedAddSource{}, fmt.Errorf("unsupported GitHub URL %q; use #<ref> to pin a git ref, e.g. github.com/%s/%s#%s", input, owner, strings.SplitN(repo, "@", 2)[0], strings.SplitN(repo, "@", 2)[1])
		}
	case strings.Contains(base, ":"):
		return resolvedAddSource{}, unsupportedAddSource(input)
	case atSkillPattern.MatchString(base):
		m := atSkillPattern.FindStringSubmatch(base)
		owner, repo = m[1], m[2]
		if result.skill == "" {
			result.skill = m[3]
		}
		result.atSkill = fragmentSkill == ""
	case shorthandPattern.MatchString(base):
		m := shorthandPattern.FindStringSubmatch(base)
		owner, repo, subpath = m[1], m[2], m[3]
	default:
		return resolvedAddSource{}, unsupportedAddSource(input)
	}
	if owner == "" || repo == "" {
		return resolvedAddSource{}, unsupportedAddSource(input)
	}
	spec := "github:" + owner + "/" + repo
	if subpath != "" {
		spec += "/" + strings.Trim(subpath, "/")
	}
	// 内部 spec 用 #ref；子路径里含 @ 时也补上 #，避免被当作旧的 @ref。
	if ref != "" || strings.Contains(subpath, "@") {
		spec += "#" + ref
	}
	if _, err := kiss.ParseGitHubSource(spec); err != nil {
		return resolvedAddSource{}, err
	}
	result.source = spec
	return result, nil
}

func unsupportedAddSource(input string) error {
	return fmt.Errorf("unsupported add source %q; use a local path (./dir, /abs, file://), owner/repo[/path], owner/repo@skill, owner/repo#ref[@skill], github:owner/repo, or a github.com URL", input)
}

// splitSourceFragment 拆出 npx skills 风格的 #ref 或 #ref@skill 片段。
func splitSourceFragment(source string) (string, string, string, error) {
	base, fragment, ok := strings.Cut(source, "#")
	if !ok || fragment == "" {
		return base, "", "", nil
	}
	ref, skill, _ := strings.Cut(fragment, "@")
	var err error
	if ref, err = url.PathUnescape(ref); err != nil {
		return "", "", "", fmt.Errorf("invalid ref in source %q: %w", source, err)
	}
	if skill, err = url.PathUnescape(skill); err != nil {
		return "", "", "", fmt.Errorf("invalid skill in source %q: %w", source, err)
	}
	return base, ref, skill, nil
}

// isLocalSource 对齐 npx skills 的 isLocalPath（绝对路径、./、../、.、..、Windows 盘符），
// 另外接受 KISS 已支持的 ~ 和 file:// URL。像 owner/repo 这样的相对路径一律视为 GitHub，与 npx skills 一致。
func isLocalSource(source string) bool {
	if strings.HasPrefix(strings.ToLower(source), "file://") {
		return true
	}
	if filepath.IsAbs(source) || strings.HasPrefix(source, "/") || windowsDrivePattern.MatchString(source) {
		return true
	}
	if source == "." || source == ".." || source == "~" {
		return true
	}
	for _, prefix := range []string{"./", "../", "~/", `.\`, `..\`} {
		if strings.HasPrefix(source, prefix) {
			return true
		}
	}
	return false
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func splitGitRefSuffixForCLI(source string) (string, string, bool) {
	at := strings.LastIndex(source, "@")
	if at <= 0 || at == len(source)-1 {
		return source, "", false
	}
	return source[:at], source[at+1:], true
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
  kiss [--kiss-home <path>] add <source> [-s|--skill <skills...>] [--all] [-l|--list] [--full-depth] [--name <name>] [--force]
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
