package kiss

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
)

const MetadataSchemaVersion = 1

type MetadataDB struct {
	SchemaVersion int                      `json:"schema_version"`
	Skills        map[string]SkillMetadata `json:"skills"`
}

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

type SourceMetadata struct {
	Kind     string `json:"kind"`
	URI      string `json:"uri"`
	Ref      string `json:"ref"`
	Resolved string `json:"resolved"`
	SHA256   string `json:"sha256"`
}

type RunnerMetadata struct {
	Type  string `json:"type"`
	Entry string `json:"entry"`
}

func EnsureMetadataDB(paths Paths) error {
	if _, err := os.Stat(paths.MetadataDB); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	db := newMetadataDB()
	return SaveMetadataDB(paths, db)
}

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
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, paths.MetadataDB)
}

func UpsertSkillMetadata(paths Paths, metadata SkillMetadata) error {
	db, err := LoadMetadataDB(paths)
	if err != nil {
		return err
	}
	db.Skills[metadata.Name] = metadata
	return SaveMetadataDB(paths, db)
}

func DeleteSkillMetadata(paths Paths, name string) error {
	db, err := LoadMetadataDB(paths)
	if err != nil {
		return err
	}
	delete(db.Skills, name)
	return SaveMetadataDB(paths, db)
}

func GetSkillMetadata(paths Paths, name string) (SkillMetadata, bool, error) {
	db, err := LoadMetadataDB(paths)
	if err != nil {
		return SkillMetadata{}, false, err
	}
	metadata, ok := db.Skills[name]
	return metadata, ok, nil
}

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
