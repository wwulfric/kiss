# KISS Skill Store 技术架构

本文只描述 KISS Skill Store 的技术设计。产品背景、目标用户、用户故事和功能优先级见 [`kiss-skill-store-prd.md`](kiss-skill-store-prd.md)。

## 1. 架构目标

KISS 是一个独立的 skill store 和 explicit context provider。它借鉴 Vercel Skills 生态中“skill 是可分发目录、通过 CLI 安装、入口是 `SKILL.md`”的简单模型，但刻意不把 KISS 管理的 skill 安装到任何 agent 的原生扫描目录。KISS 不提供 standalone LLM executor，不配置、不代理、不管理任何模型 provider；模型推理、工具调用和文件编辑始终由调用方 coding agent 负责。

技术目标：

1. `kiss run <skill> <args>` 是唯一稳定核心执行入口，但只运行已安装 skill，不自动安装。
2. `/kiss` 和 `$kiss` adapter 必须是薄封装，只调用 CLI。
3. Store 必须位于 `$KISS_HOME/skills`，默认 `$KISS_HOME=${XDG_DATA_HOME:-~/.local/share}/kiss`。
4. Loader 每次只读取一个被点名的 skill。
5. 安装必须可审计：本地数据库记录来源、ref、resolved commit、sha256、安装路径、安装时间。
6. 默认主路径只输出被点名 skill 的上下文；KISS 不直接执行 skill 内脚本。
7. `kiss run` 不依赖模型 API key、provider 配置或后台 agent loop。

## 2. 系统边界

```text
Install path:

User
  | kiss add <source> --name <skill>
  v
KISS CLI
  |-- Resolver: explicit source -> package
  |-- Installer: copy/download -> verify -> install
  |-- Store: $KISS_HOME/skills
  |-- Metadata DB: $KISS_HOME/skills.db.json

Run path:

User
  | /kiss <skill> <args>
  v
Agent slash-command / shell adapter
  |
  | kiss run <skill> <args>
  v
KISS CLI
  |-- Metadata DB: confirm installed; no auto-install
  |-- Loader: read exactly one skill
  |-- Runner: markdown envelope for the selected skill
  v
Agent receives only selected skill context
Agent performs the requested work with its own model and tools
```

KISS 不负责替代 coding agent，也不负责把 KISS 管理的 skill 同步到 Claude、Codex、Cursor、Gemini 等工具的全局目录。安装和加载是两个显式动作：`kiss add` 负责保存 skill 与元数据，`kiss run` 只负责加载已安装且被点名的 skill，并把结果交给外层 agent 消费。KISS 本身不进入 LLM 推理循环。

## 3. 本地目录布局

默认目录：

```text
$KISS_HOME/
  config.toml
  skills.db.json
  registry.lock
  skills/
    browser-test/
      kiss.skill.toml
      SKILL.md
      assets/
      refs/
      .kiss-install.json
  cache/
    downloads/
    git/
  logs/
    kiss.log
```

说明：

- `config.toml`：registry 列表、默认 trust policy、网络超时。
- `skills.db.json`：本地已安装 skill 的元数据数据库，供 `list/show/remove/doctor` 查询。
- `registry.lock`：远程 registry 名称解析结果；仅在显式远程安装或更新时使用。
- `skills/<name>`：已安装 skill。目录名是 KISS 本地别名，不必等于上游仓库名。
- `.kiss-install.json`：安装来源、commit/tag、sha256、安装时间、KISS 版本。
- `cache/`：可删除缓存，不影响已安装 skill。
- `logs/`：只记录 KISS 操作元数据，默认不记录用户传给 skill 的敏感参数。

## 4. Skill 包格式

MVP 兼容“一个目录里包含 `SKILL.md`”的模式，并增加一个很小的 KISS manifest。

```text
my-skill/
  kiss.skill.toml
  SKILL.md
  assets/
  references/
```

`kiss.skill.toml` 示例：

