# KISS Skill Store 架构

本文描述 KISS 的系统边界、组件职责和关键架构约束。产品定位和优先级见 [`kiss-skill-store-prd.md`](kiss-skill-store-prd.md)
。命令参数、字段结构、测试矩阵和实现细节以代码、测试和安装文档为准。

## 1. 架构目标

KISS 是一个独立的 skill store 和 explicit context provider。它不替代 coding agent，不进入模型推理循环，也不把 KISS 管理的
skill 注册到任何 agent 的原生扫描目录。

核心目标：

1. `run` 只加载已安装且被点名的 skill，不自动安装。
2. 每次调用只暴露一个 skill 的上下文。
3. 安装、更新、导入和删除都是用户显式动作。
4. KISS 只输出 Markdown 上下文，不执行 skill 内脚本。
5. 本地状态可审计、可备份、可删除。

## 2. 系统边界

```text
Install / manage path:

User
  -> KISS CLI
  -> Resolver / Installer / Metadata
  -> KISS Store

Run path:

User
  -> Agent bridge skill
  -> KISS CLI
  -> Loader / Envelope
  -> Agent receives selected skill context
```

KISS 的边界很窄：

- KISS 负责保存 skill、记录来源、加载被点名 skill、输出上下文。
- 外层 agent 负责理解上下文、调用工具、编辑文件和执行后续任务。
- KISS bridge skill 只负责把显式 `/kiss` / `$kiss` 请求转发给 CLI。
- KISS store 不应被注册为 Claude、Codex、Cursor、Gemini 等工具的全局 skill 目录。
- `kiss setup` 可以把 KISS bridge skill 安装到支持的 agent 原生 skill 目录，但不安装 KISS 管理的第三方
  skill；具体目录以安装指南和代码为准。

## 3. 核心组件

| 组件        | 职责                                 | 边界                             |
|-----------|------------------------------------|--------------------------------|
| CLI       | 接收显式管理和运行命令                        | 不做后台同步                         |
| Store     | 保存 KISS 管理的 skill                  | 不写入 agent 全局目录或项目目录            |
| Metadata  | 保存本地索引和来源信息                        | 不作为 agent discovery catalog 暴露 |
| Resolver  | 将明确 source 转为可安装包                  | 不支持裸名称远程搜索                     |
| Installer | 安装、替换、校验包内容                        | 不执行包内脚本                        |
| Loader    | 读取被点名 skill 的入口 Markdown           | 不枚举其他 skill                    |
| Envelope  | 生成给 agent 消费的上下文输出                 | 不调用模型 provider                 |
| Setup     | 安装 KISS bridge skill 到支持的 agent 目录 | 不安装第三方 skill                   |
| Bridge    | 连接 agent slash command 和 CLI       | 不包含用户安装的 skill                 |

## 4. 本地状态模型

KISS 的本地状态由三类内容组成：

- **已安装 skill**：存放在 KISS 专属 store 中，是 `run` 唯一会加载的内容来源。
- **元数据索引**：记录审计和管理所需的来源、版本/摘要和安装状态。
- **缓存和日志**：服务于安装、更新和排查；删除后不应影响已安装 skill 的可运行性。

架构约束：

- 已安装 skill 的目录名是 KISS 本地名称，不要求等于上游名称。
- 元数据用于管理命令和审计，不用于在 agent 启动时暴露 skill catalog。
- 删除 KISS 专属目录应能完整移除 KISS 管理的状态。
- 具体文件名、schema 和字段兼容策略属于实现细节。

## 5. Skill 包模型

KISS 兼容“目录 + Markdown 入口”的简单 skill 包模型。入口 Markdown 是 KISS 与 agent 之间的唯一上下文交接内容。

架构约束：

- skill 可以包含资料、示例或脚本文件，但 KISS 不读取脚本入口，也不执行脚本。
- skill 内容只在被安装或被点名运行时读取。
- KISS 管理的 skill 不应被提交回当前项目仓库。

## 6. 生命周期

### 6.1 安装和导入

安装和导入都属于显式管理动作。KISS 可以从本地路径、远程来源或已有全局安装快照创建 KISS store 中的副本，但不会在 `run`
时自动拉取或补装。

安装完成后，KISS 应记录足够的来源信息，支持用户审计、展示和后续显式更新。版本判断、来源解析、同源判断和导入细节由实现维护。

