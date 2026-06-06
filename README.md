# KISS Skill Store

KISS（Keep It Stupid Simple）是一个面向 coding agent 的显式 skill store 设计：用户先通过 `kiss add ...` 显式安装 skill；明确知道要用哪个 skill 时，再通过 `/kiss`、`$kiss` 或 `kiss run` 调用。`run` 只加载已安装且被点名的 skill，不自动下载或安装。

核心理念：

- **显式调用**：不依赖 agent 启动时的全局 skill discovery。
- **隔离存储**：KISS skill 不进入 agent 全局目录，也不进入项目目录。
- **显式安装**：本地没有 skill 时提示先执行 `kiss add ...`，不在 `run` 中自动拉取。
- **本地元数据**：安装后写入 `$KISS_HOME/skills.db.json`，方便 `list/show/remove/doctor` 查询；元数据同时保存本地短名 `name` 和来源限定全称 `full_name`。
- **最小协议**：一个 `SKILL.md` 加可选脚本、资源和 manifest 即可。

产品需求见 [`docs/kiss-skill-store-prd.md`](docs/kiss-skill-store-prd.md)，技术架构见 [`docs/kiss-skill-store-architecture.md`](docs/kiss-skill-store-architecture.md)。

## 技术路线摘要

当前 MVP 使用 Go 实现可编译的 `kiss` 单文件 CLI，普通用户不需要安装 Node、Bun 或 TypeScript 运行时。核心入口是 `kiss run <skill> <args>`，但必须先显式 `kiss add ...`。0.2.0 支持本地 skill 安装、显式 HTTPS/GitHub tar.gz 远程安装、`skills.db.json` 元数据数据库、`list/show/remove/doctor` 与单 skill Markdown 输出；后续再增加 `update`、静态 registry、agent adapter 和受控 command runner。

详细路线见技术架构文档的“实现技术路线”章节。

## 0.2.0 用法

```bash
go run ./cmd/kiss --version
go run ./cmd/kiss --kiss-home /tmp/kiss-demo doctor
go run ./cmd/kiss --kiss-home /tmp/kiss-demo add ./path/to/skill --name browser-test
# 或显式远程安装：
# go run ./cmd/kiss --kiss-home /tmp/kiss-demo add https://example.com/browser-test.tar.gz --name browser-test
# go run ./cmd/kiss --kiss-home /tmp/kiss-demo add github:owner/repo/skills/browser-test#v0.1.0 --name browser-test
go run ./cmd/kiss --kiss-home /tmp/kiss-demo list
go run ./cmd/kiss --kiss-home /tmp/kiss-demo show browser-test
go run ./cmd/kiss --kiss-home /tmp/kiss-demo run browser-test url=https://example.com
```

本轮实现目标见 [`docs/goal/goal-0_2_0.md`](docs/goal/goal-0_2_0.md)，历史目标见 [`docs/goal`](docs/goal)。
