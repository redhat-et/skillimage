# Remote Git sources implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task.
> Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build OCI skill images directly from Git repository URLs,
with SKILL.md-based discovery and SkillCard generation from
frontmatter.

**Architecture:** New `pkg/source` package with five files: URL
parsing, git cloning (with sparse checkout), skill discovery by
SKILL.md, SkillCard generation from frontmatter, and a top-level
resolver. The CLI build command gains URL detection and loops over
discovered skills. `pkg/oci.Build()` accepts an optional pre-built
SkillCard.

**Tech Stack:** Go stdlib `os/exec` for git, `gopkg.in/yaml.v3`
for frontmatter parsing, `filepath.Match` for glob filtering.

**Spec:** `docs/superpowers/specs/2026-04-29-remote-git-sources-design.md`

---

## File structure

| File | Action | Responsibility |
| ---- | ------ | -------------- |
| `pkg/source/giturl.go` | Create | `GitSource` struct, `ParseGitURL()` |
| `pkg/source/giturl_test.go` | Create | URL parsing tests |
| `pkg/source/clone.go` | Create | `CheckGit()`, `Clone()` with sparse checkout |
| `pkg/source/clone_test.go` | Create | Clone tests (against real git repos on disk) |
| `pkg/source/discover.go` | Create | `Discover()` — walk for SKILL.md, glob filter |
| `pkg/source/discover_test.go` | Create | Discovery tests |
| `pkg/source/generate.go` | Create | `ParseFrontmatter()`, `GenerateSkillCard()` |
| `pkg/source/generate_test.go` | Create | SkillCard generation tests |
| `pkg/source/source.go` | Create | `IsRemote()`, `Resolve()` orchestrator |
| `pkg/source/source_test.go` | Create | Orchestrator tests |
| `pkg/oci/client.go` | Modify | Add `SkillCard` field to `BuildOptions` |
| `pkg/oci/build.go` | Modify | Use pre-built SkillCard when provided |
| `pkg/oci/oci_test.go` | Modify | Test Build with pre-built SkillCard |
| `internal/cli/build.go` | Modify | Add `--ref`, `--filter` flags, remote path |
| `internal/cli/build_test.go` | Create | CLI integration tests |

---

## Task 1: URL parsing

**Files:**

- Create: `pkg/source/giturl.go`
- Create: `pkg/source/giturl_test.go`

- [ ] **Step 1: Write failing tests for ParseGitURL**

```go
package source_test

import (
	"testing"

	"github.com/redhat-et/skillimage/pkg/source"
)

func TestParseGitURL(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    source.GitSource
		wantErr bool
	}{
		{
			name: "github repo root",
			raw:  "https://github.com/anthropics/skills",
			want: source.GitSource{
				CloneURL: "https://github.com/anthropics/skills.git",
				Ref:      "",
				SubPath:  "",
			},
		},
		{
			name: "github with ref and subpath",
			raw:  "https://github.com/anthropics/skills/tree/main/skills",
			want: source.GitSource{
				CloneURL: "https://github.com/anthropics/skills.git",
				Ref:      "main",
				SubPath:  "skills",
			},
		},
		{
			name: "github with tag ref and deep subpath",
			raw:  "https://github.com/anthropics/skills/tree/v1.0/skills/internal-comms",
			want: source.GitSource{
				CloneURL: "https://github.com/anthropics/skills.git",
				Ref:      "v1.0",
				SubPath:  "skills/internal-comms",
			},
		},
		{
			name: "gitlab with tree path",
			raw:  "https://gitlab.com/org/repo/-/tree/main/path/to/skill",
			want: source.GitSource{
				CloneURL: "https://gitlab.com/org/repo.git",
				Ref:      "main",
				SubPath:  "path/to/skill",
			},
		},
		{
			name: "unknown host plain URL",
			raw:  "https://unknown-host.com/org/repo",
			want: source.GitSource{
				CloneURL: "https://unknown-host.com/org/repo",
				Ref:      "",
				SubPath:  "",
			},
		},
		{
			name: "github repo with .git suffix",
			raw:  "https://github.com/anthropics/skills.git",
			want: source.GitSource{
				CloneURL: "https://github.com/anthropics/skills.git",
				Ref:      "",
				SubPath:  "",
			},
		},
		{
			name:    "not a URL",
			raw:     "/some/local/path",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := source.ParseGitURL(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseGitURL() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got != tt.want {
				t.Errorf("ParseGitURL() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/source/ -run TestParseGitURL -v`
Expected: FAIL — package does not exist yet.

- [ ] **Step 3: Implement GitSource and ParseGitURL**

