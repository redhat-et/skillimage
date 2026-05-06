# Upgrade command implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task.
> Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `skillctl upgrade` to pull newer published versions
of installed skills, and `list --installed --upgradable` to
preview available upgrades.

**Architecture:** A `CheckUpgrades` function in `pkg/installed/`
takes installed skills and a tag-listing function, queries remote
registries for published versions, and returns upgrade candidates
via semver comparison. The CLI `upgrade` command pulls and
re-installs each candidate. The `list --upgradable` flag reuses
the same checking logic for display.

**Tech Stack:** Go, cobra, oras-go, semver/v3

---

## File structure

| File | Action | Responsibility |
|------|--------|----------------|
| `pkg/installed/check.go` | Create | `CheckUpgrades`, types, version logic |
| `pkg/installed/check_test.go` | Create | Unit tests with fake tag lister |
| `internal/cli/list.go` | Modify | Add `--upgradable` flag |
| `internal/cli/upgrade.go` | Create | `upgrade` command |
| `internal/cli/root.go` | Modify | Register `newUpgradeCmd()` |

---

## Task 1: CheckUpgrades — test

**Files:**
- Create: `pkg/installed/check_test.go`

- [ ] **Step 1: Write TestCheckUpgrades_HasUpgrade**

```go
package installed_test

import (
    "context"
    "testing"

    "github.com/redhat-et/skillimage/pkg/installed"
)

func TestCheckUpgrades_HasUpgrade(t *testing.T) {
    skills := []installed.InstalledSkill{
        {
            Name:    "my-skill",
            Version: "1.0.0",
            Source:  "quay.io/acme/my-skill:1.0.0",
            Target:  "claude",
        },
    }

    lister := func(ctx context.Context, repo string, skipTLS bool) ([]string, error) {
        return []string{"1.0.0-draft", "1.0.0-testing", "1.0.0", "2.0.0-draft", "2.0.0"}, nil
    }

    candidates, err := installed.CheckUpgrades(context.Background(), skills,
        installed.CheckOptions{TagLister: lister})
    if err != nil {
        t.Fatalf("CheckUpgrades: %v", err)
    }
    if len(candidates) != 1 {
        t.Fatalf("expected 1 candidate, got %d", len(candidates))
    }
    if candidates[0].LatestVersion != "2.0.0" {
        t.Errorf("latest = %q, want %q", candidates[0].LatestVersion, "2.0.0")
    }
    if candidates[0].LatestRef != "quay.io/acme/my-skill:2.0.0" {
        t.Errorf("ref = %q, want %q", candidates[0].LatestRef, "quay.io/acme/my-skill:2.0.0")
    }
}
```

- [ ] **Step 2: Write TestCheckUpgrades_AlreadyLatest**

```go
func TestCheckUpgrades_AlreadyLatest(t *testing.T) {
    skills := []installed.InstalledSkill{
        {
            Name:    "my-skill",
            Version: "2.0.0",
            Source:  "quay.io/acme/my-skill:2.0.0",
            Target:  "claude",
        },
    }

    lister := func(ctx context.Context, repo string, skipTLS bool) ([]string, error) {
        return []string{"1.0.0", "2.0.0"}, nil
    }

    candidates, err := installed.CheckUpgrades(context.Background(), skills,
        installed.CheckOptions{TagLister: lister})
    if err != nil {
        t.Fatalf("CheckUpgrades: %v", err)
    }
    if len(candidates) != 0 {
        t.Errorf("expected 0 candidates, got %d", len(candidates))
    }
}
```

- [ ] **Step 3: Write TestCheckUpgrades_NoProvenance**

```go
func TestCheckUpgrades_NoProvenance(t *testing.T) {
    skills := []installed.InstalledSkill{
        {
            Name:    "local-skill",
            Version: "1.0.0",
            Source:  "",
            Target:  "claude",
        },
    }

    lister := func(ctx context.Context, repo string, skipTLS bool) ([]string, error) {
        t.Fatal("should not be called for local skills")
        return nil, nil
    }

    candidates, err := installed.CheckUpgrades(context.Background(), skills,
        installed.CheckOptions{TagLister: lister})
    if err != nil {
        t.Fatalf("CheckUpgrades: %v", err)
    }
    if len(candidates) != 0 {
        t.Errorf("expected 0 candidates, got %d", len(candidates))
    }
}
```

