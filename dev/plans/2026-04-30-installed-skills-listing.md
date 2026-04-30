# Provenance tracking and installed skills listing — implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task.
> Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Populate provenance metadata during `skillctl install`
and add `list --installed` to show installed skills across agent
directories.

**Architecture:** A `ResolveDigest()` method on `pkg/oci.Client`
returns the digest for a local ref. The install command uses it
to populate `Provenance.source` and `Provenance.commit` in
`skill.yaml` after unpacking. A new `pkg/installed/` package
scans agent target directories for installed skills. The `list`
command gains `--installed`, `--target`, and `--output` flags.

**Tech Stack:** Go, cobra, oras-go, yaml.v3

---

## File structure

| File | Action | Responsibility |
|------|--------|----------------|
| `pkg/oci/resolve.go` | Create | `ResolveDigest` method |
| `pkg/oci/resolve_test.go` | Create | Tests for `ResolveDigest` |
| `internal/cli/install.go` | Modify | Write provenance after unpack |
| `pkg/installed/installed.go` | Create | `InstalledSkill` type, `Scan` function |
| `pkg/installed/installed_test.go` | Create | Tests for `Scan` |
| `internal/cli/list.go` | Modify | Add `--installed`, `--target`, `-o` flags |

---

## Task 1: ResolveDigest — test

**Files:**
- Create: `pkg/oci/resolve_test.go`

- [ ] **Step 1: Write TestResolveDigest**

```go
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
```

- [ ] **Step 2: Write TestResolveDigestNotFound**

```go
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
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./pkg/oci/ -run "TestResolveDigest" -v`

Expected: compilation failure — `client.ResolveDigest` undefined.

- [ ] **Step 4: Commit**

```bash
git add pkg/oci/resolve_test.go
git commit -s -m "test: add failing tests for Client.ResolveDigest"
```

---

## Task 2: ResolveDigest — implementation

**Files:**
- Create: `pkg/oci/resolve.go`

- [ ] **Step 1: Implement ResolveDigest**

