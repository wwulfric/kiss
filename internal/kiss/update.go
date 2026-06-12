package kiss

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type updatePreview struct {
	Manifest     Manifest
	EntryContent []byte
	SourceSHA256 string
}

// UpdateSkill 从已记录的 source 预览或执行一次显式更新。
func UpdateSkill(paths Paths, name string, yes bool, out io.Writer) error {
	if err := ValidateSkillName(name); err != nil {
		return err
	}
	if err := paths.EnsureBase(); err != nil {
		return err
	}
	metadata, ok, err := GetSkillMetadata(paths, name)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("skill %q is not installed; run kiss add <source> --name %s first", name, name)
	}
	sourceSpec, err := sourceSpecFromMetadata(metadata)
	if err != nil {
		return err
	}
	if !yes {
		// 默认只打印计划，避免 update 在用户确认前改变已安装 skill。
		current, err := previewInstalledSkill(paths, name)
		if err != nil {
			return err
		}
		preview, cleanup, err := previewSourceSpec(paths, sourceSpec, name)
		if err != nil {
			return err
		}
		defer cleanup()
		return printUpdatePlan(out, name, metadata, current, preview, sourceSpec)
	}
	if err := installSourceSpec(paths, sourceSpec, name); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Updated %s from %s\n", name, sourceSpec)
	return err
}

func previewInstalledSkill(paths Paths, name string) (updatePreview, error) {
	skillDir := paths.SkillDir(name)
	manifest, err := LoadManifest(skillDir)
	if err != nil {
		return updatePreview{}, err
	}
	content, err := os.ReadFile(filepath.Join(skillDir, manifest.Entry))
	if err != nil {
		return updatePreview{}, err
	}
	return updatePreview{Manifest: manifest, EntryContent: content}, nil
}

func previewSourceSpec(paths Paths, sourceSpec, name string) (updatePreview, func(), error) {
	if strings.HasPrefix(sourceSpec, "https://") || strings.HasPrefix(sourceSpec, "github:") {
		return previewRemoteSource(paths, sourceSpec, name)
	}
	manifest, err := LoadManifest(sourceSpec)
	if err != nil {
		return updatePreview{}, func() {}, err
	}
	content, err := os.ReadFile(filepath.Join(sourceSpec, manifest.Entry))
	if err != nil {
		return updatePreview{}, func() {}, err
	}
	return updatePreview{Manifest: manifest, EntryContent: content}, func() {}, nil
}

func previewRemoteSource(paths Paths, sourceSpec, name string) (updatePreview, func(), error) {
	_, downloadURL, subdir, err := ResolveRemoteSource(sourceSpec)
	if err != nil {
		return updatePreview{}, func() {}, err
	}
	if err := paths.EnsureBase(); err != nil {
		return updatePreview{}, func() {}, err
	}
	archivePath, sum, err := downloadArchive(paths, downloadURL, name)
	if err != nil {
		return updatePreview{}, func() {}, err
	}
	tmpParent := filepath.Join(paths.Home, ".tmp")
	if err := os.MkdirAll(tmpParent, 0o755); err != nil {
		return updatePreview{}, func() {}, err
	}
	extractDir, err := os.MkdirTemp(tmpParent, "update-")
	if err != nil {
		return updatePreview{}, func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(extractDir) }
	if err := extractTarGz(archivePath, extractDir); err != nil {
		cleanup()
		return updatePreview{}, func() {}, err
	}
	var skillDir string
	if subdir != "" {
		found, err := findExtractedSubdir(extractDir, subdir)
		if err != nil {
			cleanup()
			return updatePreview{}, func() {}, err
		}
		skillDir = found
	} else {
		found, err := findArchiveSkillRoot(extractDir)
		if err != nil {
			cleanup()
			return updatePreview{}, func() {}, err
		}
		skillDir = found
	}
	manifest, err := LoadManifest(skillDir)
	if err != nil {
		cleanup()
		return updatePreview{}, func() {}, err
	}
	content, err := os.ReadFile(filepath.Join(skillDir, manifest.Entry))
	if err != nil {
		cleanup()
		return updatePreview{}, func() {}, err
	}
	return updatePreview{Manifest: manifest, EntryContent: content, SourceSHA256: sum}, cleanup, nil
}

func printUpdatePlan(out io.Writer, name string, metadata SkillMetadata, current, targetPreview updatePreview, sourceSpec string) error {
	currentEntrySum := sha256Bytes(current.EntryContent)
	targetEntrySum := sha256Bytes(targetPreview.EntryContent)
	entryStatus := "unchanged"
	if currentEntrySum != targetEntrySum {
		entryStatus = "changed"
	}
	writer := errWriter{out: out}
	writer.printf("# KISS update plan: %s\n\n", name)
	writer.printf("- Current version: %s\n", current.Manifest.Version)
	writer.printf("- Target version: %s\n", targetPreview.Manifest.Version)
	writer.printf("- Current runner: %s\n", current.Manifest.RunnerType)
	writer.printf("- Target runner: %s\n", targetPreview.Manifest.RunnerType)
	writer.printf("- Current entry: %s\n", current.Manifest.Entry)
	writer.printf("- Target entry: %s\n", targetPreview.Manifest.Entry)
	writer.printf("- Entry content: %s\n", entryStatus)
	writer.printf("- Current entry sha256: %s\n", currentEntrySum)
	writer.printf("- Target entry sha256: %s\n", targetEntrySum)
	writer.printf("- Current source: %s\n", sourceSpecFromMetadataForDisplay(metadata))
	writer.printf("- Target source: %s\n", sourceSpec)
	if targetPreview.SourceSHA256 != "" {
		writer.printf("- Target sha256: %s\n", targetPreview.SourceSHA256)
	}
	writer.println()
	writer.println("## Entry diff")
	writer.println()
	if entryStatus == "unchanged" {
		writer.println("Entry content unchanged; no diff.")
	} else {
		diff := buildEntryDiff(current.Manifest.Entry, targetPreview.Manifest.Entry, current.EntryContent, targetPreview.EntryContent)
		if diff.Note != "" {
			writer.println(diff.Note)
		}
		if diff.Text != "" {
			fence := markdownFenceFor(diff.Text)
			writer.printf("%sdiff\n%s\n%s\n", fence, diff.Text, fence)
			if diff.Truncated {
				writer.printf("\nDiff truncated to %d lines.\n", updateDiffMaxLines)
			}
		}
	}
	writer.println()
	writer.printf("Run `kiss update %s --yes` to apply.\n", name)
	return writer.err
}

func sha256Bytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func sourceSpecFromMetadata(metadata SkillMetadata) (string, error) {
	switch metadata.Source.Kind {
	case "local":
		return metadata.Source.URI, nil
	case "https":
		return metadata.Source.URI, nil
	case "github":
		spec := "github:" + metadata.Source.URI
		if metadata.Source.Ref != "" {
			spec += "#" + metadata.Source.Ref
		}
		return spec, nil
	default:
		return "", fmt.Errorf("unsupported source kind %q", metadata.Source.Kind)
	}
}

func installSourceSpec(paths Paths, sourceSpec, name string) error {
	if strings.HasPrefix(sourceSpec, "https://") || strings.HasPrefix(sourceSpec, "github:") {
		return AddRemoteSkill(paths, sourceSpec, name)
	}
	return AddLocalSkill(paths, sourceSpec, name)
}

func sourceSpecFromMetadataForDisplay(metadata SkillMetadata) string {
	sourceSpec, err := sourceSpecFromMetadata(metadata)
	if err != nil {
		return metadata.Source.Kind + ":" + metadata.Source.URI
	}
	return sourceSpec
}