- [ ] **Step 4: Write TestCheckUpgrades_OnlyDraftTags**

```go
func TestCheckUpgrades_OnlyDraftTags(t *testing.T) {
    skills := []installed.InstalledSkill{
        {
            Name:    "my-skill",
            Version: "1.0.0",
            Source:  "quay.io/acme/my-skill:1.0.0",
            Target:  "claude",
        },
    }

    lister := func(ctx context.Context, repo string, skipTLS bool) ([]string, error) {
        return []string{"1.0.0", "2.0.0-draft", "3.0.0-testing"}, nil
    }

    candidates, err := installed.CheckUpgrades(context.Background(), skills,
        installed.CheckOptions{TagLister: lister})
    if err != nil {
        t.Fatalf("CheckUpgrades: %v", err)
    }
    if len(candidates) != 0 {
        t.Errorf("expected 0 candidates (only draft/testing newer), got %d", len(candidates))
    }
}
```

- [ ] **Step 5: Write TestCheckUpgrades_LocalRef**

```go
func TestCheckUpgrades_LocalRef(t *testing.T) {
    skills := []installed.InstalledSkill{
        {
            Name:    "my-skill",
            Version: "1.0.0",
            Source:  "toddward/red-hat-quick-deck:0.1.0-draft",
            Target:  "opencode",
        },
    }

    lister := func(ctx context.Context, repo string, skipTLS bool) ([]string, error) {
        t.Fatal("should not be called for local refs")
        return nil, nil
    }

    candidates, err := installed.CheckUpgrades(context.Background(), skills,
        installed.CheckOptions{TagLister: lister})
    if err != nil {
        t.Fatalf("CheckUpgrades: %v", err)
    }
    if len(candidates) != 0 {
        t.Errorf("expected 0 candidates for local ref, got %d", len(candidates))
    }
}
```

- [ ] **Step 6: Write TestCheckUpgrades_InvalidSemver**

```go
func TestCheckUpgrades_InvalidSemver(t *testing.T) {
    skills := []installed.InstalledSkill{
        {
            Name:    "my-skill",
            Version: "not-a-version",
            Source:  "quay.io/acme/my-skill:latest",
            Target:  "claude",
        },
    }

    lister := func(ctx context.Context, repo string, skipTLS bool) ([]string, error) {
        t.Fatal("should not be called for non-semver versions")
        return nil, nil
    }

    candidates, err := installed.CheckUpgrades(context.Background(), skills,
        installed.CheckOptions{TagLister: lister})
    if err != nil {
        t.Fatalf("CheckUpgrades: %v", err)
    }
    if len(candidates) != 0 {
        t.Errorf("expected 0 candidates, got %d", len(candidates))
    }
}
```

- [ ] **Step 7: Run tests to verify they fail**

Run: `go test ./pkg/installed/ -run "TestCheckUpgrades" -v`

Expected: compilation failure — `CheckUpgrades` undefined.

- [ ] **Step 8: Commit**

```bash
git add pkg/installed/check_test.go
git commit -s -m "test: add failing tests for CheckUpgrades"
```

---

## Task 2: CheckUpgrades — implementation

**Files:**
- Create: `pkg/installed/check.go`

- [ ] **Step 1: Implement CheckUpgrades**