```toml
name = "browser-test"
version = "0.1.0"
description = "Run a focused browser test workflow when explicitly invoked."
entry = "SKILL.md"

[runner]
type = "markdown"
```

字段约定：

- `name`：KISS 本地唯一名称。
- `version`：语义化版本或上游 commit/tag。
- `description`：只供 `kiss list` 和人工查看；不会在 agent 启动时暴露。
- `entry`：默认 `SKILL.md`。
- `runner.type`：只支持 `markdown`；KISS 不实现 command runner。

## 5. 本地元数据数据库

KISS 安装 skill 后，必须在 `$KISS_HOME/skills.db.json` 保存元数据。MVP 使用 JSON 文件作为核心数据库，而不是 SQLite、DuckDB 或 Parquet，理由是：可读、可审计、易删除、方便测试、无额外 native 依赖。DuckDB + Parquet 不进入 `run/list/show` 的核心路径。

数据库结构：

```json
{
  "schema_version": 1,
  "skills": {
    "browser-test": {
      "name": "browser-test",
      "full_name": "github:vercel-labs/agent-skills/skills/browser-test",
      "manifest_name": "browser-test",
      "version": "0.1.0",
      "description": "Run focused browser test workflow.",
      "source": {
        "kind": "github",
        "uri": "vercel-labs/agent-skills/skills/browser-test",
        "ref": "v0.1.0",
        "resolved": "",
        "sha256": ""
      },
      "runner": {
        "type": "markdown",
        "entry": "SKILL.md"
      },
      "installed_path": "$KISS_HOME/skills/browser-test",
      "installed_at": "2026-06-02T00:00:00Z",
      "updated_at": "2026-06-02T00:00:00Z",
      "kiss_version": "0.1.2"
    }
  }
}
```

字段说明：

- `schema_version`：数据库结构版本，用于兼容结构变更。
- `skills`：以 KISS 本地 skill 名称为 key 的对象。
- `name`：KISS 本地短名，也是 `$KISS_HOME/skills/<name>` 的目录名；用户用它执行 `kiss run <name>`。
- `full_name`：来源限定全称，用于审计、去重、迁移和 update。格式建议为 `github:owner/repo/path/to/skill`、`https://example.com/skill.tar.gz` 或 `local:/abs/path/to/skill`。
- `manifest_name`：skill manifest 中声明的名称。
- `source.kind`：`local`、`github`、`https` 等来源类型。
- `source.uri`：本地绝对路径、GitHub spec 或 URL。
- `source.ref` / `source.resolved`：远程来源的用户指定 ref 和解析后的 commit；本地来源可为空。
- `source.sha256`：安装包或内容摘要；本地 MVP 可为空，远程安装必须写入。
- `runner`：runner 类型和入口文件。
- `installed_path`：安装后的 skill 目录。
- `installed_at` / `updated_at`：安装和最近更新的 UTC 时间。
- `kiss_version`：写入该记录的 KISS 版本。

`kiss list` 默认只读取数据库中的 `name/version/source.kind/full_name`。`kiss show <name>` 输出单条完整元数据。`kiss run <name>` 可以读取数据库确认安装存在，但仍只加载该 skill 的 manifest 和 `SKILL.md`。

### 5.1 为什么不把 DuckDB + Parquet 作为核心数据库

DuckDB + Parquet 可以用于分析，但不适合作为 KISS MVP 的核心元数据数据库：

- KISS 元数据是小规模 key-value 查询，核心操作是 `get(name)`、`list`、`show` 和 `remove`，不需要列式分析能力。
- DuckDB 会引入 native 依赖和跨平台分发复杂度，不符合默认单文件 CLI 的低依赖目标。
- Parquet 适合批量分析和压缩存储，不适合频繁的小规模记录更新。
- JSON 更便于用户直接查看、备份、删除和手动修复。

因此 `kiss run`、`kiss list` 和 `kiss show` 不依赖 DuckDB。

## 6. Registry 与解析

KISS registry 可以先保持为一个简单 JSON 或 TOML 索引：

