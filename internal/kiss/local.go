package kiss

import (
	"fmt"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
)

// AddLocalSkill 从本地目录或 file:// URL 安装 skill。
func AddLocalSkill(paths Paths, sourcePath, name string) error {
	absSource, err := NormalizeLocalSourcePath(sourcePath)
	if err != nil {
		return err
	}
	source := SourceMetadata{Kind: "local", URI: absSource}
	return installSkillFromDir(paths, absSource, name, "local:"+absSource, source)
}

// InferLocalSkillName 读取本地 source manifest 并返回声明的 skill name。
func InferLocalSkillName(sourcePath string) (string, error) {
	absSource, err := NormalizeLocalSourcePath(sourcePath)
	if err != nil {
		return "", err
	}
	manifest, err := LoadManifest(absSource)
	if err != nil {
		return "", err
	}
	if err := ValidateSkillName(manifest.Name); err != nil {
		return "", err
	}
	return manifest.Name, nil
}

// NormalizeLocalSourcePath 把本地路径和支持的 file:// URL 转成绝对路径。
func NormalizeLocalSourcePath(sourcePath string) (string, error) {
	if strings.HasPrefix(strings.ToLower(sourcePath), "file://") {
		sourcePathFromURL, err := localPathFromFileURL(sourcePath)
		if err != nil {
			return "", err
		}
		sourcePath = sourcePathFromURL
	}
	return filepath.Abs(sourcePath)
}

func localPathFromFileURL(source string) (string, error) {
	parsed, err := url.Parse(source)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(parsed.Scheme, "file") {
		return "", fmt.Errorf("expected file:// URL")
	}
	if parsed.Opaque != "" {
		return "", fmt.Errorf("file URL must use file:///absolute/path form")
	}
	host := strings.ToLower(parsed.Host)
	if host != "" && host != "localhost" {
		return "", fmt.Errorf("file URL host %q is not supported; use a local file:/// path", parsed.Host)
	}
	path, err := url.PathUnescape(parsed.EscapedPath())
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", fmt.Errorf("file URL must include a path")
	}
	if runtime.GOOS == "windows" {
		if len(path) >= 3 && path[0] == '/' && path[2] == ':' {
			path = path[1:]
		}
		return filepath.FromSlash(path), nil
	}
	if !strings.HasPrefix(path, "/") {
		return "", fmt.Errorf("file URL must use an absolute path")
	}
	return path, nil
}
