package kiss

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func LoadManifest(skillDir string) (Manifest, error) {
	manifest := Manifest{Entry: "SKILL.md", RunnerType: "markdown"}
	path := filepath.Join(skillDir, "kiss.skill.toml")
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			if _, statErr := os.Stat(filepath.Join(skillDir, "SKILL.md")); statErr != nil {
				return Manifest{}, fmt.Errorf("skill must contain SKILL.md: %w", statErr)
			}
			manifest.Name = filepath.Base(skillDir)
			manifest.Version = "0.0.0"
			return manifest, nil
		}
		return Manifest{}, err
	}
	defer file.Close()

	section := ""
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = parseTomlScalar(value)
		switch {
		case section == "" && key == "name":
			manifest.Name = value
		case section == "" && key == "version":
			manifest.Version = value
		case section == "" && key == "description":
			manifest.Description = value
		case section == "" && key == "entry":
			manifest.Entry = value
		case section == "runner" && key == "type":
			manifest.RunnerType = value
		}
	}
	if err := scanner.Err(); err != nil {
		return Manifest{}, err
	}
	if manifest.Name == "" {
		manifest.Name = filepath.Base(skillDir)
	}
	if manifest.Version == "" {
		manifest.Version = "0.0.0"
	}
	if manifest.Entry == "" {
		manifest.Entry = "SKILL.md"
	}
	if manifest.RunnerType == "" {
		manifest.RunnerType = "markdown"
	}
	if manifest.RunnerType != "markdown" {
		return Manifest{}, fmt.Errorf("runner type %q is not supported in this iteration", manifest.RunnerType)
	}
	if filepath.IsAbs(manifest.Entry) || strings.Contains(manifest.Entry, `\`) || strings.HasPrefix(manifest.Entry, "/") {
		return Manifest{}, fmt.Errorf("entry must be a safe relative path")
	}
	for _, segment := range strings.Split(manifest.Entry, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return Manifest{}, fmt.Errorf("entry must be a safe relative path")
		}
	}
	if _, err := os.Stat(filepath.Join(skillDir, manifest.Entry)); err != nil {
		return Manifest{}, fmt.Errorf("entry %q not found: %w", manifest.Entry, err)
	}
	return manifest, nil
}

func parseTomlScalar(value string) string {
	value = strings.TrimSpace(value)
	if hash := strings.Index(value, " #"); hash >= 0 {
		value = strings.TrimSpace(value[:hash])
	}
	value = strings.Trim(value, "\"")
	return value
}
