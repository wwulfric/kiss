package kiss

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type updatePreview struct {
	Descriptor   SkillDescriptor
	EntryContent []byte
	SourceSHA256 string
	Source       SourceMetadata
	FullName     string
}

// UpdateOptions 控制显式 update 的来源 ref 和是否应用。
type UpdateOptions struct {
	Yes bool
	Ref string
}

// UpdateSkill 从已记录的 source 预览或执行一次显式更新。
func UpdateSkill(paths Paths, name string, yes bool, out io.Writer) error {
	return UpdateSkillWithOptions(paths, name, UpdateOptions{Yes: yes}, out)
}

// UpdateSkillWithOptions 从已记录的 source 预览或执行一次显式更新。
func UpdateSkillWithOptions(paths Paths, name string, options UpdateOptions, out io.Writer) error {
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
	sourceSpec, err := sourceSpecFromMetadataWithRef(metadata, options.Ref)
	if err != nil {
		return err
	}
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
	if !options.Yes {
		applyTarget := name
		if options.Ref != "" {
			applyTarget += "@" + options.Ref
		}
		return printUpdatePlan(out, name, applyTarget, metadata, current, preview, sourceSpec)
	}
	if current.Descriptor.Version == preview.Descriptor.Version {
		if err := updateInstalledMetadataFromPreview(paths, name, metadata, preview); err != nil {
			return err
		}
		if options.Ref != "" && preview.Source.Kind == "github" {
			_, err = fmt.Fprintf(out, "Updated %s source to %s; skill folder unchanged.\n", name, sourceSpec)
			return err
		}
		_, err = fmt.Fprintf(out, "%s is already up to date.\n", name)
		return err
	}
	if err := installSourceSpec(paths, sourceSpec, name); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Updated %s from %s\n", name, sourceSpec)
	return err
}

func previewInstalledSkill(paths Paths, name string) (updatePreview, error) {
	skillDir := paths.SkillDir(name)
	descriptor, err := LoadSkillDescriptor(skillDir)
	if err != nil {
		return updatePreview{}, err
	}
	content, err := os.ReadFile(filepath.Join(skillDir, descriptor.Entry))
	if err != nil {
		return updatePreview{}, err
	}
	return updatePreview{Descriptor: descriptor, EntryContent: content}, nil
}

func previewSourceSpec(paths Paths, sourceSpec, name string) (updatePreview, func(), error) {
	if strings.HasPrefix(sourceSpec, "github:") {
		return previewRemoteSource(paths, sourceSpec, name)
	}
	descriptor, err := LoadSkillDescriptor(sourceSpec)
	if err != nil {
		return updatePreview{}, func() {}, err
	}
	content, err := os.ReadFile(filepath.Join(sourceSpec, descriptor.Entry))
	if err != nil {
		return updatePreview{}, func() {}, err
	}
	source := SourceMetadata{Kind: "local", URI: sourceSpec}
	return updatePreview{Descriptor: descriptor, EntryContent: content, Source: source, FullName: "local:" + sourceSpec}, func() {}, nil
}

func previewRemoteSource(paths Paths, sourceSpec, name string) (updatePreview, func(), error) {
	resolved, downloadURL, subdir, err := ResolveRemoteSource(sourceSpec)
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
	descriptor, err := LoadSkillDescriptor(skillDir)
	if err != nil {
		cleanup()
		return updatePreview{}, func() {}, err
	}
	content, err := os.ReadFile(filepath.Join(skillDir, descriptor.Entry))
	if err != nil {
		cleanup()
		return updatePreview{}, func() {}, err
	}
	source := SourceMetadata{
		Kind:     resolved.Kind,
		URI:      resolved.URI,
		Ref:      resolved.Ref,
		Resolved: resolved.Resolved,
		SHA256:   sum,
	}
	return updatePreview{Descriptor: descriptor, EntryContent: content, SourceSHA256: sum, Source: source, FullName: resolved.FullName}, cleanup, nil
}

