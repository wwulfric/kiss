# Goal 0.1.3：数据库存储形态决策

## 本轮目标

回答“是否可以使用 DuckDB 配合 Parquet 类型文件作为 KISS 本地数据库”的问题，并把决策写入技术架构。

## 结论

MVP **不采用 DuckDB + Parquet 作为核心元数据数据库**，继续使用 `$KISS_HOME/skills.db.json`。

原因：

- KISS 当前元数据规模很小，主要是按本地短名 `name` 做 `get/list/show/remove`，JSON 足够。
- DuckDB 会引入额外 native 依赖、平台分发复杂度和文件锁/并发语义，需要违背“单文件 CLI、低依赖、可删除”的优先级。
- Parquet 更适合列式分析和批量扫描，不适合本轮这种小规模 key-value 元数据 CRUD。
- JSON 文件人工可读，便于用户审计和手动恢复。

## 允许的未来方向

DuckDB + Parquet 可以作为**可选分析/导出层**，而不是核心运行数据库。例如：

- `kiss export metadata --format parquet`
- `kiss analytics` 读取历史日志、安装记录和 registry 统计
- 团队级 registry 后台服务使用 DuckDB/Parquet 做离线分析

## 决策规则

- `kiss run`、`kiss list`、`kiss show` 的核心路径不得依赖 DuckDB。
- 如果未来引入 DuckDB，必须是 optional build/tag 或独立子命令，不影响默认单文件 CLI 的基础能力。
- 只有当本地 skill 数量、查询复杂度、并发写入需求明显超过 JSON 能力时，才重新评估 SQLite 或 DuckDB。
