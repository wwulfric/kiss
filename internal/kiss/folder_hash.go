package kiss

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const installRecordFileName = ".kiss-install.json"

type skillFolderHashFile struct {
	relativePath string
	content      []byte
}

// computeSkillFolderHash 计算 skill 目录的内容 hash，用作 KISS 的 skill 版本信息。
func computeSkillFolderHash(skillDir string) (string, error) {
	var files []skillFolderHashFile
	if err := collectSkillFolderHashFiles(skillDir, skillDir, &files); err != nil {
		return "", err
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].relativePath < files[j].relativePath
	})

	hash := sha256.New()
	for _, file := range files {
		writeSkillFolderHashField(hash, []byte("file"))
		writeSkillFolderHashField(hash, []byte(file.relativePath))
		writeSkillFolderHashField(hash, file.content)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func collectSkillFolderHashFiles(baseDir, currentDir string, files *[]skillFolderHashFile) error {
	entries, err := os.ReadDir(currentDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		fullPath := filepath.Join(currentDir, entry.Name())
		relativePath, err := filepath.Rel(baseDir, fullPath)
		if err != nil {
			return err
		}
		relativePath = filepath.ToSlash(relativePath)
		if shouldSkipSkillPackageEntry(relativePath, entry.IsDir()) {
			continue
		}
		if entry.IsDir() {
			if err := collectSkillFolderHashFiles(baseDir, fullPath, files); err != nil {
				return err
			}
			continue
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported file type in skill package: %s", fullPath)
		}
		content, err := os.ReadFile(fullPath)
		if err != nil {
			return err
		}
		*files = append(*files, skillFolderHashFile{
			relativePath: strings.TrimPrefix(relativePath, "./"),
			content:      content,
		})
	}
	return nil
}

func writeSkillFolderHashField(hash hash.Hash, data []byte) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(data)))
	_, _ = hash.Write(length[:])
	_, _ = hash.Write(data)
}

func shouldSkipSkillPackageEntry(relativePath string, isDir bool) bool {
	cleanPath := filepath.ToSlash(filepath.Clean(relativePath))
	if cleanPath == "." {
		return false
	}
	if !isDir {
		return cleanPath == installRecordFileName
	}
	return cleanPath == ".git" ||
		cleanPath == "node_modules" ||
		strings.HasSuffix(cleanPath, "/.git") ||
		strings.HasSuffix(cleanPath, "/node_modules")
}