```go
package source

import (
	"fmt"
	"net/url"
	"strings"
)

type GitSource struct {
	CloneURL string
	Ref      string
	SubPath  string
}

func ParseGitURL(raw string) (GitSource, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		return GitSource{}, fmt.Errorf("not a valid URL: %s", raw)
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return GitSource{}, fmt.Errorf("URL must contain at least org/repo: %s", raw)
	}

	host := u.Host

	switch {
	case host == "github.com":
		return parseGitHub(u, parts)
	case host == "gitlab.com" || containsGitLab(parts):
		return parseGitLab(u, parts)
	default:
		return parseGeneric(u, parts)
	}
}

func parseGitHub(u *url.URL, parts []string) (GitSource, error) {
	org := parts[0]
	repo := strings.TrimSuffix(parts[1], ".git")
	cloneURL := fmt.Sprintf("https://%s/%s/%s.git", u.Host, org, repo)

	var ref, subPath string
	if len(parts) > 3 && parts[2] == "tree" {
		ref = parts[3]
		if len(parts) > 4 {
			subPath = strings.Join(parts[4:], "/")
		}
	}

	return GitSource{CloneURL: cloneURL, Ref: ref, SubPath: subPath}, nil
}

func parseGitLab(u *url.URL, parts []string) (GitSource, error) {
	org := parts[0]
	repo := strings.TrimSuffix(parts[1], ".git")
	cloneURL := fmt.Sprintf("https://%s/%s/%s.git", u.Host, org, repo)

	var ref, subPath string
	// GitLab: /-/tree/<ref>/path
	for i, p := range parts {
		if p == "-" && i+2 < len(parts) && parts[i+1] == "tree" {
			ref = parts[i+2]
			if i+3 < len(parts) {
				subPath = strings.Join(parts[i+3:], "/")
			}
			break
		}
	}

	return GitSource{CloneURL: cloneURL, Ref: ref, SubPath: subPath}, nil
}

func parseGeneric(u *url.URL, parts []string) (GitSource, error) {
	return GitSource{
		CloneURL: u.String(),
		Ref:      "",
		SubPath:  "",
	}, nil
}

func containsGitLab(parts []string) bool {
	for _, p := range parts {
		if p == "-" {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/source/ -run TestParseGitURL -v`
Expected: PASS — all 7 cases.

- [ ] **Step 5: Commit**

```bash
git add pkg/source/giturl.go pkg/source/giturl_test.go
git commit -s -m "feat(source): add Git URL parser for GitHub, GitLab, and generic hosts"
```

---

## Task 2: Git availability check and clone

**Files:**

- Create: `pkg/source/clone.go`
- Create: `pkg/source/clone_test.go`

- [ ] **Step 1: Write failing tests for CheckGit and Clone**

```go
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

	// Create a local bare repo to clone from.
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

	// Initialize a bare repo with a SKILL.md.
	run(workDir, "init")
	run(workDir, "config", "user.email", "test@test.com")
	run(workDir, "config", "user.name", "Test")
	skillDir := filepath.Join(workDir, "skills", "hello")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: hello\n---\nHello skill."), 0o644)
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

	// Verify SKILL.md exists at resolved path.
	if _, err := os.Stat(filepath.Join(result.Dir, "SKILL.md")); err != nil {
		t.Errorf("SKILL.md not found in cloned directory: %v", err)
	}

	if result.CommitSHA == "" {
		t.Error("expected non-empty CommitSHA")
	}
}

func TestCloneBadURL(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	src := source.GitSource{
		CloneURL: "https://example.com/nonexistent/repo.git",
	}

	ctx := context.Background()
	_, err := source.Clone(ctx, src, source.CloneOptions{})
	if err == nil {
		t.Fatal("expected error for bad clone URL")
	}
}

func TestCloneSubPathNotFound(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	// Create a minimal repo with no subdirectories.
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
	os.WriteFile(filepath.Join(workDir, "README.md"), []byte("hi"), 0o644)
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
	os.WriteFile(filepath.Join(workDir, "SKILL.md"), []byte("v1"), 0o644)
	run(workDir, "add", ".")
	run(workDir, "commit", "-m", "v1")
	run(workDir, "tag", "v1.0")
	os.WriteFile(filepath.Join(workDir, "SKILL.md"), []byte("v2"), 0o644)
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/source/ -run "TestCheck|TestClone" -v`
Expected: FAIL — functions not defined.

- [ ] **Step 3: Implement CheckGit, CloneResult, and Clone**

