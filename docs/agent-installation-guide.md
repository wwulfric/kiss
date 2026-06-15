# KISS Agent 安装指导

本文件用于交给 Codex、Claude Code、Cursor、Cline 等 coding agent，让 agent 帮用户安装 KISS CLI 和 KISS bridge skill。

KISS 有两类安装对象：

- **KISS CLI**：本机命令行程序，提供 `kiss add`、`kiss run`、`kiss list` 等命令。
- **KISS bridge skill**：安装到 agent 原生 skill 目录中的薄 skill，让用户在 agent 里输入 `/kiss <skill> ...` 或 `$kiss <skill> ...` 时，由 agent 调用 `kiss run <skill> ...`。

不要把 KISS 管理的第三方 skill 安装到 agent 原生 skill 目录。第三方 skill 必须通过 `kiss add` 安装到 KISS 专属目录。

## Agent 执行规则

1. 安装前先告诉用户会修改哪些位置，并获得明确确认。
2. 如果 `kiss` 已存在，先运行 `kiss --version`；不要重复安装同版本。
3. 如果需要安装 KISS CLI，先识别操作系统，再推荐对应命令。
4. 如果需要安装 KISS bridge skill，使用 `npx skills add wwulfric/kiss --skill kiss`。
5. 不要枚举、读取、同步或总结 `$KISS_HOME/skills` 里的 skill。
6. 不要在用户输入 `/kiss <skill>` 时自动执行 `kiss add`；缺失 skill 时提示用户显式安装。
7. 不要替 KISS 配置任何模型 provider、API key 或 standalone executor。
8. 不要执行第三方 skill 里的脚本；KISS 默认只把被点名 skill 的 Markdown 上下文交给 agent。

## 快速流程

### 1. 检查当前环境

macOS / Linux：

```bash
uname -s
uname -m
command -v kiss || true
command -v npx || true
```

Windows PowerShell：

```powershell
$PSVersionTable
[System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
Get-Command kiss -ErrorAction SilentlyContinue
Get-Command npx -ErrorAction SilentlyContinue
```

如果 `kiss` 已安装：

```bash
kiss --version
kiss doctor
```

### 2. 安装或更新 KISS CLI

macOS / Linux：

```bash
curl -fsSL https://raw.githubusercontent.com/wwulfric/kiss/master/scripts/install.sh | bash
kiss --version
```

如果没有 `/usr/local/bin` 写权限，改装到用户目录：

```bash
mkdir -p "$HOME/.local/bin"
curl -fsSL https://raw.githubusercontent.com/wwulfric/kiss/master/scripts/install.sh | env KISS_INSTALL_DIR="$HOME/.local/bin" bash
kiss --version
```

如果 `kiss` 找不到，提示用户把安装目录加入 `PATH`。

Windows PowerShell：

```powershell
irm https://raw.githubusercontent.com/wwulfric/kiss/master/scripts/install.ps1 | iex
kiss --version
```

如果当前终端找不到 `kiss`，让用户打开一个新的 PowerShell 窗口后再验证。

安装脚本默认安装 GitHub Release 的 latest 版本。脚本会比较本机版本和目标版本：本机版本相同或更新时跳过，本机版本更旧时更新。需要指定版本时使用：

macOS / Linux：

```bash
curl -fsSL https://raw.githubusercontent.com/wwulfric/kiss/master/scripts/install.sh | env KISS_VERSION=v0.0.1 bash
```

Windows PowerShell：

```powershell
$env:KISS_VERSION = "v0.0.1"
irm https://raw.githubusercontent.com/wwulfric/kiss/master/scripts/install.ps1 | iex
```

### 3. 安装 KISS bridge skill

优先使用 Vercel Skills / skills.sh 兼容 CLI，并显式指定只安装 `kiss`：

```bash
npx skills add wwulfric/kiss --skill kiss
```

不要省略 `--skill kiss`。本仓库还包含项目私有的 `release` skill，省略 `--skill kiss` 时部分安装工具会进入交互选择，甚至安装全部 skill。

如果没有 `npx`，不要自动安装 Node.js；先询问用户是否允许安装 Node.js，或者使用下面的手动安装方式。

#### Codex 手动安装

用户级：

```bash
mkdir -p "$HOME/.agents/skills/kiss"
curl -fsSL https://raw.githubusercontent.com/wwulfric/kiss/master/skills/kiss/SKILL.md \
  -o "$HOME/.agents/skills/kiss/SKILL.md"
```

项目级：

```bash
mkdir -p ".agents/skills/kiss"
curl -fsSL https://raw.githubusercontent.com/wwulfric/kiss/master/skills/kiss/SKILL.md \
  -o ".agents/skills/kiss/SKILL.md"
```

#### Claude Code 手动安装

用户级：

```bash
mkdir -p "$HOME/.claude/skills/kiss"
curl -fsSL https://raw.githubusercontent.com/wwulfric/kiss/master/skills/kiss/SKILL.md \
  -o "$HOME/.claude/skills/kiss/SKILL.md"
```

