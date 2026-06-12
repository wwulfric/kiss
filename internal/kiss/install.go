package kiss

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func installSkillFromDir(paths Paths, sourcePath, name, fullName string, source SourceMetadata) error {
	if err := ValidateSkillName(name); err != nil {
		return err
	}
	absSource, err := filepath.Abs(sourcePath)
	if err != nil {
		return err
	}
	info, err := os.Stat(absSource)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("source must be a directory")
	}
	if err := paths.EnsureBase(); err != nil {
		return err
	}
	manifest, err := LoadManifest(absSource)
	if err != nil {
		return err
	}

	tmpParent := filepath.Join(paths.Home, ".tmp")
	if err := os.MkdirAll(tmpParent, 0o755); err != nil {
		return err
	}
	tmpDir, err := os.MkdirTemp(tmpParent, name+"-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	if err := copySkillDir(absSource, tmpDir); err != nil {
		return err
	}
	installedAt := time.Now().UTC().Format(time.RFC3339)
	record := InstallRecord{
		Name:        name,
		Source:      source.URI,
		Kind:        source.Kind,
		Ref:         source.Ref,
		Resolved:    source.Resolved,
		SHA256:      source.SHA256,
		InstalledAt: installedAt,
		KissVersion: Version,
	}
	if err := writeInstallRecord(tmpDir, record); err != nil {
		return err
	}

	// 先保留旧目录，等新目录和 metadata 都成功写入后再清理，失败时尽量回滚。
	dest := paths.SkillDir(name)
	backup := dest + ".old"
	_ = os.RemoveAll(backup)
	if _, err := os.Stat(dest); err == nil {
		if err := os.Rename(dest, backup); err != nil {
			return err
		}
	}
	if err := os.Rename(tmpDir, dest); err != nil {
		if _, statErr := os.Stat(backup); statErr == nil {
			_ = os.Rename(backup, dest)
		}
		return err
	}
	metadata := SkillMetadata{
		Name:         name,
		FullName:     fullName,
		ManifestName: manifest.Name,
		Version:      manifest.Version,
		Description:  manifest.Description,
		Source:       source,
		Runner: RunnerMetadata{
			Type:  manifest.RunnerType,
			Entry: manifest.Entry,
		},
		InstalledPath: dest,
		InstalledAt:   installedAt,
		UpdatedAt:     installedAt,
		KissVersion:   Version,
	}
	if err := UpsertSkillMetadata(paths, metadata); err != nil {
		_ = os.RemoveAll(dest)
		if _, statErr := os.Stat(backup); statErr == nil {
			_ = os.Rename(backup, dest)
		}
		return err
	}
	_ = os.RemoveAll(backup)
	return nil
}

func copySkillDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		// 不跟随 symlink，避免 skill 包把内容指向 store 外部路径。
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink is not allowed in skill package: %s", path)
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		clean := filepath.Clean(rel)
		if clean == "." {
			return nil
		}
		if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || filepath.IsAbs(clean) {
			return fmt.Errorf("unsafe path in skill package: %s", rel)
		}
		target := filepath.Join(dst, clean)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported file type in skill package: %s", path)
		}
		return copyFile(path, target, info.Mode().Perm())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func writeInstallRecord(skillDir string, record InstallRecord) error {
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(skillDir, ".kiss-install.json"), data, 0o644)
}
