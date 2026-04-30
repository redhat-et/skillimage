package oci_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/redhat-et/skillimage/pkg/oci"
)

func TestRemove(t *testing.T) {
	skillDir := t.TempDir()
	writeTestSkill(t, skillDir)

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	_, err = client.Build(ctx, skillDir, oci.BuildOptions{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	err = client.Remove(ctx, "test/test-skill:1.0.0-draft")
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}

	images, err := client.ListLocal()
	if err != nil {
		t.Fatalf("ListLocal: %v", err)
	}
	if len(images) != 0 {
		t.Errorf("expected 0 images after remove, got %d", len(images))
	}
}

func TestRemoveNotFound(t *testing.T) {
	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	err = client.Remove(context.Background(), "no/such-image:1.0.0")
	if err == nil {
		t.Fatal("expected error for non-existent image")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "not found")
	}
}

func TestRemoveMultipleImages(t *testing.T) {
	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()

	// Build first image
	skillDir1 := t.TempDir()
	writeTestSkill(t, skillDir1)
	_, err = client.Build(ctx, skillDir1, oci.BuildOptions{})
	if err != nil {
		t.Fatalf("Build first: %v", err)
	}

	// Build second image with different name
	skillDir2 := t.TempDir()
	writeTestSkillNamed(t, skillDir2, "other-skill")
	_, err = client.Build(ctx, skillDir2, oci.BuildOptions{})
	if err != nil {
		t.Fatalf("Build second: %v", err)
	}

	// Remove only the first
	err = client.Remove(ctx, "test/test-skill:1.0.0-draft")
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}

	images, err := client.ListLocal()
	if err != nil {
		t.Fatalf("ListLocal: %v", err)
	}
	if len(images) != 1 {
		t.Fatalf("expected 1 image after remove, got %d", len(images))
	}
	if images[0].Name != "test/other-skill" {
		t.Errorf("remaining image = %q, want %q", images[0].Name, "test/other-skill")
	}
}

func writeTestSkillNamed(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillYAML := fmt.Appendf(nil, `apiVersion: skillimage.io/v1alpha1
kind: SkillCard
metadata:
  name: %s
  namespace: test
  version: 1.0.0
  description: A test skill.
spec:
  prompt: SKILL.md
`, name)
	if err := os.WriteFile(filepath.Join(dir, "skill.yaml"), skillYAML, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("Test prompt."), 0o644); err != nil {
		t.Fatal(err)
	}
}
