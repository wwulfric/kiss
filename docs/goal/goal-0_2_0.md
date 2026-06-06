# Goal 0.2.0：显式远程安装与 sha256 元数据

## 本轮目标

在继续坚持“`kiss run` 不自动安装”的前提下，给 `kiss add` 增加显式远程安装能力，并把远程包的 sha256、ref、resolved、full_name 写入 `$KISS_HOME/skills.db.json`。

## 必须完成

- `kiss add https://...tar.gz --name <name>`：显式从 HTTPS tar.gz 安装 skill。
- `kiss add github:owner/repo/path/to/skill#ref --name <name>`：显式从 GitHub archive 安装仓库内某个 skill 子目录。
- 远程包下载到 `$KISS_HOME/cache/downloads`，并计算 sha256。
- 解包时拒绝绝对路径、`..` 逃逸和 symlink。
- metadata 写入：`source.kind`、`source.uri`、`source.ref`、`source.resolved`、`source.sha256`、`full_name`。
- `kiss run` 仍然只运行已安装 skill，不触发 remote resolver 或下载。
- 补充测试覆盖 HTTPS tar.gz 安装、sha256 落库、GitHub source 解析和 missing run 不安装。

## 暂不完成

- registry / `registry.lock`。
- `kiss update`。
- GitHub API commit resolution；本轮 `resolved` 先记录 ref。
- zip archive 支持。
- 签名校验。

## 验收标准

```bash
kiss add https://example.com/browser-test.tar.gz --name browser-test
kiss show browser-test
kiss run browser-test url=https://example.com
```

`kiss show` 中必须包含非空 `source.sha256`；如果是 GitHub 来源，`full_name` 应类似 `github:owner/repo/path/to/skill`。