```go
package source

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type CloneOptions struct {
	RefOverride string
}

type CloneResult struct {
	Dir       string
	CommitSHA string
	Cleanup   func()
}

func CheckGit() error {
	_, err := exec.LookPath("git")
	if err != nil {
		return fmt.Errorf("git is required for remote sources — install it from https://git-scm.com")
	}
	return nil
}

func Clone(ctx context.Context, src GitSource, opts CloneOptions) (*CloneResult, error) {
	if err := CheckGit(); err != nil {
		return nil, err
	}

	tmpDir, err := os.MkdirTemp("", "skillctl-clone-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp directory: %w", err)
	}
	cleanup := func() { os.RemoveAll(tmpDir) }

	ref := opts.RefOverride
	if ref == "" {
		ref = src.Ref
	}

	if err := cloneRepo(ctx, src.CloneURL, ref, src.SubPath, tmpDir); err != nil {
		cleanup()
		return nil, err
	}

	resolvedDir := tmpDir
	if src.SubPath != "" {
		resolvedDir = filepath.Join(tmpDir, src.SubPath)
		if _, err := os.Stat(resolvedDir); err != nil {
			cleanup()
			return nil, fmt.Errorf("path %q not found in repository %s", src.SubPath, src.CloneURL)
		}
	}

	sha, err := commitSHA(ctx, tmpDir)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("reading commit SHA: %w", err)
	}

	return &CloneResult{Dir: resolvedDir, CommitSHA: sha, Cleanup: cleanup}, nil
}

func cloneRepo(ctx context.Context, cloneURL, ref, subPath, destDir string) error {
	if subPath != "" {
		if err := sparseClone(ctx, cloneURL, ref, subPath, destDir); err == nil {
			return nil
		}
	}

	return shallowClone(ctx, cloneURL, ref, destDir)
}

func sparseClone(ctx context.Context, cloneURL, ref, subPath, destDir string) error {
	args := []string{"clone", "--depth", "1", "--filter=blob:none", "--sparse"}
	if ref != "" {
		args = append(args, "--branch", ref)
	}
	args = append(args, cloneURL, destDir)

	if err := runGit(ctx, "", args...); err != nil {
		return err
	}

	return runGit(ctx, destDir, "sparse-checkout", "set", subPath)
}

func shallowClone(ctx context.Context, cloneURL, ref, destDir string) error {
	args := []string{"clone", "--depth", "1"}
	if ref != "" {
		args = append(args, "--branch", ref)
	}
	args = append(args, cloneURL, destDir)

	return runGit(ctx, "", args...)
}

func commitSHA(ctx context.Context, repoDir string) (string, error) {
	var stdout bytes.Buffer
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = repoDir
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(stdout.String()), nil
}

func runGit(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %v: %s", args, stderr.String())
	}
	return nil
}
```

Note: add `"strings"` to the import block (needed by `commitSHA`).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/source/ -run "TestCheck|TestClone" -v`
Expected: PASS — all 4 tests (TestCloneBadURL may take a few
seconds due to network timeout).

- [ ] **Step 5: Commit**

```bash
git add pkg/source/clone.go pkg/source/clone_test.go
git commit -s -m "feat(source): add git clone with sparse checkout and ref override"
```

---

## Task 3: Skill discovery by SKILL.md

**Files:**

- Create: `pkg/source/discover.go`
- Create: `pkg/source/discover_test.go`

- [ ] **Step 1: Write failing tests for Discover**

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/source/ -run TestDiscover -v`
Expected: FAIL — `Discover` not defined, `DiscoveredSkill` not
defined.

- [ ] **Step 3: Implement Discover**

```go
package source

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type DiscoveredSkill struct {
	Dir  string
	Name string
}

func Discover(dir string, filter string) ([]DiscoveredSkill, error) {
	// If the target directory itself contains SKILL.md, it's a single skill.
	if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err == nil {
		name := resolveSkillName(dir)
		if filter != "" {
			if matched, _ := filepath.Match(filter, name); !matched {
				return nil, fmt.Errorf("no skills matching %q in %s", filter, dir)
			}
		}
		return []DiscoveredSkill{{Dir: dir, Name: name}}, nil
	}

	var skills []DiscoveredSkill
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading directory %s: %w", dir, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		subDir := filepath.Join(dir, entry.Name())
		if _, err := os.Stat(filepath.Join(subDir, "SKILL.md")); err != nil {
			// Not a skill directory; recurse one more level.
			nested, _ := discoverNested(subDir, filter)
			skills = append(skills, nested...)
			continue
		}

		name := resolveSkillName(subDir)
		if filter != "" {
			if matched, _ := filepath.Match(filter, name); !matched {
				continue
			}
		}
		skills = append(skills, DiscoveredSkill{Dir: subDir, Name: name})
	}

	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })

	if len(skills) == 0 {
		if filter != "" {
			return nil, fmt.Errorf("no skills matching %q in %s", filter, dir)
		}
		return nil, fmt.Errorf("no skills found in %s", dir)
	}

	return skills, nil
}

func discoverNested(dir string, filter string) ([]DiscoveredSkill, error) {
	var skills []DiscoveredSkill
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		subDir := filepath.Join(dir, entry.Name())
		if _, err := os.Stat(filepath.Join(subDir, "SKILL.md")); err != nil {
			continue
		}
		name := resolveSkillName(subDir)
		if filter != "" {
			if matched, _ := filepath.Match(filter, name); !matched {
				continue
			}
		}
		skills = append(skills, DiscoveredSkill{Dir: subDir, Name: name})
	}
	return skills, nil
}

func resolveSkillName(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		return filepath.Base(dir)
	}
	fm := parseFrontmatterRaw(data)
	if name, ok := fm["name"].(string); ok && name != "" {
		return name
	}
	return filepath.Base(dir)
}
```

