# KISS Skill Store

KISS（Keep It Stupid Simple）是一个面向 coding agent 的显式 skill store：先 `kiss add` 安装，后 `/kiss <skill>` 调用。KISS 只把被点名的 skill 上下文交给 agent，不替 agent 推理或执行任务。

特点：

- **显式**：只在用户输入 `/kiss <skill>`、`$kiss <skill>` 或 `kiss run <skill>` 时加载。
- **隔离**：KISS 管理的 skill 存在 `$KISS_HOME/skills`，不写入 agent 全局 skill 目录或当前项目目录。
- **可审计**：本地保存来源、版本、resolved commit、sha256 和安装时间。
- **不自动拉取**：`run` 不下载、不安装、不更新；缺失时提示先 `kiss add`。
- **不接管执行**：不调用模型 provider，不执行 skill 脚本；后续工作由外层 agent 完成。

KISS 非常适合如下场景：

- 临时测试使用一个skill，不希望这个skill进入全局或者项目级 skill 目录
- 在非明确调用的场景下不想使用的 skill，可以通过 KISS 来使用


## 快速开始

### 安装 CLI

macOS / Linux：

```bash
curl -fsSL https://raw.githubusercontent.com/wwulfric/kiss/main/scripts/install.sh | bash
```

Windows PowerShell：

```powershell
irm https://raw.githubusercontent.com/wwulfric/kiss/main/scripts/install.ps1 | iex
```

### 安装 Skill

先把 KISS bridge skill 安装到 agent 的全局或项目 skill 目录：

```bash
npx skills add wwulfric/kiss
```

再把要使用的 skill 安装到 KISS 专属目录：

```bash
kiss add vercel-labs/agent-skills --skill browser-test
```

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

安装本地 skill：

```bash
kiss add ./path/to/skill
kiss add file:///abs/path/to/skill --name browser-test
```

安装 GitHub 仓库里的 skill：

```bash
kiss add owner/repo
kiss add vercel-labs/agent-skills --skill frontend-design
kiss add github:owner/repo/skills/browser-test#v0.1.0 --name browser-test
```

安装 HTTPS tarball：

```bash
kiss add https://example.com/browser-test.tar.gz --name browser-test
```

查看、更新和删除：

```bash
kiss list
kiss show browser-test
kiss update browser-test
kiss update browser-test --yes
kiss remove browser-test
kiss doctor
```

`kiss update <name>` 默认只预览版本、来源和入口文件 diff；加 `--yes` 才会真正更新。`kiss run` 不会自动安装、自动更新或访问远端。

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
