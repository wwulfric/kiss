package kiss

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
)

// MetadataSchemaVersion 是 skills.db.json 当前支持的 schema version。
const MetadataSchemaVersion = 1

// MetadataDB 是 KISS store 的本地 skill 索引。
type MetadataDB struct {
	SchemaVersion int                      `json:"schema_version"`
	Skills        map[string]SkillMetadata `json:"skills"`
}

// SkillMetadata 记录一个已安装 skill 的 manifest、source 和安装位置。
type SkillMetadata struct {
	Name          string         `json:"name"`
	FullName      string         `json:"full_name"`
	ManifestName  string         `json:"manifest_name"`
	Version       string         `json:"version"`
	Description   string         `json:"description,omitempty"`
	Source        SourceMetadata `json:"source"`
	Runner        RunnerMetadata `json:"runner"`
	InstalledPath string         `json:"installed_path"`
	InstalledAt   string         `json:"installed_at"`
	UpdatedAt     string         `json:"updated_at"`
	KissVersion   string         `json:"kiss_version"`
}

// SourceMetadata 记录 skill 的安装来源和可复现更新所需的信息。
type SourceMetadata struct {
	Kind     string `json:"kind"`
	URI      string `json:"uri"`
	Ref      string `json:"ref"`
	Resolved string `json:"resolved"`
	SHA256   string `json:"sha256"`
}

// RunnerMetadata 记录 KISS 当前支持的 Markdown runner 入口。
type RunnerMetadata struct {
	Type  string `json:"type"`
	Entry string `json:"entry"`
}

// EnsureMetadataDB 确保 skills.db.json 存在。
func EnsureMetadataDB(paths Paths) error {
	if _, err := os.Stat(paths.MetadataDB); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	db := newMetadataDB()
	return SaveMetadataDB(paths, db)
}

// LoadMetadataDB 读取并校验 metadata DB。
func LoadMetadataDB(paths Paths) (MetadataDB, error) {
	if err := EnsureMetadataDB(paths); err != nil {
		return MetadataDB{}, err
	}
	data, err := os.ReadFile(paths.MetadataDB)
	if err != nil {
		return MetadataDB{}, err
	}
	var db MetadataDB
	if err := json.Unmarshal(data, &db); err != nil {
		return MetadataDB{}, err
	}
	if db.SchemaVersion == 0 {
		db.SchemaVersion = MetadataSchemaVersion
	}
	if db.SchemaVersion != MetadataSchemaVersion {
		return MetadataDB{}, fmt.Errorf("unsupported metadata schema version %d", db.SchemaVersion)
	}
	if db.Skills == nil {
		db.Skills = map[string]SkillMetadata{}
	}
	return db, nil
}

// SaveMetadataDB 写入 metadata DB，并在失败时尽量保留旧文件。
func SaveMetadataDB(paths Paths, db MetadataDB) error {
	if db.SchemaVersion == 0 {
		db.SchemaVersion = MetadataSchemaVersion
	}
	if db.Skills == nil {
		db.Skills = map[string]SkillMetadata{}
	}
	data, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(paths.Home, "skills.db-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	backup := paths.MetadataDB + ".old"
	_ = os.Remove(backup)
	// 使用临时文件和旧文件备份，降低 CLI 中断时留下半截 JSON 的概率。
	if _, err := os.Stat(paths.MetadataDB); err == nil {
		if err := os.Rename(paths.MetadataDB, backup); err != nil {
			return err
		}
	}
	if err := os.Rename(tmpName, paths.MetadataDB); err != nil {
		if _, statErr := os.Stat(backup); statErr == nil {
			_ = os.Rename(backup, paths.MetadataDB)
		}
		return err
	}
	_ = os.Remove(backup)
	return nil
}

// UpsertSkillMetadata 新增或替换某个 skill 的 metadata。
func UpsertSkillMetadata(paths Paths, metadata SkillMetadata) error {
	db, err := LoadMetadataDB(paths)
	if err != nil {
		return err
	}
	db.Skills[metadata.Name] = metadata
	return SaveMetadataDB(paths, db)
}

// DeleteSkillMetadata 从 metadata DB 删除某个 skill。
func DeleteSkillMetadata(paths Paths, name string) error {
	db, err := LoadMetadataDB(paths)
	if err != nil {
		return err
	}
	delete(db.Skills, name)
	return SaveMetadataDB(paths, db)
}

// GetSkillMetadata 查询某个 skill 的 metadata。
func GetSkillMetadata(paths Paths, name string) (SkillMetadata, bool, error) {
	db, err := LoadMetadataDB(paths)
	if err != nil {
		return SkillMetadata{}, false, err
	}
	metadata, ok := db.Skills[name]
	return metadata, ok, nil
}

// ListSkillMetadata 按 skill name 排序返回所有 metadata。
func ListSkillMetadata(paths Paths) ([]SkillMetadata, error) {
	db, err := LoadMetadataDB(paths)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(db.Skills))
	for name := range db.Skills {
		names = append(names, name)
	}
	sort.Strings(names)
	items := make([]SkillMetadata, 0, len(names))
	for _, name := range names {
		items = append(items, db.Skills[name])
	}
	return items, nil
}

// WriteSkillMetadata 以 JSON 格式输出单个 skill 的 metadata。
func WriteSkillMetadata(out io.Writer, metadata SkillMetadata) error {
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(data))
	return err
}

func newMetadataDB() MetadataDB {
	return MetadataDB{SchemaVersion: MetadataSchemaVersion, Skills: map[string]SkillMetadata{}}
}
