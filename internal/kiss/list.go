package kiss

import (
	"fmt"
	"io"
	"os"
)

func ListSkills(paths Paths, out io.Writer) error {
	if err := paths.EnsureBase(); err != nil {
		return err
	}
	items, err := ListSkillMetadata(paths)
	if err != nil {
		return err
	}
	for _, item := range items {
		fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", item.Name, item.Version, item.Source.Kind, item.FullName)
	}
	return nil
}

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
	lockErr := DeleteRegistryLockEntry(paths, name)
	if err := DeleteSkillMetadata(paths, name); err != nil {
		return err
	}
	return lockErr
}

func Doctor(paths Paths, out io.Writer) error {
	if err := paths.EnsureBase(); err != nil {
		return err
	}
	fmt.Fprintf(out, "KISS version: %s\n", Version)
	fmt.Fprintf(out, "KISS_HOME: %s\n", paths.Home)
	fmt.Fprintf(out, "Skills dir: %s\n", paths.Skills)
	fmt.Fprintf(out, "Metadata DB: %s\n", paths.MetadataDB)
	fmt.Fprintln(out, "Status: ok")
	return nil
}
