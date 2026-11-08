package kiss

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, body := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func skillMd(name string) string {
	return "---\nname: " + name + "\ndescription: " + name + " skill\n---\n\n# " + name + "\n"
}

func discoveredRelPaths(skills []discoveredSkill) []string {
	var paths []string
	for _, skill := range skills {
		paths = append(paths, skill.Name+"="+skill.RelPath)
	}
	sort.Strings(paths)
	return paths
}

func assertDiscovered(t *testing.T, root, subpath string, want ...string) {
	t.Helper()
	skills, err := discoverSkills(root, subpath, discoveryOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got := discoveredRelPaths(skills)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("discovered %v, want %v", got, want)
	}
}

func TestParseSkillFrontmatter(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    skillFrontmatter
	}{
		{
			name:    "plain",
			content: "---\nname: tdd\ndescription: Use for TDD. # comment\n---\nbody",
			want:    skillFrontmatter{Name: "tdd", Description: "Use for TDD."},
		},
		{
			name:    "quoted and crlf with bom",
			content: "\uFEFF---\r\nname: \"why\"\r\ndescription: 'it''s why'\r\n---\r\n",
			want:    skillFrontmatter{Name: "why", Description: "it's why"},
		},
		{
			name:    "block scalar",
			content: "---\nname: teach\ndescription: >-\n  Explain a body\n  of work.\n---\n",
			want:    skillFrontmatter{Name: "teach", Description: "Explain a body of work."},
		},
		{
			name:    "nested internal",
			content: "---\nname: release\ndescription: x\nmetadata:\n  author: me\n  internal: true\n---\n",
			want:    skillFrontmatter{Name: "release", Description: "x", Internal: true},
		},
		{
			name:    "flow internal",
			content: "---\nname: release\ndescription: x\nmetadata: {internal: true}\n---\n",
			want:    skillFrontmatter{Name: "release", Description: "x", Internal: true},
		},
		{
			name:    "string true is not internal",
			content: "---\nname: release\ndescription: x\nmetadata:\n  internal: \"true\"\n---\n",
			want:    skillFrontmatter{Name: "release", Description: "x"},
		},
		{
			name:    "deeper internal is not metadata.internal",
			content: "---\nname: a\ndescription: x\nmetadata:\n  nested:\n    internal: true\n---\n",
			want:    skillFrontmatter{Name: "a", Description: "x"},
		},
		{
			name:    "no frontmatter",
			content: "Just instructions.\n",
			want:    skillFrontmatter{},
		},
		{
			name:    "unclosed frontmatter",
			content: "---\nname: a\n",
			want:    skillFrontmatter{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseSkillFrontmatter(tt.content); got != tt.want {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDiscoverSkillsLayouts(t *testing.T) {
	t.Run("root skill wins and stops search", func(t *testing.T) {
		root := t.TempDir()
		writeTree(t, root, map[string]string{
			"SKILL.md":            skillMd("root"),
			"skills/x/SKILL.md":   skillMd("x"),
			"examples/y/SKILL.md": skillMd("y"),
		})
		assertDiscovered(t, root, "", "root=")
	})
	t.Run("root skill without frontmatter uses dir name", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "plain-skill")
		writeTree(t, root, map[string]string{"SKILL.md": "Just instructions.\n"})
		assertDiscovered(t, root, "", "plain-skill=")
	})
	t.Run("skills dir like pstack", func(t *testing.T) {
		root := t.TempDir()
		writeTree(t, root, map[string]string{
			"skills/tdd/SKILL.md": skillMd("tdd"),
			"skills/why/SKILL.md": skillMd("why"),
			"plugin.json":         `{"name":"pstack"}`,
		})
		assertDiscovered(t, root, "", "tdd=skills/tdd", "why=skills/why")
	})
	t.Run("single top-level skill dir", func(t *testing.T) {
		root := t.TempDir()
		writeTree(t, root, map[string]string{"browser-test/SKILL.md": skillMd("browser-test")})
		assertDiscovered(t, root, "", "browser-test=browser-test")
	})
	t.Run("agent dirs and curated", func(t *testing.T) {
		root := t.TempDir()
		writeTree(t, root, map[string]string{
			".claude/skills/a/SKILL.md":          skillMd("a"),
			".agents/skills/b/SKILL.md":          skillMd("b"),
			"skills/.curated/c/SKILL.md":         skillMd("c"),
			"skills/group/nested/d/SKILL.md":     skillMd("d"),
			"skills/g1/g2/g3/too-deep/SKILL.md":  skillMd("too-deep"),
			"node_modules/pkg/skills/e/SKILL.md": skillMd("e"),
		})
		assertDiscovered(t, root, "",
			"a=.claude/skills/a",
			"b=.agents/skills/b",
			"c=skills/.curated/c",
			"d=skills/group/nested/d",
		)
	})
	t.Run("internal skills are hidden unless env set", func(t *testing.T) {
		root := t.TempDir()
		writeTree(t, root, map[string]string{
			"skills/kiss/SKILL.md":            skillMd("kiss"),
			".agents/skills/release/SKILL.md": "---\nname: release\ndescription: x\nmetadata:\n  internal: true\n---\n",
		})
		assertDiscovered(t, root, "", "kiss=skills/kiss")
		t.Setenv("INSTALL_INTERNAL_SKILLS", "1")
		assertDiscovered(t, root, "", "kiss=skills/kiss", "release=.agents/skills/release")
	})
	t.Run("duplicate names keep first priority match", func(t *testing.T) {
		root := t.TempDir()
		writeTree(t, root, map[string]string{
			"skills/tdd/SKILL.md":         skillMd("tdd"),
			".claude/skills/tdd/SKILL.md": skillMd("tdd"),
		})
		assertDiscovered(t, root, "", "tdd=skills/tdd")
	})
	t.Run("vendored project skills from skills-lock are skipped", func(t *testing.T) {
		root := t.TempDir()
		writeTree(t, root, map[string]string{
			"skills/own/SKILL.md":                 skillMd("own"),
			".agents/skills/find-skills/SKILL.md": skillMd("find-skills"),
			"skills-lock.json":                    `{"version":1,"skills":{"find-skills":{}}}`,
		})
		assertDiscovered(t, root, "", "own=skills/own")
	})
	t.Run("plugin manifest paths", func(t *testing.T) {
		root := t.TempDir()
		writeTree(t, root, map[string]string{
			".claude-plugin/plugin.json":                      `{"name":"p","skills":["./custom/alpha"]}`,
			"custom/alpha/SKILL.md":                           skillMd("alpha"),
			".claude-plugin/marketplace.json":                 `{"metadata":{"pluginRoot":"./plugins"},"plugins":[{"name":"m","source":"./m"},{"name":"bad","source":"../escape"}]}`,
			"plugins/m/skills/beta/SKILL.md":                  skillMd("beta"),
			"plugins/m/skills/nested/too-deep/gamma/SKILL.md": skillMd("gamma"),
		})
		assertDiscovered(t, root, "", "alpha=custom/alpha", "beta=plugins/m/skills/beta")
	})
	t.Run("skills.sh.json paths", func(t *testing.T) {
		root := t.TempDir()
		writeTree(t, root, map[string]string{
			"skills.sh.json":           `{"paths":["./catalog","../escape","/abs"]}`,
			"catalog/x/SKILL.md":       skillMd("x"),
			"catalog/group/y/SKILL.md": skillMd("y"),
			"skills/regular/SKILL.md":  skillMd("regular"),
		})
		assertDiscovered(t, root, "", "x=catalog/x", "y=catalog/group/y", "regular=skills/regular")
	})
	t.Run("full depth fallback", func(t *testing.T) {
		root := t.TempDir()
		writeTree(t, root, map[string]string{
			"a/b/c/deep/SKILL.md": skillMd("deep"),
			"build/x/SKILL.md":    skillMd("ignored"),
		})
		assertDiscovered(t, root, "", "deep=a/b/c/deep")
	})
	t.Run("subpath narrows search", func(t *testing.T) {
		root := t.TempDir()
		writeTree(t, root, map[string]string{
			"skills/tdd/SKILL.md":          skillMd("tdd"),
			"packages/p/skills/x/SKILL.md": skillMd("x"),
		})
		assertDiscovered(t, root, "packages/p", "x=packages/p/skills/x")
		assertDiscovered(t, root, "skills/tdd", "tdd=skills/tdd")
		if _, err := discoverSkills(root, "../escape", discoveryOptions{}); err == nil {
			t.Fatal("expected unsafe subpath error")
		}
		if _, err := discoverSkills(root, "missing", discoveryOptions{}); err == nil {
			t.Fatal("expected missing subpath error")
		}
	})
}

func TestSelectDiscoveredSkills(t *testing.T) {
	skills := []discoveredSkill{
		{Name: "Frontend Design", Dir: "/r/skills/frontend", RelPath: "skills/frontend"},
		{Name: "tdd", Dir: "/r/skills/tdd", RelPath: "skills/tdd"},
	}
	names := func(selected []discoveredSkill) string {
		var out []string
		for _, skill := range selected {
			out = append(out, skill.Name)
		}
		return strings.Join(out, ",")
	}
	if _, err := selectDiscoveredSkills(skills, skillSelection{}); err == nil || !strings.Contains(err.Error(), "Frontend Design, tdd") {
		t.Fatalf("expected multi-skill error listing names, got %v", err)
	}
	for filter, want := range map[string]string{
		"TDD":             "tdd",
		"frontend-design": "Frontend Design",
		"frontend_design": "Frontend Design",
		"frontend":        "Frontend Design",
	} {
		got, err := selectDiscoveredSkills(skills, skillSelection{Filters: []string{filter}})
		if err != nil || names(got) != want {
			t.Fatalf("filter %q: got %s err=%v, want %q", filter, names(got), err, want)
		}
	}
	if got, err := selectDiscoveredSkills(skills, skillSelection{Filters: []string{"tdd", "TDD", "frontend"}}); err != nil || names(got) != "tdd,Frontend Design" {
		t.Fatalf("expected deduplicated multi selection, got %s err=%v", names(got), err)
	}
	for _, selection := range []skillSelection{{All: true}, {Filters: []string{"*"}}} {
		if got, err := selectDiscoveredSkills(skills, selection); err != nil || len(got) != 2 {
			t.Fatalf("expected all skills for %+v, got %s err=%v", selection, names(got), err)
		}
	}
	if got, err := selectDiscoveredSkills(skills, skillSelection{PreferredName: "tdd"}); err != nil || names(got) != "tdd" {
		t.Fatalf("expected preferred name selection, got %s err=%v", names(got), err)
	}
	if _, err := selectDiscoveredSkills(skills, skillSelection{Filters: []string{"tdd", "missing"}}); err == nil || !strings.Contains(err.Error(), "no matching skills found for: missing") {
		t.Fatalf("expected not found error, got %v", err)
	}
	if _, err := selectDiscoveredSkills(nil, skillSelection{}); err == nil {
		t.Fatal("expected no skills error")
	}
	if !(skillSelection{Filters: []string{"release"}}).includesInternal() || (skillSelection{Filters: []string{"*"}}).includesInternal() {
		t.Fatal("explicit names should include internal skills, * should not")
	}

	name, err := installNameForSkill(skills[0], "")
	if err != nil || name != "frontend" {
		t.Fatalf("expected dir-name fallback for invalid frontmatter name, got %q err=%v", name, err)
	}
}

func TestDiscoverSkillsFullDepthAndInternalOptions(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"SKILL.md":               skillMd("root"),
		"skills/x/SKILL.md":      skillMd("x"),
		"skills/hidden/SKILL.md": "---\nname: hidden\ndescription: x\nmetadata:\n  internal: true\n---\n",
	})
	skills, err := discoverSkills(root, "", discoveryOptions{FullDepth: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(discoveredRelPaths(skills), ","); got != "root=,x=skills/x" {
		t.Fatalf("full depth discovered %s", got)
	}
	skills, err = discoverSkills(root, "", discoveryOptions{FullDepth: true, IncludeInternal: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(discoveredRelPaths(skills), ","); got != "hidden=skills/hidden,root=,x=skills/x" {
		t.Fatalf("include internal discovered %s", got)
	}
}

func TestAddSkillsMultipleAndAll(t *testing.T) {
	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := t.TempDir()
	writeTree(t, repo, map[string]string{
		"skills/tdd/SKILL.md":             skillMd("tdd"),
		"skills/why/SKILL.md":             skillMd("why"),
		"skills/teach/SKILL.md":           skillMd("teach"),
		".agents/skills/release/SKILL.md": "---\nname: release\ndescription: x\nmetadata:\n  internal: true\n---\n",
	})
	installed, err := AddSkills(paths, repo, "", AddOptions{Skills: []string{"tdd", "why"}})
	if err != nil || strings.Join(installed, ",") != "tdd,why" {
		t.Fatalf("multi install: %v err=%v", installed, err)
	}
	if _, err := AddSkills(paths, repo, "custom", AddOptions{Skills: []string{"tdd", "why"}}); err == nil || !strings.Contains(err.Error(), "--name") {
		t.Fatalf("expected --name with multiple skills error, got %v", err)
	}
	installed, err = AddSkills(paths, repo, "", AddOptions{All: true})
	if err != nil || strings.Join(installed, ",") != "tdd,teach,why" {
		t.Fatalf("--all install: %v err=%v", installed, err)
	}
	// 与 npx skills 一致：显式点名时可以安装 internal skill。
	if _, err := AddSkills(paths, repo, "", AddOptions{Skills: []string{"release"}}); err != nil {
		t.Fatalf("explicit internal skill: %v", err)
	}
	listed, err := ListSourceSkills(paths, repo, false)
	if err != nil || len(listed) != 3 || listed[0].Description == "" {
		t.Fatalf("list source skills: %+v err=%v", listed, err)
	}
}

func makeTreeArchive(t *testing.T, root string, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	rels := make([]string, 0, len(files))
	for rel := range files {
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	for _, rel := range rels {
		body := files[rel]
		if err := tw.WriteHeader(&tar.Header{Name: root + "/" + rel, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestAddRemoteMultiSkillRepo(t *testing.T) {
	sha := "2222222222222222222222222222222222222222"
	archive := makeTreeArchive(t, "pstack-"+sha, map[string]string{
		"skills/tdd/SKILL.md":             skillMd("tdd"),
		"skills/why/SKILL.md":             skillMd("why"),
		".agents/skills/release/SKILL.md": "---\nname: release\ndescription: x\nmetadata:\n  internal: true\n---\n",
	})
	defer withGitHubTestServer(t, sha, archive, nil)()
	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	err = AddRemoteSkill(paths, "github:owner/pstack", "")
	if err == nil || !strings.Contains(err.Error(), "use --skill") || !strings.Contains(err.Error(), "tdd, why") {
		t.Fatalf("expected multi-skill hint without internal skill, got %v", err)
	}

	if err := AddRemoteSkillWithOptions(paths, "github:owner/pstack", "", AddOptions{Skills: []string{"tdd"}}); err != nil {
		t.Fatalf("add --skill tdd: %v", err)
	}
	metadata, ok, err := GetSkillMetadata(paths, "tdd")
	if err != nil || !ok {
		t.Fatalf("metadata missing ok=%v err=%v", ok, err)
	}
	if metadata.Source.URI != "owner/pstack/skills/tdd" || metadata.FullName != "github:owner/pstack/skills/tdd" {
		t.Fatalf("expected discovered path in source metadata, got %+v full=%q", metadata.Source, metadata.FullName)
	}

	// update 复用记录下来的精确路径。
	var out bytes.Buffer
	if err := UpdateSkill(paths, "tdd", false, &out); err != nil {
		t.Fatalf("update preview: %v", err)
	}

	// --name 在多 skill source 中可作为选择依据。
	if err := AddRemoteSkill(paths, "github:owner/pstack", "why"); err != nil {
		t.Fatalf("add with --name why: %v", err)
	}
}

func TestUpdateLegacyRecordWithoutPathInMultiSkillRepo(t *testing.T) {
	sha := "3333333333333333333333333333333333333333"
	archive := makeTreeArchive(t, "repo-"+sha, map[string]string{
		"browser-test/SKILL.md": skillMd("browser-test"),
	})
	defer withGitHubTestServer(t, sha, archive, nil)()
	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := AddRemoteSkill(paths, "github:owner/repo", ""); err != nil {
		t.Fatalf("add single top-level skill: %v", err)
	}
	metadata, ok, err := GetSkillMetadata(paths, "browser-test")
	if err != nil || !ok {
		t.Fatalf("metadata missing ok=%v err=%v", ok, err)
	}
	// 模拟旧版本只记录了仓库根的 metadata。
	metadata.Source.URI = "owner/repo"
	if err := UpsertSkillMetadata(paths, metadata); err != nil {
		t.Fatal(err)
	}

	// 仓库之后新增了其他 skill；update 仍能按已安装 name 选中原 skill。
	archive2 := makeTreeArchive(t, "repo-"+sha, map[string]string{
		"browser-test/SKILL.md": skillMd("browser-test") + "v2\n",
		"other/SKILL.md":        skillMd("other"),
	})
	defer withGitHubTestServer(t, sha, archive2, nil)()
	var out bytes.Buffer
	if err := UpdateSkill(paths, "browser-test", true, &out); err != nil {
		t.Fatalf("update legacy record: %v", err)
	}
	updated, _, err := GetSkillMetadata(paths, "browser-test")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Source.URI != "owner/repo/browser-test" {
		t.Fatalf("expected update to record discovered path, got %+v", updated.Source)
	}
}

func makeArchiveWithLink(t *testing.T, root string, files map[string]string, linkPath, linkTarget string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for rel, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: root + "/" + rel, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.WriteHeader(&tar.Header{Name: root + "/" + linkPath, Typeflag: tar.TypeSymlink, Linkname: linkTarget}); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestAddRemoteIgnoresLinksOutsideSelectedSkill(t *testing.T) {
	sha := "4444444444444444444444444444444444444444"
	archive := makeArchiveWithLink(t, "repo-"+sha, map[string]string{
		"AGENTS.md":                    "agents\n",
		"skills/browser-test/SKILL.md": skillMd("browser-test"),
	}, "CLAUDE.md", "AGENTS.md")
	defer withGitHubTestServer(t, sha, archive, nil)()
	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := AddRemoteSkillWithOptions(paths, "github:owner/repo", "", AddOptions{Skills: []string{"browser-test"}}); err != nil {
		t.Fatalf("expected repo-level link to be ignored: %v", err)
	}
}

func TestAddRemoteRejectsLinksInsideSelectedSkill(t *testing.T) {
	sha := "5555555555555555555555555555555555555555"
	archive := makeArchiveWithLink(t, "repo-"+sha, map[string]string{
		"skills/browser-test/SKILL.md": skillMd("browser-test"),
	}, "skills/browser-test/secret", "/etc/passwd")
	defer withGitHubTestServer(t, sha, archive, nil)()
	paths, err := NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	err = AddRemoteSkillWithOptions(paths, "github:owner/repo", "", AddOptions{Skills: []string{"browser-test"}})
	if err == nil || !strings.Contains(err.Error(), "link entry") {
		t.Fatalf("expected link-entry rejection, got %v", err)
	}
}