```go
package installed

import (
    "context"
    "strings"

    "github.com/Masterminds/semver/v3"
)

// TagLister queries a remote registry for available tags.
// The repo argument is a full repository reference without a tag
// (e.g., "quay.io/acme/my-skill").
type TagLister func(ctx context.Context, repo string, skipTLSVerify bool) ([]string, error)

// CheckOptions configures the CheckUpgrades operation.
type CheckOptions struct {
    SkipTLSVerify bool
    TagLister     TagLister
}

// UpgradeCandidate describes an installed skill that has a newer
// published version available in its source registry.
type UpgradeCandidate struct {
    Installed     InstalledSkill
    LatestVersion string
    LatestRef     string
}

// CheckUpgrades queries source registries for each installed skill
// and returns candidates that have newer published versions available.
// Skills without provenance, with non-semver versions, or with
// local-only source refs are silently skipped.
func CheckUpgrades(ctx context.Context, skills []InstalledSkill, opts CheckOptions) ([]UpgradeCandidate, error) {
    var candidates []UpgradeCandidate

    for _, skill := range skills {
        if skill.Source == "" {
            continue
        }

        if !looksRemote(skill.Source) {
            continue
        }

        installedVer, err := semver.StrictNewVersion(skill.Version)
        if err != nil {
            continue
        }

        repo := repoFromRef(skill.Source)

        tags, err := opts.TagLister(ctx, repo, opts.SkipTLSVerify)
        if err != nil {
            continue
        }

        latest := highestPublished(tags)
        if latest == nil || !latest.GreaterThan(installedVer) {
            continue
        }

        candidates = append(candidates, UpgradeCandidate{
            Installed:     skill,
            LatestVersion: latest.Original(),
            LatestRef:     repo + ":" + latest.Original(),
        })
    }

    return candidates, nil
}

// looksRemote returns true if the ref contains a registry host.
// A ref is remote if its first path segment contains a dot or colon
// (e.g., "quay.io/...", "localhost:5000/...").
func looksRemote(ref string) bool {
    first := ref
    if idx := strings.Index(ref, "/"); idx >= 0 {
        first = ref[:idx]
    }
    return strings.ContainsAny(first, ".:")
}

// repoFromRef strips the tag or digest from a ref, returning the
// repository portion (e.g., "quay.io/acme/skill:1.0.0" becomes
// "quay.io/acme/skill").
func repoFromRef(ref string) string {
    if idx := strings.Index(ref, "@"); idx >= 0 {
        return ref[:idx]
    }
    lastSlash := strings.LastIndex(ref, "/")
    if lastSlash < 0 {
        if idx := strings.LastIndex(ref, ":"); idx >= 0 {
            return ref[:idx]
        }
        return ref
    }
    tail := ref[lastSlash+1:]
    if idx := strings.LastIndex(tail, ":"); idx >= 0 {
        return ref[:lastSlash+1+idx]
    }
    return ref
}

// highestPublished finds the highest semver version among tags that
// represent published skills (no -draft or -testing suffix).
func highestPublished(tags []string) *semver.Version {
    var best *semver.Version
    for _, tag := range tags {
        if strings.HasSuffix(tag, "-draft") || strings.HasSuffix(tag, "-testing") {
            continue
        }
        v, err := semver.StrictNewVersion(tag)
        if err != nil {
            continue
        }
        if best == nil || v.GreaterThan(best) {
            best = v
        }
    }
    return best
}
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `go test ./pkg/installed/ -run "TestCheckUpgrades" -v`

Expected: all six tests pass.

- [ ] **Step 3: Run full test suite**

Run: `go test ./...`

Expected: all tests pass.

- [ ] **Step 4: Commit**

```bash
git add pkg/installed/check.go
git commit -s -m "feat: add CheckUpgrades for version comparison"
```

---

## Task 3: list --installed --upgradable

**Files:**
- Modify: `internal/cli/list.go`

- [ ] **Step 1: Add --upgradable flag and update runListInstalled**

In `newListCmd()`, add the flag after the existing flags:

```go
cmd.Flags().BoolVarP(&upgradable, "upgradable", "u", false, "show only upgradable skills (requires --installed)")
```

Add `var upgradable bool` alongside the existing flag variables
in `newListCmd()`.

Update the closure to pass `upgradable`:

```go
RunE: func(cmd *cobra.Command, args []string) error {
    if upgradable && !showInstalled {
        return fmt.Errorf("--upgradable requires --installed")
    }
    if showInstalled {
        return runListInstalled(cmd, target, outputDir, upgradable)
    }
    return runList(cmd)
},
```

Update `runListInstalled` signature and add the upgradable
branch. Replace the entire `runListInstalled` function:

```go
func runListInstalled(cmd *cobra.Command, target, outputDir string, upgradable bool) error {
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

    if !upgradable {
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

    candidates, err := installed.CheckUpgrades(cmd.Context(), skills,
        installed.CheckOptions{
            TagLister: oci.ListTagsForRepo,
        })
    if err != nil {
        return fmt.Errorf("checking upgrades: %w", err)
    }

    if len(candidates) == 0 {
        fmt.Fprintln(cmd.OutOrStdout(), "All installed skills are up to date.")
        return nil
    }

    w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
    fmt.Fprintln(w, "NAME\tVERSION\tLATEST\tSOURCE\tTARGET")
    for _, c := range candidates {
        fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
            c.Installed.Name, c.Installed.Version, c.LatestVersion,
            c.LatestRef, c.Installed.Target)
    }
    return w.Flush()
}
```

Add `"github.com/redhat-et/skillimage/pkg/oci"` to the imports.

- [ ] **Step 2: Add ListTagsForRepo to pkg/oci**

Create a thin wrapper in `pkg/oci/catalog.go` that matches the
`TagLister` signature — it takes a full repo ref (no tag) and
splits it into registry+repo for `ListRemoteTags`:

Add this function at the end of `pkg/oci/catalog.go`:

```go
// ListTagsForRepo lists all tags for a repository given a full
// repository reference without a tag (e.g., "quay.io/acme/skill").
// This is a convenience wrapper around ListRemoteTags.
func ListTagsForRepo(ctx context.Context, repo string, skipTLSVerify bool) ([]string, error) {
    idx := strings.Index(repo, "/")
    if idx < 0 {
        return nil, fmt.Errorf("invalid repository reference: %s (no registry host)", repo)
    }
    registry := repo[:idx]
    repoName := repo[idx+1:]
    return ListRemoteTags(ctx, registry, repoName, skipTLSVerify)
}
```

- [ ] **Step 3: Verify build**

Run: `go build ./cmd/skillctl/`

Expected: clean build.

- [ ] **Step 4: Run linter**

Run: `make lint`

Expected: no new warnings.

- [ ] **Step 5: Run full test suite**

Run: `go test ./...`

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/cli/list.go pkg/oci/catalog.go
git commit -s -m "feat: add list --installed --upgradable flag"
```

