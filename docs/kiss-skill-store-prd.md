# KISS Skill Store PRD

## 1. 产品定位

KISS 是一个面向 coding agent 的显式 skill store 和 context provider。它解决的问题不是“让 agent 自动发现更多能力”，而是让用户在明确知道本轮需要哪个
skill 时，可以只加载这个 skill，并避免全局 skill 扫描带来的上下文污染。

用户先显式安装 skill，再通过 `/kiss <skill> ...`、`$kiss <skill> ...` 或 `kiss run <skill> ...` 显式调用。KISS 只负责把被点名
skill 的 Markdown 上下文交给外层 agent；推理、工具调用和文件编辑仍由外层 agent 完成。

## 2. 产品目标

1. **显式调用**：只有用户明确调用时才加载 skill。
2. **隔离存储**：KISS 管理的 skill 不进入 agent 全局目录，也不进入项目目录。
3. **显式安装**：本地没有 skill 时，提示用户先安装；`run` 不自动下载或安装。
4. **最小暴露**：agent 只看到本次被调用的 skill，不看到 KISS store 中的其他 skill。
5. **不接管执行**：KISS 不直接调用模型，也不执行 skill 内脚本。
6. **可审计、可删除**：用户能查看来源和安装状态；删除 KISS 专属目录即可移除全部 KISS skill。

## 3. 非目标

- 不自动向所有 agent 安装 skill。
- 不在 agent 启动时提供 skill catalog。
- 不把下载后的 skill 放入项目目录。
- 不做后台同步、长期 daemon 或自动全局注入。
- 不实现本地命令执行器、standalone LLM executor 或模型 provider 管理。
- 不提供远程 skill 搜索市场；用户必须给出明确 source。
- 不在产品文档中承载包格式、元数据字段、签名验证或平台分发细节。

## 4. 目标用户

- 明确知道本轮要用哪个 skill 的 coding-agent 用户。
- 维护私有工作流、团队流程或临时能力的用户。
- 希望通过 slash command、shell-style command 或 CLI 一致调用 skill 的 terminal-first 用户。

## 5. 核心场景

### 5.1 调用未安装 skill

用户输入 `/kiss browser-test url=https://example.com`，但本地没有 `browser-test`。KISS 应返回明确错误，并提示用户先显式安装；不得自动下载，也不得向
agent 暴露任何未安装 skill 内容。

### 5.2 安装并调用 skill

用户先执行 `kiss add ...`，之后通过 `/kiss browser-test ...` 或 `kiss run browser-test ...` 调用。KISS 应只加载
`browser-test`，不枚举其他 skill，不访问网络，不调用模型，并把最小上下文交给外层 agent。

### 5.3 管理本地 store

用户可以查看、更新、删除、诊断和显式导入本地 skill。管理命令可以改变 KISS store，但不能改变 agent 的默认 discovery 行为；只有显式
`run` / `/kiss` / `$kiss` 才会加载 skill。

## 6. 产品原则

1. **显式优先**：安装、更新、导入、运行都由用户触发。
2. **默认不打扰 agent**：KISS store 不注册为任何 agent 的原生 skill 目录。
3. **上下文交接而非执行**：KISS 输出当前 skill 上下文，外层 agent 决定后续动作。
4. **简单可审计**：实现应容易理解、备份、删除和排查。
5. **文档分层**：PRD 只描述产品边界和体验目标；具体实现以架构文档、安装指南和代码为准。

## 7. 功能需求

| 编号    | 需求                                                                                  | 优先级 |
|-------|-------------------------------------------------------------------------------------|-----|
| FR-1  | 用户可以显式运行一个已安装 skill，并把当前 skill 上下文交给调用方 agent                                       | P0  |
| FR-2  | 本地没有 skill 时，运行必须失败并提示先安装，不得自动安装                                                    | P0  |
| FR-3  | KISS 只向 agent 暴露当前被调用 skill 的内容                                                     | P0  |
| FR-4  | KISS 管理的 skill 必须存放在 KISS 专属目录                                                      | P0  |
| FR-5  | 用户可以管理本地 skill，包括查看、更新、删除、诊断和显式导入                                                   | P1  |
| FR-6  | 用户可以通过 `kiss setup` 为支持的 agent 安装 KISS bridge skill，让 `/kiss` / `$kiss` 请求转成 CLI 调用 | P1  |
| NFR-1 | CLI 默认使用不应要求用户预装 Node、Bun 或 TypeScript 运行时                                          | P0  |
| NFR-2 | KISS 不要求任何模型 API key 或 provider 配置                                                  | P0  |
| NFR-3 | 本地状态必须可审计、可备份、可删除                                                                   | P0  |

## 8. 体验约束

- `/kiss`、`$kiss` 和 `kiss run` 是显式加载入口。
- 缺失 skill 时只提示安装，不自动安装。
- 已安装 skill 的重复运行不访问网络。
- 纯终端运行 `kiss run` 时，输出应适合 agent 消费，而不是 standalone 对话结果。
- KISS bridge skill 应保持很薄，只做显式请求转发。
- `kiss setup` 只安装 KISS bridge skill，不安装或同步 KISS 管理的第三方 skill。
- 下载、更新、删除等管理操作失败时，应给出可执行的下一步建议。

## 9. 成功指标

- 调用 `/kiss <name> ...` 时，agent 上下文中只出现 `<name>` 对应 skill。
- 删除 KISS 专属目录后，KISS 管理的 skill 被完整移除。
- 未安装 skill 的运行请求不会触发自动安装。
- `kiss run` 不要求用户配置模型 provider，也不会直接调用模型 API。
- 管理命令不会改变 agent 的默认 skill discovery 行为。

## 10. 产品路线图

1. **本地闭环**：完成安装、运行、管理和诊断。
2. **远程来源**：支持明确 source 的安装与显式更新。
3. **Agent 接入**：提供薄 bridge skill 和必要安装辅助。
4. **显式迁移**：允许用户把已有安装记录导入 KISS store。