```go
package oci

import (
    "context"
    "errors"
    "fmt"

    "oras.land/oras-go/v2/errdef"
)

// ResolveDigest returns the digest of a locally stored image
// identified by its tag reference.
func (c *Client) ResolveDigest(ctx context.Context, ref string) (string, error) {
    desc, err := c.store.Resolve(ctx, ref)
    if err != nil {
        if errors.Is(err, errdef.ErrNotFound) {
            return "", fmt.Errorf("image not found: %s", ref)
        }
        return "", fmt.Errorf("resolving %s: %w", ref, err)
    }
    return desc.Digest.String(), nil
}
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `go test ./pkg/oci/ -run "TestResolveDigest" -v`

Expected: both tests pass.

- [ ] **Step 3: Run full test suite**

Run: `go test ./...`

Expected: all tests pass.

- [ ] **Step 4: Commit**

```bash
git add pkg/oci/resolve.go
git commit -s -m "feat: add Client.ResolveDigest for local ref lookup"
```

---

## Task 3: Install provenance — test and implementation

**Files:**
- Modify: `internal/cli/install.go`

This task modifies `runInstall` to write provenance into
`skill.yaml` after unpacking. Since the install command is a CLI
function (not a library method), we test it end-to-end at the
`pkg/oci` level by verifying the provenance flow: build, unpack,
read back, check fields.

- [ ] **Step 1: Write TestInstallProvenance in pkg/oci/resolve_test.go**

Add this test to `pkg/oci/resolve_test.go` (it tests the
provenance write-back flow using the same build/unpack pattern
the install command uses):

```go
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
    f.Close()
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
    wf.Close()
    if err != nil {
        t.Fatalf("serializing skill.yaml: %v", err)
    }

    // Read it back and verify provenance fields.
    f2, err := os.Open(skillPath)
    if err != nil {
        t.Fatalf("re-opening skill.yaml: %v", err)
    }
    defer f2.Close()
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
```

Add `"os"`, `"path/filepath"`, and
`"github.com/redhat-et/skillimage/pkg/skillcard"` to the imports
(skillcard is already imported in oci_test.go's package — this
file is in the same package `oci_test`).

- [ ] **Step 2: Run test to verify it passes**

Run: `go test ./pkg/oci/ -run "TestInstallProvenance" -v`

Expected: PASS (the test exercises Parse/Serialize, not the
install CLI — this verifies the provenance roundtrip works).

- [ ] **Step 3: Modify runInstall to write provenance**

In `internal/cli/install.go`, update `runInstall`. The new
function body replaces the current one:

```go
func runInstall(cmd *cobra.Command, ref string, target string, outputDir string) error {
    if target == "" && outputDir == "" {
        return fmt.Errorf("specify --target <agent> or -o <directory>")
    }
    if target != "" && outputDir != "" {
        return fmt.Errorf("use --target or -o, not both")
    }

    if target != "" {
        relPath, ok := agentTargets[strings.ToLower(target)]
        if !ok {
            var names []string
            for k := range agentTargets {
                names = append(names, k)
            }
            return fmt.Errorf("unknown target %q (supported: %s)", target, strings.Join(names, ", "))
        }
        home, err := os.UserHomeDir()
        if err != nil {
            return fmt.Errorf("finding home directory: %w", err)
        }
        outputDir = filepath.Join(home, relPath)
    }

    if err := os.MkdirAll(outputDir, 0o755); err != nil {
        return fmt.Errorf("creating directory %s: %w", outputDir, err)
    }

    client, err := defaultClient()
    if err != nil {
        return err
    }

    ctx := cmd.Context()
    if err := client.Unpack(ctx, ref, outputDir); err != nil {
        return fmt.Errorf("installing %s: %w", ref, err)
    }

    // Extract skill name for path and output message.
    skillName := ref
    if idx := strings.LastIndex(ref, "/"); idx >= 0 {
        skillName = ref[idx+1:]
    }
    if idx := strings.LastIndex(skillName, ":"); idx >= 0 {
        skillName = skillName[:idx]
    }
    dest := filepath.Join(outputDir, skillName)

    // Write provenance into skill.yaml.
    if err := writeProvenance(ctx, client, ref, dest); err != nil {
        return fmt.Errorf("writing provenance: %w", err)
    }

    fmt.Fprintf(cmd.OutOrStdout(), "Installed %s to %s\n", ref, dest)
    return nil
}
```

Add the `writeProvenance` helper function below `runInstall`:

```go
func writeProvenance(ctx context.Context, client *oci.Client, ref, skillDir string) error {
    digest, err := client.ResolveDigest(ctx, ref)
    if err != nil {
        return err
    }

    skillPath := filepath.Join(skillDir, "skill.yaml")
    f, err := os.Open(skillPath)
    if err != nil {
        return fmt.Errorf("opening skill.yaml: %w", err)
    }
    sc, err := skillcard.Parse(f)
    f.Close()
    if err != nil {
        return fmt.Errorf("parsing skill.yaml: %w", err)
    }

    if sc.Provenance == nil {
        sc.Provenance = &skillcard.Provenance{}
    }
    sc.Provenance.Source = ref
    sc.Provenance.Commit = digest

    wf, err := os.Create(skillPath)
    if err != nil {
        return fmt.Errorf("creating skill.yaml: %w", err)
    }
    defer wf.Close()
    return skillcard.Serialize(sc, wf)
}
```

Update the imports in `install.go`:

```go
import (
    "context"
    "fmt"
    "os"
    "path/filepath"
    "strings"

    "github.com/spf13/cobra"

    "github.com/redhat-et/skillimage/pkg/oci"
    "github.com/redhat-et/skillimage/pkg/skillcard"
)
```

- [ ] **Step 4: Verify build**

Run: `go build ./cmd/skillctl/`

Expected: clean build.

- [ ] **Step 5: Run full test suite**

Run: `go test ./...`

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/cli/install.go pkg/oci/resolve_test.go
git commit -s -m "feat: write provenance into skill.yaml during install"
```

