package oci_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/redhat-et/skillimage/pkg/oci"
)

func writeTestCollection(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "collection.yaml")
	content := []byte(`apiVersion: skillimage.io/v1alpha1
kind: SkillCollection
metadata:
  name: test-collection
  version: 1.0.0
  description: Test collection
skills:
  - name: skill-a
    image: quay.io/org/skill-a:1.0.0
  - name: skill-b
    image: ghcr.io/org/skill-b:2.0.0
`)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBuildCollectionArtifact(t *testing.T) {
	dir := t.TempDir()
	yamlPath := writeTestCollection(t, dir)

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	desc, err := client.BuildCollectionArtifact(ctx, yamlPath, "test/test-collection:1.0.0")
	if err != nil {
		t.Fatalf("BuildCollectionArtifact: %v", err)
	}
	if desc.Digest.String() == "" {
		t.Error("expected non-empty digest")
	}
}
