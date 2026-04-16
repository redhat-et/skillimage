package oci_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/redhat-et/oci-skill-registry/pkg/oci"
)

func writeTestSkill(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillYAML := []byte(`apiVersion: skills.redhat.io/v1alpha1
kind: SkillCard
metadata:
  name: test-skill
  namespace: test
  version: 1.0.0
  description: A test skill.
spec:
  prompt: SKILL.md
`)
	if err := os.WriteFile(filepath.Join(dir, "skill.yaml"), skillYAML, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("Test prompt content."), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPackAndListLocal(t *testing.T) {
	skillDir := t.TempDir()
	writeTestSkill(t, skillDir)

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	desc, err := client.Pack(ctx, skillDir, oci.PackOptions{})
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}
	if desc.Digest.String() == "" {
		t.Error("expected non-empty digest")
	}

	images, err := client.ListLocal()
	if err != nil {
		t.Fatalf("ListLocal: %v", err)
	}
	if len(images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(images))
	}
	if images[0].Name != "test/test-skill" {
		t.Errorf("image name = %q, want %q", images[0].Name, "test/test-skill")
	}
	if images[0].Version != "1.0.0" {
		t.Errorf("image version = %q, want %q", images[0].Version, "1.0.0")
	}
	if images[0].Tag != "1.0.0-draft" {
		t.Errorf("image tag = %q, want %q", images[0].Tag, "1.0.0-draft")
	}
}

func TestPackValidatesSkillCard(t *testing.T) {
	skillDir := t.TempDir()
	badYAML := []byte(`apiVersion: wrong/v1
kind: SkillCard
metadata:
  name: BAD
  namespace: test
  version: 1.0.0
  description: test
`)
	if err := os.WriteFile(filepath.Join(skillDir, "skill.yaml"), badYAML, 0o644); err != nil {
		t.Fatal(err)
	}

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = client.Pack(context.Background(), skillDir, oci.PackOptions{})
	if err == nil {
		t.Fatal("expected error for invalid SkillCard")
	}
}

func TestPackMissingSkillYAML(t *testing.T) {
	emptyDir := t.TempDir()

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = client.Pack(context.Background(), emptyDir, oci.PackOptions{})
	if err == nil {
		t.Fatal("expected error for missing skill.yaml")
	}
}