Note: `parseFrontmatterRaw` is a helper that will be implemented
in Task 4 (`generate.go`). For now, add a stub to make this
compile:

```go
// stub in discover.go — will move to generate.go in Task 4
func parseFrontmatterRaw(data []byte) map[string]interface{} {
	s := string(data)
	if !strings.HasPrefix(s, "---") {
		return nil
	}
	end := strings.Index(s[3:], "\n---")
	if end < 0 {
		return nil
	}
	fmStr := s[4 : 3+end]
	var m map[string]interface{}
	if err := yaml.Unmarshal([]byte(fmStr), &m); err != nil {
		return nil
	}
	return m
}
```

Add `"gopkg.in/yaml.v3"` to imports.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/source/ -run TestDiscover -v`
Expected: PASS — all 7 tests.

- [ ] **Step 5: Commit**

```bash
git add pkg/source/discover.go pkg/source/discover_test.go
git commit -s -m "feat(source): add SKILL.md-based skill discovery with glob filtering"
```

---

## Task 4: SkillCard generation from frontmatter

**Files:**

- Create: `pkg/source/generate.go`
- Create: `pkg/source/generate_test.go`
- Modify: `pkg/source/discover.go` — move `parseFrontmatterRaw`
  stub to `generate.go`

- [ ] **Step 1: Write failing tests for GenerateSkillCard**

```go
package source_test

import (
	"testing"

	"github.com/redhat-et/skillimage/pkg/source"
)

func TestGenerateSkillCardFromFrontmatter(t *testing.T) {
	dir := t.TempDir()
	content := `---
name: code-review
description: Reviews code for quality and bugs.
license: Apache-2.0
compatibility: claude-3.5-sonnet
metadata:
  author: anthropic
  version: "2.0"
---
You are a code review assistant.
`
	writeSkillMD(t, dir, content)

	sc, err := source.GenerateSkillCard(dir, "https://github.com/anthropics/skills.git", "anthropics")
	if err != nil {
		t.Fatalf("GenerateSkillCard: %v", err)
	}

	if sc.APIVersion != "skillimage.io/v1alpha1" {
		t.Errorf("APIVersion = %q", sc.APIVersion)
	}
	if sc.Kind != "SkillCard" {
		t.Errorf("Kind = %q", sc.Kind)
	}
	if sc.Metadata.Name != "code-review" {
		t.Errorf("Name = %q, want code-review", sc.Metadata.Name)
	}
	if sc.Metadata.Description != "Reviews code for quality and bugs." {
		t.Errorf("Description = %q", sc.Metadata.Description)
	}
	if sc.Metadata.Version != "2.0.0" {
		t.Errorf("Version = %q, want 2.0.0", sc.Metadata.Version)
	}
	if sc.Metadata.License != "Apache-2.0" {
		t.Errorf("License = %q", sc.Metadata.License)
	}
	if sc.Metadata.Compatibility != "claude-3.5-sonnet" {
		t.Errorf("Compatibility = %q", sc.Metadata.Compatibility)
	}
	if sc.Metadata.Namespace != "anthropics" {
		t.Errorf("Namespace = %q, want anthropics", sc.Metadata.Namespace)
	}
	if len(sc.Metadata.Authors) != 1 || sc.Metadata.Authors[0].Name != "anthropic" {
		t.Errorf("Authors = %+v", sc.Metadata.Authors)
	}
	if sc.Spec == nil || sc.Spec.Prompt != "SKILL.md" {
		t.Errorf("Spec.Prompt = %v", sc.Spec)
	}
}

func TestGenerateSkillCardFallbacks(t *testing.T) {
	dir := t.TempDir()
	// No frontmatter, just raw content.
	writeSkillMD(t, dir, "You are a helpful assistant. This does many things.")

	sc, err := source.GenerateSkillCard(dir, "https://github.com/acme/tools.git", "acme")
	if err != nil {
		t.Fatalf("GenerateSkillCard: %v", err)
	}

	// Falls back to directory name.
	if sc.Metadata.Name == "" {
		t.Error("Name should not be empty")
	}
	// Falls back to first sentence.
	if sc.Metadata.Description != "You are a helpful assistant." {
		t.Errorf("Description = %q, want first sentence", sc.Metadata.Description)
	}
	if sc.Metadata.Version != "0.1.0" {
		t.Errorf("Version = %q, want 0.1.0", sc.Metadata.Version)
	}
	if sc.Metadata.Namespace != "acme" {
		t.Errorf("Namespace = %q, want acme", sc.Metadata.Namespace)
	}
}

