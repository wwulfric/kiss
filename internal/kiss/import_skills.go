package kiss

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const skillsImportSourceKind = "skills-import"

// ImportSkillsOptions 描述从 Vercel Skills CLI 全局安装记录导入的目标范围。
type ImportSkillsOptions struct {
	Names  []string
	All    bool
	DryRun bool
}

type skillsLockFile struct {
	Version int                        `json:"version"`
	Skills  map[string]skillsLockEntry `json:"skills"`
}

type skillsLockEntry struct {
	Source          string `json:"source"`
	SourceType      string `json:"sourceType"`
	SourceURL       string `json:"sourceUrl"`
	Ref             string `json:"ref"`
	SkillPath       string `json:"skillPath"`
	SkillFolderHash string `json:"skillFolderHash"`
	PluginName      string `json:"pluginName"`
	InstalledAt     string `json:"installedAt"`
	UpdatedAt       string `json:"updatedAt"`
}

type skillsImportCandidate struct {
	name       string
	sourceDir  string
	sourceInfo SourceMetadata
	fullName   string
}

// ImportSkills 把 Vercel Skills CLI 全局 canonical copy 快照导入 KISS store。
func ImportSkills(paths Paths, options ImportSkillsOptions, out io.Writer) error {
	candidates, err := resolveSkillsImportCandidates(options)
	if err != nil {
		return err
	}
	writer := errWriter{out: out}
	if options.DryRun {
		for _, candidate := range candidates {
			writer.printf("Would import %s from %s\n", candidate.name, candidate.sourceDir)
		}
		writer.printf("Dry run: %d skill(s) would be imported.\n", len(candidates))
		return writer.err
	}
	for _, candidate := range candidates {
		if err := installSkillFromDir(paths, candidate.sourceDir, candidate.name, candidate.fullName, candidate.sourceInfo); err != nil {
			return err
		}
		writer.printf("Imported %s from %s\n", candidate.name, candidate.sourceDir)
	}
	writer.printf("Imported %d skill(s).\n", len(candidates))
	return writer.err
}

func resolveSkillsImportCandidates(options ImportSkillsOptions) ([]skillsImportCandidate, error) {
	names, err := skillsImportNames(options)
	if err != nil {
		return nil, err
	}
	lockPath, err := skillsGlobalLockPath()
	if err != nil {
		return nil, err
	}
	lock, err := loadSkillsLock(lockPath)
	if err != nil {
		return nil, err
	}
	if options.All {
		names = namesFromSkillsLock(lock)
		if len(names) == 0 {
			return nil, fmt.Errorf("no skills found in %s", lockPath)
		}
	}
	canonicalDir, err := skillsGlobalCanonicalDir()
	if err != nil {
		return nil, err
	}
	candidates := make([]skillsImportCandidate, 0, len(names))
	for _, name := range names {
		entry, ok := lock.Skills[name]
		if !ok {
			return nil, fmt.Errorf("skill %q not found in vercel skills lock file %s", name, lockPath)
		}
		sourceDir := filepath.Join(canonicalDir, name)
		if err := ensureSkillsImportSourceDir(canonicalDir, sourceDir); err != nil {
			return nil, err
		}
		if _, err := LoadSkillDescriptor(sourceDir); err != nil {
			return nil, fmt.Errorf("load imported skill %q: %w", name, err)
		}
		sourceInfo, fullName := resolveSkillsImportSourceInfo(name, entry, sourceDir, lockPath, !options.DryRun)
		candidates = append(candidates, skillsImportCandidate{
			name:       name,
			sourceDir:  sourceDir,
			sourceInfo: sourceInfo,
			fullName:   fullName,
		})
	}
	return candidates, nil
}

func resolveSkillsImportSourceInfo(name string, entry skillsLockEntry, sourceDir, lockPath string, fetchSource bool) (SourceMetadata, string) {
	audit := skillsImportAuditMetadata(entry, sourceDir, lockPath)
	fallback := SourceMetadata{
		Kind:         skillsImportSourceKind,
		URI:          sourceDir,
		Ref:          entry.Ref,
		SkillsImport: audit,
	}
	fallbackFullName := skillsImportFullName(name, entry)
	if !fetchSource {
		return fallback, fallbackFullName
	}
	switch entry.SourceType {
	case "github":
		sourceSpec, ok := skillsImportGitHubSourceSpec(entry)
		if !ok {
			return fallback, fallbackFullName
		}
		resolved, _, _, err := ResolveRemoteSource(sourceSpec)
		if err != nil {
			return fallback, fallbackFullName
		}
		return SourceMetadata{
			Kind:         resolved.Kind,
			URI:          resolved.URI,
			Ref:          resolved.Ref,
			SkillsImport: audit,
		}, resolved.FullName
	default:
		return fallback, fallbackFullName
	}
}

