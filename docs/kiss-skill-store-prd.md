# KISS Skill Store PRD

## 1. 背景

很多 coding agent 会在启动时扫描全局或项目级 skill 目录，并把 skill 的 `name`、`description` 等元信息放进上下文。这个机制适合“让 agent 自己发现能力”，但不适合以下场景：

- 用户已经明确知道本轮需要哪个 skill，不需要 agent 先阅读所有 skill 描述。
- 某些 skill 是个人工作流、私有知识或临时能力，不希望进入 agent 的全局技能目录。
- 希望把 skill 的安装、缓存和加载变成一个显式动作，减少默认上下文污染。

KISS 的产品定位是：一个面向 coding agent 的显式 skill store 和 context provider。用户必须先通过 `kiss add ...` 显式安装 skill；之后再通过 `/kiss <skill_name> ...`、`$kiss <skill_name> ...` 或 `kiss run <skill_name> ...` 指定要用的 skill。`run` 在默认 `markdown` runner 下只加载已安装且被点名的 skill，并把最小上下文输出给调用方 agent；后续推理、工具调用和文件编辑由外层 agent 完成。`run` 不自动下载或安装，也不调用任何大模型。对支持 Vercel Skills / skills.sh 的 agent，推荐安装本仓库 `skills/kiss` 中的 KISS bridge skill，让它把 `/kiss` 显式请求转发为 `kiss run`。

## 2. 产品目标

1. **显式调用**：只有用户明确调用时才加载 skill。
2. **隔离存储**：KISS 管理的 skill 不进入 agent 全局目录，也不进入项目目录。
3. **显式安装**：本地没有 skill 时，提示用户先执行 `kiss add ...`；`run` 不自动下载或安装。
4. **最小上下文暴露**：agent 只看到本次被调用的 skill，不看到 KISS store 里的其他 skill 名称和描述。
5. **不接管模型执行**：LLM 推理由外层 coding agent 负责；KISS 不配置、不管理、不代理任何模型接入点。
6. **明确交接给 agent**：`/kiss <skill> <params>` 的效果是把当前 skill 上下文交给 agent，而不是让 KISS 自己完成任务。
7. **可审计、可删除**：用户可以查看 skill 来源、版本、校验和；删除 KISS 专属目录即可移除全部 KISS skill。

## 3. 非目标

- 不自动向所有 agent 安装 skill。
- 不在 agent 启动时提供 skill catalog。
- 不把下载后的 skill 放入项目目录。
- 不做复杂依赖解析；skill 依赖应尽量 vendored 或在 `doctor` 中提示。
- 不把 KISS 设计成长期后台 daemon。
- 不实现 standalone LLM executor，也不提供脱离外层 agent 的自主推理循环。
- 不配置、读取、代理或管理 OpenAI、Anthropic、Gemini 等模型 provider 接入点。
- 不实现 command runner；KISS 不直接执行 skill 内脚本。
- 不承诺安全运行任意不可信代码；第三方 skill 默认不获得写入项目或执行任意命令的能力。
- 不提供 npm / pnpm / bunx shim、Linux 发行版包、Windows 包管理器入口或 MSI。

## 4. 目标用户

- **明确知道要用哪个 skill 的用户**：不希望 agent 先扫描所有 skill 描述。
- **维护私有工作流的用户或团队**：希望 skill 可复用，但不进入 agent 全局目录。
- **terminal-first / coding-agent 用户**：希望通过 slash command、shell-style command 或 CLI 一致调用 skill。

## 5. 核心用户故事

### 5.1 首次运行一个未安装 skill

作为用户，我输入：

```bash
/kiss browser-test url=https://example.com
```

期望：

1. KISS 识别 `browser-test` 和参数。
2. KISS 发现本地未安装该 skill。
3. KISS 不自动下载或安装，而是返回明确错误，提示用户先执行 `kiss add ...`。
4. agent 不收到任何未安装 skill 的内容。

