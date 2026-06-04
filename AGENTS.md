# AGENTS.md

本仓库用于设计和实现 KISS（Keep It Stupid Simple）skill store。

## 目标

- KISS 管理的 skill 必须存放在 KISS 专属目录中，不写入 coding agent 的全局 skill 目录，也不写入当前项目目录。
- coding agent 不应在会话启动时枚举、读取或总结 KISS 管理的 skill。
- 只有当用户显式调用 `/kiss <skill_name> ...`、`$kiss <skill_name> ...` 或等价 CLI 命令时，才解析、下载、加载并执行对应 skill。
- 优先保持实现简单、可审计、可删除；避免后台同步、自动全局注入和复杂的多 agent 适配层。

## 工作约定

- 文档与注释优先使用中文；命令、配置键和协议字段保持英文。
- 新增产品决策时，优先更新 `docs/kiss-skill-store-prd.md`；新增技术架构决策时，优先更新 `docs/kiss-skill-store-architecture.md`。
- 不要把示例 skill、缓存 skill 或下载后的第三方 skill 提交到本仓库；如需示例，请使用不可执行的文档片段。
- 不要在 import 外层添加 try/catch。

## 验证建议

- 文档变更至少运行 `git diff --check`。
- 如果新增代码，再补充对应语言生态的格式化、类型检查或测试命令。
