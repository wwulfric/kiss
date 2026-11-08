# KISS Skill Store

KISS（Keep It Stupid Simple）是一个面向 coding agent 的显式 skill store：先 `kiss add` 安装，后 `/kiss <skill>` 调用。KISS 只把被点名的 skill 上下文交给 agent，不替 agent 推理或执行任务。

特点：

- **显式**：只在用户输入 `/kiss <skill>`、`$kiss <skill>` 或 `kiss run <skill>` 时加载。
- **隔离**：KISS 管理的 skill 存在 `$KISS_HOME/skills`，不写入 agent 全局 skill 目录或当前项目目录。
- **可审计**：本地保存来源、`skillFolderHash` 版本、resolved commit、sha256 和安装时间。
- **不自动拉取**：`run` 不下载、不安装、不更新；缺失时提示先 `kiss add`。
- **不接管执行**：不调用模型 provider，不执行 skill 脚本；后续工作由外层 agent 完成。

KISS 非常适合如下场景：

- 临时测试使用一个skill，不希望这个skill进入全局或者项目级 skill 目录
- 在非明确调用的场景下不想使用的 skill，可以通过 KISS 来使用


## 快速开始

### 安装 CLI

macOS / Linux：

```bash
curl -fsSL https://raw.githubusercontent.com/wwulfric/kiss/master/scripts/install.sh | bash
```

如果 `/usr/local/bin` 不可写，安装脚本会默认安装到 `$HOME/.local/bin`。若安装后当前 shell 找不到 `kiss`，把该目录加入 `PATH`。

Windows PowerShell：

```powershell
irm https://raw.githubusercontent.com/wwulfric/kiss/master/scripts/install.ps1 | iex
```

### 安装 Skill

先让 agent 能识别 `/kiss`：

```bash
kiss setup
# 或 Claude Code
kiss setup --agent claude
```

如果要让 Codex、Claude Code 等 agent 代为安装，可把 [`docs/agent-installation-guide.md`](docs/agent-installation-guide.md) 交给它执行。

再把要使用的 skill 安装到 KISS 专属目录：

```bash
kiss add owner/repo --skill browser-test
```

`kiss add` 的 source 和 flag 语法与 `npx skills add` 相同，可以直接把 `npx skills add ...` 换成 `kiss add ...`。

### 使用 Skill

在 agent 里显式调用已安装的 skill：

```text
/kiss browser-test url=https://example.com
```

也可以在终端直接查看 KISS 交给 agent 的上下文：

```bash
kiss run browser-test url=https://example.com
```

## 管理 Skill

`kiss add` 的安装语法跟随 `npx skills add`：source 写法、skill 选择和发现规则（`skills/`、`.agents/skills`、`.claude/skills`、
plugin manifest 等）都相同，能被 `npx skills` 安装的仓库也能被 `kiss add` 安装。

```bash
# 列出 source 里可安装的 skill，不安装
kiss add backnotprop/pstack --list

# 仓库里只有一个 skill 时直接安装；本地名取 SKILL.md frontmatter 的 name
kiss add owner/repo

# 选择 skill：@skill 简写，或 -s/--skill（可跟多个名称）
kiss add backnotprop/pstack@tdd
kiss add backnotprop/pstack --skill tdd why

# 安装全部 skill
kiss add backnotprop/pstack --all

# 指定 Git ref 用 #ref；#ref@skill 同时选择 skill
kiss add 'owner/repo#v1.0.0'
kiss add 'owner/repo#main@browser-test'
kiss add https://github.com/owner/repo/tree/main/skills/browser-test

# 仓库子路径
kiss add owner/repo/.claude/skills

# 本地目录（单个 skill 或多 skill 仓库）
kiss add ./path/to/skill
kiss add file:///abs/path/to/skill --name browser-test

# 已有同名但确认要覆盖时使用 --force
kiss add owner/repo@browser-test --force
```

与 `npx skills add` 的差异：

- 仓库里有多个 skill 且没有 `--skill`/`--all` 时，KISS 不弹交互选择，而是报错列出可选名称。
- `-g`、`-y` 静默接受；`-a/--agent`、`--copy` 会被忽略，因为 KISS 只安装到自己的 store。
- `--name`（单个 skill 时改本地名）和 `--force` 是 KISS 自己的 flag。
- 注意 `owner/repo@xxx` 里的 `xxx` 是 skill 名，不是 Git ref；Git ref 要写成 `owner/repo#xxx`。
- frontmatter 标记 `metadata.internal: true` 的 skill 默认不会被发现，显式 `--skill <name>` 点名时才安装，与 `npx skills` 一致。

从 Vercel Skills CLI 的全局安装记录导入当前快照：

```bash
kiss import --skill find-skills
kiss import --all --dry-run
```


查看、更新和删除：

```bash
kiss list
kiss show browser-test
kiss update browser-test
kiss update browser-test@master
kiss update browser-test --yes
kiss remove browser-test
kiss doctor
```

升级 KISS CLI 自身：

```bash
kiss upgrade
kiss upgrade --version v0.0.5
kiss upgrade --force
```

## 本地开发

直接用 Go 运行或构建：

```bash
go run ./cmd/kiss --version
go run ./cmd/kiss --kiss-home /tmp/kiss-demo doctor
go build -o bin/kiss ./cmd/kiss
```

常用验证命令：

```bash
gofmt -w internal/kiss/*.go cmd/kiss/*.go
go test ./...
go vet ./...
golangci-lint run
git diff --check
```
