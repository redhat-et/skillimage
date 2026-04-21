package oci_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/redhat-et/skillimage/pkg/lifecycle"
	"github.com/redhat-et/skillimage/pkg/oci"
)

func writeTestSkill(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillYAML := []byte(`apiVersion: skillimage.io/v1alpha1
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

func TestCopyToAndBack(t *testing.T) {
	// Pack into source store
	skillDir := t.TempDir()
	writeTestSkill(t, skillDir)

	srcStoreDir := t.TempDir()
	srcClient, err := oci.NewClient(srcStoreDir)
	if err != nil {
		t.Fatalf("NewClient (src): %v", err)
	}

	ctx := context.Background()
	_, err = srcClient.Pack(ctx, skillDir, oci.PackOptions{})
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}

	// Copy to destination store
	dstStoreDir := t.TempDir()
	dstClient, err := oci.NewClient(dstStoreDir)
	if err != nil {
		t.Fatalf("NewClient (dst): %v", err)
	}

	ref := "test/test-skill:1.0.0-draft"
	err = srcClient.CopyTo(ctx, ref, dstClient)
	if err != nil {
		t.Fatalf("CopyTo: %v", err)
	}

	// Verify the image exists in destination
	images, err := dstClient.ListLocal()
	if err != nil {
		t.Fatalf("ListLocal: %v", err)
	}
	if len(images) != 1 {
		t.Fatalf("expected 1 image after copy, got %d", len(images))
	}
}

func TestUnpack(t *testing.T) {
	skillDir := t.TempDir()
	writeTestSkill(t, skillDir)

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	_, err = client.Pack(ctx, skillDir, oci.PackOptions{})
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}

	// Unpack to output directory
	outputDir := t.TempDir()
	err = client.Unpack(ctx, "test/test-skill:1.0.0-draft", outputDir)
	if err != nil {
		t.Fatalf("Unpack: %v", err)
	}

	// Verify: auto-creates subdirectory named after skill
	expectedFile := filepath.Join(outputDir, "test-skill", "skill.yaml")
	if _, err := os.Stat(expectedFile); err != nil {
		t.Errorf("expected %s to exist: %v", expectedFile, err)
	}

	promptFile := filepath.Join(outputDir, "test-skill", "SKILL.md")
	if _, err := os.Stat(promptFile); err != nil {
		t.Errorf("expected %s to exist: %v", promptFile, err)
	}
}

func TestInspect(t *testing.T) {
	skillDir := t.TempDir()
	writeTestSkill(t, skillDir)

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	_, err = client.Pack(ctx, skillDir, oci.PackOptions{})
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}

	result, err := client.Inspect(ctx, "test/test-skill:1.0.0-draft")
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if result.Version != "1.0.0" {
		t.Errorf("version = %q, want %q", result.Version, "1.0.0")
	}
	if result.Status != "draft" {
		t.Errorf("status = %q, want %q", result.Status, "draft")
	}
	if result.Name != "test/test-skill" {
		t.Errorf("name = %q, want %q", result.Name, "test/test-skill")
	}
	if result.LayerCount != 1 {
		t.Errorf("layer count = %d, want 1", result.LayerCount)
	}
}

func TestPromoteLocal(t *testing.T) {
	skillDir := t.TempDir()
	writeTestSkill(t, skillDir)

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	_, err = client.Pack(ctx, skillDir, oci.PackOptions{})
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}

	// Promote draft -> testing
	err = client.PromoteLocal(ctx, "test/test-skill:1.0.0-draft", lifecycle.Testing)
	if err != nil {
		t.Fatalf("Promote to testing: %v", err)
	}

	// Verify new tag exists with correct status
	result, err := client.Inspect(ctx, "test/test-skill:1.0.0-testing")
	if err != nil {
		t.Fatalf("Inspect after promote to testing: %v", err)
	}
	if result.Status != "testing" {
		t.Errorf("status = %q, want %q", result.Status, "testing")
	}

	// Promote testing -> published
	err = client.PromoteLocal(ctx, "test/test-skill:1.0.0-testing", lifecycle.Published)
	if err != nil {
		t.Fatalf("Promote to published: %v", err)
	}

	// Verify published tag
	result, err = client.Inspect(ctx, "test/test-skill:1.0.0")
	if err != nil {
		t.Fatalf("Inspect after publish: %v", err)
	}
	if result.Status != "published" {
		t.Errorf("status = %q, want %q", result.Status, "published")
	}

	// Verify latest tag also exists
	result, err = client.Inspect(ctx, "test/test-skill:latest")
	if err != nil {
		t.Fatalf("Inspect latest after publish: %v", err)
	}
	if result.Status != "published" {
		t.Errorf("latest status = %q, want %q", result.Status, "published")
	}
}

func TestAnnotationsIncludeTags(t *testing.T) {
	skillDir := t.TempDir()
	skillYAML := []byte(`apiVersion: skillimage.io/v1alpha1
kind: SkillCard
metadata:
  name: annotated-skill
  namespace: test
  version: 2.0.0
  description: Skill with tags and compat.
  tags:
    - kubernetes
    - debugging
  compatibility: claude-3.5-sonnet
spec:
  prompt: SKILL.md
`)
	if err := os.WriteFile(filepath.Join(skillDir, "skill.yaml"), skillYAML, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("one two three four five"), 0o644); err != nil {
		t.Fatal(err)
	}

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	_, err = client.Pack(ctx, skillDir, oci.PackOptions{})
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}

	result, err := client.Inspect(ctx, "test/annotated-skill:2.0.0-draft")
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}

	if result.Tags != `["kubernetes","debugging"]` {
		t.Errorf("tags = %q, want %q", result.Tags, `["kubernetes","debugging"]`)
	}
	if result.Compatibility != "claude-3.5-sonnet" {
		t.Errorf("compatibility = %q, want %q", result.Compatibility, "claude-3.5-sonnet")
	}
	if result.WordCount != "5" {
		t.Errorf("wordcount = %q, want %q", result.WordCount, "5")
	}
}

func TestWordCountExcludesFrontmatter(t *testing.T) {
	skillDir := t.TempDir()
	skillYAML := []byte(`apiVersion: skillimage.io/v1alpha1
kind: SkillCard
metadata:
  name: frontmatter-skill
  namespace: test
  version: 1.0.0
  description: Skill with YAML frontmatter in SKILL.md.
spec:
  prompt: SKILL.md
`)
	skillMD := []byte(`---
name: frontmatter-skill
description: This has frontmatter that should not be counted.
license: Apache-2.0
---

These five words are counted.
`)
	if err := os.WriteFile(filepath.Join(skillDir, "skill.yaml"), skillYAML, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), skillMD, 0o644); err != nil {
		t.Fatal(err)
	}

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	_, err = client.Pack(ctx, skillDir, oci.PackOptions{})
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}

	result, err := client.Inspect(ctx, "test/frontmatter-skill:1.0.0-draft")
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}

	if result.WordCount != "5" {
		t.Errorf("wordcount = %q, want %q (frontmatter should be excluded)", result.WordCount, "5")
	}
}

func TestPromoteInvalidTransition(t *testing.T) {
	skillDir := t.TempDir()
	writeTestSkill(t, skillDir)

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	_, err = client.Pack(ctx, skillDir, oci.PackOptions{})
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}

	// Try invalid transition: draft -> published
	err = client.PromoteLocal(ctx, "test/test-skill:1.0.0-draft", lifecycle.Published)
	if err == nil {
		t.Fatal("expected error for invalid transition draft -> published")
	}
}

func TestAnnotationsOmittedWhenEmpty(t *testing.T) {
	skillDir := t.TempDir()
	skillYAML := []byte(`apiVersion: skillimage.io/v1alpha1
kind: SkillCard
metadata:
  name: minimal-skill
  namespace: test
  version: 1.0.0
  description: Minimal skill with no optional fields.
spec:
  prompt: SKILL.md
`)
	if err := os.WriteFile(filepath.Join(skillDir, "skill.yaml"), skillYAML, 0o644); err != nil {
		t.Fatal(err)
	}

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	_, err = client.Pack(ctx, skillDir, oci.PackOptions{})
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}

	result, err := client.Inspect(ctx, "test/minimal-skill:1.0.0-draft")
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}

	if result.Tags != "" {
		t.Errorf("tags should be empty, got %q", result.Tags)
	}
	if result.Compatibility != "" {
		t.Errorf("compatibility should be empty, got %q", result.Compatibility)
	}
	if result.WordCount != "" {
		t.Errorf("wordcount should be empty, got %q", result.WordCount)
	}
}

func TestTag(t *testing.T) {
	skillDir := t.TempDir()
	writeTestSkill(t, skillDir)

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	_, err = client.Pack(ctx, skillDir, oci.PackOptions{})
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}

	err = client.Tag(ctx, "test/test-skill:1.0.0-draft", "quay.io/myorg/test-skill:1.0.0-draft")
	if err != nil {
		t.Fatalf("Tag: %v", err)
	}

	// Inspect via the new tag should return the same image.
	result, err := client.Inspect(ctx, "quay.io/myorg/test-skill:1.0.0-draft")
	if err != nil {
		t.Fatalf("Inspect new tag: %v", err)
	}
	if result.Version != "1.0.0" {
		t.Errorf("version = %q, want %q", result.Version, "1.0.0")
	}
	if result.Status != "draft" {
		t.Errorf("status = %q, want %q", result.Status, "draft")
	}
}

func TestPackRedHatMediaType(t *testing.T) {
	skillDir := t.TempDir()
	writeTestSkill(t, skillDir)

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	desc, err := client.Pack(ctx, skillDir, oci.PackOptions{
		MediaType: oci.MediaTypeRedHat,
	})
	if err != nil {
		t.Fatalf("Pack with redhat media type: %v", err)
	}
	if desc.Digest.String() == "" {
		t.Error("expected non-empty digest")
	}

	result, err := client.Inspect(ctx, "test/test-skill:1.0.0-draft")
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if result.MediaType != "application/vnd.oci.image.manifest.v1+json" {
		t.Errorf("manifest media type = %q, want standard OCI manifest type", result.MediaType)
	}
	if result.ConfigMediaType != oci.RedHatMediaTypeSkillConfig {
		t.Errorf("config media type = %q, want %q", result.ConfigMediaType, oci.RedHatMediaTypeSkillConfig)
	}
	if result.LayerMediaType != oci.RedHatMediaTypeSkillLayer {
		t.Errorf("layer media type = %q, want %q", result.LayerMediaType, oci.RedHatMediaTypeSkillLayer)
	}
}

func TestParseMediaTypeProfileRejectsInvalid(t *testing.T) {
	_, err := oci.ParseMediaTypeProfile("bogus")
	if err == nil {
		t.Fatal("expected error for invalid media type profile")
	}
}

func TestAnnotationsEmptySKILLmd(t *testing.T) {
	skillDir := t.TempDir()
	skillYAML := []byte(`apiVersion: skillimage.io/v1alpha1
kind: SkillCard
metadata:
  name: empty-md-skill
  namespace: test
  version: 1.0.0
  description: Skill with empty SKILL.md.
spec:
  prompt: SKILL.md
`)
	if err := os.WriteFile(filepath.Join(skillDir, "skill.yaml"), skillYAML, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	_, err = client.Pack(ctx, skillDir, oci.PackOptions{})
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}

	result, err := client.Inspect(ctx, "test/empty-md-skill:1.0.0-draft")
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}

	if result.WordCount != "" {
		t.Errorf("wordcount should be empty for empty SKILL.md, got %q", result.WordCount)
	}
}
