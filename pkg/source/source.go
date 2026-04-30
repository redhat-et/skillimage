package source

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/redhat-et/skillimage/pkg/skillcard"
)

func IsRemote(input string) bool {
	return strings.HasPrefix(input, "https://") || strings.HasPrefix(input, "http://")
}

func OrgFromCloneURL(cloneURL string) string {
	u, err := url.Parse(cloneURL)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) >= 2 {
		return parts[0]
	}
	return ""
}

type ResolveResult struct {
	Skills    []ResolvedSkill
	Cleanup   func()
	SourceURL string
}

type ResolvedSkill struct {
	Dir       string
	Name      string
	SkillCard *skillcard.SkillCard
}

func Resolve(ctx context.Context, input string, ref string, filter string) (*ResolveResult, error) {
	if !IsRemote(input) {
		return nil, fmt.Errorf("not a remote source: %s", input)
	}

	src, err := ParseGitURL(input)
	if err != nil {
		return nil, err
	}

	cloneResult, err := Clone(ctx, src, CloneOptions{RefOverride: ref})
	if err != nil {
		return nil, err
	}

	discovered, err := Discover(cloneResult.Dir, filter)
	if err != nil {
		cloneResult.Cleanup()
		return nil, err
	}

	org := OrgFromCloneURL(src.CloneURL)

	var skills []ResolvedSkill
	for _, d := range discovered {
		var sc *skillcard.SkillCard

		if hasSkillYAML(d.Dir) {
			parsed, parseErr := parseSkillYAML(d.Dir)
			if parseErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: skipping %s: %v\n", d.Name, parseErr)
				continue
			}
			sc = parsed
		} else {
			generated, genErr := GenerateSkillCard(d.Dir, src.CloneURL, org)
			if genErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: skipping %s: %v\n", d.Name, genErr)
				continue
			}
			if generated.Metadata.Namespace == "" || generated.Metadata.Name == "" {
				fmt.Fprintf(os.Stderr, "Warning: skipping %s: generated SkillCard missing namespace or name\n", d.Name)
				continue
			}
			sc = generated
		}

		relPath := relativeToClone(cloneResult.Dir, d.Dir, src.SubPath)
		if sc.Provenance == nil {
			sc.Provenance = &skillcard.Provenance{}
		}
		sc.Provenance.Source = src.CloneURL
		sc.Provenance.Commit = cloneResult.CommitSHA
		sc.Provenance.Path = relPath

		skills = append(skills, ResolvedSkill{Dir: d.Dir, Name: d.Name, SkillCard: sc})
	}

	if len(skills) == 0 {
		cloneResult.Cleanup()
		return nil, fmt.Errorf("no skills could be resolved from %s", input)
	}

	return &ResolveResult{
		Skills:    skills,
		Cleanup:   cloneResult.Cleanup,
		SourceURL: input,
	}, nil
}

func hasSkillYAML(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "skill.yaml"))
	return err == nil
}

func parseSkillYAML(dir string) (*skillcard.SkillCard, error) {
	f, err := os.Open(filepath.Join(dir, "skill.yaml"))
	if err != nil {
		return nil, fmt.Errorf("opening skill.yaml: %w", err)
	}
	defer func() { _ = f.Close() }()
	return skillcard.Parse(f)
}

func relativeToClone(cloneDir, skillDir, subPath string) string {
	rel, err := filepath.Rel(cloneDir, skillDir)
	if err != nil {
		return filepath.Base(skillDir)
	}
	if subPath != "" {
		return filepath.ToSlash(filepath.Join(subPath, rel))
	}
	return filepath.ToSlash(rel)
}
