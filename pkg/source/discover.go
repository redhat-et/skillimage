package source

import (
	"fmt"
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
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading directory %s: %w", dir, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		subDir := filepath.Join(dir, entry.Name())
		if _, err := os.Stat(filepath.Join(subDir, "SKILL.md")); err != nil {
			nested, _ := discoverNested(subDir, filter)
			skills = append(skills, nested...)
			continue
		}

		name := resolveSkillName(subDir)
		if filter != "" {
			if matched, _ := filepath.Match(filter, name); !matched {
				continue
			}
		}
		skills = append(skills, DiscoveredSkill{Dir: subDir, Name: name})
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

func discoverNested(dir string, filter string) ([]DiscoveredSkill, error) {
	var skills []DiscoveredSkill
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		subDir := filepath.Join(dir, entry.Name())
		if _, err := os.Stat(filepath.Join(subDir, "SKILL.md")); err != nil {
			continue
		}
		name := resolveSkillName(subDir)
		if filter != "" {
			if matched, _ := filepath.Match(filter, name); !matched {
				continue
			}
		}
		skills = append(skills, DiscoveredSkill{Dir: subDir, Name: name})
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
