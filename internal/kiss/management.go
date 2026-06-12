package kiss

import (
	"fmt"
	"io"
	"os"
)

// ListSkills writes the installed skill list in a compact tab-separated format.
func ListSkills(paths Paths, out io.Writer) error {
	if err := paths.EnsureBase(); err != nil {
		return err
	}
	items, err := ListSkillMetadata(paths)
	if err != nil {
		return err
	}
	writer := errWriter{out: out}
	for _, item := range items {
		writer.printf("%s\t%s\t%s\t%s\n", item.Name, item.Version, item.Source.Kind, item.FullName)
	}
	return writer.err
}

// ShowSkill writes one installed skill's metadata as JSON.
func ShowSkill(paths Paths, name string, out io.Writer) error {
	if err := ValidateSkillName(name); err != nil {
		return err
	}
	if err := paths.EnsureBase(); err != nil {
		return err
	}
	metadata, ok, err := GetSkillMetadata(paths, name)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("skill %q is not installed; run kiss add <source> --name %s first", name, name)
	}
	return WriteSkillMetadata(out, metadata)
}

// RemoveSkill deletes an installed skill and removes its metadata entry.
func RemoveSkill(paths Paths, name string) error {
	if err := ValidateSkillName(name); err != nil {
		return err
	}
	if err := paths.EnsureBase(); err != nil {
		return err
	}
	if err := os.RemoveAll(paths.SkillDir(name)); err != nil {
		return err
	}
	if err := DeleteSkillMetadata(paths, name); err != nil {
		return err
	}
	return nil
}

// Doctor checks and prints the local KISS store configuration.
func Doctor(paths Paths, out io.Writer) error {
	if err := paths.EnsureBase(); err != nil {
		return err
	}
	writer := errWriter{out: out}
	writer.printf("KISS version: %s\n", Version)
	writer.printf("KISS_HOME: %s\n", paths.Home)
	writer.printf("Skills dir: %s\n", paths.Skills)
	writer.printf("Metadata DB: %s\n", paths.MetadataDB)
	writer.println("Status: ok")
	return writer.err
}