func skillsImportAuditMetadata(entry skillsLockEntry, sourceDir, lockPath string) *SkillsImportSourceMetadata {
	return &SkillsImportSourceMetadata{
		OriginalPath:    sourceDir,
		LockPath:        lockPath,
		Source:          entry.Source,
		SourceType:      entry.SourceType,
		SourceURL:       entry.SourceURL,
		Ref:             entry.Ref,
		SkillPath:       entry.SkillPath,
		SkillFolderHash: entry.SkillFolderHash,
		PluginName:      entry.PluginName,
		InstalledAt:     entry.InstalledAt,
		UpdatedAt:       entry.UpdatedAt,
	}
}

func skillsImportGitHubSourceSpec(entry skillsLockEntry) (string, bool) {
	source := strings.TrimSpace(entry.Source)
	if source == "" {
		source = githubSourceFromURL(entry.SourceURL)
	}
	if source == "" {
		return "", false
	}
	source = appendSkillsImportSkillFolder(source, entry.SkillPath)
	spec := "github:" + source
	if entry.Ref != "" {
		spec += "#" + entry.Ref
	}
	return spec, true
}

func githubSourceFromURL(rawURL string) string {
	trimmed := strings.TrimSpace(rawURL)
	trimmed = strings.TrimSuffix(trimmed, ".git")
	trimmed = strings.TrimPrefix(trimmed, "https://github.com/")
	trimmed = strings.TrimPrefix(trimmed, "http://github.com/")
	trimmed = strings.Trim(trimmed, "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return ""
	}
	return parts[0] + "/" + parts[1]
}

func skillsImportNames(options ImportSkillsOptions) ([]string, error) {
	if options.All == (len(options.Names) > 0) {
		return nil, fmt.Errorf("usage: kiss import (--skill <name> | --all) [--dry-run]")
	}
	seen := map[string]bool{}
	names := make([]string, 0, len(options.Names))
	for _, name := range options.Names {
		if err := ValidateSkillName(name); err != nil {
			return nil, err
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func namesFromSkillsLock(lock skillsLockFile) []string {
	names := make([]string, 0, len(lock.Skills))
	for name := range lock.Skills {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func loadSkillsLock(path string) (skillsLockFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return skillsLockFile{}, fmt.Errorf("vercel skills lock file not found at %s", path)
		}
		return skillsLockFile{}, err
	}
	var lock skillsLockFile
	if err := json.Unmarshal(data, &lock); err != nil {
		return skillsLockFile{}, err
	}
	if lock.Skills == nil {
		return skillsLockFile{}, fmt.Errorf("vercel skills lock file %s does not contain skills", path)
	}
	return lock, nil
}

func skillsGlobalLockPath() (string, error) {
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		return filepath.Abs(filepath.Join(xdg, "skills", ".skill-lock.json"))
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Abs(filepath.Join(home, ".agents", ".skill-lock.json"))
}

func skillsGlobalCanonicalDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Abs(filepath.Join(home, ".agents", "skills"))
}

func ensureSkillsImportSourceDir(baseDir, sourceDir string) error {
	cleanBase := filepath.Clean(baseDir)
	cleanSource := filepath.Clean(sourceDir)
	if cleanSource != cleanBase && !strings.HasPrefix(cleanSource, cleanBase+string(filepath.Separator)) {
		return fmt.Errorf("unsafe skills import source path: %s", sourceDir)
	}
	info, err := os.Stat(sourceDir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("vercel skills canonical copy not found at %s", sourceDir)
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("vercel skills canonical copy is not a directory: %s", sourceDir)
	}
	return nil
}

func skillsImportFullName(name string, entry skillsLockEntry) string {
	if entry.Source != "" && entry.SkillPath != "" {
		skillDir := strings.TrimSuffix(entry.SkillPath, "/SKILL.md")
		if skillDir != "" {
			return skillsImportSourceKind + ":" + entry.Source + "/" + skillDir
		}
	}
	if entry.Source != "" {
		return skillsImportSourceKind + ":" + entry.Source
	}
	return skillsImportSourceKind + ":" + name
}
