package kiss

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const adapterMarker = "KISS adapter generated file"

const slashAdapterTemplate = `# KISS adapter generated file
# KISS slash command adapter

Name: /kiss
Usage: /kiss <skill> [args...]

Behavior:
- Execute: kiss run <skill> [args...]
- Pass stdout to the invoking agent as current-turn context.
- Treat stderr or a non-zero exit code as command failure.
- Do not list or scan $KISS_HOME/skills.
- Do not install, update, or remove skills from this adapter.
- Do not call any model provider from this adapter.
`

const skillAdapterTemplate = `# KISS adapter generated file
---
name: kiss
description: Explicitly load one installed KISS skill when the user asks for /kiss <skill> [args...].
---

# KISS Bridge Skill

When the user explicitly asks for:

` + "```text" + `
/kiss <skill> [args...]
` + "```" + `

or an equivalent $kiss <skill> [args...] request, run:

` + "```bash" + `
kiss run <skill> [args...]
` + "```" + `

Use the command stdout as current-turn context, then continue the user's task with your normal model reasoning and tools.

Rules:

- Load only the skill named by the user.
- Do not inspect or preload $KISS_HOME/skills.
- Do not install, update, remove, or modify registries from this bridge.
- Do not call any model provider on behalf of KISS.
- Treat stderr or a non-zero exit code as command failure and report it to the user.
`

const shellAdapterTemplate = `#!/bin/sh
# KISS adapter generated file
set -eu

if [ "$#" -lt 1 ]; then
  echo "usage: $0 <skill> [args...]" >&2
  exit 2
fi

exec kiss run "$@"
`

const powershellAdapterTemplate = `# KISS adapter generated file
# KISS PowerShell adapter
param(
  [Parameter(ValueFromRemainingArguments = $true)]
  [string[]] $KissArgs
)

if ($KissArgs.Count -lt 1) {
  Write-Error "usage: kiss-skill.ps1 <skill> [args...]"
  exit 2
}

& kiss run @KissArgs
exit $LASTEXITCODE
`

func PrintAdapter(kind string, out io.Writer) error {
	template, err := AdapterTemplate(kind)
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(out, template)
	return err
}

func AdapterTemplate(kind string) (string, error) {
	switch kind {
	case "slash":
		return slashAdapterTemplate, nil
	case "skill":
		return skillAdapterTemplate, nil
	case "shell":
		return shellAdapterTemplate, nil
	case "powershell":
		return powershellAdapterTemplate, nil
	default:
		return "", fmt.Errorf("unknown adapter %q; expected slash, skill, shell, or powershell", kind)
	}
}

func InstallAdapter(kind, targetPath string, force bool) error {
	if targetPath == "" {
		return fmt.Errorf("--path is required")
	}
	template, err := AdapterTemplate(kind)
	if err != nil {
		return err
	}
	if _, err := os.Stat(targetPath); err == nil && !force {
		return fmt.Errorf("adapter target already exists: %s; use --force to overwrite", targetPath)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	mode := os.FileMode(0o644)
	if kind == "shell" {
		mode = 0o755
	}
	return writeFileAtomicMode(targetPath, []byte(template), mode)
}

func UninstallAdapter(targetPath string) error {
	if targetPath == "" {
		return fmt.Errorf("--path is required")
	}
	data, err := os.ReadFile(targetPath)
	if err != nil {
		return err
	}
	if !strings.Contains(string(data), adapterMarker) {
		return fmt.Errorf("refusing to remove non-KISS adapter file: %s", targetPath)
	}
	return os.Remove(targetPath)
}

func writeFileAtomicMode(targetPath string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".kiss-adapter-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}
	return os.Rename(tmpName, targetPath)
}