### 5.2 显式安装并调用一个 skill

作为用户，我先安装再运行：

```bash
kiss add ./skills/browser-test
/kiss browser-test url=https://example.com
```

期望：

1. `kiss add` 把 skill 保存到 KISS 专属目录，并写入本地元数据数据库。
2. KISS 命中本地已安装 skill。
3. `run` 默认不访问网络，也不自动更新。
4. KISS 只加载 `browser-test`，不枚举其他 skill。
5. agent 收到最小 skill 上下文，并由 agent 自己决定如何使用该上下文完成任务。

### 5.3 管理本地 skill

作为用户，我可以执行：

```bash
kiss add vercel-labs/agent-skills --skill browser-test
kiss add file:///abs/path/to/local-skill --name browser-test
kiss run browser-test url=https://example.com
kiss list
kiss update browser-test
kiss remove browser-test
kiss doctor
```

期望：管理命令不改变 agent 的默认 discovery 行为；只有 `kiss run` 或显式 `/kiss` / `$kiss` 才会加载 skill。

## 6. 产品原则

1. **KISS first**：MVP 只做 install、run、list、remove、update、doctor，不做后台同步、不做自动注入。
2. **默认不打扰 agent**：KISS skill 不通过 AGENTS.md、agent 全局 skill 目录或项目目录暴露给 agent。
3. **人工可理解**：skill 包结构应以 `SKILL.md` 为核心，可选 manifest、脚本和资源。
4. **安全默认值**：远端 skill 默认不可信；默认主路径只做上下文交接，不执行远端脚本。
5. **不内置模型层**：不要为 `kiss run` 增加模型 provider、API key 管理或 standalone agent loop。
6. **bridge skill 优先**：支持 Vercel Skills 的 agent 通过仓库内 `skills/kiss/SKILL.md` 接入。

## 7. 功能需求

| 编号 | 需求 | 优先级 |
| --- | --- | --- |
| FR-1 | 用户可以通过 `kiss run <skill> <args>` 显式加载一个已安装 skill，并把当前 skill 上下文输出给调用方 agent | P0 |
| FR-2 | 用户可以通过 `/kiss <skill> <args>` 或 `$kiss <skill> <args>` 触发同等行为 | P0 |
| FR-3 | 本地没有 skill 时，`run` 必须提示先执行显式 `kiss add ...`，不得自动安装 | P0 |
| FR-4 | 本地已有 skill 时，默认直接加载上下文，不访问网络 | P0 |
| FR-5 | KISS 只向 agent 暴露当前被调用 skill 的内容 | P0 |
| FR-6 | 用户可以 list、update、remove、doctor | P1 |
| FR-7 | 用户可以查看 skill 来源、版本、sha256、安装路径和安装时间 | P1 |
| FR-8 | 用户可以通过 `npx skills add wwulfric/kiss` 安装 KISS bridge skill，让 agent 把显式 `/kiss` 请求转成 `kiss run`；`--skill kiss` 可作为显式指定写法 | P1 |
| NFR-1 | 默认安装形态不要求普通用户预装 Node、Bun 或 TypeScript 运行时 | P0 |
| NFR-2 | 本地必须保存 skill 元数据数据库，供 `list/show/update` 查询，不依赖 agent 扫描 skill 内容 | P0 |
| NFR-3 | skill 元数据必须同时保存本地短名 `name` 和来源限定全称 `full_name` | P0 |
| NFR-4 | 默认核心数据库不得依赖 DuckDB/Parquet 等额外 native 分析引擎；这类能力只能作为可选导出/分析层 | P1 |
| NFR-5 | KISS 不要求任何模型 API key 或 provider 配置；`doctor` 不应把模型接入点作为健康检查前提 | P0 |
| NFR-6 | GitHub Release 产物必须提供 sha256 校验；签名验证可作为显式开启的增强能力，不作为默认安装前提 | P1 |

