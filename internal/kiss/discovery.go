package kiss

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// 本文件对齐 Vercel Skills CLI（npx skills）的 skill 发现规则，让同一个 source 在两边解析出相同的 skill 集合。
// KISS 每次 add 仍只安装一个 skill；发现结果只用于选择，不会批量安装。

// discoverySkipDirs 与 npx skills 的 SKIP_DIRS 保持一致。
var discoverySkipDirs = map[string]bool{
	"node_modules": true,
	".git":         true,
	"dist":         true,
	"build":        true,
	"__pycache__":  true,
}

// agentProjectSkillDirs 与 npx skills 的 AGENT_PROJECT_SKILL_DIRS 保持一致。
var agentProjectSkillDirs = []string{
	".agents/skills",
	".claude/skills",
	".cline/skills",
	".codebuddy/skills",
	".codex/skills",
	".commandcode/skills",
	".continue/skills",
	".factory/skills",
	".github/skills",
	".goose/skills",
	".grok/skills",
	".iflow/skills",
	".junie/skills",
	".kilo/skills",
	".kilocode/skills",
	".kimchi/skills",
	".kiro/skills",
	".minimax/skills",
	".mux/skills",
	".neovate/skills",
	".opencode/skills",
	".openhands/skills",
	".pi/skills",
	".posit/assistant/skills",
	".qoder/skills",
	".roo/skills",
	".trae/skills",
	".windsurf/skills",
	".zcode/skills",
	".zencoder/skills",
}

const (
	discoveryDeepWalkDepth = 3
	discoveryFullMaxDepth  = 5
)

// discoveredSkill 是在 source 目录中找到的一个候选 skill。
type discoveredSkill struct {
	// Name 优先取 SKILL.md frontmatter 的 name；缺失时退回目录名。
	Name string
	// Dir 是 skill 目录的绝对路径。
	Dir string
	// RelPath 是相对 source 根目录的 slash 路径；source 根目录本身是 skill 时为空。
	RelPath string
	// Description 是 frontmatter 的 description，仅用于 --list 展示。
	Description string
}

// discoveryOptions 对应 npx skills 的 includeInternal 和 --full-depth。
type discoveryOptions struct {
	// IncludeInternal 为 true 时不隐藏 metadata.internal 的 skill；npx skills 在用户显式点名 skill 时开启。
	IncludeInternal bool
	// FullDepth 为 true 时即使根目录是 skill 或优先目录已有结果，也继续全量搜索。
	FullDepth bool
}

// discoverSkills 在 root（可选 subpath）下按 npx skills 的规则发现 skill。
func discoverSkills(root, subpath string, options discoveryOptions) ([]discoveredSkill, error) {
	if err := validateOptionalSafeRelativePath(subpath); err != nil {
		return nil, fmt.Errorf("skill path %q must be safe relative path: %w", subpath, err)
	}
	searchPath := root
	if subpath != "" {
		searchPath = filepath.Join(root, filepath.FromSlash(subpath))
	}
	info, err := os.Stat(searchPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("skill path %q not found in source", subpath)
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("skill path %q is not a directory", subpath)
	}

	d := &skillDiscoverer{
		root:            root,
		includeInternal: options.IncludeInternal || installInternalSkills(),
		parsed:          map[string]bool{},
		seenNames:       map[string]bool{},
		lockedNames:     readSourceLockedSkillNames(root),
	}

	// 被点名的目录本身就是 skill 时，只返回它（--full-depth 除外）；这也是 KISS 旧的单 skill 仓库行为。
	if hasSkillMd(searchPath) {
		skill, ok := d.parse(searchPath)
		if ok && !d.isVendoredProjectSkill(skill) {
			d.skills = append(d.skills, skill)
			d.seenNames[skill.Name] = true
			if !options.FullDepth {
				return d.skills, nil
			}
		}
	}

	type searchDir struct {
		path  string
		depth int
	}
	var dirs []searchDir
	dirs = append(dirs, searchDir{searchPath, 1})
	deep := []string{
		"skills",
		"skills/.curated",
		"skills/.experimental",
		"skills/.system",
	}
	deep = append(deep, agentProjectSkillDirs...)
	for _, rel := range deep {
		dirs = append(dirs, searchDir{filepath.Join(searchPath, filepath.FromSlash(rel)), discoveryDeepWalkDepth})
	}
	for _, dir := range skillsShSearchDirs(searchPath) {
		dirs = append(dirs, searchDir{dir, discoveryDeepWalkDepth})
	}
	for _, dir := range pluginSkillSearchDirs(searchPath) {
		dirs = append(dirs, searchDir{dir, 1})
	}
	for _, dir := range dirs {
		d.walk(dir.path, dir.depth, 1)
	}

	if len(d.skills) == 0 || options.FullDepth {
		for _, dir := range findSkillDirs(searchPath, 0) {
			d.add(dir)
		}
	}
	return d.skills, nil
}

