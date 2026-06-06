# Goal 0.1.2：明确 JSON 数据库与 skill 全称

## 本轮目标

回答并固化两个元数据设计问题：

1. MVP 是否使用 JSON 作为本地数据库存储。
2. skill 除了本地短名 `name`，是否需要类似 Vercel Skills 的来源限定全称。

本轮结论：MVP 继续使用 `$KISS_HOME/skills.db.json` 作为本地元数据数据库；每条 skill 元数据新增 `full_name`，表示来源限定的规范全称。

## 设计决策

### JSON 数据库

MVP 使用 JSON 文件作为数据库，而不是 SQLite：

- 优点：可读、可审计、易删除、无需额外运行时或 native 依赖。
- 适用范围：个人本地 skill store、低并发 CLI 写入、几十到几百条 skill 元数据。
- 迁移条件：需要并发写入锁、复杂查询、索引或跨进程事务时，再迁移 SQLite。

### skill 全称

- `name`：用户本地短名，用于 `kiss run <name>`，例如 `browser-test`。
- `full_name`：来源限定全称，用于审计、去重、迁移和未来 registry/update，例如：
  - `local:/Users/alice/.kiss-sources/browser-test`
  - `github:vercel-labs/agent-skills/skills/browser-test`
  - `https://example.com/skills/browser-test.tar.gz`

`skills.db.json` 仍以本地短名作为 map key，因为用户运行时需要稳定、短、可重命名的名字；`full_name` 保存来源身份。

## 必须完成

- `SkillMetadata` 新增 `full_name` 字段。
- 本地安装时生成 `local:<absolute-source-path>` 作为 `full_name`。
- `kiss list` 展示本地短名、版本、来源类型和全称。
- `kiss show` 输出 `full_name`。
- PRD 和技术架构补充 `name` / `full_name` 的区别。
- 测试覆盖 `full_name` 落库和查询输出。

## 暂不完成

- 远程 source parser。
- 同一 `full_name` 多本地 alias 的冲突策略。
- SQLite 迁移。
