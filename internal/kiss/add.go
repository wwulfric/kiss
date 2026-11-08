package kiss

import (
	"fmt"
	"strings"
)

// AddOptions 控制显式 add 命令的 skill 选择和替换行为，选择语义对齐 npx skills add。
type AddOptions struct {
	// Skills 对应 --skill（可多个）：按 SKILL.md frontmatter name（兼容目录名）选择；"*" 表示全部。
	Skills []string
	// All 对应 --all：安装 source 中发现的全部 skill。
	All bool
	// FullDepth 对应 --full-depth：根目录是 skill 时也继续搜索子目录。
	FullDepth bool
	// Force 允许用异源 skill 覆盖已安装的同名 skill。
	Force bool
}

// SourceSkill 是 kiss add --list 展示的 source 内 skill。
type SourceSkill struct {
	Name        string
	Description string
	Path        string
}

// skillSource 是已打开的 source：本地目录，或下载解包后的远程仓库。
type skillSource struct {
	root    string
	subpath string
	cleanup func()
	// check 在安装前校验选中的 skill（例如远程 archive 中的 link）。
	check func(discoveredSkill) error
	// metadata 生成写入 store 的来源信息，记录实际选中的 skill 路径。
	metadata func(discoveredSkill) (SourceMetadata, string)
}

func openSkillSource(paths Paths, source string) (*skillSource, error) {
	if strings.HasPrefix(source, "github:") {
		return openRemoteSkillSource(paths, source)
	}
	return openLocalSkillSource(source)
}

func (s *skillSource) discover(options discoveryOptions) ([]discoveredSkill, error) {
	return discoverSkills(s.root, s.subpath, options)
}

// AddSkills 从 source 安装一个或多个 skill，返回写入 store 的本地名称。
// source 是本地路径、file:// URL，或 CLI 规范化后的 github:owner/repo[/path][#ref]。
// 未指定 --skill/--all 时：source 只有一个 skill 则直接安装；有多个则报错列出候选，不进入交互选择。
func AddSkills(paths Paths, source, name string, options AddOptions) ([]string, error) {
	if name != "" {
		if err := ValidateSkillName(name); err != nil {
			return nil, err
		}
	}
	opened, err := openSkillSource(paths, source)
	if err != nil {
		return nil, err
	}
	defer opened.cleanup()
	selection := skillSelection{Filters: options.Skills, All: options.All, PreferredName: name}
	skills, err := opened.discover(discoveryOptions{IncludeInternal: selection.includesInternal(), FullDepth: options.FullDepth})
	if err != nil {
		return nil, err
	}
	selected, err := selectDiscoveredSkills(skills, selection)
	if err != nil {
		return nil, err
	}
	if name != "" && len(selected) > 1 {
		return nil, fmt.Errorf("--name can only be used when exactly one skill is selected (selected %d)", len(selected))
	}
	if err := paths.EnsureBase(); err != nil {
		return nil, err
	}

	type plannedInstall struct {
		skill    discoveredSkill
		name     string
		source   SourceMetadata
		fullName string
	}
	// 先完成全部校验，再开始写 store，避免多 skill 安装在中途因可预见的错误留下半成品。
	plans := make([]plannedInstall, 0, len(selected))
	seenNames := map[string]string{}
	for _, skill := range selected {
		installName, err := installNameForSkill(skill, name)
		if err != nil {
			return nil, err
		}
		if previous, ok := seenNames[installName]; ok {
			return nil, fmt.Errorf("skills %q and %q would both be installed as %q", previous, skill.RelPath, installName)
		}
		seenNames[installName] = skill.RelPath
		if err := opened.check(skill); err != nil {
			return nil, err
		}
		sourceMetadata, fullName := opened.metadata(skill)
		if err := ensureAddSourceAllowed(paths, installName, sourceMetadata, options.Force); err != nil {
			return nil, err
		}
		plans = append(plans, plannedInstall{skill: skill, name: installName, source: sourceMetadata, fullName: fullName})
	}
	installed := make([]string, 0, len(plans))
	for _, plan := range plans {
		if err := installSkillFromDir(paths, plan.skill.Dir, plan.name, plan.fullName, plan.source); err != nil {
			if len(installed) > 0 {
				return installed, fmt.Errorf("install %s failed after installing %s: %w", plan.name, strings.Join(installed, ", "), err)
			}
			return nil, err
		}
		installed = append(installed, plan.name)
	}
	return installed, nil
}

// ListSourceSkills 列出 source 中可安装的 skill，不写入 store；对应 npx skills add --list。
func ListSourceSkills(paths Paths, source string, fullDepth bool) ([]SourceSkill, error) {
	opened, err := openSkillSource(paths, source)
	if err != nil {
		return nil, err
	}
	defer opened.cleanup()
	skills, err := opened.discover(discoveryOptions{FullDepth: fullDepth})
	if err != nil {
		return nil, err
	}
	result := make([]SourceSkill, 0, len(skills))
	for _, skill := range skills {
		result = append(result, SourceSkill{Name: skill.Name, Description: skill.Description, Path: skill.RelPath})
	}
	return result, nil
}

// resolvedSourceSkill 是从已记录 source 中重新定位到的单个 skill，供 update 预览使用。
type resolvedSourceSkill struct {
	Skill    discoveredSkill
	Source   SourceMetadata
	FullName string
}

// resolveSourceSkillForUpdate 打开已记录的 source，并按已安装名称定位原 skill。
func resolveSourceSkillForUpdate(paths Paths, source, name string) (resolvedSourceSkill, func(), error) {
	opened, err := openSkillSource(paths, source)
	if err != nil {
		return resolvedSourceSkill{}, func() {}, err
	}
	fail := func(err error) (resolvedSourceSkill, func(), error) {
		opened.cleanup()
		return resolvedSourceSkill{}, func() {}, err
	}
	skills, err := opened.discover(discoveryOptions{IncludeInternal: true})
	if err != nil {
		return fail(err)
	}
	selected, err := selectDiscoveredSkills(skills, skillSelection{PreferredName: name})
	if err != nil {
		return fail(err)
	}
	if err := opened.check(selected[0]); err != nil {
		return fail(err)
	}
	sourceMetadata, fullName := opened.metadata(selected[0])
	return resolvedSourceSkill{Skill: selected[0], Source: sourceMetadata, FullName: fullName}, opened.cleanup, nil
}
