# AGENTS.md

本仓库用于设计和实现 KISS（Keep It Stupid Simple）skill store。

## 项目文档

- 产品定位、目标、非目标和用户场景见 `docs/kiss-skill-store-prd.md`。
- 架构边界、组件职责和关键约束见 `docs/kiss-skill-store-architecture.md`。
- 具体安装指导见 `docs/agent-installation-guide.md`。

## 工作约定

- 文档与注释优先使用中文；命令、配置键和协议字段保持英文。
- 新增产品决策时，优先更新 `docs/kiss-skill-store-prd.md`；新增技术架构决策时，优先更新 `docs/kiss-skill-store-architecture.md`。
- 不要在 `AGENTS.md` 重复产品目标或架构设计；需要时引用上述项目文档。
- 不要把示例 skill、缓存 skill 或下载后的第三方 skill 提交到本仓库；如需示例，请使用不可执行的文档片段。
- 不要在 import 外层添加 try/catch。

## 代码地图

```text
CLI 入口
  cmd/kiss/main.go
    全局参数解析、子命令分发，以及 kiss add 的 source 解析。
  cmd/kiss/main_test.go
    CLI 层参数解析和 runAdd 行为测试。

核心包 internal/kiss
  基础模型与存储
    types.go
      SkillDescriptor、InstallRecord、Version 等核心类型和版本变量。
    paths.go
      KISS_HOME、store/cache/logs 路径解析和基础目录初始化。
    metadata.go
      skills.db.json 的读写、查询、更新和 schema version 校验。
    skill_descriptor.go
      只加载根目录 SKILL.md；不读取 kiss.skill.toml。
    folder_hash.go
      计算 skillFolderHash，并作为 KISS 的 skill 版本信息。
    validation.go
      skill name 校验和安全相对路径校验。
    write.go
      内部 writer error 聚合 helper，用于多行 CLI 输出。
    setup.go
      `kiss setup` 的 agent skill 安装逻辑；当前支持用户目录下的默认 .agents 和 Claude .claude 目录。

  安装与 source 处理（add/update 共用）
    local.go
      本地目录和 file:// source 解析，并调用通用安装核心。
    remote.go
      GitHub source 解析、ref 到 commit 解析、下载、sha256 记录和安全解包。
    import_skills.go
      从 Vercel Skills CLI 全局 lock 和 canonical copy 导入当前快照；优先还原真实来源，失败时保留导入快照来源。
    install.go
      已解析 skill 目录的通用安装核心、安全复制、.kiss-install.json 写入和元数据生成。
    source_identity.go
      add/update/import 共用的来源身份比较；同源刷新不需要 --force，异源替换需要 --force。

  运行
    runner.go
      kiss run 的 Markdown envelope 输出；只加载被点名 skill，不执行脚本、不调用模型。

  管理命令
    management.go
      list、show、remove、doctor 命令实现。

  更新与 diff
    update.go
      基于已安装 source 的显式 update、预览和应用。
    diff.go
      update 预览使用的受限文本 diff。
    upgrade.go
      KISS CLI 自升级：下载 release artifact、校验 checksum，并替换当前二进制。

  测试
    install_test.go
    remote_test.go
    import_skills_test.go
    update_test.go
    skill_descriptor_test.go
    folder_hash_test.go
    setup_test.go
    upgrade_test.go
    runner_test.go
    diff_test.go
    management_test.go
    test_helpers_test.go
      核心包行为测试，按功能拆分；保留在同一 package 以测试 unexported helper。

KISS bridge skill
  skills/kiss/SKILL.md
    提供给 Vercel Skills / skills.sh 兼容工具安装的 KISS bridge skill。

产品与安装文档
  docs/kiss-skill-store-prd.md
    KISS 的产品定位、目标、非目标、用户故事和路线图。
  docs/kiss-skill-store-architecture.md
    KISS 的系统边界、组件职责和关键架构约束。
  docs/agent-installation-guide.md
    给 Codex、Claude Code 等 agent 使用的 KISS CLI 与 bridge skill 安装指导。

项目私有 agent skills
  .agents/skills/release/SKILL.md
    仅用于本项目发布：验证、打 tag、触发手动 GitHub Release workflow、校验产物。

安装与发布脚本
  scripts/install.sh
    macOS/Linux 安装脚本；会比较本机版本和目标版本，相同或本机更新时默认跳过。
  scripts/install.ps1
    Windows 安装脚本；会比较本机版本和目标版本，相同或本机更新时默认跳过。
  scripts/build-release.sh
    构建平台 tarball 和 checksums。
  scripts/sync-kiss-skill.sh
    把公开 KISS bridge skill 同步到 Go 内置 setup asset；release 前用 --check 校验。
  scripts/generate-homebrew-formula.sh
    生成 Homebrew formula。
  .github/workflows/release.yml
    手动 release workflow，生成平台产物、checksums、Homebrew formula 和 cosign 签名产物。

工程配置
  .golangci.yml
    项目级 Go lint 配置，GoLand 和 CLI 应共用它。
```

## 验证建议

- 文档变更至少运行 `git diff --check`。
- 如果新增 Go 代码，至少运行 `gofmt`、`go test ./...`、`go vet ./...` 和 `golangci-lint run`。