func TestGenerateSkillCardMalformedFrontmatter(t *testing.T) {
	dir := t.TempDir()
	writeSkillMD(t, dir, "---\nbad: [yaml: {{\n---\nContent here.")

	sc, err := source.GenerateSkillCard(dir, "https://github.com/org/repo.git", "org")
	if err != nil {
		t.Fatalf("GenerateSkillCard: %v (should not fail on bad frontmatter)", err)
	}
	// Should fall back to directory name and first sentence.
	if sc.Metadata.Description != "Content here." {
		t.Errorf("Description = %q", sc.Metadata.Description)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/source/ -run TestGenerate -v`
Expected: FAIL — `GenerateSkillCard` not defined.

- [ ] **Step 3: Implement GenerateSkillCard**

```go
package source

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/redhat-et/skillimage/pkg/skillcard"
	"gopkg.in/yaml.v3"
)

func GenerateSkillCard(skillDir, cloneURL, orgFallback string) (*skillcard.SkillCard, error) {
	data, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		return nil, fmt.Errorf("reading SKILL.md: %w", err)
	}

	fm := parseFrontmatterRaw(data)
	body := stripFrontmatterStr(string(data))

	sc := &skillcard.SkillCard{
		APIVersion: "skillimage.io/v1alpha1",
		Kind:       "SkillCard",
		Metadata: skillcard.Metadata{
			Name:        stringFromMap(fm, "name", filepath.Base(skillDir)),
			Namespace:   orgFallback,
			Version:     normalizeVersion(stringFromMapNested(fm, "metadata", "version", "0.1.0")),
			Description: stringFromMap(fm, "description", firstSentence(body)),
			License:     stringFromMap(fm, "license", ""),
			Compatibility: stringFromMap(fm, "compatibility", ""),
		},
		Spec: &skillcard.Spec{
			Prompt: "SKILL.md",
		},
	}

	author := stringFromMapNested(fm, "metadata", "author", "")
	if author == "" {
		author = orgFallback
	}
	if author != "" {
		sc.Metadata.Authors = []skillcard.Author{{Name: author}}
	}

	return sc, nil
}

func parseFrontmatterRaw(data []byte) map[string]interface{} {
	s := string(data)
	if !strings.HasPrefix(s, "---") {
		return nil
	}
	end := strings.Index(s[3:], "\n---")
	if end < 0 {
		return nil
	}
	fmStr := s[4 : 3+end]
	var m map[string]interface{}
	if err := yaml.Unmarshal([]byte(fmStr), &m); err != nil {
		return nil
	}
	return m
}

func stripFrontmatterStr(s string) string {
	if !strings.HasPrefix(s, "---") {
		return s
	}
	end := strings.Index(s[3:], "\n---")
	if end < 0 {
		return s
	}
	return strings.TrimSpace(s[3+end+4:])
}

func stringFromMap(m map[string]interface{}, key, fallback string) string {
	if m == nil {
		return fallback
	}
	if v, ok := m[key].(string); ok && v != "" {
		return v
	}
	return fallback
}

func stringFromMapNested(m map[string]interface{}, outer, inner, fallback string) string {
	if m == nil {
		return fallback
	}
	sub, ok := m[outer].(map[string]interface{})
	if !ok {
		return fallback
	}
	if v, ok := sub[inner].(string); ok && v != "" {
		return v
	}
	return fallback
}

func firstSentence(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	if idx := strings.Index(body, "."); idx >= 0 {
		return body[:idx+1]
	}
	if idx := strings.Index(body, "\n"); idx >= 0 {
		return strings.TrimSpace(body[:idx])
	}
	return body
}

func normalizeVersion(v string) string {
	parts := strings.Split(v, ".")
	for len(parts) < 3 {
		parts = append(parts, "0")
	}
	return strings.Join(parts[:3], ".")
}
```

Remove the `parseFrontmatterRaw` stub from `discover.go` (it now
lives in `generate.go`). Remove the `"gopkg.in/yaml.v3"` import
from `discover.go` if it was added.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/source/ -run TestGenerate -v`
Expected: PASS — all 3 tests.

- [ ] **Step 5: Also re-run discover tests to verify no regression**

Run: `go test ./pkg/source/ -v`
Expected: PASS — all tests in the package.

- [ ] **Step 6: Commit**

```bash
git add pkg/source/generate.go pkg/source/generate_test.go pkg/source/discover.go
git commit -s -m "feat(source): generate SkillCard from SKILL.md frontmatter"
```

---

## Task 5: Build with pre-built SkillCard

**Files:**

- Modify: `pkg/oci/client.go:27-33` — add `SkillCard` field to
  `BuildOptions`
