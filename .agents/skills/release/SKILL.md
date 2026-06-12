---
name: release
description: 仅用于 kiss 项目的发布流程：自动计算版本号、验证、打 tag、触发手动 GitHub Release workflow、校验产物。
---

# KISS Release Skill

本 skill 只用于 KISS 项目。不要用于其他项目。即使用户说“发布”或“release”，也必须先确认当前工作目录是 KISS 仓库。

识别方式：

- 当前目录或其上级目录包含 `go.mod`，且 module 是 `github.com/wwulfric/kiss`。
- 当前目录或其上级目录包含 `.github/workflows/release.yml` 和 `scripts/build-release.sh`。

## 发布约束

- 用户可以给出明确版本 tag，例如 `v0.1.0`，也可以给出 `patch`、`minor`、`major`。
- 用户没有给任何版本参数时，默认按 `patch` 自动计算下一个版本。
- 自动计算出的版本必须回报给用户；触发 GitHub Release workflow 前仍需用户确认发布意图。
- 本地验证通过前，不创建 tag，不 push tag。
- 工作区不干净时，不发布；除非用户明确要求先整理、提交这些变更。
- 本地或远端 tag 已存在时，不 force-update、不删除 tag；除非用户明确批准。
- GitHub Release workflow 是手动触发：只有用户确认版本和发布意图后，才触发 `.github/workflows/release.yml`。
- 本机不做 Linux/Windows 等跨平台 release 构建；完整 release artifacts 由 GitHub Workflow 生成。

## 版本计算

把用户输入解释为 `release_input`：

- 空输入：`patch`
- `patch`：基于最新语义化 tag 增加 patch
- `minor`：增加 minor，patch 归零
- `major`：增加 major，minor 和 patch 归零
- `vX.Y.Z` 或 `X.Y.Z`：使用该明确版本；没有 `v` 前缀时补成 `vX.Y.Z`

使用以下逻辑计算 `<version>`：

```bash
git fetch origin --tags

release_input="${RELEASE_INPUT:-patch}"

case "$release_input" in
  "" | patch | minor | major)
    bump="${release_input:-patch}"
    latest=$(git tag --list 'v[0-9]*.[0-9]*.[0-9]*' --sort=-v:refname | head -n 1)
    if [ -z "$latest" ]; then
      latest="v0.0.0"
    fi
    version_parts=${latest#v}
    IFS=. read -r major minor patch <<EOF
$version_parts
EOF
    case "$bump" in
      patch) patch=$((patch + 1)) ;;
      minor) minor=$((minor + 1)); patch=0 ;;
      major) major=$((major + 1)); minor=0; patch=0 ;;
    esac
    version="v${major}.${minor}.${patch}"
    ;;
  v[0-9]*.[0-9]*.[0-9]*)
    version="$release_input"
    ;;
  [0-9]*.[0-9]*.[0-9]*)
    version="v$release_input"
    ;;
  *)
    echo "release input must be empty, patch, minor, major, vX.Y.Z, or X.Y.Z" >&2
    exit 1
    ;;
esac

case "$version" in
  v[0-9]*.[0-9]*.[0-9]*) ;;
  *) echo "computed invalid release version: $version" >&2; exit 1 ;;
esac

printf '%s\n' "$version"
```

计算出 `<version>` 后，向用户说明来源，例如：

```text
最新 tag 是 v0.2.3，用户未指定版本，默认 patch，所以本次计划发布 v0.2.4。
```

## 本地验证

在当前项目下运行：

```bash
GO=${GO:-go}
if ! command -v "$GO" >/dev/null 2>&1; then
  echo "go is required; install Go or set GO to a Go binary" >&2
  exit 1
fi

"$GO" fmt ./...
"$GO" test ./...
"$GO" vet ./...
golangci-lint run
git diff --check
git status --short
sh -n scripts/install.sh
```

如果本机有 `pwsh`，也检查 Windows 安装脚本语法：

```bash
pwsh -NoProfile -Command '$ErrorActionPreference="Stop"; [scriptblock]::Create((Get-Content -Raw scripts/install.ps1)) > $null'
```

发布前只在本机构建当前平台 binary，验证版本注入是否正常：

```bash
go build -trimpath -ldflags="-s -w -X github.com/wwulfric/kiss/internal/kiss.Version=<version>" -o /tmp/kiss-release-check ./cmd/kiss
/tmp/kiss-release-check --version
```

输出版本应等于 `<version>`。

不要在本机运行 `scripts/build-release.sh` 作为发布前置检查；Linux、Windows、macOS release artifacts、`checksums.txt`、Homebrew formula 和 cosign 签名都交给 GitHub Workflow 生成。

如果确实需要调试本地 release 脚本，可以手动运行：

```bash
KISS_VERSION=<version> scripts/build-release.sh
```

此命令会生成 `dist/`，只作为调试产物，默认不要提交。

## 准备 Tag

打 tag 前运行：

```bash
git fetch origin --tags
git status --short
git branch --show-current
git rev-parse HEAD
git rev-parse -q --verify "refs/tags/<version>" || true
git ls-remote --tags origin "refs/tags/<version>" || true
```

期望状态：

- 当前分支是目标发布分支，通常是 `master`。
- 工作区干净；如果不干净，先让用户确认是否需要提交。
- local/remote tag 查询都没有输出 `<version>`。
- 要打 tag 的 commit 已经 push 到 `origin`。

创建并推送 annotated tag：

```bash
git tag -a "<version>" -m "Release <version>"
git push origin "<version>"
```

## 发布

在 tag ref 上触发手动 GitHub Actions release workflow：

```bash
gh workflow run release.yml --ref "<version>" -f version="<version>"
```

查找并等待 workflow：

```bash
gh run list --workflow release.yml --limit 10
gh run watch <run-id> --exit-status
```

如果 `gh` 不可用或未登录，要求用户在 GitHub Actions 页面手动触发 `Release` workflow，ref 选择 `<version>`，input 填 `version=<version>`。

## 发布后验证

workflow 成功后，从 GitHub Release 下载 workflow 生成的多平台产物并校验：

```bash
gh release view "<version>"
tmp_dir=$(mktemp -d)
gh release download "<version>" --pattern checksums.txt --pattern "*.tar.gz" -D "$tmp_dir"
(cd "$tmp_dir" && shasum -a 256 -c checksums.txt)
```

可选：用安装脚本在隔离目录验证新版本：

```bash
tmp_bin=$(mktemp -d)
KISS_REPO=wwulfric/kiss KISS_VERSION=<version> KISS_INSTALL_DIR="$tmp_bin" bash scripts/install.sh
"$tmp_bin/kiss" --version
```

输出版本应等于 `<version>`。

## 回报格式

完成后向用户说明：

- 发布版本和 tag 对应 commit SHA。
- 运行过哪些验证命令，是否通过。
- GitHub Actions run URL 或 run ID。
- GitHub Release URL。
- 有哪些检查被跳过，以及跳过原因。
