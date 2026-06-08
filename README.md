# KISS Skill Store

KISS（Keep It Stupid Simple）是一个面向 coding agent 的显式 skill store 和 context provider：用户先通过 `kiss add ...` 显式安装 skill；明确知道要用哪个 skill 时，再通过 `/kiss`、`$kiss` 或 `kiss run` 调用。`run` 只加载已安装且被点名的 skill，把当前 skill 上下文交给外层 agent，不自动下载或安装。

核心理念：

- **显式调用**：不依赖 agent 启动时的全局 skill discovery。
- **隔离存储**：KISS skill 不进入 agent 全局目录，也不进入项目目录。
- **显式安装**：本地没有 skill 时提示先执行 `kiss add ...`，不在 `run` 中自动拉取。
- **本地元数据**：安装后写入 `$KISS_HOME/skills.db.json`，方便 `list/show/remove/doctor` 查询；元数据同时保存本地短名 `name` 和来源限定全称 `full_name`。
- **不接管模型执行**：KISS 不配置、不代理、不管理模型 provider；LLM 推理由外层 coding agent 负责。
- **交给 agent 完成工作**：`/kiss <skill> <params>` 的效果是把对应 skill 信息给到 agent，后续推理、工具调用和文件编辑由 agent 完成。
- **最小协议**：一个 `SKILL.md` 加可选引用资料、资源和 manifest 即可。

产品需求见 [`docs/kiss-skill-store-prd.md`](docs/kiss-skill-store-prd.md)，技术架构见 [`docs/kiss-skill-store-architecture.md`](docs/kiss-skill-store-architecture.md)。

## 技术路线摘要

当前 MVP 使用 Go 实现可编译的 `kiss` 单文件 CLI，普通用户不需要安装 Node、Bun 或 TypeScript 运行时。核心入口是 `kiss run <skill> <args>`，但必须先显式 `kiss add ...`。当前实现支持本地 skill 安装、显式 HTTPS/GitHub tar.gz 远程安装、GitHub ref 到 commit SHA 解析、静态 registry、多 registry 容错解析、可选签名 registry entry、registry trust policy、`registry.lock`、带受限文本 diff 的受控 `update --yes`、薄 adapter 模板和显式路径安装、KISS bridge skill、PowerShell adapter、预编译 release 脚手架、macOS/Linux `install.sh` 安装入口、Windows `install.ps1` 安装入口、Homebrew formula artifact、release checksum keyless cosign 签名、`skills.db.json` 元数据数据库、`list/show/remove/doctor` 与单 skill Markdown 输出。

详细路线见技术架构文档的“实现技术路线”章节。

## 安装

macOS / Linux 默认从 GitHub Release 安装预编译单文件 CLI：

```bash
curl -fsSL https://raw.githubusercontent.com/wwulfric/kiss/main/scripts/install.sh | bash
```

Windows PowerShell 默认从 GitHub Release 安装预编译 `kiss.exe`：

```powershell
irm https://raw.githubusercontent.com/wwulfric/kiss/main/scripts/install.ps1 | iex
```

可通过环境变量覆盖来源和安装目录：

```bash
KISS_REPO=wwulfric/kiss KISS_VERSION=latest KISS_INSTALL_DIR="$HOME/.local/bin" bash scripts/install.sh
```

```powershell
$env:KISS_REPO = "wwulfric/kiss"
$env:KISS_VERSION = "latest"
$env:KISS_INSTALL_DIR = "$env:LOCALAPPDATA\Programs\kiss\bin"
irm https://raw.githubusercontent.com/wwulfric/kiss/main/scripts/install.ps1 | iex
```

默认安装流程会用 `checksums.txt` 校验平台 tarball。需要额外验证 release workflow 对 `checksums.txt` 的 keyless cosign 签名时，先安装 `cosign`，再显式开启：

```bash
curl -fsSL https://raw.githubusercontent.com/wwulfric/kiss/main/scripts/install.sh | KISS_VERIFY_SIGNATURE=1 bash
```

