package kiss

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

func LoadManifest(skillDir string) (Manifest, error) {
	manifest := Manifest{Entry: "SKILL.md", RunnerType: "markdown"}
	path := filepath.Join(skillDir, "kiss.skill.toml")
	if _, err := os.Stat(path); err != nil {
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
	var raw struct {
		Name        string `toml:"name"`
		Version     string `toml:"version"`
		Description string `toml:"description"`
		Entry       string `toml:"entry"`
		Runner      struct {
			Type string `toml:"type"`
		} `toml:"runner"`
	}
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		return Manifest{}, err
	}
	if raw.Name != "" {
		manifest.Name = raw.Name
	}
	if raw.Version != "" {
		manifest.Version = raw.Version
	}
	if raw.Description != "" {
		manifest.Description = raw.Description
	}
	if raw.Entry != "" {
		manifest.Entry = raw.Entry
	}
	if raw.Runner.Type != "" {
		manifest.RunnerType = raw.Runner.Type
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
	if err := validateSafeRelativePath(manifest.Entry); err != nil {
		return Manifest{}, fmt.Errorf("entry %q must be a safe relative path: %w", manifest.Entry, err)
	}
	if _, err := os.Stat(filepath.Join(skillDir, manifest.Entry)); err != nil {
		return Manifest{}, fmt.Errorf("entry %q not found: %w", manifest.Entry, err)
	}
	return manifest, nil
}
