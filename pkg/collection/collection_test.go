package collection_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/redhat-et/skillimage/pkg/collection"
)

func TestParseValid(t *testing.T) {
	input := `apiVersion: skillimage.io/v1alpha1
kind: SkillCollection
metadata:
  name: hr-skills
  version: 1.0.0
  description: Skills for HR document processing
skills:
  - name: document-summarizer
    image: quay.io/skillimage/business/document-summarizer:1.0.0
  - name: document-reviewer
    image: ghcr.io/acme/document-reviewer:2.1.0
`
	col, err := collection.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if col.Metadata.Name != "hr-skills" {
		t.Errorf("name = %q, want %q", col.Metadata.Name, "hr-skills")
	}
	if col.Metadata.Version != "1.0.0" {
		t.Errorf("version = %q, want %q", col.Metadata.Version, "1.0.0")
	}
	if len(col.Skills) != 2 {
		t.Fatalf("skills count = %d, want 2", len(col.Skills))
	}
	if col.Skills[0].Name != "document-summarizer" {
		t.Errorf("skill[0].name = %q, want %q", col.Skills[0].Name, "document-summarizer")
	}
	if col.Skills[1].Image != "ghcr.io/acme/document-reviewer:2.1.0" {
		t.Errorf("skill[1].image = %q", col.Skills[1].Image)
	}
}

func TestParseInvalidKind(t *testing.T) {
	input := `apiVersion: skillimage.io/v1alpha1
kind: SkillCard
metadata:
  name: test
  version: 1.0.0
skills:
  - name: s1
    image: quay.io/org/s1:1.0.0
`
	_, err := collection.Parse(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for wrong kind")
	}
}

func TestValidateDuplicateNames(t *testing.T) {
	col := &collection.SkillCollection{
		APIVersion: "skillimage.io/v1alpha1",
		Kind:       "SkillCollection",
		Metadata:   collection.Metadata{Name: "test", Version: "1.0.0"},
		Skills: []collection.SkillRef{
			{Name: "skill-a", Image: "quay.io/org/skill-a:1.0.0"},
			{Name: "skill-a", Image: "ghcr.io/org/skill-a:2.0.0"},
		},
	}
	errs := collection.Validate(col)
	if len(errs) == 0 {
		t.Fatal("expected validation error for duplicate names")
	}
}

func TestValidateMissingFields(t *testing.T) {
	col := &collection.SkillCollection{
		APIVersion: "skillimage.io/v1alpha1",
		Kind:       "SkillCollection",
		Metadata:   collection.Metadata{Name: "test", Version: "1.0.0"},
		Skills: []collection.SkillRef{
			{Name: "", Image: "quay.io/org/s:1.0.0"},
			{Name: "s2", Image: ""},
		},
	}
	errs := collection.Validate(col)
	if len(errs) < 2 {
		t.Errorf("expected at least 2 errors, got %d", len(errs))
	}
}

func TestValidateEmptySkills(t *testing.T) {
	col := &collection.SkillCollection{
		APIVersion: "skillimage.io/v1alpha1",
		Kind:       "SkillCollection",
		Metadata:   collection.Metadata{Name: "test", Version: "1.0.0"},
		Skills:     nil,
	}
	errs := collection.Validate(col)
	if len(errs) == 0 {
		t.Fatal("expected error for empty skills list")
	}
}

func TestParseFile(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/collection.yaml"
	content := []byte(`apiVersion: skillimage.io/v1alpha1
kind: SkillCollection
metadata:
  name: test-col
  version: 1.0.0
skills:
  - name: s1
    image: quay.io/org/s1:1.0.0
`)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	col, err := collection.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if col.Metadata.Name != "test-col" {
		t.Errorf("name = %q, want %q", col.Metadata.Name, "test-col")
	}
}

func TestGeneratePodmanVolumes(t *testing.T) {
	col := &collection.SkillCollection{
		Skills: []collection.SkillRef{
			{Name: "skill-a", Image: "quay.io/org/skill-a:1.0.0"},
			{Name: "skill-b", Image: "ghcr.io/org/skill-b:2.0.0"},
		},
	}
	var buf bytes.Buffer
	collection.GeneratePodmanVolumes(&buf, col, "/skills")
	output := buf.String()
	if !strings.Contains(output, "podman pull quay.io/org/skill-a:1.0.0") {
		t.Errorf("missing pull command for skill-a")
	}
	if !strings.Contains(output, "podman volume create --driver image") {
		t.Errorf("missing volume create command")
	}
	if !strings.Contains(output, "-v skill-a:/skills/skill-a:ro") {
		t.Errorf("missing mount hint for skill-a in output:\n%s", output)
	}
}