- Modify: `pkg/oci/build.go:27-52` — use pre-built SkillCard when
  provided
- Modify: `pkg/oci/oci_test.go` — add test

- [ ] **Step 1: Write failing test for Build with pre-built SkillCard**

Add to `pkg/oci/oci_test.go`:

```go
func TestBuildWithPrebuiltSkillCard(t *testing.T) {
	skillDir := t.TempDir()
	// Write only SKILL.md, no skill.yaml.
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(
		filepath.Join(skillDir, "SKILL.md"),
		[]byte("You are a test skill."),
		0o644,
	)

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	sc := &skillcard.SkillCard{
		APIVersion: "skillimage.io/v1alpha1",
		Kind:       "SkillCard",
		Metadata: skillcard.Metadata{
			Name:        "prebuilt-test",
			Namespace:   "test",
			Version:     "1.0.0",
			Description: "A prebuilt test skill.",
		},
		Spec: &skillcard.Spec{Prompt: "SKILL.md"},
	}

	ctx := context.Background()
	desc, err := client.Build(ctx, skillDir, oci.BuildOptions{SkillCard: sc})
	if err != nil {
		t.Fatalf("Build: %v", err)
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
	if images[0].Name != "test/prebuilt-test" {
		t.Errorf("image name = %q, want %q", images[0].Name, "test/prebuilt-test")
	}
}
```

