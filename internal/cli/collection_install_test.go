package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestCollectionInstallRequiresInput(t *testing.T) {
	cmd := NewRootCmd("test")
	cmd.SetArgs([]string{"collection", "install", "--target", "claude"})
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when no -f or URL given")
	}
}

func TestCollectionInstallRequiresTarget(t *testing.T) {
	dir := t.TempDir()
	colFile := filepath.Join(dir, "collection.yaml")
	content := []byte(`apiVersion: skillimage.io/v1alpha1
kind: SkillCollection
metadata:
  name: test
  version: 1.0.0
skills:
  - name: s1
    image: quay.io/org/s1:1.0.0
`)
	if err := os.WriteFile(colFile, content, 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCmd("test")
	cmd.SetArgs([]string{"collection", "install", "-f", colFile})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when no --target or -o given")
	}
}

func TestCollectionInstallInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	colFile := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(colFile, []byte("not: valid: yaml: ["), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCmd("test")
	cmd.SetArgs([]string{"collection", "install", "-f", colFile, "-o", dir})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestCollectionInstallValidationError(t *testing.T) {
	dir := t.TempDir()
	colFile := filepath.Join(dir, "collection.yaml")
	content := []byte(`apiVersion: skillimage.io/v1alpha1
kind: SkillCollection
metadata:
  name: test
  version: 1.0.0
skills:
  - name: both-set
    image: quay.io/org/s:1.0.0
    source: https://github.com/org/repo/tree/main/s
`)
	if err := os.WriteFile(colFile, content, 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCmd("test")
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"collection", "install", "-f", colFile, "-o", dir})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected validation error")
	}
}
