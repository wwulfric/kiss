package kiss

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var skillNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// ValidateSkillName 校验用户可见的 skill name 是否适合用作 store 目录名和 CLI 参数。
func ValidateSkillName(name string) error {
	if !skillNamePattern.MatchString(name) {
		return fmt.Errorf("invalid skill name %q", name)
	}
	return nil
}

func validateSafeRelativePath(path string) error {
	// Manifest 和 archive 内部路径必须保持相对，避免写出 skill 目录边界。
	if path == "" {
		return fmt.Errorf("path must be a safe relative path")
	}
	if filepath.IsAbs(path) || strings.Contains(path, `\`) {
		return fmt.Errorf("path must be a safe relative path")
	}
	for _, segment := range strings.Split(path, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return fmt.Errorf("path must be a safe relative path")
		}
	}
	return nil
}

func validateOptionalSafeRelativePath(path string) error {
	if path == "" {
		return nil
	}
	return validateSafeRelativePath(path)
}
