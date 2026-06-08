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
	target, hasRegistryTarget, err := registryUpdateTarget(paths, name)
	if err != nil {
		return err
	}
	sourceSpec := ""
	expectedSHA256 := ""
	if hasRegistryTarget {
		sourceSpec = target.SourceSpec
		expectedSHA256 = target.SHA256
	} else {
		sourceSpec, err = sourceSpecFromMetadata(metadata)
		if err != nil {
			return err
		}
	}
	if !yes {
		current, err := previewInstalledSkill(paths, name)
		if err != nil {
			return err
		}
		preview, cleanup, err := previewSourceSpec(paths, sourceSpec, name, expectedSHA256)
		if err != nil {
			return err
		}
		defer cleanup()
		printUpdatePlan(out, name, metadata, current, preview, sourceSpec, expectedSHA256, hasRegistryTarget, target)
		return nil
	}
	if err := installSourceSpec(paths, sourceSpec, name, expectedSHA256); err != nil {
		return err
	}
	if hasRegistryTarget {
		if err := UpsertRegistryLockEntry(paths, target); err != nil {
			return err
		}
	}
	fmt.Fprintf(out, "Updated %s from %s\n", name, sourceSpec)
	return nil
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

func previewSourceSpec(paths Paths, sourceSpec, name, expectedSHA256 string) (updatePreview, func(), error) {
	if strings.HasPrefix(sourceSpec, "https://") || strings.HasPrefix(sourceSpec, "github:") {
		return previewRemoteSource(paths, sourceSpec, name, expectedSHA256)
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

func previewRemoteSource(paths Paths, sourceSpec, name, expectedSHA256 string) (updatePreview, func(), error) {
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
	if expectedSHA256 != "" && !strings.EqualFold(sum, expectedSHA256) {
		_ = os.Remove(archivePath)
		return updatePreview{}, func() {}, fmt.Errorf("sha256 mismatch for %s: expected %s, got %s", sourceSpec, expectedSHA256, sum)
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
	skillDir := extractDir
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

func printUpdatePlan(out io.Writer, name string, metadata SkillMetadata, current, targetPreview updatePreview, sourceSpec, expectedSHA256 string, hasRegistryTarget bool, target RegistryLockEntry) {
	currentEntrySum := sha256Bytes(current.EntryContent)
	targetEntrySum := sha256Bytes(targetPreview.EntryContent)
	entryStatus := "unchanged"
	if currentEntrySum != targetEntrySum {
		entryStatus = "changed"
	}
	targetSHA := expectedSHA256
	if targetSHA == "" {
		targetSHA = targetPreview.SourceSHA256
	}
	fmt.Fprintf(out, "# KISS update plan: %s\n\n", name)
	fmt.Fprintf(out, "- Current version: %s\n", current.Manifest.Version)
	fmt.Fprintf(out, "- Target version: %s\n", targetPreview.Manifest.Version)
	fmt.Fprintf(out, "- Current runner: %s\n", current.Manifest.RunnerType)
	fmt.Fprintf(out, "- Target runner: %s\n", targetPreview.Manifest.RunnerType)
	fmt.Fprintf(out, "- Current entry: %s\n", current.Manifest.Entry)
	fmt.Fprintf(out, "- Target entry: %s\n", targetPreview.Manifest.Entry)
	fmt.Fprintf(out, "- Entry content: %s\n", entryStatus)
	fmt.Fprintf(out, "- Current entry sha256: %s\n", currentEntrySum)
	fmt.Fprintf(out, "- Target entry sha256: %s\n", targetEntrySum)
	fmt.Fprintf(out, "- Current source: %s\n", sourceSpecFromMetadataForDisplay(metadata))
	fmt.Fprintf(out, "- Target source: %s\n", sourceSpec)
	if targetSHA != "" {
		fmt.Fprintf(out, "- Target sha256: %s\n", targetSHA)
	}
	if hasRegistryTarget {
		fmt.Fprintf(out, "- Registry: %s\n", target.Registry)
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "## Entry diff")
	fmt.Fprintln(out)
	if entryStatus == "unchanged" {
		fmt.Fprintln(out, "Entry content unchanged; no diff.")
	} else {
		diff := buildEntryDiff(current.Manifest.Entry, targetPreview.Manifest.Entry, current.EntryContent, targetPreview.EntryContent)
		if diff.Note != "" {
			fmt.Fprintln(out, diff.Note)
		}
		if diff.Text != "" {
			fence := markdownFenceFor(diff.Text)
			fmt.Fprintf(out, "%sdiff\n%s\n%s\n", fence, diff.Text, fence)
			if diff.Truncated {
				fmt.Fprintf(out, "\nDiff truncated to %d lines.\n", updateDiffMaxLines)
			}
		}
	}
	fmt.Fprintln(out)
	fmt.Fprintf(out, "Run `kiss update %s --yes` to apply.\n", name)
}

func sha256Bytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func registryUpdateTarget(paths Paths, name string) (RegistryLockEntry, bool, error) {
	if _, ok, err := GetRegistryLockEntry(paths, name); err != nil {
		return RegistryLockEntry{}, false, err
	} else if !ok {
		return RegistryLockEntry{}, false, nil
	}
	entry, err := ResolveRegistrySkill(paths, name)
	if err != nil {
		return RegistryLockEntry{}, false, err
	}
	return entry, true, nil
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

func sourceSpecFromMetadataForDisplay(metadata SkillMetadata) string {
	sourceSpec, err := sourceSpecFromMetadata(metadata)
	if err != nil {
		return metadata.Source.Kind + ":" + metadata.Source.URI
	}
	return sourceSpec
}