---

## Task 4: Installed skills scanner — test

**Files:**
- Create: `pkg/installed/installed_test.go`

- [ ] **Step 1: Write TestScan**

```go
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
```

- [ ] **Step 2: Write TestScanNonExistentDir**

```go
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
```

- [ ] **Step 3: Write TestScanMalformedSkillYAML**

```go
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
```

- [ ] **Step 4: Run tests to verify they fail**

Run: `go test ./pkg/installed/ -v`

Expected: compilation failure — package does not exist.

- [ ] **Step 5: Commit**

```bash
git add pkg/installed/installed_test.go
git commit -s -m "test: add failing tests for installed skills scanner"
```

---

## Task 5: Installed skills scanner — implementation

**Files:**
- Create: `pkg/installed/installed.go`

- [ ] **Step 1: Implement the InstalledSkill type and Scan function**

```go
package installed

import (
    "fmt"
    "os"
    "path/filepath"

    "github.com/redhat-et/skillimage/pkg/skillcard"
)

// InstalledSkill holds metadata for a skill installed to an agent directory.
type InstalledSkill struct {
    Name    string
    Version string
    Source  string
    Digest  string
    Target  string
    Path    string
}

// Scan reads agent target directories and returns metadata for each
// installed skill. targets maps target names (e.g., "claude") to
// directory paths. Directories that don't exist are silently skipped.
// Malformed skill.yaml files are skipped with a warning to stderr.
func Scan(targets map[string]string) ([]InstalledSkill, error) {
    var skills []InstalledSkill

    for target, dir := range targets {
        entries, err := os.ReadDir(dir)
        if err != nil {
            if os.IsNotExist(err) {
                continue
            }
            return nil, fmt.Errorf("reading %s: %w", dir, err)
        }

        for _, entry := range entries {
            if !entry.IsDir() {
                continue
            }

            skillPath := filepath.Join(dir, entry.Name(), "skill.yaml")
            f, err := os.Open(skillPath)
            if err != nil {
                continue
            }

            sc, err := skillcard.Parse(f)
            f.Close()
            if err != nil {
                fmt.Fprintf(os.Stderr, "Warning: skipping %s: %v\n", skillPath, err)
                continue
            }

            skill := InstalledSkill{
                Name:    sc.Metadata.Name,
                Version: sc.Metadata.Version,
                Target:  target,
                Path:    filepath.Join(dir, entry.Name()),
            }
            if sc.Provenance != nil {
                skill.Source = sc.Provenance.Source
                skill.Digest = sc.Provenance.Commit
            }

            skills = append(skills, skill)
        }
    }

    return skills, nil
}
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `go test ./pkg/installed/ -v`

Expected: all three tests pass.

- [ ] **Step 3: Run full test suite**

Run: `go test ./...`

Expected: all tests pass.

- [ ] **Step 4: Commit**

```bash
git add pkg/installed/installed.go
git commit -s -m "feat: add installed skills scanner package"
```

---

## Task 6: list --installed flags and output

**Files:**
- Modify: `internal/cli/list.go`

- [ ] **Step 1: Rewrite list.go with new flags**

Replace the entire file content:

```go
package cli

import (
    "fmt"
    "os"
    "path/filepath"
    "strings"
    "text/tabwriter"

    "github.com/spf13/cobra"

    "github.com/redhat-et/skillimage/pkg/installed"
)

