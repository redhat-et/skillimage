package oci_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/redhat-et/skillimage/pkg/oci"
)

func writeTestBundle(t *testing.T, dir string) {
	t.Helper()

	for _, name := range []string{"skill-a", "skill-b"} {
		skillDir := filepath.Join(dir, name)
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatal(err)
		}
		skillYAML := []byte(`apiVersion: skillimage.io/v1alpha1
kind: SkillCard
metadata:
  name: ` + name + `
  namespace: test
  version: 1.0.0
  description: Test skill ` + name + `.
spec:
  prompt: SKILL.md
`)
		if err := os.WriteFile(filepath.Join(skillDir, "skill.yaml"), skillYAML, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("Prompt for "+name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBuildBundle(t *testing.T) {
	bundleDir := t.TempDir()
	writeTestBundle(t, bundleDir)

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	desc, err := client.BuildBundle(ctx, bundleDir, oci.BundleBuildOptions{
		Tag: "1.0.0-draft",
	})
	if err != nil {
		t.Fatalf("BuildBundle: %v", err)
	}
	if desc.Digest.String() == "" {
		t.Error("expected non-empty digest")
	}

	images, err := client.ListLocal()
	if err != nil {
		t.Fatalf("ListLocal: %v", err)
	}
	if len(images) == 0 {
		t.Fatal("expected at least 1 image after BuildBundle")
	}

	found := false
	for _, img := range images {
		if img.Tag == "1.0.0-draft" {
			found = true
			break
		}
	}
	if !found {
		t.Error("bundle image with tag 1.0.0-draft not found in local store")
	}
}

func TestBuildBundleRequiresTag(t *testing.T) {
	bundleDir := t.TempDir()
	writeTestBundle(t, bundleDir)

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = client.BuildBundle(context.Background(), bundleDir, oci.BundleBuildOptions{})
	if err == nil {
		t.Fatal("expected error when tag is empty")
	}
}

func TestBuildBundleEmptyDir(t *testing.T) {
	emptyDir := t.TempDir()

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = client.BuildBundle(context.Background(), emptyDir, oci.BundleBuildOptions{
		Tag: "1.0.0-draft",
	})
	if err == nil {
		t.Fatal("expected error for empty bundle directory")
	}
}

func TestBuildBundleInvalidSkill(t *testing.T) {
	bundleDir := t.TempDir()
	skillDir := filepath.Join(bundleDir, "bad-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
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

	_, err = client.BuildBundle(context.Background(), bundleDir, oci.BundleBuildOptions{
		Tag: "1.0.0-draft",
	})
	if err == nil {
		t.Fatal("expected error for invalid skill in bundle")
	}
}

func TestBuildBundleNamespaceMismatch(t *testing.T) {
	bundleDir := t.TempDir()

	for _, tc := range []struct {
		name string
		ns   string
	}{
		{"skill-a", "team1"},
		{"skill-b", "team2"},
	} {
		skillDir := filepath.Join(bundleDir, tc.name)
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatal(err)
		}
		skillYAML := []byte(`apiVersion: skillimage.io/v1alpha1
kind: SkillCard
metadata:
  name: ` + tc.name + `
  namespace: ` + tc.ns + `
  version: 1.0.0
  description: Test skill.
spec:
  prompt: SKILL.md
`)
		if err := os.WriteFile(filepath.Join(skillDir, "skill.yaml"), skillYAML, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("prompt"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = client.BuildBundle(context.Background(), bundleDir, oci.BundleBuildOptions{
		Tag: "1.0.0-draft",
	})
	if err == nil {
		t.Fatal("expected error for namespace mismatch")
	}
	if !strings.Contains(err.Error(), "namespace mismatch") {
		t.Errorf("error = %q, want to contain 'namespace mismatch'", err.Error())
	}
}

func TestBuildBundleAnnotations(t *testing.T) {
	bundleDir := t.TempDir()
	writeTestBundle(t, bundleDir)

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	_, err = client.BuildBundle(ctx, bundleDir, oci.BundleBuildOptions{
		Tag: "1.0.0-draft",
	})
	if err != nil {
		t.Fatalf("BuildBundle: %v", err)
	}

	images, err := client.ListLocal()
	if err != nil {
		t.Fatalf("ListLocal: %v", err)
	}
	if len(images) == 0 {
		t.Fatal("expected at least 1 image")
	}
	if images[0].Status != "draft" {
		t.Errorf("status = %q, want %q", images[0].Status, "draft")
	}
}
