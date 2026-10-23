package agentplugin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateDirExcludesVCSMetadata(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"plugin.json":                       `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"test-plugin"}`,
		".git":                              "gitdir: ../.git/worktrees/test-plugin\n",
		"assets/readme.md":                  "Plugin documentation.\n",
		"assets/.git/objects/pack/metadata": "git object history",
		"assets/.hg/store/metadata":         "mercurial metadata",
		"assets/.svn/entries":               "subversion metadata",
		".claude-plugin/plugin.json":        `{"name":"test-plugin"}`,
	}
	for name, contents := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create parent for %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	info, err := ValidateDir(root)
	if err != nil {
		t.Fatalf("ValidateDir: %v", err)
	}
	if info.FileCount != 5 {
		t.Errorf("FileCount = %d, want 5 entries excluding VCS metadata", info.FileCount)
	}
	if !IsVCSMetadataEntry(".git") || !IsVCSMetadataEntry(".hg") || !IsVCSMetadataEntry(".svn") {
		t.Error("expected VCS metadata names to be excluded")
	}
	if IsVCSMetadataEntry(".claude-plugin") || IsVCSMetadataEntry(".gitignore") {
		t.Error("expected non-VCS dotfiles and directories to remain included")
	}
}
