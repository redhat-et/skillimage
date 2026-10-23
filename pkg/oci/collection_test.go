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

func writeTestCollectionWithSkill(t *testing.T, dir, imageRef string) string {
	t.Helper()
	path := filepath.Join(dir, "collection.yaml")
	content := []byte(fmt.Sprintf(`apiVersion: skillimage.io/v1alpha1
kind: SkillCollection
metadata:
  name: test-collection
  version: 1.0.0
skills:
  - name: test-skill
    image: %s
`, imageRef))
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

func TestPullCollectionOnlyPullsSkillsWhenOutputDirIsSet(t *testing.T) {
	ctx := context.Background()
	sourceStoreDir := t.TempDir()
	sourceClient, err := oci.NewClient(sourceStoreDir)
	if err != nil {
		t.Fatalf("NewClient (source): %v", err)
	}

	skillDir := t.TempDir()
	writeTestSkill(t, skillDir)
	skillDesc, err := sourceClient.Build(ctx, skillDir, oci.BuildOptions{})
	if err != nil {
		t.Fatalf("Build skill: %v", err)
	}
	skillRegistry := newReadOnlyTestRegistry(t, sourceStoreDir, "test/test-skill", "1.0.0-draft", skillDesc)
	skillRef := strings.TrimPrefix(skillRegistry.URL, "https://") + "/test/test-skill:1.0.0-draft"

	yamlPath := writeTestCollectionWithSkill(t, t.TempDir(), skillRef)
	collectionDesc, err := sourceClient.BuildCollectionArtifact(ctx, yamlPath, "test/test-collection:1.0.0")
	if err != nil {
		t.Fatalf("BuildCollectionArtifact: %v", err)
	}
	collectionRegistry := newReadOnlyTestRegistry(t, sourceStoreDir, "test/test-collection", "1.0.0", collectionDesc)
	collectionRef := strings.TrimPrefix(collectionRegistry.URL, "https://") + "/test/test-collection:1.0.0"

	withoutOutput, err := oci.NewClient(t.TempDir())
	if err != nil {
		t.Fatalf("NewClient (without output): %v", err)
	}
	col, err := withoutOutput.PullCollection(ctx, collectionRef, oci.PullOptions{SkipTLSVerify: true})
	if err != nil {
		t.Fatalf("PullCollection without output: %v", err)
	}
	if col.Metadata.Name != "test-collection" {
		t.Errorf("collection name = %q, want test-collection", col.Metadata.Name)
	}
	if _, err := withoutOutput.Inspect(ctx, skillRef); err == nil {
		t.Error("member skill was pulled even though no output directory was set")
	}

	withOutput, err := oci.NewClient(t.TempDir())
	if err != nil {
		t.Fatalf("NewClient (with output): %v", err)
	}
	outputDir := t.TempDir()
	if _, err := withOutput.PullCollection(ctx, collectionRef, oci.PullOptions{
		OutputDir:     outputDir,
		SkipTLSVerify: true,
	}); err != nil {
		t.Fatalf("PullCollection with output: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "test-skill", "SKILL.md")); err != nil {
		t.Errorf("expected member skill to be unpacked: %v", err)
	}
}