---

## Task 4: upgrade command

**Files:**
- Create: `internal/cli/upgrade.go`
- Modify: `internal/cli/root.go` (add `cmd.AddCommand(newUpgradeCmd())`)

- [ ] **Step 1: Create the upgrade command**

```go
package cli

import (
    "context"
    "fmt"
    "os"
    "path/filepath"
    "strings"

    "github.com/spf13/cobra"

    "github.com/redhat-et/skillimage/pkg/installed"
    "github.com/redhat-et/skillimage/pkg/oci"
)

func newUpgradeCmd() *cobra.Command {
    var target string
    var outputDir string
    var all bool
    var tlsVerify bool

    cmd := &cobra.Command{
        Use:   "upgrade [skill-name]",
        Short: "Upgrade installed skills to latest published version",
        Long: `Upgrade one or all installed skills to their latest published
version from the source registry.

Requires --target or -o to locate installed skills.

Examples:
  skillctl upgrade red-hat-quick-deck --target opencode
  skillctl upgrade --all --target claude
  skillctl upgrade my-skill -o ~/custom/skills/`,
        Args: cobra.MaximumNArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            skillName := ""
            if len(args) == 1 {
                skillName = args[0]
            }
            return runUpgrade(cmd, skillName, target, outputDir, all, !tlsVerify)
        },
    }

    cmd.Flags().StringVarP(&target, "target", "t", "", "agent name (claude, cursor, windsurf, opencode, openclaw)")
    cmd.Flags().StringVarP(&outputDir, "output", "o", "", "custom skill directory")
    cmd.Flags().BoolVarP(&all, "all", "a", false, "upgrade all installed skills")
    cmd.Flags().BoolVar(&tlsVerify, "tls-verify", true, "require HTTPS and verify certificates")

    return cmd
}

func runUpgrade(cmd *cobra.Command, skillName, target, outputDir string, all, skipTLSVerify bool) error {
    if skillName != "" && all {
        return fmt.Errorf("cannot specify skill name with --all")
    }
    if skillName == "" && !all {
        return fmt.Errorf("specify a skill name or use --all")
    }
    if target == "" && outputDir == "" {
        return fmt.Errorf("specify --target <agent> or -o <directory>")
    }
    if target != "" && outputDir != "" {
        return fmt.Errorf("use --target or -o, not both")
    }

    targets, err := resolveUpgradeTarget(target, outputDir)
    if err != nil {
        return err
    }

    skills, err := installed.Scan(targets)
    if err != nil {
        return fmt.Errorf("scanning installed skills: %w", err)
    }

    if skillName != "" {
        found := false
        for i, s := range skills {
            if s.Name == skillName {
                skills = skills[i : i+1]
                found = true
                break
            }
        }
        if !found {
            targetLabel := target
            if targetLabel == "" {
                targetLabel = outputDir
            }
            return fmt.Errorf("skill not found: %s in target %s", skillName, targetLabel)
        }

        if skills[0].Source == "" {
            return fmt.Errorf("no source registry for %s (installed locally)", skillName)
        }
    }

    candidates, err := installed.CheckUpgrades(cmd.Context(), skills,
        installed.CheckOptions{
            SkipTLSVerify: skipTLSVerify,
            TagLister:     oci.ListTagsForRepo,
        })
    if err != nil {
        return fmt.Errorf("checking upgrades: %w", err)
    }

    if len(candidates) == 0 {
        if all {
            fmt.Fprintln(cmd.OutOrStdout(), "All skills are up to date.")
        } else {
            fmt.Fprintf(cmd.OutOrStdout(), "%s is already at the latest version.\n", skillName)
        }
        return nil
    }

    client, err := defaultClient()
    if err != nil {
        return err
    }

    ctx := cmd.Context()
    var upgraded int
    for _, c := range candidates {
        if err := upgradeSkill(ctx, client, c, skipTLSVerify); err != nil {
            fmt.Fprintf(cmd.ErrOrStderr(), "Error upgrading %s: %v\n", c.Installed.Name, err)
            continue
        }
        fmt.Fprintf(cmd.OutOrStdout(), "Upgraded %s %s → %s (%s)\n",
            c.Installed.Name, c.Installed.Version, c.LatestVersion, c.Installed.Target)
        upgraded++
    }

    if all && upgraded > 0 {
        fmt.Fprintf(cmd.OutOrStdout(), "\nUpgraded %d skill(s).\n", upgraded)
    }

    return nil
}

func upgradeSkill(ctx context.Context, client *oci.Client, c installed.UpgradeCandidate, skipTLSVerify bool) error {
    _, err := client.Pull(ctx, c.LatestRef, oci.PullOptions{
        SkipTLSVerify: skipTLSVerify,
    })
    if err != nil {
        return fmt.Errorf("pulling %s: %w", c.LatestRef, err)
    }

    parentDir := filepath.Dir(c.Installed.Path)
    if err := client.Unpack(ctx, c.LatestRef, parentDir); err != nil {
        return fmt.Errorf("unpacking %s: %w", c.LatestRef, err)
    }

    if err := writeProvenance(ctx, client, c.LatestRef, c.Installed.Path); err != nil {
        return fmt.Errorf("writing provenance: %w", err)
    }

    return nil
}

func resolveUpgradeTarget(target, outputDir string) (map[string]string, error) {
    if outputDir != "" {
        if strings.HasPrefix(outputDir, "~/") || outputDir == "~" {
            h, err := os.UserHomeDir()
            if err != nil {
                return nil, fmt.Errorf("finding home directory: %w", err)
            }
            outputDir = filepath.Join(h, outputDir[1:])
        }
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
```

- [ ] **Step 2: Register the command in root.go**

Add this line after `cmd.AddCommand(newRmCmd())`:

```go
cmd.AddCommand(newUpgradeCmd())
```

- [ ] **Step 3: Verify build**

Run: `go build ./cmd/skillctl/`

Expected: clean build.

- [ ] **Step 4: Run linter**

Run: `make lint`

Expected: no new warnings.

- [ ] **Step 5: Run full test suite**

Run: `go test ./...`

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/cli/upgrade.go internal/cli/root.go
git commit -s -m "feat: add skillctl upgrade command

Part of #27"
```
