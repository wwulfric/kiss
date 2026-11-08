package kiss

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// AddLocalSkill 从本地目录或 file:// URL 安装 skill。
func AddLocalSkill(paths Paths, sourcePath, name string) error {
	return AddLocalSkillWithOptions(paths, sourcePath, name, AddOptions{})
}

// AddLocalSkillWithOptions 从本地 source 安装 skill；skill 选择和替换规则见 AddSkills。
func AddLocalSkillWithOptions(paths Paths, sourcePath, name string, options AddOptions) error {
	_, err := AddSkills(paths, sourcePath, name, options)
	return err
}

// openLocalSkillSource 把本地目录或 file:// URL 作为可供发现的 source。
func openLocalSkillSource(sourcePath string) (*skillSource, error) {
	absSource, err := NormalizeLocalSourcePath(sourcePath)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(absSource)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("source must be a directory")
	}
	return &skillSource{
		root:    absSource,
		cleanup: func() {},
		check:   func(discoveredSkill) error { return nil },
		metadata: func(skill discoveredSkill) (SourceMetadata, string) {
			return SourceMetadata{Kind: "local", URI: skill.Dir}, "local:" + skill.Dir
		},
	}, nil
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