type skillDiscoverer struct {
	root            string
	includeInternal bool
	skills          []discoveredSkill
	parsed          map[string]bool
	seenNames       map[string]bool
	lockedNames     map[string]bool
}

func (d *skillDiscoverer) walk(dir string, maxDepth, depth int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		child := filepath.Join(dir, entry.Name())
		if d.add(child) || depth >= maxDepth || discoverySkipDirs[entry.Name()] {
			continue
		}
		d.walk(child, maxDepth, depth+1)
	}
}

// add 尝试把 dir 作为 skill 加入结果；dir 含 SKILL.md 时返回 true（即使因重名、internal 等原因被跳过）。
func (d *skillDiscoverer) add(dir string) bool {
	if !hasSkillMd(dir) {
		return false
	}
	skill, ok := d.parse(dir)
	if !ok || d.seenNames[skill.Name] || d.isVendoredProjectSkill(skill) {
		return true
	}
	d.skills = append(d.skills, skill)
	d.seenNames[skill.Name] = true
	return true
}

func (d *skillDiscoverer) parse(dir string) (discoveredSkill, bool) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return discoveredSkill{}, false
	}
	if d.parsed[abs] {
		return discoveredSkill{}, false
	}
	d.parsed[abs] = true
	content, err := os.ReadFile(filepath.Join(abs, "SKILL.md"))
	if err != nil {
		return discoveredSkill{}, false
	}
	fm := parseSkillFrontmatter(string(content))
	if fm.Internal && !d.includeInternal {
		return discoveredSkill{}, false
	}
	name := fm.Name
	if name == "" {
		name = filepath.Base(abs)
	}
	rel, err := filepath.Rel(d.root, abs)
	if err != nil {
		return discoveredSkill{}, false
	}
	rel = filepath.ToSlash(rel)
	if rel == "." {
		rel = ""
	}
	return discoveredSkill{Name: name, Dir: abs, RelPath: rel, Description: fm.Description}, true
}

// isVendoredProjectSkill 跳过 source 仓库自己通过 npx skills 安装进 agent 目录、并记录在 skills-lock.json 中的第三方 skill。
func (d *skillDiscoverer) isVendoredProjectSkill(skill discoveredSkill) bool {
	if len(d.lockedNames) == 0 {
		return false
	}
	inAgentDir := false
	for _, dir := range agentProjectSkillDirs {
		if skill.RelPath == dir || strings.HasPrefix(skill.RelPath, dir+"/") {
			inAgentDir = true
			break
		}
	}
	if !inAgentDir {
		return false
	}
	return d.lockedNames[normalizeDiscoveryName(skill.Name)] || d.lockedNames[normalizeDiscoveryName(filepath.Base(skill.Dir))]
}

func findSkillDirs(dir string, depth int) []string {
	if depth > discoveryFullMaxDepth {
		return nil
	}
	var result []string
	if hasSkillMd(dir) {
		result = append(result, dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return result
	}
	for _, entry := range entries {
		if !entry.IsDir() || discoverySkipDirs[entry.Name()] {
			continue
		}
		result = append(result, findSkillDirs(filepath.Join(dir, entry.Name()), depth+1)...)
	}
	return result
}

func hasSkillMd(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "SKILL.md"))
	return err == nil && info.Mode().IsRegular()
}