### 6.2 运行

运行路径必须保持最小：确认 skill 已安装，只读取被点名 skill 的入口 Markdown，并输出给调用方 agent。`run` 不访问网络、不更新本地
store、不调用模型、不读取其他 skill。

### 6.3 更新和删除

更新是基于已安装来源的显式刷新，删除是对本地 store 和 metadata 的显式清理。更新只在内容变化时替换已安装 skill。两者都不改变
agent 的默认 discovery 行为。

## 7. 安全边界

KISS 的安全模型以“不自动暴露、不自动执行”为核心：

- 远程 skill 默认不可信。
- 安装过程必须防止文件写入逃逸 KISS 管理边界。
- `run` 不执行脚本，不读取模型 API key，不访问网络。
- agent 只获得当前被调用 skill 的上下文。
- Bridge skill 不把 KISS store 注册为 agent 原生 skill 目录。

KISS 不能保证第三方 skill 内容本身安全；它只保证 KISS 主路径不会自动执行这些内容。

## 8. 架构决策

### 8.1 独立 Store

KISS 使用专属 store，而不是复用 agent 全局 skill 目录。这样可以避免 agent 启动时自动扫描 KISS skill，也让删除、备份和审计有明确边界。

### 8.2 CLI 优先

CLI 是稳定核心。Bridge skill、安装脚本和其他集成方式都应尽量薄，只转发到 CLI，不复制业务逻辑。

### 8.3 Markdown Context Handoff

KISS 的输出是给 agent 消费的 Markdown 上下文，不是一个独立执行结果。这个边界避免 KISS 变成另一个 agent runtime。

### 8.4 简单本地索引

KISS 的核心状态规模很小，优先使用可读、可审计、易删除的本地索引。分析型存储或复杂数据库不进入核心路径。

### 8.5 安装语法与 source 发现跟随 npx skills

`kiss add` 的安装语法跟随 Vercel Skills CLI（`npx skills add`）：同一条命令把 `npx skills` 换成 `kiss` 后应选中相同的 skill，
用户不需要为 KISS 改写仓库结构或学习另一套 source 语法。

source 语法：`owner/repo`、`owner/repo/path`、`owner/repo@skill`、`#ref` 与 `#ref@skill` 片段、`github:` 前缀、`github.com`
URL（含 `/tree/<ref>/<path>`），以及本地路径。`@` 在 source 中只表示 skill，Git ref 只用 `#ref` 表达。flag 语法：`-s/--skill`
（可多个，`*` 表示全部）、`--all`、`-l/--list`、`--full-depth`。只影响 agent 目录安装的 flag（`-g`、`-y`、`-a`、`--copy`）被接受但
不生效。KISS 只支持其中的本地路径和 GitHub source。

发现规则：
KISS 跟随 npx skills 的发现规则：被点名目录自身含 `SKILL.md` 时只取它；否则依次搜索 `skills/`（含 `.curated`、`.experimental`、
`.system`）、各 agent 的项目 skill 目录（如 `.agents/skills`、`.claude/skills`）、`skills.sh.json` 和 `.claude-plugin` manifest
声明的位置；都没有时再做有限深度的全量搜索。frontmatter 标记 `metadata.internal: true` 的 skill 默认隐藏，source 仓库通过
`skills-lock.json` 引入的第三方 skill 不计入。

差异和约束：

- KISS 不做交互选择：source 含多个 skill 且未指定 `--skill`/`--all` 时报错列出候选；点名但不存在的 skill 直接报错。
- `--skill` 按 frontmatter `name` 匹配（大小写、空格/下划线不敏感），并兼容按目录名匹配；显式点名时可选中 internal skill。
- 多 skill 安装先完成全部校验（命名冲突、异源保护、link 检查）再写 store。
- metadata 记录实际选中的 skill 路径，`update` 据此精确刷新，同源判断也与 `kiss import` 记录的路径一致。
- `kiss update <name>@ref` 不属于安装语法，保持原样。
- 远程 archive 中的 link 条目永远不会写到磁盘；只有落在被选中 skill 目录内的 link 才会导致安装失败。

## 9. 参考资料

- Vercel Skills: `https://vercel.com/docs/agent-resources/skills`
- Skills CLI: `https://skills.sh/docs/cli`