## 8. 体验约束

- `/kiss` 和 `$kiss` 是用户入口；底层统一调用 `kiss run`。
- `/kiss`、`$kiss` 和 `kiss run` 不自动安装 skill；缺失时只提示用户先显式安装。
- KISS bridge skill 是支持 Vercel Skills / skills.sh 的 agent 的推荐接入方式；它的职责是把用户输入 `/kiss <skill> [args...]` 转成 `kiss run <skill> [args...]`，再把 stdout 作为当前轮上下文交给 agent。
- 在纯终端直接运行 `kiss run` 时，默认输出可被 agent 消费的 Markdown envelope，而不是 standalone LLM 对话结果。
- `kiss list` 默认按 `name<TAB>version<TAB>source.kind<TAB>full_name` 输出；`kiss show <name>` 才显示更完整元数据。
- KISS 不应要求用户把 `$KISS_HOME/skills` 注册给任何 agent。
- KISS 的默认安装方式应提供预编译 CLI；macOS / Linux 主入口是 `curl .../install.sh | bash`，Windows 主入口是 `irm .../install.ps1 | iex`，不得要求用户预装 Node、Bun 或 TypeScript。
- 安装脚本应识别本机已有版本；目标版本等于当前版本时默认不下载、不更新并说明原因，目标版本低于当前版本时默认不降级，目标版本高于当前版本时执行更新。
- KISS 不提供 npm / pnpm / bunx shim；这些入口不能进入核心路径。
- 下载、更新、删除操作要可解释；失败时给出下一步建议。

## 9. 成功指标

- 调用 `/kiss <name> ...` 时，agent 上下文中只出现 `<name>` 对应 skill。
- 删除 `$KISS_HOME` 后，KISS 管理的 skill 被完整移除。
- 已安装 skill 的重复运行不访问网络。
- 未安装 skill 的运行请求不会触发自动安装。
- `kiss run` 不要求用户配置模型 provider，也不会直接调用模型 API。
- `kiss list` 和 `kiss show` 可从本地数据库读取元数据。
- 每个 skill 都有本地短名 `name` 和来源限定全称 `full_name`，例如 `github:owner/repo/path`。
- 远端安装能记录来源、版本、full_name 和 sha256。
- KISS bridge skill 逻辑足够薄，行为与 CLI 一致。
- Release tarball 默认可通过 `checksums.txt` 校验；需要更强供应链审计时，可显式验证 `checksums.txt` 的签名。

## 10. 产品路线图

### Phase 0：文档和约定

- 初始化仓库 AGENTS.md。
- 固化 KISS 的产品边界、目录、manifest 和 run 协议。
- 明确“不进入 agent 全局目录、不进入项目目录”的边界。

### Phase 1：本地 CLI MVP

- `kiss add <local-path> [--name <name>]`。
- `kiss add file:///abs/path/to/skill [--name <name>]`。
- `kiss run <name> [args...]`。
- `kiss list/show/remove/doctor`。
- 本地元数据数据库 `skills.db.json`。
- 验证 agent 只获得当前 skill 的上下文。

### Phase 2：远程来源与更新

- 支持 Vercel 风格 `kiss add owner/repo` 和 `kiss add owner/repo --skill <skill-name>`，安装目标仍是 KISS 专属目录。
- 支持显式 `kiss add github:...` / `kiss add https://...tar.gz`；仍不在 `run` 中自动安装。
- 支持 GitHub ref pinning、resolved commit 记录、sha256 记录和更新 diff。
- 不支持通过裸 skill 名称查询远程索引；`kiss add` 必须接收明确 source。

### Phase 3：Bridge skill 与 context handoff

- 仓库内 `skills/kiss` KISS bridge skill，支持 `npx skills add wwulfric/kiss`；脚本化场景可用 `--skill kiss` 显式指定。
- 明确 context handoff 协议：KISS 输出当前 skill 信息，agent 做后续工作。
