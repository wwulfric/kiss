package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/kiss/kiss/internal/kiss"
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
				return fmt.Errorf("usage: kiss add <local-path> --name <name>")
			}
			source = args[i]
		}
	}
	if source == "" {
		return fmt.Errorf("usage: kiss add <local-path> --name <name>")
	}
	if name == "" {
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

func runSkill(paths kiss.Paths, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: kiss run <name> [args...]")
	}
	return kiss.RunSkill(paths, args[0], args[1:], os.Stdout)
}

func printHelp() {
	fmt.Println(`kiss 0.2.0

Usage:
  kiss [--kiss-home <path>] add <local-path> --name <name>
  kiss [--kiss-home <path>] run <name> [args...]
  kiss [--kiss-home <path>] list
  kiss [--kiss-home <path>] show <name>
  kiss [--kiss-home <path>] remove <name>
  kiss [--kiss-home <path>] doctor
  kiss --version`)
}
