package kiss

import (
	"fmt"
	"path"
	"strings"
)

type sourceIdentity struct {
	kind string
	uri  string
	ref  string
}

func ensureAddSourceAllowed(paths Paths, name string, source SourceMetadata, force bool) error {
	existing, ok, err := GetSkillMetadata(paths, name)
	if err != nil {
		return err
	}
	if !ok || sameSourceIdentity(existing.Source, source) || sameSourcePackageIdentity(existing.Source, source) || force {
		return nil
	}
	return fmt.Errorf("skill %q is already installed from %s; use --force to replace it with %s", name, displaySourceIdentity(existing.Source), displaySourceIdentity(source))
}

func sameSourceIdentity(left, right SourceMetadata) bool {
	leftID, leftOK := comparableSourceIdentity(left)
	rightID, rightOK := comparableSourceIdentity(right)
	return leftOK && rightOK && leftID == rightID
}

func displaySourceIdentity(source SourceMetadata) string {
	if id, ok := comparableSourceIdentity(source); ok {
		if id.ref != "" {
			return id.kind + ":" + id.uri + "@" + id.ref
		}
		return id.kind + ":" + id.uri
	}
	if source.URI != "" {
		return source.Kind + ":" + source.URI
	}
	return source.Kind
}

func sameSourcePackageIdentity(left, right SourceMetadata) bool {
	leftID, leftOK := comparableSourcePackageIdentity(left)
	rightID, rightOK := comparableSourcePackageIdentity(right)
	return leftOK && rightOK && leftID == rightID
}

func comparableSourcePackageIdentity(source SourceMetadata) (sourceIdentity, bool) {
	id, ok := comparableSourceIdentity(source)
	if !ok {
		return sourceIdentity{}, false
	}
	id.ref = ""
	return id, true
}

func comparableSourceIdentity(source SourceMetadata) (sourceIdentity, bool) {
	switch source.Kind {
	case "local":
		return sourceIdentity{kind: "local", uri: source.URI}, source.URI != ""
	case "github":
		if source.URI == "" {
			return sourceIdentity{}, false
		}
		return sourceIdentity{kind: "github", uri: source.URI, ref: normalizeGitHubIdentityRef(source.Ref)}, true
	case skillsImportSourceKind:
		return skillsImportComparableIdentity(source)
	default:
		return sourceIdentity{}, false
	}
}

func skillsImportComparableIdentity(source SourceMetadata) (sourceIdentity, bool) {
	if source.SkillsImport == nil {
		return sourceIdentity{}, false
	}
	importSource := source.SkillsImport
	switch importSource.SourceType {
	case "github":
		if importSource.Source == "" {
			return sourceIdentity{}, false
		}
		uri := appendSkillsImportSkillFolder(importSource.Source, importSource.SkillPath)
		ref := importSource.Ref
		if ref == "" {
			ref = source.Ref
		}
		return sourceIdentity{kind: "github", uri: uri, ref: normalizeGitHubIdentityRef(ref)}, true
	case "local":
		uri := importSource.SourceURL
		if uri == "" {
			uri = importSource.Source
		}
		if uri == "" {
			uri = importSource.OriginalPath
		}
		return sourceIdentity{kind: "local", uri: uri}, uri != ""
	default:
		return sourceIdentity{}, false
	}
}

func appendSkillsImportSkillFolder(source, skillPath string) string {
	folder := strings.TrimSuffix(strings.TrimSuffix(skillPath, "/SKILL.md"), "SKILL.md")
	folder = strings.Trim(folder, "/")
	if folder == "" {
		return strings.Trim(source, "/")
	}
	return path.Join(strings.Trim(source, "/"), folder)
}

func normalizeGitHubIdentityRef(ref string) string {
	if ref == "" {
		return "main"
	}
	return ref
}
