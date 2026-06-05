package kiss

import (
	"fmt"
	"path/filepath"
	"strings"
)

func validateSafeRelativePath(path string) error {
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
