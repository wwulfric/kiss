package kiss

import (
	"os"
	"path/filepath"
)

// Paths 集中保存 KISS 自己管理的目录和元数据文件位置。
type Paths struct {
	Home       string
	Skills     string
	MetadataDB string
	Cache      string
	Logs       string
}

// ResolveHome 按显式参数、KISS_HOME、XDG_DATA_HOME 和用户 home 的顺序解析 KISS_HOME。
func ResolveHome(explicit string) (string, error) {
	if explicit != "" {
		return filepath.Abs(explicit)
	}
	if env := os.Getenv("KISS_HOME"); env != "" {
		return filepath.Abs(env)
	}
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Abs(filepath.Join(xdg, "kiss"))
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Abs(filepath.Join(home, ".local", "share", "kiss"))
}

// NewPaths 根据 KISS_HOME 构造 store、cache、logs 和 metadata 路径。
func NewPaths(explicit string) (Paths, error) {
	home, err := ResolveHome(explicit)
	if err != nil {
		return Paths{}, err
	}
	return Paths{
		Home:       home,
		Skills:     filepath.Join(home, "skills"),
		MetadataDB: filepath.Join(home, "skills.db.json"),
		Cache:      filepath.Join(home, "cache"),
		Logs:       filepath.Join(home, "logs"),
	}, nil
}

// EnsureBase 创建 KISS 运行所需的基础目录和 metadata DB。
func (p Paths) EnsureBase() error {
	for _, dir := range []string{p.Home, p.Skills, filepath.Join(p.Cache, "downloads"), p.Logs} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return EnsureMetadataDB(p)
}

// SkillDir 返回某个已安装 skill 在 KISS store 中的目录。
func (p Paths) SkillDir(name string) string {
	return filepath.Join(p.Skills, name)
}
