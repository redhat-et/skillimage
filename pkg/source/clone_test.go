package source_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/redhat-et/skillimage/pkg/source"
)

func TestCheckGit(t *testing.T) {
	err := source.CheckGit()
	if err != nil {
		t.Skipf("git not available in test environment: %v", err)
	}
}

func TestCloneShallow(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	bareDir := t.TempDir()
	workDir := t.TempDir()

	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
	}

	run(workDir, "init")
	run(workDir, "config", "user.email", "test@test.com")
	run(workDir, "config", "user.name", "Test")
	skillDir := filepath.Join(workDir, "skills", "hello")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("creating skill directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: hello\n---\nHello skill."), 0o644); err != nil {
		t.Fatalf("writing SKILL.md: %v", err)
	}
	run(workDir, "add", ".")
	run(workDir, "commit", "-m", "init")
	run(workDir, "clone", "--bare", ".", bareDir+"/repo.git")

	src := source.GitSource{
		CloneURL: bareDir + "/repo.git",
		Ref:      "",
		SubPath:  "skills/hello",
	}

	ctx := context.Background()
	result, err := source.Clone(ctx, src, source.CloneOptions{})
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	defer result.Cleanup()

	if _, err := os.Stat(filepath.Join(result.Dir, "SKILL.md")); err != nil {
		t.Errorf("SKILL.md not found in cloned directory: %v", err)
	}

	if result.CommitSHA == "" {
		t.Error("expected non-empty CommitSHA")
	}
}

func TestCloneSubPathNotFound(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	workDir := t.TempDir()
	bareDir := t.TempDir()

	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
	}

	run(workDir, "init")
	run(workDir, "config", "user.email", "test@test.com")
	run(workDir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(workDir, "README.md"), []byte("hi"), 0o644); err != nil {
		t.Fatalf("writing README.md: %v", err)
	}
	run(workDir, "add", ".")
	run(workDir, "commit", "-m", "init")
	run(workDir, "clone", "--bare", ".", bareDir+"/repo.git")

	src := source.GitSource{
		CloneURL: bareDir + "/repo.git",
		SubPath:  "nonexistent/path",
	}

	ctx := context.Background()
	_, err := source.Clone(ctx, src, source.CloneOptions{})
	if err == nil {
		t.Fatal("expected error for nonexistent subpath")
	}
}

func TestCloneRefOverride(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	workDir := t.TempDir()
	bareDir := t.TempDir()

	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
	}

	run(workDir, "init")
	run(workDir, "config", "user.email", "test@test.com")
	run(workDir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(workDir, "SKILL.md"), []byte("v1"), 0o644); err != nil {
		t.Fatalf("writing SKILL.md v1: %v", err)
	}
	run(workDir, "add", ".")
	run(workDir, "commit", "-m", "v1")
	run(workDir, "tag", "v1.0")
	if err := os.WriteFile(filepath.Join(workDir, "SKILL.md"), []byte("v2"), 0o644); err != nil {
		t.Fatalf("writing SKILL.md v2: %v", err)
	}
	run(workDir, "add", ".")
	run(workDir, "commit", "-m", "v2")
	run(workDir, "clone", "--bare", ".", bareDir+"/repo.git")

	src := source.GitSource{
		CloneURL: bareDir + "/repo.git",
	}

	ctx := context.Background()
	result, err := source.Clone(ctx, src, source.CloneOptions{RefOverride: "v1.0"})
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	defer result.Cleanup()

	data, err := os.ReadFile(filepath.Join(result.Dir, "SKILL.md"))
	if err != nil {
		t.Fatalf("reading SKILL.md: %v", err)
	}
	if string(data) != "v1" {
		t.Errorf("SKILL.md content = %q, want %q (tag v1.0)", string(data), "v1")
	}
}