Add imports for `"os"`, `"path/filepath"`, and
`"github.com/redhat-et/skillimage/pkg/skillcard"` to the test
file.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/oci/ -run TestBuildWithPrebuiltSkillCard -v`
Expected: FAIL — `BuildOptions` has no field `SkillCard`.

- [ ] **Step 3: Add SkillCard field to BuildOptions**

In `pkg/oci/client.go`, change lines 27-33:

```go
type BuildOptions struct {
	Tag       string
	MediaType MediaTypeProfile
	SkillCard *skillcard.SkillCard
}
```

Add import `"github.com/redhat-et/skillimage/pkg/skillcard"` to
`client.go`.

- [ ] **Step 4: Update Build() to use pre-built SkillCard**

In `pkg/oci/build.go`, replace lines 27-52 (the SkillCard
read/parse/validate block) with:

```go
func (c *Client) Build(ctx context.Context, skillDir string, opts BuildOptions) (ocispec.Descriptor, error) {
	var sc *skillcard.SkillCard

	if opts.SkillCard != nil {
		sc = opts.SkillCard
	} else {
		// 1. Read and parse skill.yaml.
		skillPath := filepath.Join(skillDir, "skill.yaml")
		f, err := os.Open(skillPath)
		if err != nil {
			return ocispec.Descriptor{}, fmt.Errorf("opening skill.yaml: %w", err)
		}
		defer func() { _ = f.Close() }()

		var parseErr error
		sc, parseErr = skillcard.Parse(f)
		if parseErr != nil {
			return ocispec.Descriptor{}, fmt.Errorf("parsing skill.yaml: %w", parseErr)
		}

		// 2. Validate the SkillCard.
		validationErrors, valErr := skillcard.Validate(sc)
		if valErr != nil {
			return ocispec.Descriptor{}, fmt.Errorf("validating skill.yaml: %w", valErr)
		}
		if len(validationErrors) > 0 {
			var msgs []string
			for _, ve := range validationErrors {
				msgs = append(msgs, ve.String())
			}
			return ocispec.Descriptor{}, fmt.Errorf("skill.yaml validation failed: %s", strings.Join(msgs, "; "))
		}
	}

	// 2b. Count words in SKILL.md if present, excluding YAML frontmatter.
	// ... rest of the method unchanged from line 54 onward ...
```

- [ ] **Step 5: Run all OCI tests to verify nothing broke**

Run: `go test ./pkg/oci/ -v`
Expected: PASS — all existing tests plus the new one.

- [ ] **Step 6: Commit**

```bash
git add pkg/oci/client.go pkg/oci/build.go pkg/oci/oci_test.go
git commit -s -m "feat(oci): accept pre-built SkillCard in BuildOptions"
```

---

## Task 6: Source resolver orchestrator

**Files:**

- Create: `pkg/source/source.go`
- Create: `pkg/source/source_test.go`

- [ ] **Step 1: Write failing tests for IsRemote and OrgFromCloneURL**

```go
package source_test

import (
	"testing"

	"github.com/redhat-et/skillimage/pkg/source"
)

func TestIsRemote(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"https://github.com/org/repo", true},
		{"http://gitlab.com/org/repo", true},
		{"/local/path/to/skills", false},
		{"./relative/path", false},
		{"skills/", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := source.IsRemote(tt.input); got != tt.want {
				t.Errorf("IsRemote(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestOrgFromCloneURL(t *testing.T) {
	tests := []struct {
		cloneURL string
		want     string
	}{
		{"https://github.com/anthropics/skills.git", "anthropics"},
		{"https://gitlab.com/org/repo.git", "org"},
		{"https://example.com/repo", ""},
	}

	for _, tt := range tests {
		t.Run(tt.cloneURL, func(t *testing.T) {
			if got := source.OrgFromCloneURL(tt.cloneURL); got != tt.want {
				t.Errorf("OrgFromCloneURL(%q) = %q, want %q", tt.cloneURL, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/source/ -run "TestIsRemote|TestOrgFrom" -v`
Expected: FAIL — functions not defined.

- [ ] **Step 3: Implement IsRemote, OrgFromCloneURL, and Resolve**

```go
package source

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/redhat-et/skillimage/pkg/skillcard"
)

func IsRemote(input string) bool {
	return strings.HasPrefix(input, "https://") || strings.HasPrefix(input, "http://")
}

func OrgFromCloneURL(cloneURL string) string {
	u, err := url.Parse(cloneURL)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) >= 2 {
		return parts[0]
	}
	return ""
}

type ResolveResult struct {
	Skills    []ResolvedSkill
	Cleanup   func()
	SourceURL string
}

type ResolvedSkill struct {
	Dir       string
	Name      string
	SkillCard *skillcard.SkillCard
}

func Resolve(ctx context.Context, input string, ref string, filter string) (*ResolveResult, error) {
	if !IsRemote(input) {
		return nil, fmt.Errorf("not a remote source: %s", input)
	}

	src, err := ParseGitURL(input)
	if err != nil {
		return nil, err
	}

	cloneResult, err := Clone(ctx, src, CloneOptions{RefOverride: ref})
	if err != nil {
		return nil, err
	}

	discovered, err := Discover(cloneResult.Dir, filter)
	if err != nil {
		cloneResult.Cleanup()
		return nil, err
	}

	org := OrgFromCloneURL(src.CloneURL)

	var skills []ResolvedSkill
	for _, d := range discovered {
		var sc *skillcard.SkillCard

		// Use existing skill.yaml if present.
		if hasSkillYAML(d.Dir) {
			skills = append(skills, ResolvedSkill{Dir: d.Dir, Name: d.Name, SkillCard: nil})
			continue
		}

		sc, err := GenerateSkillCard(d.Dir, src.CloneURL, org)
		if err != nil {
			fmt.Printf("Warning: skipping %s: %v\n", d.Name, err)
			continue
		}

		// Inject provenance.
		relPath := relativeToClone(cloneResult.Dir, d.Dir, src.SubPath)
		sc.Provenance = &skillcard.Provenance{
			Source: src.CloneURL,
			Commit: cloneResult.CommitSHA,
			Path:   relPath,
		}

		skills = append(skills, ResolvedSkill{Dir: d.Dir, Name: d.Name, SkillCard: sc})
	}

	if len(skills) == 0 {
		cloneResult.Cleanup()
		return nil, fmt.Errorf("no skills could be resolved from %s", input)
	}

	return &ResolveResult{
		Skills:    skills,
		Cleanup:   cloneResult.Cleanup,
		SourceURL: input,
	}, nil
}

func hasSkillYAML(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "skill.yaml"))
	return err == nil
}

func relativeToClone(cloneDir, skillDir, subPath string) string {
	rel, err := filepath.Rel(cloneDir, skillDir)
	if err != nil {
		return filepath.Base(skillDir)
	}
	if subPath != "" {
		return filepath.ToSlash(filepath.Join(subPath, rel))
	}
	return filepath.ToSlash(rel)
}
```

Add `"os"` and `"path/filepath"` to imports.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/source/ -run "TestIsRemote|TestOrgFrom" -v`
Expected: PASS.

- [ ] **Step 5: Run all source package tests**

Run: `go test ./pkg/source/ -v`
Expected: PASS — all tests in the package.

- [ ] **Step 6: Commit**

```bash
git add pkg/source/source.go pkg/source/source_test.go
git commit -s -m "feat(source): add Resolve orchestrator with provenance injection"
```

---

## Task 7: CLI integration

**Files:**

- Modify: `internal/cli/build.go` — add `--ref`, `--filter` flags,
  remote source path

- [ ] **Step 1: Update build command with new flags and remote path**

Replace the full content of `internal/cli/build.go`:

```go
package cli

import (
	"context"
	"fmt"

	"github.com/redhat-et/skillimage/pkg/oci"
	"github.com/redhat-et/skillimage/pkg/source"
	"github.com/spf13/cobra"
)

func newBuildCmd() *cobra.Command {
	var tag string
	var mediaType string
	var ref string
	var filter string
	cmd := &cobra.Command{
		Use:   "build <dir-or-url>",
		Short: "Build a skill directory or Git repo into local OCI images",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if source.IsRemote(args[0]) {
				return runBuildRemote(cmd, args[0], tag, mediaType, ref, filter)
			}
			return runBuild(cmd, args[0], tag, mediaType)
		},
	}
	cmd.Flags().StringVar(&tag, "tag", "", "override the image tag (default: <version>-draft)")
	cmd.Flags().StringVar(&mediaType, "media-type", "", `media type profile: "standard" (default) or "redhat" (for oc-mirror)`)
	cmd.Flags().StringVar(&ref, "ref", "", "Git ref to checkout (branch, tag, or commit SHA)")
	cmd.Flags().StringVar(&filter, "filter", "", "glob pattern to filter skills by name")
	return cmd
}

func runBuild(cmd *cobra.Command, dir, tag, mediaType string) error {
	profile, err := oci.ParseMediaTypeProfile(mediaType)
	if err != nil {
		return err
	}

	client, err := defaultClient()
	if err != nil {
		return err
	}

	desc, err := client.Build(context.Background(), dir, oci.BuildOptions{
		Tag:       tag,
		MediaType: profile,
	})
	if err != nil {
		return fmt.Errorf("building %s: %w", dir, err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Built %s\nDigest: %s\n", dir, desc.Digest)
	return nil
}

func runBuildRemote(cmd *cobra.Command, rawURL, tag, mediaType, ref, filter string) error {
	profile, err := oci.ParseMediaTypeProfile(mediaType)
	if err != nil {
		return err
	}

	ctx := context.Background()

	fmt.Fprintf(cmd.OutOrStdout(), "Cloning %s", rawURL)
	if ref != "" {
		fmt.Fprintf(cmd.OutOrStdout(), " (ref: %s)", ref)
	}
	fmt.Fprintln(cmd.OutOrStdout(), "...")

	result, err := source.Resolve(ctx, rawURL, ref, filter)
	if err != nil {
		return err
	}
	defer result.Cleanup()

	if tag != "" && len(result.Skills) > 1 {
		return fmt.Errorf("--tag cannot be used when building multiple skills")
	}

	client, err := defaultClient()
	if err != nil {
		return err
	}

	var built, failed int
	for i, skill := range result.Skills {
		fmt.Fprintf(cmd.OutOrStdout(), "Building %s (%d/%d)...\n", skill.Name, i+1, len(result.Skills))

		desc, err := client.Build(ctx, skill.Dir, oci.BuildOptions{
			Tag:       tag,
			MediaType: profile,
			SkillCard: skill.SkillCard,
		})
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "  Error: %v\n", err)
			failed++
			continue
		}
		fmt.Fprintf(cmd.OutOrStdout(), "  Digest: %s\n", desc.Digest)
		built++
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\nBuilt %d skills from %s\n", built, rawURL)
	if failed > 0 {
		return fmt.Errorf("%d skill(s) failed to build", failed)
	}
	return nil
}
```

- [ ] **Step 2: Run lint and compile check**

Run: `go build ./...`
Expected: compiles with no errors.

- [ ] **Step 3: Run all tests to verify nothing is broken**

Run: `go test ./... -v`
Expected: PASS — all tests across all packages.

- [ ] **Step 4: Commit**

```bash
git add internal/cli/build.go
git commit -s -m "feat(cli): support Git URLs in build command with --ref and --filter"
```

---

## Task 8: End-to-end manual verification

- [ ] **Step 1: Build from a local directory (regression)**

Run: `go run ./cmd/skillctl build examples/hello-world`
Expected: `Built examples/hello-world\nDigest: sha256:...`

- [ ] **Step 2: Build from a public GitHub repo**

Run: `go run ./cmd/skillctl build https://github.com/anthropics/skills`
Expected: clones the repo, discovers skills, builds each one,
prints summary.

If the Anthropic skills repo doesn't exist or has a different
structure, use this project's own examples:

Run: `go run ./cmd/skillctl build https://github.com/redhat-et/skillimage/tree/main/examples`
Expected: discovers and builds the example skills.

- [ ] **Step 3: Build with --ref flag**

Run: `go run ./cmd/skillctl build https://github.com/redhat-et/skillimage/tree/main/examples --ref main`
Expected: same result as step 2 (explicit ref matches default).

- [ ] **Step 4: Build with --filter flag**

Run: `go run ./cmd/skillctl build https://github.com/redhat-et/skillimage/tree/main/examples --filter "hello-*"`
Expected: only builds hello-world, skips others.

- [ ] **Step 5: Verify images were stored locally**

Run: `go run ./cmd/skillctl list`
Expected: lists the images built in steps 1-4.

- [ ] **Step 6: Run full test suite and linter**

Run: `make test && make lint`
Expected: all pass.

- [ ] **Step 7: Commit any fixes from manual testing**

```bash
git add -A
git commit -s -m "fix: adjustments from end-to-end testing"
```

Only if changes were needed; skip if everything passed.
