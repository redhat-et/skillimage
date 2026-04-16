package skillcard_test

import (
	"bytes"
	"fmt"
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

func TestValidateValid(t *testing.T) {
	sc, err := skillcard.Parse(strings.NewReader(validSkillYAML))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	errs, err := skillcard.Validate(sc)
	if err != nil {
		t.Fatalf("validate error: %v", err)
	}
	if len(errs) != 0 {
		t.Errorf("expected no validation errors, got %v", errs)
	}
}

func TestValidateMissingRequiredFields(t *testing.T) {
	yaml := `apiVersion: skills.redhat.io/v1alpha1
kind: SkillCard
metadata:
  name: test
`
	sc, err := skillcard.Parse(strings.NewReader(yaml))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	errs, err := skillcard.Validate(sc)
	if err != nil {
		t.Fatalf("validate error: %v", err)
	}
	if len(errs) == 0 {
		t.Fatal("expected validation errors for missing required fields")
	}
	for _, required := range []string{"namespace", "version", "description"} {
		found := false
		for _, e := range errs {
			if strings.Contains(e.Field, required) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected error for missing %q, got errors: %v", required, errs)
		}
	}
}

func TestValidateInvalidName(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"valid", "hello-world", false},
		{"valid single char", "a", false},
		{"uppercase", "Hello", true},
		{"spaces", "hello world", true},
		{"leading hyphen", "-hello", true},
		{"trailing hyphen", "hello-", true},
		{"consecutive hyphens", "hello--world", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yaml := fmt.Sprintf(`apiVersion: skills.redhat.io/v1alpha1
kind: SkillCard
metadata:
  name: %s
  namespace: test
  version: 1.0.0
  description: test
`, tt.value)
			sc, err := skillcard.Parse(strings.NewReader(yaml))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			errs, _ := skillcard.Validate(sc)
			hasErr := len(errs) > 0
			if hasErr != tt.wantErr {
				t.Errorf("name=%q: hasErr=%v, wantErr=%v, errs=%v",
					tt.value, hasErr, tt.wantErr, errs)
			}
		})
	}
}

func TestValidateInvalidSemver(t *testing.T) {
	yaml := `apiVersion: skills.redhat.io/v1alpha1
kind: SkillCard
metadata:
  name: test
  namespace: test
  version: not-semver
  description: test
`
	sc, err := skillcard.Parse(strings.NewReader(yaml))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	errs, _ := skillcard.Validate(sc)
	found := false
	for _, e := range errs {
		if strings.Contains(e.Field, "version") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected semver validation error, got: %v", errs)
	}
}

func TestValidateWrongAPIVersion(t *testing.T) {
	yaml := `apiVersion: wrong/v1
kind: SkillCard
metadata:
  name: test
  namespace: test
  version: 1.0.0
  description: test
`
	sc, err := skillcard.Parse(strings.NewReader(yaml))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	errs, _ := skillcard.Validate(sc)
	if len(errs) == 0 {
		t.Fatal("expected error for wrong apiVersion")
	}
}