```toml
[skills.browser-test]
source = "github"
repo = "example/agent-skills"
path = "skills/browser-test"
ref = "v0.1.0"
sha256 = "..."
public_key = "base64-ed25519-public-key"
signature = "base64-ed25519-signature"
```

签名字段是可选校验层；未签名 entry 继续兼容。签名 entry 使用 Ed25519，`signature` 校验以下 payload：

```text
kiss-registry-entry-v1
name=<local-skill-name>
source=<resolved-source-spec>
sha256=<lowercase-sha256>
```

当 `public_key` 和 `signature` 同时存在时，KISS 必须验证签名；两者只配置一个、签名失败或签名 entry 缺少 `sha256` 时，该 registry entry 不得安装。签名验证成功后，`registry.lock` 记录 `signature_verified` 和 `public_key_sha256` 供人工审计。

本地 registry trust policy 存在 `$KISS_HOME/config.toml` 中：

```toml
[registries.team]
url = "https://example.com/kiss-registry.toml"
require_signature = true
trusted_keys = ["<ed25519-public-key-sha256>"]
```

- `require_signature = true`：该 registry 命中的 entry 必须带有效签名。
- `trusted_keys`：非空时，entry 必须由列表中的 public key fingerprint 签名。
- `kiss registry require-signature <name>`：启用签名必需策略。
- `kiss registry trust <name> <public-key-base64>`：把 Ed25519 public key 的 sha256 fingerprint 写入 trusted keys。

解析只发生在显式安装或更新命令中，`kiss run` 不触发 resolver。解析顺序：

1. 显式来源：例如 `kiss add owner/repo --skill name`。
2. 本地 alias：`$KISS_HOME/config.toml` 中的 `[aliases]`。
3. 已配置 registry：官方、团队、个人 registry 依次查询。
4. `registry.lock` 中的固定解析结果。

MVP 不需要中心化服务；可以直接使用 GitHub raw、Git tag、压缩包 URL 或本地路径。签名 registry entry 和本地 trust policy 只校验 registry entry，不代表远端 trust discovery 或多签名 quorum。

## 7. Run 协议

`kiss run <skill> <args>` 只处理已安装 skill；如果 skill 不在本地数据库或安装目录中，返回错误并提示先执行 `kiss add ...`，不得自动下载。默认 `markdown` runner 中，`run` 的含义是加载、校验、封装并输出当前 skill 的上下文，而不是由 KISS 直接调用大模型或直接完成用户任务。它的输出应当适合 agent 消费，而不是把所有已安装 skill 都暴露给 agent。

用户在 agent 中输入 `/kiss <skill> <args>` 时，KISS bridge skill 或 slash adapter 只做一件事：调用 `kiss run <skill> <args>`，把 stdout 作为当前轮上下文交给 agent。之后的推理、工具调用、脚本选择和文件修改都由外层 agent 按自己的权限模型完成。

推荐输出结构：

```markdown
# KISS loaded skill: browser-test

## User invocation

- Skill: browser-test
- Args: url=https://example.com

## Skill instructions

<SKILL.md content>

## KISS runtime notes

- Only this skill was loaded.
- Do not inspect $KISS_HOME unless the user explicitly asks.
- KISS did not call any model provider; the invoking agent decides how to use these instructions.
- KISS does not execute skill scripts; the invoking agent decides what work to perform.
```

## 8. 安全模型

### 8.1 默认不信任

从远端下载的 skill 默认是不受信任内容。KISS 应至少做以下检查：

- 下载到临时目录，校验摘要后再原子安装。
- 拒绝包含绝对路径、`..` 逃逸路径或 symlink 逃逸的包。
- `kiss.skill.toml` 中必须声明脚本入口；未声明脚本不可执行。
- 默认不允许 skill 自行修改 `$KISS_HOME/config.toml` 或其他 skill。

### 8.2 最小暴露

