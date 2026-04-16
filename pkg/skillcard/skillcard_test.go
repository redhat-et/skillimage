package skillcard_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/redhat-et/oci-skill-registry/pkg/skillcard"
)

const validSkillYAML = `apiVersion: skills.redhat.io/v1alpha1
kind: SkillCard
metadata:
  name: hello-world
  namespace: examples
  version: 1.0.0
  display-name: "Hello World"
  description: A simple example skill.
  license: Apache-2.0
  tags:
    - example
  authors:
    - name: Test Author
      email: test@example.com
spec:
  prompt: SKILL.md
`

func TestParse(t *testing.T) {
	sc, err := skillcard.Parse(strings.NewReader(validSkillYAML))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sc.APIVersion != "skills.redhat.io/v1alpha1" {
		t.Errorf("apiVersion = %q, want %q", sc.APIVersion, "skills.redhat.io/v1alpha1")
	}
	if sc.Kind != "SkillCard" {
		t.Errorf("kind = %q, want %q", sc.Kind, "SkillCard")
	}
	if sc.Metadata.Name != "hello-world" {
		t.Errorf("name = %q, want %q", sc.Metadata.Name, "hello-world")
	}
	if sc.Metadata.Namespace != "examples" {
		t.Errorf("namespace = %q, want %q", sc.Metadata.Namespace, "examples")
	}
	if sc.Metadata.Version != "1.0.0" {
		t.Errorf("version = %q, want %q", sc.Metadata.Version, "1.0.0")
	}
	if sc.Metadata.DisplayName != "Hello World" {
		t.Errorf("display-name = %q, want %q", sc.Metadata.DisplayName, "Hello World")
	}
	if len(sc.Metadata.Authors) != 1 || sc.Metadata.Authors[0].Name != "Test Author" {
		t.Errorf("authors = %v, want [{Test Author test@example.com}]", sc.Metadata.Authors)
	}
	if sc.Spec == nil || sc.Spec.Prompt != "SKILL.md" {
		t.Errorf("spec.prompt = %v, want SKILL.md", sc.Spec)
	}
}

func TestParseInvalidYAML(t *testing.T) {
	_, err := skillcard.Parse(strings.NewReader("not: [valid: yaml"))
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestSerialize(t *testing.T) {
	sc, err := skillcard.Parse(strings.NewReader(validSkillYAML))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var buf bytes.Buffer
	if err := skillcard.Serialize(sc, &buf); err != nil {
		t.Fatalf("serialize: %v", err)
	}
	roundtrip, err := skillcard.Parse(&buf)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if roundtrip.Metadata.Name != sc.Metadata.Name {
		t.Errorf("roundtrip name = %q, want %q", roundtrip.Metadata.Name, sc.Metadata.Name)
	}
	if roundtrip.Metadata.Version != sc.Metadata.Version {
		t.Errorf("roundtrip version = %q, want %q", roundtrip.Metadata.Version, sc.Metadata.Version)
	}
}