```powershell
$env:KISS_VERIFY_SIGNATURE = "1"
irm https://raw.githubusercontent.com/wwulfric/kiss/main/scripts/install.ps1 | iex
```

KISS 不提供 npm / pnpm / bunx shim，也不依赖 Node 技术栈。Windows 包管理器入口不属于当前交付范围。

本地构建 release artifacts：

```bash
scripts/build-release.sh
```

构建完成后，`dist/kiss.rb` 是由本次 release tarball 和 `checksums.txt` 生成的 Homebrew formula，可发布到独立 tap 或作为 GitHub Release artifact 审计。GitHub Release workflow 会进一步对 `dist/checksums.txt` 做 keyless cosign 签名，并上传 `checksums.txt.sig` 和 `checksums.txt.pem`。

## 版本

KISS 的发布版本以 Git tag 为唯一来源。源码默认版本是 `dev`；`scripts/build-release.sh` 会把 `KISS_VERSION` 或 `git describe --tags --always --dirty` 的结果注入到 binary，所以 release artifact 里的 `kiss --version` 应与 release tag 一致。

GitHub Release workflow 只支持手动触发。发布时先创建形如 `v0.1.0` 的 tag，再在同一个 tag ref 上手动运行 release workflow，并填写相同的 `version` 输入。

## 用法

```bash
go run ./cmd/kiss --version
go run ./cmd/kiss --kiss-home /tmp/kiss-demo doctor
go run ./cmd/kiss --kiss-home /tmp/kiss-demo add ./path/to/skill --name browser-test
# 或显式远程安装：
# go run ./cmd/kiss --kiss-home /tmp/kiss-demo add https://example.com/browser-test.tar.gz --name browser-test
# go run ./cmd/kiss --kiss-home /tmp/kiss-demo add github:owner/repo/skills/browser-test#v0.1.0 --name browser-test
go run ./cmd/kiss --kiss-home /tmp/kiss-demo registry add personal https://example.com/kiss-registry.toml
# 可选：要求 registry entry 签名，并信任某个 Ed25519 public key
# go run ./cmd/kiss --kiss-home /tmp/kiss-demo registry require-signature personal
# go run ./cmd/kiss --kiss-home /tmp/kiss-demo registry trust personal <public-key-base64>
# go run ./cmd/kiss --kiss-home /tmp/kiss-demo add browser-test
go run ./cmd/kiss --kiss-home /tmp/kiss-demo update browser-test
# go run ./cmd/kiss --kiss-home /tmp/kiss-demo update browser-test --yes
go run ./cmd/kiss --kiss-home /tmp/kiss-demo adapter print slash
go run ./cmd/kiss --kiss-home /tmp/kiss-demo adapter print skill
go run ./cmd/kiss --kiss-home /tmp/kiss-demo adapter print shell
go run ./cmd/kiss --kiss-home /tmp/kiss-demo adapter print powershell
# adapter install 只写入显式指定路径，不自动修改任何 agent 全局目录
go run ./cmd/kiss --kiss-home /tmp/kiss-demo adapter install skill --path ./KISS.md
go run ./cmd/kiss --kiss-home /tmp/kiss-demo adapter install shell --path "$HOME/.local/bin/kiss-skill"
go run ./cmd/kiss --kiss-home /tmp/kiss-demo adapter install slash --path ./kiss.slash.md
go run ./cmd/kiss --kiss-home /tmp/kiss-demo adapter install powershell --path ./kiss-skill.ps1
# go run ./cmd/kiss --kiss-home /tmp/kiss-demo adapter uninstall --path ./kiss.slash.md
go run ./cmd/kiss --kiss-home /tmp/kiss-demo list
go run ./cmd/kiss --kiss-home /tmp/kiss-demo show browser-test
go run ./cmd/kiss --kiss-home /tmp/kiss-demo run browser-test url=https://example.com
```

`adapter print skill` 输出的是 KISS bridge skill：它只告诉 agent 在用户显式输入 `/kiss <skill> [args...]` 时调用 `kiss run`，并把 stdout 作为当前轮上下文。KISS 不会替 agent 调用模型或完成任务。
