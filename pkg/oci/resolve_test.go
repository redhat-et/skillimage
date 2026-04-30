package oci_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/redhat-et/skillimage/pkg/oci"
	"github.com/redhat-et/skillimage/pkg/skillcard"
)

func TestResolveDigest(t *testing.T) {
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

	digest, err := client.ResolveDigest(ctx, "test/test-skill:1.0.0-draft")
	if err != nil {
		t.Fatalf("ResolveDigest: %v", err)
	}
	if !strings.HasPrefix(digest, "sha256:") {
		t.Errorf("digest = %q, want sha256: prefix", digest)
	}
}

func TestResolveDigestNotFound(t *testing.T) {
	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = client.ResolveDigest(context.Background(), "no/such:1.0.0")
	if err == nil {
		t.Fatal("expected error for non-existent ref")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "not found")
	}
}

func TestInstallProvenance(t *testing.T) {
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

	ref := "test/test-skill:1.0.0-draft"

	// Unpack to a temp dir (simulates install).
	outputDir := t.TempDir()
	err = client.Unpack(ctx, ref, outputDir)
	if err != nil {
		t.Fatalf("Unpack: %v", err)
	}

	// Resolve digest.
	digest, err := client.ResolveDigest(ctx, ref)
	if err != nil {
		t.Fatalf("ResolveDigest: %v", err)
	}

	// Read skill.yaml back, set provenance, write it back.
	skillPath := filepath.Join(outputDir, "test-skill", "skill.yaml")
	f, err := os.Open(skillPath)
	if err != nil {
		t.Fatalf("opening skill.yaml: %v", err)
	}
	sc, err := skillcard.Parse(f)
	_ = f.Close()
	if err != nil {
		t.Fatalf("parsing skill.yaml: %v", err)
	}

	if sc.Provenance == nil {
		sc.Provenance = &skillcard.Provenance{}
	}
	sc.Provenance.Source = ref
	sc.Provenance.Commit = digest

	wf, err := os.Create(skillPath)
	if err != nil {
		t.Fatalf("creating skill.yaml for write: %v", err)
	}
	err = skillcard.Serialize(sc, wf)
	_ = wf.Close()
	if err != nil {
		t.Fatalf("serializing skill.yaml: %v", err)
	}

	// Read it back and verify provenance fields.
	f2, err := os.Open(skillPath)
	if err != nil {
		t.Fatalf("re-opening skill.yaml: %v", err)
	}
	defer func() { _ = f2.Close() }()
	sc2, err := skillcard.Parse(f2)
	if err != nil {
		t.Fatalf("re-parsing skill.yaml: %v", err)
	}

	if sc2.Provenance == nil {
		t.Fatal("expected provenance to be set")
	}
	if sc2.Provenance.Source != ref {
		t.Errorf("source = %q, want %q", sc2.Provenance.Source, ref)
	}
	if sc2.Provenance.Commit != digest {
		t.Errorf("commit = %q, want %q", sc2.Provenance.Commit, digest)
	}
}
