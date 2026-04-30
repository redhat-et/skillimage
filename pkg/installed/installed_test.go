package installed_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/redhat-et/skillimage/pkg/installed"
)

func writeSkillYAML(t *testing.T, dir, name, version, source, commit string) {
	t.Helper()
	skillDir := filepath.Join(dir, name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := "apiVersion: skillimage.io/v1alpha1\n" +
		"kind: SkillCard\n" +
		"metadata:\n" +
		"  name: " + name + "\n" +
		"  namespace: test\n" +
		"  version: " + version + "\n" +
		"  description: A test skill.\n" +
		"spec:\n" +
		"  prompt: SKILL.md\n"
	if source != "" {
		yaml += "provenance:\n" +
			"  source: " + source + "\n" +
			"  commit: " + commit + "\n"
	}
	if err := os.WriteFile(filepath.Join(skillDir, "skill.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScan(t *testing.T) {
	claudeDir := t.TempDir()
	cursorDir := t.TempDir()

	writeSkillYAML(t, claudeDir, "hello-world", "1.0.0",
		"test/hello-world:1.0.0-draft", "sha256:abc123")
	writeSkillYAML(t, cursorDir, "summarizer", "2.0.0", "", "")

	targets := map[string]string{
		"claude": claudeDir,
		"cursor": cursorDir,
	}

	skills, err := installed.Scan(targets)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(skills))
	}

	// Find the hello-world skill (has provenance).
	var hw, sm *installed.InstalledSkill
	for i := range skills {
		switch skills[i].Name {
		case "hello-world":
			hw = &skills[i]
		case "summarizer":
			sm = &skills[i]
		}
	}

	if hw == nil {
		t.Fatal("hello-world skill not found")
	}
	if hw.Version != "1.0.0" {
		t.Errorf("hw version = %q, want %q", hw.Version, "1.0.0")
	}
	if hw.Source != "test/hello-world:1.0.0-draft" {
		t.Errorf("hw source = %q, want %q", hw.Source, "test/hello-world:1.0.0-draft")
	}
	if hw.Target != "claude" {
		t.Errorf("hw target = %q, want %q", hw.Target, "claude")
	}

	if sm == nil {
		t.Fatal("summarizer skill not found")
	}
	if sm.Source != "" {
		t.Errorf("sm source = %q, want empty (local)", sm.Source)
	}
	if sm.Target != "cursor" {
		t.Errorf("sm target = %q, want %q", sm.Target, "cursor")
	}
}

func TestScanNonExistentDir(t *testing.T) {
	targets := map[string]string{
		"claude": "/nonexistent/path/that/does/not/exist",
	}

	skills, err := installed.Scan(targets)
	if err != nil {
		t.Fatalf("Scan should not error on missing dir: %v", err)
	}
	if len(skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(skills))
	}
}

func TestScanMalformedSkillYAML(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "bad-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(skillDir, "skill.yaml"),
		[]byte("this is not valid yaml: [[["),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	targets := map[string]string{"claude": dir}
	skills, err := installed.Scan(targets)
	if err != nil {
		t.Fatalf("Scan should not error on malformed yaml: %v", err)
	}
	if len(skills) != 0 {
		t.Errorf("expected 0 skills (malformed skipped), got %d", len(skills))
	}
}
