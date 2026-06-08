package main

import (
	"fmt"
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
	case "registry":
		return runRegistry(paths, remaining[1:])
	case "update":
		return runUpdate(paths, remaining[1:])
	case "adapter":
		return runAdapter(paths, remaining[1:])
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
	source := ""
	name := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--name", "--skill":
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a value", args[i])
			}
			name = args[i+1]
			i++
		default:
			if source != "" {
				return fmt.Errorf("usage: kiss add <source> --name <name>")
			}
			source = args[i]
		}
	}
	if source == "" {
		return fmt.Errorf("usage: kiss add <source> --name <name>")
	}
	if name == "" {
		if !isRemoteSource(source) && !looksLikePath(source) {
			return kiss.AddRegistrySkill(paths, source)
		}
		return fmt.Errorf("--name is required")
	}
	if isRemoteSource(source) {
		return kiss.AddRemoteSkill(paths, source, name)
	}
	return kiss.AddLocalSkill(paths, source, name)
}

func isRemoteSource(source string) bool {
	return strings.HasPrefix(source, "https://") || strings.HasPrefix(source, "github:")
}

func looksLikePath(source string) bool {
	return strings.HasPrefix(source, ".") || strings.HasPrefix(source, "~") || strings.ContainsAny(source, `/\`)
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

func runRegistry(paths kiss.Paths, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: kiss registry <add|list|require-signature|trust> ...")
	}
	switch args[0] {
	case "add":
		if len(args) != 3 {
			return fmt.Errorf("usage: kiss registry add <name> <url-or-path>")
		}
		return kiss.AddRegistry(paths, args[1], args[2])
	case "list":
		if len(args) != 1 {
			return fmt.Errorf("usage: kiss registry list")
		}
		return kiss.ListRegistries(paths, os.Stdout)
	case "require-signature":
		if len(args) != 2 {
			return fmt.Errorf("usage: kiss registry require-signature <name>")
		}
		return kiss.RequireRegistrySignature(paths, args[1])
	case "trust":
		if len(args) != 3 {
			return fmt.Errorf("usage: kiss registry trust <name> <public-key-base64>")
		}
		return kiss.TrustRegistryKey(paths, args[1], args[2])
	default:
		return fmt.Errorf("unknown registry command %q", args[0])
	}
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

func runAdapter(_ kiss.Paths, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: kiss adapter <print|install|uninstall> ...")
	}
	switch args[0] {
	case "print":
		if len(args) != 2 {
			return fmt.Errorf("usage: kiss adapter print <slash|skill|shell|powershell>")
		}
		return kiss.PrintAdapter(args[1], os.Stdout)
	case "install":
		return runAdapterInstall(args[1:])
	case "uninstall":
		return runAdapterUninstall(args[1:])
	default:
		return fmt.Errorf("unknown adapter command %q", args[0])
	}
}

func runAdapterInstall(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: kiss adapter install <slash|skill|shell|powershell> --path <path> [--force]")
	}
	kind := args[0]
	path := ""
	force := false
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--path":
			if i+1 >= len(args) {
				return fmt.Errorf("--path requires a value")
			}
			path = args[i+1]
			i++
		case "--force":
			force = true
		default:
			return fmt.Errorf("usage: kiss adapter install <slash|skill|shell|powershell> --path <path> [--force]")
		}
	}
	return kiss.InstallAdapter(kind, path, force)
}

func runAdapterUninstall(args []string) error {
	path := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--path":
			if i+1 >= len(args) {
				return fmt.Errorf("--path requires a value")
			}
			path = args[i+1]
			i++
		default:
			return fmt.Errorf("usage: kiss adapter uninstall --path <path>")
		}
	}
	return kiss.UninstallAdapter(path)
}

func printHelp() {
	fmt.Printf(`kiss %s

Usage:
  kiss [--kiss-home <path>] add <source> --name <name>
  kiss [--kiss-home <path>] add <registry-skill>
  kiss [--kiss-home <path>] run <name> [args...]
  kiss [--kiss-home <path>] registry add <name> <url-or-path>
  kiss [--kiss-home <path>] registry list
  kiss [--kiss-home <path>] registry require-signature <name>
  kiss [--kiss-home <path>] registry trust <name> <public-key-base64>
  kiss [--kiss-home <path>] update <name> [--yes]
  kiss [--kiss-home <path>] adapter print <slash|skill|shell|powershell>
  kiss [--kiss-home <path>] adapter install <slash|skill|shell|powershell> --path <path> [--force]
  kiss [--kiss-home <path>] adapter uninstall --path <path>
  kiss [--kiss-home <path>] list
  kiss [--kiss-home <path>] show <name>
  kiss [--kiss-home <path>] remove <name>
  kiss [--kiss-home <path>] doctor
  kiss --version
`, kiss.Version)
}