- agent 只获得当前被调用 skill 的内容。
- `kiss list` 默认只输出名称和版本；`kiss show <name>` 才显示描述和来源。
- `/kiss` adapter 不把 `$KISS_HOME/skills` 注册为 agent skill 目录。

### 8.3 可复现

- 安装记录固定 `ref`、commit 和 sha256。
- `kiss update` 生成 diff 摘要，用户确认后替换。
- `registry.lock` 记录解析结果，避免同名 skill 漂移。

## 9. 与 AGENTS.md 的关系

AGENTS.md 适合放项目级、总是需要的约束；KISS skill 适合放“用户显式调用时才需要”的过程知识。

推荐项目根目录 `AGENTS.md` 只写 KISS 的少量约束：

```markdown
- 只有当用户显式输入 `/kiss <skill>` 或 `$kiss <skill>` 时才调用 KISS。
- 不要枚举或读取 `$KISS_HOME/skills` 来寻找可用 skill。
- KISS skill 不属于本项目文件；不要把下载后的 skill 提交到仓库。
```

这样 agent 知道如何尊重 KISS 的边界，但不会在每次会话开始时读到所有 skill 的名称和描述。

## 10. MVP 组件

| 组件 | 职责 | MVP 实现建议 |
| --- | --- | --- |
| CLI | `kiss add/run/list/show/update/remove/doctor` | Go 单文件 CLI |
| Store | 管理 `$KISS_HOME` | 普通文件系统 + 原子 rename |
| Metadata DB | 管理 `skills.db.json` | JSON 文件 + schema version |
| Resolver | 名称到来源 | 本地 TOML registry |
| Installer | 下载和校验 | Git archive 或 tarball |
| Loader | 只读取一个 skill | 读取 manifest + `SKILL.md` |
| Runner | 输出当前 skill 上下文，不调用 LLM、不执行脚本 | `markdown` runner |
| Adapter | `/kiss`、`$kiss`、KISS bridge skill 接入 | 薄封装，调用 CLI |

## 11. 实现技术路线

KISS 的实现路线应先把“可运行的本地 CLI”做扎实，再增加 registry、签名和更多 agent adapter。核心判断是：**不要先做平台，不要先做 UI，不要先做复杂生态；先做一个能被 `/kiss` 和 `$kiss` 稳定调用的 `kiss run`，并把结果干净地交给外层 agent**。这个路线不包含 standalone LLM executor、command runner 或本地 untrusted-code sandbox。

### 11.1 技术选型与分发策略

MVP 采用 **Go** 实现 CLI。这个选择优先满足 PRD 的 NFR-1：普通用户不应为了使用 `kiss` 预装 Node、Bun 或 TypeScript 运行时。

推荐策略：

| 阶段 | 实现方式 | 用户是否需要 Node/Bun | 说明 |
| --- | --- | --- | --- |
| MVP 开发 | Go CLI | 不需要 | 标准库即可完成本地 store、manifest 解析、Markdown 输出和测试。 |
| MVP 分发 | GitHub Release 预编译单文件 CLI + `install.sh` / `install.ps1` | 不需要 | macOS / Linux 主入口是 `curl .../install.sh \| bash`；Windows 主入口是 `irm .../install.ps1 \| iex`。 |
| macOS 包管理 | Homebrew formula artifact | 不需要 | 作为 GitHub Release artifact 生成，不自动推送外部 tap。 |
| 非目标入口 | npm / pnpm / bunx shim、Linux 发行版包、Windows 包管理器、MSI | 不适用 | 不进入当前交付范围。 |
| 长期稳定 | Go CLI + 签名 release | 不需要 | 在 release 阶段补充 checksums、签名和平台矩阵。 |

