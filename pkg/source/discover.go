package source

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type DiscoveredSkill struct {
	Dir  string
	Name string
}

func Discover(dir string, filter string) ([]DiscoveredSkill, error) {
	if filter != "" {
		if _, err := filepath.Match(filter, "test"); err != nil {
			return nil, fmt.Errorf("invalid --filter pattern %q: %w", filter, err)
		}
	}

	if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err == nil {
		name := resolveSkillName(dir)
		if filter != "" {
			if matched, _ := filepath.Match(filter, name); !matched {
				return nil, fmt.Errorf("no skills matching %q in %s", filter, dir)
			}
		}
		return []DiscoveredSkill{{Dir: dir, Name: name}}, nil
	}

	var skills []DiscoveredSkill
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
			return fs.SkipDir
		}
		if path == dir {
			return nil
		}
		if _, statErr := os.Stat(filepath.Join(path, "SKILL.md")); statErr != nil {
			return nil
		}
		name := resolveSkillName(path)
		if filter != "" {
			if matched, _ := filepath.Match(filter, name); !matched {
				return fs.SkipDir
			}
		}
		skills = append(skills, DiscoveredSkill{Dir: path, Name: name})
		return fs.SkipDir
	})
	if err != nil {
		return nil, fmt.Errorf("walking %s: %w", dir, err)
	}

	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })

	if len(skills) == 0 {
		if filter != "" {
			return nil, fmt.Errorf("no skills matching %q in %s", filter, dir)
		}
		return nil, fmt.Errorf("no skills found in %s", dir)
	}

	return skills, nil
}

func resolveSkillName(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		return filepath.Base(dir)
	}
	fm := parseFrontmatterRaw(data)
	if name, ok := fm["name"].(string); ok && name != "" {
		return name
	}
	return filepath.Base(dir)
}
