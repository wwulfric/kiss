# KISS Skill Store PRD

## 1. 背景

很多 coding agent 会在启动时扫描全局或项目级 skill 目录，并把 skill 的 `name`、`description` 等元信息放进上下文。这个机制适合“让 agent 自己发现能力”，但不适合以下场景：

- 用户已经明确知道本轮需要哪个 skill，不需要 agent 先阅读所有 skill 描述。
- 某些 skill 是个人工作流、私有知识或临时能力，不希望进入 agent 的全局技能目录。
- 希望把 skill 的安装、缓存和执行变成一个显式动作，减少默认上下文污染。

KISS 的产品定位是：一个面向 coding agent 的显式 skill store。用户必须先通过 `kiss add ...` 显式安装 skill；之后再通过 `/kiss <skill_name> ...`、`$kiss <skill_name> ...` 或 `kiss run <skill_name> ...` 指定要用的 skill。`run` 只加载已安装且被点名的 skill，不自动下载或安装。

## 2. 产品目标

1. **显式调用**：只有用户明确调用时才加载 skill。
2. **隔离存储**：KISS 管理的 skill 不进入 agent 全局目录，也不进入项目目录。
3. **显式安装**：本地没有 skill 时，提示用户先执行 `kiss add ...`；`run` 不自动下载或安装。
4. **最小上下文暴露**：agent 只看到本次被调用的 skill，不看到 KISS store 里的其他 skill 名称和描述。
5. **可审计、可删除**：用户可以查看 skill 来源、版本、校验和；删除 KISS 专属目录即可移除全部 KISS skill。

## 3. 非目标

- 不自动向所有 agent 安装 skill。
- 不在 agent 启动时提供 skill catalog。
- 不把下载后的 skill 放入项目目录。
- 不做复杂依赖解析；skill 依赖应尽量 vendored 或在 `doctor` 中提示。
- 不把 KISS 设计成长期后台 daemon。
- 不让第三方 skill 默认获得写入项目或执行任意命令的能力。

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
kiss add ./skills/browser-test --name browser-test
/kiss browser-test url=https://example.com
```

期望：

1. `kiss add` 把 skill 保存到 KISS 专属目录，并写入本地元数据数据库。
2. KISS 命中本地已安装 skill。
3. `run` 默认不访问网络，也不自动更新。
4. KISS 只加载 `browser-test`，不枚举其他 skill。
5. agent 收到最小执行上下文。

### 5.3 管理本地 skill

作为用户，我可以执行：

```bash
kiss add vercel-labs/agent-skills --skill browser-test
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
4. **安全默认值**：远端 skill 默认不可信；脚本执行必须显式声明并通过策略检查。
5. **渐进增强**：先保证本地 CLI 跑通，再做远程下载、registry、adapter 和 command runner。

## 7. 功能需求

| 编号 | 需求 | 优先级 |
| --- | --- | --- |
| FR-1 | 用户可以通过 `kiss run <skill> <args>` 显式运行一个 skill | P0 |
| FR-2 | 用户可以通过 `/kiss <skill> <args>` 或 `$kiss <skill> <args>` 触发同等行为 | P0 |
| FR-3 | 本地没有 skill 时，`run` 必须提示先执行显式 `kiss add ...`，不得自动安装 | P0 |
| FR-4 | 本地已有 skill 时，默认直接运行，不访问网络 | P0 |
| FR-5 | KISS 只向 agent 暴露当前被调用 skill 的内容 | P0 |
| FR-6 | 用户可以 list、update、remove、doctor | P1 |
| FR-7 | 用户可以查看 skill 来源、版本、sha256、安装路径和安装时间 | P1 |
| FR-8 | command runner 必须经过权限策略检查 | P2 |
| NFR-1 | 默认安装形态不要求普通用户预装 Node、Bun 或 TypeScript 运行时 | P0 |
| NFR-2 | 本地必须保存 skill 元数据数据库，供 `list/show/update` 查询，不依赖 agent 扫描 skill 内容 | P0 |
| NFR-3 | skill 元数据必须同时保存本地短名 `name` 和来源限定全称 `full_name` | P0 |
| NFR-4 | 默认核心数据库不得依赖 DuckDB/Parquet 等额外 native 分析引擎；这类能力只能作为可选导出/分析层 | P1 |

## 8. 体验约束

- `/kiss` 和 `$kiss` 是用户入口；底层统一调用 `kiss run`。
- `/kiss`、`$kiss` 和 `kiss run` 不自动安装 skill；缺失时只提示用户先显式安装。
- `kiss list` 默认按 `name<TAB>version<TAB>source.kind<TAB>full_name` 输出；`kiss show <name>` 才显示更完整元数据。
- KISS 不应要求用户把 `$KISS_HOME/skills` 注册给任何 agent。
- KISS 的默认安装方式应提供预编译 CLI；`npx`、`pnpm dlx`、`bunx` 只能作为可选入口。
- 下载、更新、删除操作要可解释；失败时给出下一步建议。

## 9. 成功指标

- 调用 `/kiss <name> ...` 时，agent 上下文中只出现 `<name>` 对应 skill。
- 删除 `$KISS_HOME` 后，KISS 管理的 skill 被完整移除。
- 已安装 skill 的重复运行不访问网络。
- 未安装 skill 的运行请求不会触发自动安装。
- `kiss list` 和 `kiss show` 可从本地数据库读取元数据。
- 每个 skill 都有本地短名 `name` 和来源限定全称 `full_name`，例如 `github:owner/repo/path`。
- 远端安装能记录来源、版本、full_name 和 sha256。
- adapter 逻辑足够薄，行为与 CLI 一致。

## 10. 产品路线图

### Phase 0：文档和约定

- 初始化仓库 AGENTS.md。
- 固化 KISS 的产品边界、目录、manifest 和 run 协议。
- 明确“不进入 agent 全局目录、不进入项目目录”的边界。

### Phase 1：本地 CLI MVP

- `kiss add <local-path> --name <name>`。
- `kiss run <name> [args...]`。
- `kiss list/show/remove/doctor`。
- 本地元数据数据库 `skills.db.json`。
- 验证 agent 只获得当前 skill 的上下文。

### Phase 2：远程来源与锁文件

- 支持显式 `kiss add github:...` / `kiss add https://...tar.gz`；仍不在 `run` 中自动安装。
- 支持 `registry.toml` / `registry.lock`。
- 支持版本 pinning、sha256 校验、更新 diff。

### Phase 3：Adapter 与受控执行

- `/kiss`、`$kiss` adapter。
- 权限策略文件。
- 沙箱化 command runner。
- 可选的 web registry UI。