Release 构建应输出平台 tarball、`checksums.txt` 和由 checksum 派生的 `kiss.rb` Homebrew formula。macOS / Linux 用户默认通过 `curl .../install.sh | bash` 安装，Windows 用户默认通过 `irm .../install.ps1 | iex` 安装，脚本从 GitHub Release 下载对应平台 tarball。KISS 发布版本以 Git tag 为唯一来源：源码默认版本是 `dev`，`scripts/build-release.sh` 用 `KISS_VERSION` 或 `git describe --tags --always --dirty` 通过 Go `ldflags -X` 注入 binary。GitHub Release workflow 只支持手动触发，必须在 release tag ref 上运行，并要求 `version` 输入与 tag 一致。Workflow 使用 OIDC + keyless cosign 对 `checksums.txt` 生成 detached signature 与 certificate，并作为 `checksums.txt.sig` / `checksums.txt.pem` 上传。安装脚本默认只做 sha256 校验；当用户显式设置 `KISS_VERIFY_SIGNATURE=1` 且本机存在 `cosign` 时，再验证 `checksums.txt` 的签名。`kiss.rb` 可以作为 GitHub Release artifact 或复制到独立 tap；KISS 本仓库不自动推送外部 tap。

分发验收标准：

```bash
# 在没有 node、npm、bun 的干净环境中也应能运行
kiss --version
kiss doctor --kiss-home /tmp/kiss-test
```

建议包结构：

```text
cmd/kiss/
  main.go          # 参数解析和命令分发
internal/kiss/
  paths.go         # KISS_HOME、临时目录、原子 rename
  manifest.go      # kiss.skill.toml 解析和校验
  install.go       # 本地安装、safe copy、安装记录
  remote.go        # 显式 HTTPS/GitHub 远程安装、sha256、safe extract
  metadata.go      # skills.db.json 读写和查询
  runner.go        # markdown runner
  list.go          # list/show/remove/doctor
  types.go         # 共享类型和版本
  kiss_test.go     # 核心行为测试
```

### 11.2 MVP 里程碑

#### M0：仓库脚手架

- 初始化 `cmd/kiss` 和 `internal/kiss`。
- 使用 Go 标准库 `flag` 做命令解析。
- 使用 `go test` 做单元测试。
- 约定所有文件系统测试使用临时 `KISS_HOME`，禁止污染用户真实目录。

验收标准：

```bash
kiss --help
kiss doctor --kiss-home /tmp/kiss-test
```

#### M1：本地 skill 可运行

先不接远程 registry，只支持本地路径安装和运行：

```bash
kiss add ./fixtures/skills/browser-test --name browser-test
kiss show browser-test
kiss run browser-test url=https://example.com
```

实现重点：

1. `paths.go` 解析 `$KISS_HOME`，默认 `${XDG_DATA_HOME:-~/.local/share}/kiss`。
2. `manifest.go` 解析 `kiss.skill.toml`；如果没有 manifest，但存在 `SKILL.md`，自动补一个内存默认 manifest。
3. `install.go` 从本地目录复制到临时目录，校验路径安全后原子安装到 `$KISS_HOME/skills/<name>`。
4. `metadata.go` 写入 `$KISS_HOME/skills.db.json`，支持 `list/show/remove` 查询和更新。
5. `runner.go` 只读取被点名 skill 的 manifest 和 `SKILL.md`，并输出 Markdown envelope。
6. 本轮先不执行任何脚本。
7. 不读取模型 API key，不要求配置模型 provider。

验收标准：`kiss run` 输出中只能包含当前 skill，不包含 `$KISS_HOME/skills` 下其他 skill 的名称或描述。

#### M2：显式远程安装与锁定

已开始支持 GitHub tarball / explicit HTTPS tar.gz：只允许通过 `kiss add ...` 显式安装，不允许 `kiss run` 自动安装：

```bash
kiss add github:owner/repo/path/to/skill#v0.1.0 --name browser-test
kiss show browser-test
kiss run browser-test url=https://example.com
```

实现重点：

- `remote.go` 支持 `github:`、`https:` 两类显式远程 source；`local` 仍由 `install.go` 处理。
- `remote.go` 下载到 `$KISS_HOME/cache/downloads`，计算 sha256，safe extract。
- `.kiss-install.json` 和 `skills.db.json` 记录 source、ref、resolved、sha256、installed_at。
- 如果 registry 提供 sha256，安装时必须匹配；如果用户直接指定 URL，首次安装可以生成并记录 sha256。