func installInternalSkills() bool {
	value := os.Getenv("INSTALL_INTERNAL_SKILLS")
	return value == "1" || value == "true"
}

func normalizeDiscoveryName(name string) string {
	name = strings.ToLower(name)
	return strings.Join(strings.FieldsFunc(name, func(r rune) bool {
		return r == '_' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	}), "-")
}

func readSourceLockedSkillNames(root string) map[string]bool {
	data, err := os.ReadFile(filepath.Join(root, "skills-lock.json"))
	if err != nil {
		return nil
	}
	var lock struct {
		Skills map[string]json.RawMessage `json:"skills"`
	}
	if json.Unmarshal(data, &lock) != nil {
		return nil
	}
	names := map[string]bool{}
	for name := range lock.Skills {
		names[normalizeDiscoveryName(name)] = true
	}
	return names
}

// skillsShSearchDirs 读取 skills.sh.json 的 paths（只接受 ./ 开头的安全相对路径）。
func skillsShSearchDirs(base string) []string {
	data, err := os.ReadFile(filepath.Join(base, "skills.sh.json"))
	if err != nil {
		return nil
	}
	var config struct {
		Paths []any `json:"paths"`
	}
	if json.Unmarshal(data, &config) != nil {
		return nil
	}
	var dirs []string
	seen := map[string]bool{}
	for _, raw := range config.Paths {
		entry, ok := raw.(string)
		if !ok || !strings.HasPrefix(entry, "./") || strings.Contains(entry, `\`) {
			continue
		}
		rel := strings.TrimRight(filepath.ToSlash(filepath.Clean(entry)), "/")
		if rel == "." || rel == "" || rel == ".." || strings.HasPrefix(rel, "../") || filepath.IsAbs(rel) {
			continue
		}
		if !seen[rel] {
			seen[rel] = true
			dirs = append(dirs, filepath.Join(base, filepath.FromSlash(rel)))
		}
	}
	return dirs
}

type pluginManifest struct {
	Skills []string `json:"skills"`
}

type marketplaceManifest struct {
	Metadata struct {
		PluginRoot *string `json:"pluginRoot"`
	} `json:"metadata"`
	Plugins []struct {
		Source json.RawMessage `json:"source"`
		Skills []string        `json:"skills"`
	} `json:"plugins"`
}

// pluginSkillSearchDirs 读取 .claude-plugin/marketplace.json 和 .claude-plugin/plugin.json 声明的 skill 位置。
func pluginSkillSearchDirs(base string) []string {
	var dirs []string
	addPlugin := func(pluginBase string, skills []string) {
		if !isContainedPath(pluginBase, base) {
			return
		}
		for _, skillPath := range skills {
			if !strings.HasPrefix(skillPath, "./") {
				continue
			}
			skillDir := filepath.Dir(filepath.Join(pluginBase, filepath.FromSlash(skillPath)))
			if isContainedPath(skillDir, base) {
				dirs = append(dirs, skillDir)
			}
		}
		dirs = append(dirs, filepath.Join(pluginBase, "skills"))
	}

	if data, err := os.ReadFile(filepath.Join(base, ".claude-plugin", "marketplace.json")); err == nil {
		var manifest marketplaceManifest
		if json.Unmarshal(data, &manifest) == nil {
			pluginRoot := ""
			validRoot := true
			if manifest.Metadata.PluginRoot != nil {
				pluginRoot = *manifest.Metadata.PluginRoot
				validRoot = strings.HasPrefix(pluginRoot, "./")
			}
			if validRoot {
				for _, plugin := range manifest.Plugins {
					source := ""
					if len(plugin.Source) > 0 && string(plugin.Source) != "null" {
						if json.Unmarshal(plugin.Source, &source) != nil || !strings.HasPrefix(source, "./") {
							continue
						}
					}
					addPlugin(filepath.Join(base, filepath.FromSlash(pluginRoot), filepath.FromSlash(source)), plugin.Skills)
				}
			}
		}
	}
	if data, err := os.ReadFile(filepath.Join(base, ".claude-plugin", "plugin.json")); err == nil {
		var manifest pluginManifest
		if json.Unmarshal(data, &manifest) == nil {
			addPlugin(base, manifest.Skills)
		}
	}
	return dirs
}

func isContainedPath(target, base string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
}

// skillSelection 是 add 的 skill 选择条件，对应 npx skills 的 --skill（可多个，"*" 表示全部）和 --all。
type skillSelection struct {
	Filters []string
	All     bool
	// PreferredName 在未指定 --skill 且 source 含多个 skill 时尝试匹配（来自 --name 或 update 的已安装名称）。
	PreferredName string
}

func (s skillSelection) selectsAll() bool {
	if s.All {
		return true
	}
	for _, filter := range s.Filters {
		if filter == "*" {
			return true
		}
	}
	return false
}

// includesInternal 与 npx skills 一致：显式点名 skill（非 "*"）时允许选中 metadata.internal 的 skill。
func (s skillSelection) includesInternal() bool {
	return len(s.Filters) > 0 && !s.selectsAll()
}

// selectDiscoveredSkills 按 npx skills 的 --skill 匹配规则选择 skill。
// 与 npx 的差异：未指定 --skill 且有多个 skill 时不进入交互选择，而是报错列出候选；点名但不存在的 skill 直接报错。
func selectDiscoveredSkills(skills []discoveredSkill, selection skillSelection) ([]discoveredSkill, error) {
	if len(skills) == 0 {
		return nil, fmt.Errorf("no skills found in source")
	}
	if selection.selectsAll() {
		return skills, nil
	}
	if len(selection.Filters) == 0 {
		if len(skills) == 1 {
			return skills, nil
		}
		if selection.PreferredName != "" {
			if skill, ok := matchDiscoveredSkill(skills, selection.PreferredName); ok {
				return []discoveredSkill{skill}, nil
			}
		}
		return nil, fmt.Errorf("source contains %d skills; use --skill <name> (or --all) to choose: %s", len(skills), discoveredSkillNames(skills))
	}
	var selected []discoveredSkill
	seen := map[string]bool{}
	var missing []string
	for _, filter := range selection.Filters {
		skill, ok := matchDiscoveredSkill(skills, filter)
		if !ok {
			missing = append(missing, filter)
			continue
		}
		if !seen[skill.Dir] {
			seen[skill.Dir] = true
			selected = append(selected, skill)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("no matching skills found for: %s; available skills: %s", strings.Join(missing, ", "), discoveredSkillNames(skills))
	}
	return selected, nil
}

// matchDiscoveredSkill 依次按 name 精确（大小写不敏感）、规范化 name、目录名匹配。
func matchDiscoveredSkill(skills []discoveredSkill, filter string) (discoveredSkill, bool) {
	matchers := []func(discoveredSkill) bool{
		func(s discoveredSkill) bool { return strings.EqualFold(s.Name, filter) },
		func(s discoveredSkill) bool { return normalizeDiscoveryName(s.Name) == normalizeDiscoveryName(filter) },
		// 兼容 KISS 旧行为：--skill 曾按目录名匹配 skills/<name>。
		func(s discoveredSkill) bool { return filepath.Base(s.Dir) == filter },
	}
	for _, match := range matchers {
		for _, skill := range skills {
			if match(skill) {
				return skill, true
			}
		}
	}
	return discoveredSkill{}, false
}

func discoveredSkillNames(skills []discoveredSkill) string {
	names := make([]string, 0, len(skills))
	for _, skill := range skills {
		names = append(names, skill.Name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// installNameForSkill 决定写入 store 的 skill name：显式 --name 优先，其次 frontmatter name，最后目录名。
func installNameForSkill(skill discoveredSkill, explicit string) (string, error) {
	if explicit != "" {
		return explicit, ValidateSkillName(explicit)
	}
	if ValidateSkillName(skill.Name) == nil {
		return skill.Name, nil
	}
	base := filepath.Base(skill.Dir)
	if ValidateSkillName(base) == nil {
		return base, nil
	}
	return "", fmt.Errorf("cannot infer a valid skill name from %q; use --name", skill.Name)
}

// skillFrontmatter 是发现阶段需要的 SKILL.md frontmatter 子集。
type skillFrontmatter struct {
	Name        string
	Description string
	Internal    bool
}

// parseSkillFrontmatter 解析 SKILL.md 开头的 YAML frontmatter 中 name、description 和 metadata.internal。
// 这里只实现 skill 生态实际使用的 YAML 子集，避免为发现逻辑引入完整 YAML 依赖。
func parseSkillFrontmatter(content string) skillFrontmatter {
	content = strings.TrimPrefix(content, "\uFEFF")
	scanner := bufio.NewScanner(strings.NewReader(content))
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	if !scanner.Scan() || strings.TrimRight(scanner.Text(), "\r") != "---" {
		return skillFrontmatter{}
	}
	var lines []string
	closed := false
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if line == "---" {
			closed = true
			break
		}
		lines = append(lines, line)
	}
	if !closed {
		return skillFrontmatter{}
	}

	var fm skillFrontmatter
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if line == "" || line[0] == ' ' || line[0] == '\t' || line[0] == '#' {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		// 收集该 key 下的缩进续行（block scalar 或嵌套 mapping）。
		var block []string
		for i+1 < len(lines) && (lines[i+1] == "" || lines[i+1][0] == ' ' || lines[i+1][0] == '\t') {
			i++
			block = append(block, lines[i])
		}
		switch key {
		case "name":
			fm.Name = yamlScalarValue(value, block)
		case "description":
			fm.Description = yamlScalarValue(value, block)
		case "metadata":
			fm.Internal = yamlMetadataInternal(value, block)
		}
	}
	return fm
}

func yamlScalarValue(value string, block []string) string {
	if strings.HasPrefix(value, "|") || strings.HasPrefix(value, ">") {
		var parts []string
		for _, line := range block {
			if trimmed := strings.TrimSpace(line); trimmed != "" {
				parts = append(parts, trimmed)
			}
		}
		return strings.Join(parts, " ")
	}
	value = unquoteYAMLScalar(value)
	// plain scalar 可以折行。
	for _, line := range block {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			value += " " + trimmed
		}
	}
	return strings.TrimSpace(value)
}

func unquoteYAMLScalar(value string) string {
	if len(value) >= 2 {
		if value[0] == '"' && value[len(value)-1] == '"' {
			return strings.ReplaceAll(value[1:len(value)-1], `\"`, `"`)
		}
		if value[0] == '\'' && value[len(value)-1] == '\'' {
			return strings.ReplaceAll(value[1:len(value)-1], "''", "'")
		}
	}
	if idx := strings.Index(value, " #"); idx >= 0 {
		value = value[:idx]
	}
	return strings.TrimSpace(value)
}

func yamlMetadataInternal(value string, block []string) bool {
	if strings.HasPrefix(value, "{") && strings.HasSuffix(value, "}") {
		for _, pair := range strings.Split(strings.Trim(value, "{}"), ",") {
			k, v, ok := strings.Cut(pair, ":")
			if ok && unquoteYAMLScalar(strings.TrimSpace(k)) == "internal" && isYAMLTrue(v) {
				return true
			}
		}
		return false
	}
	if value != "" {
		return false
	}
	indent := -1
	for _, line := range block {
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		lineIndent := len(line) - len(trimmed)
		if indent < 0 {
			indent = lineIndent
		}
		// 只看 metadata 的直接子键。
		if lineIndent != indent {
			continue
		}
		k, v, ok := strings.Cut(trimmed, ":")
		if ok && unquoteYAMLScalar(strings.TrimSpace(k)) == "internal" && isYAMLTrue(v) {
			return true
		}
	}
	return false
}

// isYAMLTrue 只接受 YAML 布尔 true；npx skills 要求 metadata.internal === true，字符串 "true" 不算。
func isYAMLTrue(value string) bool {
	raw := strings.TrimSpace(value)
	if idx := strings.Index(raw, " #"); idx >= 0 {
		raw = strings.TrimSpace(raw[:idx])
	}
	return raw == "true" || raw == "True" || raw == "TRUE"
}
