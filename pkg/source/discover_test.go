package source_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/redhat-et/skillimage/pkg/source"
)

func writeSkillMD(t *testing.T, dir, content string) {
	t.Helper()
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644)
}

func TestDiscoverMultipleSkills(t *testing.T) {
	root := t.TempDir()
	writeSkillMD(t, filepath.Join(root, "alpha"), "---\nname: alpha\n---\nAlpha.")
	writeSkillMD(t, filepath.Join(root, "beta"), "---\nname: beta\n---\nBeta.")
	writeSkillMD(t, filepath.Join(root, "gamma"), "---\nname: gamma\n---\nGamma.")

	skills, err := source.Discover(root, "")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(skills) != 3 {
		t.Fatalf("got %d skills, want 3", len(skills))
	}
	if skills[0].Name != "alpha" || skills[1].Name != "beta" || skills[2].Name != "gamma" {
		t.Errorf("skills = %+v, want alpha/beta/gamma sorted", skills)
	}
}

func TestDiscoverSingleSkill(t *testing.T) {
	root := t.TempDir()
	writeSkillMD(t, root, "---\nname: solo\n---\nSolo skill.")

	skills, err := source.Discover(root, "")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("got %d skills, want 1", len(skills))
	}
	if skills[0].Dir != root {
		t.Errorf("Dir = %q, want %q", skills[0].Dir, root)
	}
}

func TestDiscoverSkipsHiddenDirs(t *testing.T) {
	root := t.TempDir()
	writeSkillMD(t, filepath.Join(root, "visible"), "---\nname: visible\n---\n")
	writeSkillMD(t, filepath.Join(root, ".hidden"), "---\nname: hidden\n---\n")

	skills, err := source.Discover(root, "")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("got %d skills, want 1 (hidden should be skipped)", len(skills))
	}
}

func TestDiscoverWithFilter(t *testing.T) {
	root := t.TempDir()
	writeSkillMD(t, filepath.Join(root, "code-review"), "---\nname: code-review\n---\n")
	writeSkillMD(t, filepath.Join(root, "code-gen"), "---\nname: code-gen\n---\n")
	writeSkillMD(t, filepath.Join(root, "email"), "---\nname: email\n---\n")

	skills, err := source.Discover(root, "code-*")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(skills) != 2 {
		t.Fatalf("got %d skills, want 2", len(skills))
	}
}

func TestDiscoverNoSkills(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "README.md"), []byte("no skills here"), 0o644)

	_, err := source.Discover(root, "")
	if err == nil {
		t.Fatal("expected error when no skills found")
	}
}

func TestDiscoverNoMatchingFilter(t *testing.T) {
	root := t.TempDir()
	writeSkillMD(t, filepath.Join(root, "alpha"), "---\nname: alpha\n---\n")

	_, err := source.Discover(root, "zzz-*")
	if err == nil {
		t.Fatal("expected error when no skills match filter")
	}
}

func TestDiscoverDoesNotNest(t *testing.T) {
	root := t.TempDir()
	writeSkillMD(t, filepath.Join(root, "parent"), "---\nname: parent\n---\n")
	writeSkillMD(t, filepath.Join(root, "parent", "child"), "---\nname: child\n---\n")

	skills, err := source.Discover(root, "")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("got %d skills, want 1 (child should not be discovered)", len(skills))
	}
	if skills[0].Name != "parent" {
		t.Errorf("Name = %q, want %q", skills[0].Name, "parent")
	}
}
