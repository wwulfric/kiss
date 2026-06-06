# Goal 0.1.1：显式安装与本地元数据数据库

## 本轮目标

调整 KISS 的安装/运行边界：`kiss run <skill>` 不做自动安装，也不触发网络解析；用户必须先通过 `kiss add ...` 显式安装 skill，之后才能运行。同时新增本地元数据数据库，保存已安装 skill 的查询信息，避免每次查询都扫描和解析 skill 包内容。

## 范围

### 必须完成

- 更新 PRD：把“run 时本地没有就下载”改成“必须先显式 add/install”。
- 更新技术架构：定义本地元数据数据库文件和数据结构。
- 新增本地数据库文件：`$KISS_HOME/skills.db.json`。
- `kiss add <local-path> --name <name>` 安装成功后写入/更新数据库记录。
- `kiss list` 从数据库读取名称、版本和来源，不再通过扫描 skill 目录作为主路径。
- 新增 `kiss show <name>`，输出单个 skill 的元数据。
- `kiss remove <name>` 删除 skill 目录后同步删除数据库记录。
- `kiss doctor` 确保数据库存在。
- 补充自动化测试覆盖 add/list/show/remove 与数据库落盘。

### 暂不完成

- 远程 GitHub/URL 下载。
- registry 和 `registry.lock`。
- 自动修复数据库与磁盘目录不一致。
- command runner。
- `/kiss`、`$kiss` agent adapter。

## 元数据数据库结构

MVP 使用一个可读可删的 JSON 文件：

```text
$KISS_HOME/skills.db.json
```

顶层结构：

```json
{
  "schema_version": 1,
  "skills": {
    "browser-test": {
      "name": "browser-test",
      "manifest_name": "browser-test",
      "version": "0.1.0",
      "description": "Run focused browser test workflow.",
      "source": {
        "kind": "local",
        "uri": "/abs/path/to/source",
        "ref": "",
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
      "kiss_version": "0.1.1"
    }
  }
}
```

## 与 PRD / 技术架构对比

| 条目 | 本轮状态 | 说明 |
| --- | --- | --- |
| 显式安装后才能使用 | 完成 | `run` 仍然只读本地 store；文档明确不自动安装。 |
| 本地元数据查询 | 完成 | 新增 `skills.db.json`，`list/show` 使用数据库。 |
| FR-3 远程下载 | 未完成 | 本轮只明确远程下载属于显式 `add` 的未来能力。 |
| FR-7 查看来源/版本/安装时间 | 部分完成 | `show` 可查看 local source、version、installed_at；sha256 待远程/内容哈希迭代。 |

## 下一轮建议

Goal 0.2.0 再做显式远程安装：`kiss add github:... --name ...` / `kiss add https://... --name ...`，并把 sha256、ref、resolved 写入同一个 `skills.db.json`。