#### M3：registry 与更新

增加 registry 文件，但保持它是静态文件，不引入中心化服务：

```bash
kiss registry add personal https://example.com/kiss-registry.toml
kiss add browser-test
kiss update browser-test
```

实现重点：

- `registry.toml` 只做 name -> source/ref/sha256 映射。
- `registry.lock` 固定本地解析结果，避免同名 skill 漂移。
- registry entry 可选 Ed25519 签名；签名成功时 `registry.lock` 记录签名审计字段。
- registry trust policy 可要求某个 registry 必须使用签名 entry，并限制可接受 public key fingerprint。
- 多 registry 解析按确定性顺序查询；单个 registry 失败或未命中时继续查询后续 registry，并在全部失败时汇总错误摘要。
- `kiss update` 下载到 side-by-side 目录，展示 manifest 摘要和 entry 文本 diff，确认后再替换。
- entry diff 必须有输出上限；当内容过大时保留 sha256 摘要并提示 diff 被省略或截断。

#### M4：agent adapter

在 CLI 稳定后再做薄 adapter：

- `/kiss` slash command：把用户输入转成 `kiss run <skill> <args>`，把 stdout 作为本轮上下文注入。
- KISS bridge skill：给支持 skill 的 agent 一个小型桥接说明；当用户显式输入 `/kiss <skill> [args...]` 时，agent 调用 `kiss run` 并把 stdout 当作当前轮上下文。
- `$kiss` shell-style 调用：等价于 `kiss run`，适合 Codex 或 terminal-first agent。
- `powershell` wrapper：等价于 `kiss run`，适合 Windows PowerShell 或跨平台 PowerShell 用户。
- adapter 不实现 installer/loader 逻辑，也不实现模型调用逻辑，只调用 CLI，避免多份实现行为不一致。
- MVP 提供 `kiss adapter print slash`、`kiss adapter print skill`、`kiss adapter print shell` 和 `kiss adapter print powershell` 输出可审计模板；不自动写入任何 agent 的全局目录。
- 如需写入 adapter 文件，使用 `kiss adapter install <slash|skill|shell|powershell> --path <path>` 显式指定路径；`kiss adapter uninstall --path <path>` 只删除带 KISS marker 的 adapter 文件。

#### M5：移除本地执行器

KISS 不实现 command runner。manifest 中显式声明 `runner.type = "command"` 或其他非 `markdown` runner type 时，CLI 必须拒绝安装或运行。skill 中可以包含引用资料、示例或脚本片段，但 KISS 只负责把 `SKILL.md` 作为上下文交给 agent；是否执行任何后续动作由外层 agent 按自己的权限模型决定。

## 12. 测试路线

最小测试矩阵：

- `paths`：`KISS_HOME` 默认值、显式覆盖、跨平台路径。
- `manifest`：合法 TOML、缺省 manifest、非法 runner。
- `installer`：本地安装、重复安装、原子替换、sha256 mismatch、路径穿越、symlink escape。
- `metadata`：add 写入数据库、list/show 读取数据库、remove 删除记录、schema version 兼容、`full_name` 生成与展示。
- `loader`：只加载指定 skill，不读取其他 skill。
- `runner`：Markdown envelope 稳定、参数转义正确、JSON 输出可解析。
- `resolver`：local/github/https/registry/lockfile 优先级；确认 `run` 不触发 resolver。
- `adapter`：只做命令转发，不扫描 `$KISS_HOME/skills`。

## 13. 参考资料

- Vercel changelog: `https://vercel.com/changelog/introducing-skills-the-open-agent-skills-ecosystem`
- Vercel docs: `https://vercel.com/docs/agent-resources/skills`
- Skills CLI docs: `https://skills.sh/docs/cli`
