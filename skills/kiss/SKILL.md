---
name: kiss
description: Use KISS only when the user explicitly invokes /kiss <skill> [args...] or $kiss <skill> [args...] to load one installed private skill.
---

# KISS Bridge Skill

Use this skill only when the user explicitly asks for:

```text
/kiss <skill> [args...]
```

or an equivalent:

```text
$kiss <skill> [args...]
```

When invoked, run:

```bash
kiss run <skill> [args...]
```

Use stdout from that command as current-turn context, then continue the user's task with your normal model reasoning and tools.

Rules:

- Load only the skill named by the user.
- Do not inspect, list, summarize, preload, or synchronize `$KISS_HOME/skills`.
- Do not install, update, remove, or modify KISS-managed skills from this bridge.
- Do not call any model provider on behalf of KISS.
- Treat stderr or a non-zero exit code as command failure and report it to the user.
- If the `kiss` CLI is missing, detect the user's operating system before recommending an installation command.
- Do not install the `kiss` CLI automatically. Show the appropriate command for the detected OS, ask for explicit confirmation, then install only if the user agrees.
- For macOS or Linux, recommend:

```bash
curl -fsSL https://raw.githubusercontent.com/wwulfric/kiss/main/scripts/install.sh | bash
```

- For Windows PowerShell, recommend:

```powershell
irm https://raw.githubusercontent.com/wwulfric/kiss/main/scripts/install.ps1 | iex
```

- If the OS cannot be detected, ask the user whether they are on macOS, Linux, or Windows before recommending an installation command.
- After installing the `kiss` CLI, run `kiss --version` to verify the installation before retrying `kiss run <skill> [args...]`.