func TestGenerateKubeYAML(t *testing.T) {
	col := &collection.SkillCollection{
		Skills: []collection.SkillRef{
			{Name: "skill-a", Image: "quay.io/org/skill-a:1.0.0"},
			{Name: "skill-b", Image: "ghcr.io/org/skill-b:2.0.0"},
		},
	}
	var buf bytes.Buffer
	collection.GenerateKubeYAML(&buf, col, "/skills")
	output := buf.String()
	if !strings.Contains(output, "reference: quay.io/org/skill-a:1.0.0") {
		t.Errorf("missing image reference for skill-a")
	}
	if !strings.Contains(output, "mountPath: /skills/skill-a") {
		t.Errorf("missing mountPath for skill-a")
	}
	if !strings.Contains(output, "readOnly: true") {
		t.Errorf("missing readOnly")
	}
}

func TestGenerateKubeYAMLCustomMountRoot(t *testing.T) {
	col := &collection.SkillCollection{
		Skills: []collection.SkillRef{
			{Name: "skill-a", Image: "quay.io/org/skill-a:1.0.0"},
		},
	}
	var buf bytes.Buffer
	collection.GenerateKubeYAML(&buf, col, "/agent/skills")
	output := buf.String()
	if !strings.Contains(output, "mountPath: /agent/skills/skill-a") {
		t.Errorf("expected custom mount root, got:\n%s", output)
	}
}

func TestParseSourceField(t *testing.T) {
	input := `apiVersion: skillimage.io/v1alpha1
kind: SkillCollection
metadata:
  name: dev-skills
  version: 0.1.0
skills:
  - source: https://github.com/myorg/skills/tree/main/code-reviewer
  - name: stable-tool
    image: quay.io/myorg/stable-tool:1.0.0
`
	col, err := collection.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if col.Skills[0].Source != "https://github.com/myorg/skills/tree/main/code-reviewer" {
		t.Errorf("skill[0].source = %q", col.Skills[0].Source)
	}
	if col.Skills[1].Image != "quay.io/myorg/stable-tool:1.0.0" {
		t.Errorf("skill[1].image = %q", col.Skills[1].Image)
	}
}

func TestValidateSourceAndImageExclusive(t *testing.T) {
	col := &collection.SkillCollection{
		APIVersion: "skillimage.io/v1alpha1",
		Kind:       "SkillCollection",
		Metadata:   collection.Metadata{Name: "test", Version: "1.0.0"},
		Skills: []collection.SkillRef{
			{Name: "both", Image: "quay.io/org/s:1.0.0", Source: "https://github.com/org/repo/tree/main/s"},
		},
	}
	errs := collection.Validate(col)
	found := false
	for _, e := range errs {
		if strings.Contains(e, "mutually exclusive") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected mutually exclusive error, got: %v", errs)
	}
}

func TestValidateNeitherImageNorSource(t *testing.T) {
	col := &collection.SkillCollection{
		APIVersion: "skillimage.io/v1alpha1",
		Kind:       "SkillCollection",
		Metadata:   collection.Metadata{Name: "test", Version: "1.0.0"},
		Skills: []collection.SkillRef{
			{Name: "empty"},
		},
	}
	errs := collection.Validate(col)
	found := false
	for _, e := range errs {
		if strings.Contains(e, "image or source is required") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'image or source is required' error, got: %v", errs)
	}
}

func TestValidateSourceWithoutName(t *testing.T) {
	col := &collection.SkillCollection{
		APIVersion: "skillimage.io/v1alpha1",
		Kind:       "SkillCollection",
		Metadata:   collection.Metadata{Name: "test", Version: "1.0.0"},
		Skills: []collection.SkillRef{
			{Source: "https://github.com/org/repo/tree/main/skill"},
		},
	}
	errs := collection.Validate(col)
	if len(errs) != 0 {
		t.Errorf("source without name should be valid, got errors: %v", errs)
	}
}