项目级：

```bash
mkdir -p ".claude/skills/kiss"
curl -fsSL https://raw.githubusercontent.com/wwulfric/kiss/master/skills/kiss/SKILL.md \
  -o ".claude/skills/kiss/SKILL.md"
```

如果 agent 没有立刻识别新 skill，提示用户重启对应 agent 会话。

### 4. 可选：用 find-skills 做端到端验证

如果用户希望验证 KISS 不只是安装成功，而是真的可以 `add` 和 `run` 一个远端 skill，可以安装 skills.sh 生态中的 `find-skills`。

这一步是 KISS 可用性验证，不是通用第三方 skill 安装流程。执行前先说明：它会把 `find-skills` 安装到 KISS 专属目录，不会安装到 agent 原生 skill 目录，也不会安装搜索结果里的任何 skill。

```bash
kiss add vercel-labs/skills --skill find-skills
kiss run find-skills "browser testing"
```

预期行为：

1. `kiss add` 成功，或提示 `find-skills` 已经安装。
2. `kiss run` 输出 `find-skills` 的 Markdown 上下文。
3. 输出中只包含 `find-skills` 当前 skill，不暴露 KISS store 里的其他 skill。

`find-skills` 是一个指导 agent 搜索 skills.sh 的 skill。`kiss run find-skills ...` 成功后，agent 可以按输出上下文执行一次简单搜索，但不要安装搜索结果：

```bash
npx skills find "browser testing"
```

### 5. 验证 agent 调用

如果用户已经安装了某个 KISS 管理的 skill，让用户在 agent 里输入：

```text
/kiss <已安装的-skill> <参数>
```

支持 `$` 显式 skill mention 的 agent 也可以使用：

```text
$kiss <已安装的-skill> <参数>
```

如果用户尚未安装任何 KISS 管理的 skill，只验证 KISS CLI 和 KISS bridge skill 的安装状态；除非用户明确同意执行上一步 `find-skills` 端到端验证，否则不要为了验证本安装流程而自动安装第三方 skill。

不要使用以下输入做验证：

```text
/kiss kiss
```

`/kiss kiss` 会递归调用 KISS bridge skill，应直接拒绝，不要执行 `kiss run kiss`。

预期行为：

1. Agent 触发 KISS bridge skill。
2. Bridge skill 执行 `kiss run <已安装的-skill> <参数>`。
3. Agent 只获得被点名 skill 的上下文。
4. 后续推理、工具调用和文件修改由 agent 自己完成。

## 常见问题

### `kiss` 命令不存在

- 先确认安装脚本是否成功输出 `Installed kiss to ...`。
- macOS / Linux 用户目录安装时，确认 `$HOME/.local/bin` 在 `PATH` 中。
- Windows 安装后需要打开新的 PowerShell 窗口刷新 `PATH`。

### `npx skills add wwulfric/kiss --skill kiss` 失败

- 检查是否安装 Node.js / npm。
- 如果用户不想安装 Node.js，使用对应 agent 的手动安装方式。
- 手动安装后如果 agent 没识别，重启 agent 会话。

### `/kiss <skill>` 报未安装

这说明 KISS CLI 和 KISS bridge skill 已经进入调用路径，但用户点名的 KISS 管理 skill 尚未安装。除 `find-skills` 端到端验证外，本安装指导不处理第三方 skill 安装；不要替用户自动搜索、下载或安装同名 skill。

### `kiss add vercel-labs/skills --skill find-skills` 下载失败

端到端验证需要访问 GitHub 和 `codeload.github.com`。如果网络、DNS、代理或公司防火墙导致下载失败，向用户报告失败原因即可；不要改用其他第三方 skill 做自动验证，也不要自动安装搜索结果。

### 安装脚本提示当前版本相同或更新

这是预期行为。默认不重复下载、不降级。如果用户明确要求覆盖安装，macOS / Linux 可使用：

```bash
curl -fsSL https://raw.githubusercontent.com/wwulfric/kiss/master/scripts/install.sh | env KISS_FORCE_INSTALL=1 bash
```

Windows PowerShell 可使用：

```powershell
$env:KISS_FORCE_INSTALL = "1"
irm https://raw.githubusercontent.com/wwulfric/kiss/master/scripts/install.ps1 | iex
```

## 给用户的完成回报

安装完成后，向用户说明：

- KISS CLI 版本：`kiss --version` 的输出。
- KISS bridge skill 的安装方式：`npx skills add wwulfric/kiss --skill kiss` 或手动路径。
- 如果执行了端到端验证，说明 `find-skills` 已安装到 KISS 专属目录，并汇报 `kiss run find-skills ...` 的结果。
- 是否需要重启 agent 会话或打开新终端。
- 哪些检查被跳过，以及跳过原因。

## 参考资料

- [Vercel Agent Skills](https://vercel.com/docs/agent-resources/skills)
- [Codex Agent Skills](https://developers.openai.com/codex/skills)
- [Claude Code Skills](https://code.claude.com/docs/en/skills)