func newListCmd() *cobra.Command {
    var showInstalled bool
    var target string
    var outputDir string

    cmd := &cobra.Command{
        Use:     "list",
        Aliases: []string{"ls"},
        Short:   "List skill images in local store",
        Long: `List skill images stored locally, or show installed skills
across agent directories.

Without --installed, lists images in the local OCI store.
With --installed, scans agent skill directories for installed skills.

Supported targets for --installed:
  claude    ~/.claude/skills/
  cursor    ~/.cursor/skills/
  windsurf  ~/.codeium/windsurf/skills/
  opencode  ~/.config/opencode/skills/
  openclaw  ~/.openclaw/skills/`,
        Args: cobra.NoArgs,
        RunE: func(cmd *cobra.Command, args []string) error {
            if showInstalled {
                return runListInstalled(cmd, target, outputDir)
            }
            return runList(cmd)
        },
    }

    cmd.Flags().BoolVarP(&showInstalled, "installed", "i", false, "list installed skills")
    cmd.Flags().StringVarP(&target, "target", "t", "", "filter to a specific agent target")
    cmd.Flags().StringVarP(&outputDir, "output", "o", "", "scan a custom directory")

    return cmd
}

func runList(cmd *cobra.Command) error {
    client, err := defaultClient()
    if err != nil {
        return err
    }

    images, err := client.ListLocal()
    if err != nil {
        return fmt.Errorf("listing images: %w", err)
    }

    if len(images) == 0 {
        fmt.Fprintln(cmd.OutOrStdout(), "No images found in local store.")
        return nil
    }

    w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
    fmt.Fprintln(w, "NAME\tTAG\tSTATUS\tDIGEST\tCREATED")
    for _, img := range images {
        shortDigest := img.Digest
        if len(shortDigest) > 19 {
            shortDigest = shortDigest[:19]
        }
        fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
            img.Name, img.Tag, img.Status, shortDigest, img.Created)
    }
    return w.Flush()
}

func runListInstalled(cmd *cobra.Command, target, outputDir string) error {
    if target != "" && outputDir != "" {
        return fmt.Errorf("use --target or -o, not both")
    }

    targets, err := resolveListTargets(target, outputDir)
    if err != nil {
        return err
    }

    skills, err := installed.Scan(targets)
    if err != nil {
        return fmt.Errorf("scanning installed skills: %w", err)
    }

    if len(skills) == 0 {
        fmt.Fprintln(cmd.OutOrStdout(), "No installed skills found.")
        return nil
    }

    w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
    fmt.Fprintln(w, "NAME\tVERSION\tSOURCE\tTARGET")
    for _, s := range skills {
        source := s.Source
        if source == "" {
            source = "(local)"
        }
        fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", s.Name, s.Version, source, s.Target)
    }
    return w.Flush()
}

func resolveListTargets(target, outputDir string) (map[string]string, error) {
    if outputDir != "" {
        abs, err := filepath.Abs(outputDir)
        if err != nil {
            return nil, fmt.Errorf("resolving path: %w", err)
        }
        return map[string]string{outputDir: abs}, nil
    }

    home, err := os.UserHomeDir()
    if err != nil {
        return nil, fmt.Errorf("finding home directory: %w", err)
    }

    if target != "" {
        relPath, ok := agentTargets[strings.ToLower(target)]
        if !ok {
            var names []string
            for k := range agentTargets {
                names = append(names, k)
            }
            return nil, fmt.Errorf("unknown target %q (supported: %s)", target, strings.Join(names, ", "))
        }
        return map[string]string{target: filepath.Join(home, relPath)}, nil
    }

    targets := make(map[string]string, len(agentTargets))
    for name, relPath := range agentTargets {
        targets[name] = filepath.Join(home, relPath)
    }
    return targets, nil
}
```

- [ ] **Step 2: Verify build**

Run: `go build ./cmd/skillctl/`

Expected: clean build.

- [ ] **Step 3: Run linter**

Run: `make lint`

Expected: no new warnings.

- [ ] **Step 4: Run full test suite**

Run: `go test ./...`

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/list.go
git commit -s -m "feat: add list --installed to show installed skills

Closes #27 (partially)"
```
