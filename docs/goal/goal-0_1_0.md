# Goal 0.1.0：本地 KISS CLI MVP

## 本轮目标

实现一个最小但可运行的 `kiss` CLI，验证 PRD 和技术架构里最核心的闭环：用户显式调用 `kiss run <skill> <args>` 时，只加载被点名的本地 skill，不枚举或暴露 KISS store 中的其他 skill。

## 范围

### 必须完成

- 提供可编译的 CLI 入口：`cmd/kiss`。
- 支持 `--kiss-home` 和 `KISS_HOME`，默认落到用户数据目录下的 KISS 专属目录。
- 支持本地 skill 安装：`kiss add <local-path> --name <name>`。
- 支持运行已安装 skill：`kiss run <name> [args...]`。
- 支持 `kiss list`、`kiss remove <name>`、`kiss doctor`。
- `kiss run` 输出 Markdown envelope，且只读取当前 skill 的 `SKILL.md` 和 `kiss.skill.toml`。
- 本地安装时拒绝 symlink，避免安装包逃逸。
- 为核心路径、安装、加载和运行行为补充自动化测试。

### 暂不完成

- 远程 GitHub/URL 下载。
- registry 和 `registry.lock`。
- command runner。
- `/kiss`、`$kiss` agent adapter。
- 预编译 release 产物和安装脚本。

## 与 PRD / 技术架构对比

| 条目 | 本轮状态 | 说明 |
| --- | --- | --- |
| FR-1 `kiss run <skill> <args>` | 完成 | 支持已安装本地 skill 的显式运行。 |
| FR-3 本地没有 skill 时下载并安装 | 未完成 | 本轮只支持本地路径安装，远程下载进入下一轮。 |
| FR-4 本地已有 skill 默认直接运行 | 完成 | `run` 只读本地 store，不访问网络。 |
| FR-5 只暴露当前 skill | 完成 | runner 只加载指定 skill 内容。 |
| FR-6 list/remove/doctor | 部分完成 | 本轮实现 list、remove、doctor；update 下一轮。 |
| NFR-1 默认安装形态不要求 Node/Bun/TS | 部分完成 | 使用 Go 实现可编译单二进制；本轮不发布 release 产物。 |

## 下一轮建议

Goal 0.2.0 应优先实现远程来源、sha256 安装记录和 `kiss update`，补齐 PRD 中“本地没有 skill 时可从配置来源下载并安装”的能力。
