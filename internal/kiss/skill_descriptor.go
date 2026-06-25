package kiss

import (
	"fmt"
	"os"
	"path/filepath"
)

// LoadSkillDescriptor 读取根目录 SKILL.md 作为 KISS 当前唯一支持的 skill 入口。
func LoadSkillDescriptor(skillDir string) (SkillDescriptor, error) {
	const entry = "SKILL.md"
	if _, err := os.Stat(filepath.Join(skillDir, entry)); err != nil {
		return SkillDescriptor{}, fmt.Errorf("skill must contain SKILL.md: %w", err)
	}
	version, err := computeSkillFolderHash(skillDir)
	if err != nil {
		return SkillDescriptor{}, err
	}
	return SkillDescriptor{
		Name:    filepath.Base(skillDir),
		Version: version,
		Entry:   entry,
	}, nil
}
