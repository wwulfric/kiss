package kiss

import (
	"os"
	"path/filepath"
)

type Paths struct {
	Home       string
	Skills     string
	MetadataDB string
	Cache      string
	Logs       string
}

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

func (p Paths) EnsureBase() error {
	for _, dir := range []string{p.Home, p.Skills, filepath.Join(p.Cache, "downloads"), filepath.Join(p.Cache, "git"), p.Logs} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return EnsureMetadataDB(p)
}

func (p Paths) SkillDir(name string) string {
	return filepath.Join(p.Skills, name)
}
