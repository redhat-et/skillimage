package oci_test

import (
	"context"
	"strings"
	"testing"

	"github.com/redhat-et/skillimage/pkg/oci"
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