func printUpdatePlan(out io.Writer, name, applyTarget string, metadata SkillMetadata, current, targetPreview updatePreview, sourceSpec string) error {
	currentEntrySum := sha256Bytes(current.EntryContent)
	targetEntrySum := sha256Bytes(targetPreview.EntryContent)
	entryStatus := "unchanged"
	if currentEntrySum != targetEntrySum {
		entryStatus = "changed"
	}
	folderStatus := "unchanged"
	if current.Descriptor.Version != targetPreview.Descriptor.Version {
		folderStatus = "changed"
	}
	writer := errWriter{out: out}
	writer.printf("# KISS update plan: %s\n\n", name)
	writer.printf("- Current version: %s\n", current.Descriptor.Version)
	writer.printf("- Target version: %s\n", targetPreview.Descriptor.Version)
	writer.printf("- Skill folder: %s\n", folderStatus)
	writer.printf("- Current entry: %s\n", current.Descriptor.Entry)
	writer.printf("- Target entry: %s\n", targetPreview.Descriptor.Entry)
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
		diff := buildEntryDiff(current.Descriptor.Entry, targetPreview.Descriptor.Entry, current.EntryContent, targetPreview.EntryContent)
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
	writer.printf("Run `kiss update %s --yes` to apply.\n", applyTarget)
	return writer.err
}

func sha256Bytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func sourceSpecFromMetadata(metadata SkillMetadata) (string, error) {
	return sourceSpecFromMetadataWithRef(metadata, "")
}

func sourceSpecFromMetadataWithRef(metadata SkillMetadata, refOverride string) (string, error) {
	switch metadata.Source.Kind {
	case "local":
		if refOverride != "" {
			return "", fmt.Errorf("@ref can only be used with GitHub-backed skills")
		}
		return metadata.Source.URI, nil
	case "github":
		spec := "github:" + metadata.Source.URI
		ref := metadata.Source.Ref
		if refOverride != "" {
			ref = refOverride
		}
		if ref != "" {
			spec += "@" + ref
		}
		return spec, nil
	case "skills-import":
		return "", fmt.Errorf("skills-import snapshots cannot be updated; re-run kiss import to refresh %q", metadata.Name)
	default:
		return "", fmt.Errorf("unsupported source kind %q", metadata.Source.Kind)
	}
}

func installSourceSpec(paths Paths, sourceSpec, name string) error {
	if strings.HasPrefix(sourceSpec, "github:") {
		return AddRemoteSkill(paths, sourceSpec, name)
	}
	return AddLocalSkill(paths, sourceSpec, name)
}

func updateInstalledMetadataFromPreview(paths Paths, name string, existing SkillMetadata, preview updatePreview) error {
	if preview.Source.Kind == "" {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	metadata := existing
	metadata.Version = preview.Descriptor.Version
	metadata.Source = preview.Source
	if preview.FullName != "" {
		metadata.FullName = preview.FullName
	}
	metadata.UpdatedAt = now
	metadata.KissVersion = Version
	if err := UpsertSkillMetadata(paths, metadata); err != nil {
		return err
	}
	return writeInstallRecord(paths.SkillDir(name), InstallRecord{
		Name:            name,
		Source:          preview.Source.URI,
		Kind:            preview.Source.Kind,
		Ref:             preview.Source.Ref,
		Resolved:        preview.Source.Resolved,
		SHA256:          preview.Source.SHA256,
		SkillFolderHash: preview.Descriptor.Version,
		InstalledAt:     metadata.InstalledAt,
		KissVersion:     Version,
	})
}

func sourceSpecFromMetadataForDisplay(metadata SkillMetadata) string {
	sourceSpec, err := sourceSpecFromMetadata(metadata)
	if err != nil {
		return metadata.Source.Kind + ":" + metadata.Source.URI
	}
	return sourceSpec
}
