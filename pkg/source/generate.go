package source

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/redhat-et/skillimage/pkg/skillcard"
	"gopkg.in/yaml.v3"
)

func GenerateSkillCard(skillDir, cloneURL, orgFallback string) (*skillcard.SkillCard, error) {
	data, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		return nil, fmt.Errorf("reading SKILL.md: %w", err)
	}

	fm := parseFrontmatterRaw(data)
	body := stripFrontmatterStr(string(data))

	sc := &skillcard.SkillCard{
		APIVersion: "skillimage.io/v1alpha1",
		Kind:       "SkillCard",
		Metadata: skillcard.Metadata{
			Name:          stringFromMap(fm, "name", filepath.Base(skillDir)),
			Namespace:     orgFallback,
			Version:       normalizeVersion(stringFromMapNested(fm, "metadata", "version", "0.1.0")),
			Description:   stringFromMap(fm, "description", firstSentence(body)),
			License:       stringFromMap(fm, "license", ""),
			Compatibility: stringFromMap(fm, "compatibility", ""),
		},
		Spec: &skillcard.Spec{
			Prompt: "SKILL.md",
		},
	}

	author := stringFromMapNested(fm, "metadata", "author", "")
	if author == "" {
		author = orgFallback
	}
	if author != "" {
		sc.Metadata.Authors = []skillcard.Author{{Name: author}}
	}

	return sc, nil
}

func parseFrontmatterRaw(data []byte) map[string]any {
	s := string(data)
	if !strings.HasPrefix(s, "---") {
		return nil
	}
	end := strings.Index(s[3:], "\n---")
	if end < 0 {
		return nil
	}
	fmStr := s[4 : 3+end]
	var m map[string]any
	if err := yaml.Unmarshal([]byte(fmStr), &m); err != nil {
		return nil
	}
	return m
}

func stripFrontmatterStr(s string) string {
	if !strings.HasPrefix(s, "---") {
		return s
	}
	end := strings.Index(s[3:], "\n---")
	if end < 0 {
		return s
	}
	return strings.TrimSpace(s[3+end+4:])
}

func stringFromMap(m map[string]any, key, fallback string) string {
	if m == nil {
		return fallback
	}
	if v, ok := m[key].(string); ok && v != "" {
		return v
	}
	return fallback
}

func stringFromMapNested(m map[string]any, outer, inner, fallback string) string {
	if m == nil {
		return fallback
	}
	sub, ok := m[outer].(map[string]any)
	if !ok {
		return fallback
	}
	if v, ok := sub[inner].(string); ok && v != "" {
		return v
	}
	return fallback
}

func firstSentence(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	if before, _, found := strings.Cut(body, "."); found {
		return before + "."
	}
	if before, _, found := strings.Cut(body, "\n"); found {
		return strings.TrimSpace(before)
	}
	return body
}

func normalizeVersion(v string) string {
	var suffix string
	if idx := strings.IndexAny(v, "-+"); idx >= 0 {
		suffix = v[idx:]
		v = v[:idx]
	}
	parts := strings.Split(v, ".")
	for len(parts) < 3 {
		parts = append(parts, "0")
	}
	return strings.Join(parts[:3], ".") + suffix
}
