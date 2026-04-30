package installed

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/redhat-et/skillimage/pkg/skillcard"
)

// InstalledSkill holds metadata for a skill installed to an agent directory.
type InstalledSkill struct {
	Name    string
	Version string
	Source  string
	Digest  string
	Target  string
	Path    string
}

// Scan reads agent target directories and returns metadata for each
// installed skill. targets maps target names (e.g., "claude") to
// directory paths. Directories that don't exist are silently skipped.
// Malformed skill.yaml files are skipped with a warning to stderr.
func Scan(targets map[string]string) ([]InstalledSkill, error) {
	var skills []InstalledSkill

	for target, dir := range targets {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("reading %s: %w", dir, err)
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			skillPath := filepath.Join(dir, entry.Name(), "skill.yaml")
			f, err := os.Open(skillPath)
			if err != nil {
				continue
			}

			sc, err := skillcard.Parse(f)
			_ = f.Close()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: skipping %s: %v\n", skillPath, err)
				continue
			}

			skill := InstalledSkill{
				Name:    sc.Metadata.Name,
				Version: sc.Metadata.Version,
				Target:  target,
				Path:    filepath.Join(dir, entry.Name()),
			}
			if sc.Provenance != nil {
				skill.Source = sc.Provenance.Source
				skill.Digest = sc.Provenance.Commit
			}

			skills = append(skills, skill)
		}
	}

	return skills, nil
}
