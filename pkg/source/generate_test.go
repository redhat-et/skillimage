package source_test

import (
	"testing"

	"github.com/redhat-et/skillimage/pkg/source"
)

func TestGenerateSkillCardFromFrontmatter(t *testing.T) {
	dir := t.TempDir()
	content := "---\nname: code-review\ndescription: Reviews code for quality and bugs.\nlicense: Apache-2.0\ncompatibility: claude-3.5-sonnet\nmetadata:\n  author: anthropic\n  version: \"2.0\"\n---\nYou are a code review assistant.\n"
	writeSkillMD(t, dir, content)

	sc, err := source.GenerateSkillCard(dir, "https://github.com/anthropics/skills.git", "anthropics")
	if err != nil {
		t.Fatalf("GenerateSkillCard: %v", err)
	}

	if sc.APIVersion != "skillimage.io/v1alpha1" {
		t.Errorf("APIVersion = %q", sc.APIVersion)
	}
	if sc.Kind != "SkillCard" {
		t.Errorf("Kind = %q", sc.Kind)
	}
	if sc.Metadata.Name != "code-review" {
		t.Errorf("Name = %q, want code-review", sc.Metadata.Name)
	}
	if sc.Metadata.Description != "Reviews code for quality and bugs." {
		t.Errorf("Description = %q", sc.Metadata.Description)
	}
	if sc.Metadata.Version != "2.0.0" {
		t.Errorf("Version = %q, want 2.0.0", sc.Metadata.Version)
	}
	if sc.Metadata.License != "Apache-2.0" {
		t.Errorf("License = %q", sc.Metadata.License)
	}
	if sc.Metadata.Compatibility != "claude-3.5-sonnet" {
		t.Errorf("Compatibility = %q", sc.Metadata.Compatibility)
	}
	if sc.Metadata.Namespace != "anthropics" {
		t.Errorf("Namespace = %q, want anthropics", sc.Metadata.Namespace)
	}
	if len(sc.Metadata.Authors) != 1 || sc.Metadata.Authors[0].Name != "anthropic" {
		t.Errorf("Authors = %+v", sc.Metadata.Authors)
	}
	if sc.Spec == nil || sc.Spec.Prompt != "SKILL.md" {
		t.Errorf("Spec.Prompt = %v", sc.Spec)
	}
}

func TestGenerateSkillCardFallbacks(t *testing.T) {
	dir := t.TempDir()
	writeSkillMD(t, dir, "You are a helpful assistant. This does many things.")

	sc, err := source.GenerateSkillCard(dir, "https://github.com/acme/tools.git", "acme")
	if err != nil {
		t.Fatalf("GenerateSkillCard: %v", err)
	}

	if sc.Metadata.Name == "" {
		t.Error("Name should not be empty")
	}
	if sc.Metadata.Description != "You are a helpful assistant." {
		t.Errorf("Description = %q, want first sentence", sc.Metadata.Description)
	}
	if sc.Metadata.Version != "0.1.0" {
		t.Errorf("Version = %q, want 0.1.0", sc.Metadata.Version)
	}
	if sc.Metadata.Namespace != "acme" {
		t.Errorf("Namespace = %q, want acme", sc.Metadata.Namespace)
	}
}

func TestGenerateSkillCardColonSeparatedName(t *testing.T) {
	dir := t.TempDir()
	writeSkillMD(t, dir, "---\nname: agnosticv:catalog-builder\ndescription: Builds catalogs.\n---\nContent.")

	sc, err := source.GenerateSkillCard(dir, "https://github.com/rhpds/rhdp-skills.git", "rhpds")
	if err != nil {
		t.Fatalf("GenerateSkillCard: %v", err)
	}
	if sc.Metadata.Name != "catalog-builder" {
		t.Errorf("Name = %q, want catalog-builder", sc.Metadata.Name)
	}
	if sc.Metadata.Namespace != "agnosticv" {
		t.Errorf("Namespace = %q, want agnosticv (from colon prefix)", sc.Metadata.Namespace)
	}
}

func TestGenerateSkillCardFrontmatterNamespace(t *testing.T) {
	dir := t.TempDir()
	writeSkillMD(t, dir, "---\nname: resume-reviewer\ndescription: Reviews resumes.\nmetadata:\n  namespace: business/hr\n---\nContent.")

	sc, err := source.GenerateSkillCard(dir, "https://github.com/acme/skills.git", "acme")
	if err != nil {
		t.Fatalf("GenerateSkillCard: %v", err)
	}
	if sc.Metadata.Name != "resume-reviewer" {
		t.Errorf("Name = %q, want resume-reviewer", sc.Metadata.Name)
	}
	if sc.Metadata.Namespace != "business/hr" {
		t.Errorf("Namespace = %q, want business/hr", sc.Metadata.Namespace)
	}
}

func TestGenerateSkillCardNamespaceOverridesColon(t *testing.T) {
	dir := t.TempDir()
	writeSkillMD(t, dir, "---\nname: old-group:my-skill\nmetadata:\n  namespace: correct-ns\n---\nContent.")

	sc, err := source.GenerateSkillCard(dir, "https://github.com/org/repo.git", "org")
	if err != nil {
		t.Fatalf("GenerateSkillCard: %v", err)
	}
	if sc.Metadata.Name != "my-skill" {
		t.Errorf("Name = %q, want my-skill", sc.Metadata.Name)
	}
	if sc.Metadata.Namespace != "correct-ns" {
		t.Errorf("Namespace = %q, want correct-ns (explicit overrides colon)", sc.Metadata.Namespace)
	}
}

func TestGenerateSkillCardMalformedFrontmatter(t *testing.T) {
	dir := t.TempDir()
	writeSkillMD(t, dir, "---\nbad: [yaml: {{\n---\nContent here.")

	sc, err := source.GenerateSkillCard(dir, "https://github.com/org/repo.git", "org")
	if err != nil {
		t.Fatalf("GenerateSkillCard: %v (should not fail on bad frontmatter)", err)
	}
	if sc.Metadata.Description != "Content here." {
		t.Errorf("Description = %q", sc.Metadata.Description)
	}
}
