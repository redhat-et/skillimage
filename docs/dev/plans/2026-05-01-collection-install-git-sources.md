# Collection install with Git source support

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task.
> Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `skillctl collection install` to install skills
from a collection YAML (local file or Git URL) into an agent's
skill directory, with support for `source:` entries pointing to
Git repos and SHA/digest-based skip logic.

**Architecture:** Extend `SkillRef` in `pkg/collection/` with a
`Source` field. Add `LsRemote()` to `pkg/source/` for lightweight
SHA checking. Add `collection install` subcommand in
`internal/cli/collection.go` that orchestrates the flow: parse
collection, check provenance, build/pull, unpack, write provenance.

**Tech Stack:** Go, Cobra/Viper, oras-go, git CLI

---

### Task 1: Extend `SkillRef` struct and update validation

**Files:**

- Modify: `pkg/collection/collection.go`
- Modify: `pkg/collection/collection_test.go`

- [ ] **Step 1: Write failing tests for new validation rules**

Add to `pkg/collection/collection_test.go`:

```go
func TestParseSourceField(t *testing.T) {
	input := `apiVersion: skillimage.io/v1alpha1
kind: SkillCollection
metadata:
  name: dev-skills
  version: 0.1.0
skills:
  - source: https://github.com/myorg/skills/tree/main/code-reviewer
  - name: stable-tool
    image: quay.io/myorg/stable-tool:1.0.0
`
	col, err := collection.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if col.Skills[0].Source != "https://github.com/myorg/skills/tree/main/code-reviewer" {
		t.Errorf("skill[0].source = %q", col.Skills[0].Source)
	}
	if col.Skills[1].Image != "quay.io/myorg/stable-tool:1.0.0" {
		t.Errorf("skill[1].image = %q", col.Skills[1].Image)
	}
}

func TestValidateSourceAndImageExclusive(t *testing.T) {
	col := &collection.SkillCollection{
		APIVersion: "skillimage.io/v1alpha1",
		Kind:       "SkillCollection",
		Metadata:   collection.Metadata{Name: "test", Version: "1.0.0"},
		Skills: []collection.SkillRef{
			{Name: "both", Image: "quay.io/org/s:1.0.0", Source: "https://github.com/org/repo/tree/main/s"},
		},
	}
	errs := collection.Validate(col)
	found := false
	for _, e := range errs {
		if strings.Contains(e, "mutually exclusive") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected mutually exclusive error, got: %v", errs)
	}
}

func TestValidateNeitherImageNorSource(t *testing.T) {
	col := &collection.SkillCollection{
		APIVersion: "skillimage.io/v1alpha1",
		Kind:       "SkillCollection",
		Metadata:   collection.Metadata{Name: "test", Version: "1.0.0"},
		Skills: []collection.SkillRef{
			{Name: "empty"},
		},
	}
	errs := collection.Validate(col)
	found := false
	for _, e := range errs {
		if strings.Contains(e, "image or source is required") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'image or source is required' error, got: %v", errs)
	}
}

func TestValidateSourceWithoutName(t *testing.T) {
	col := &collection.SkillCollection{
		APIVersion: "skillimage.io/v1alpha1",
		Kind:       "SkillCollection",
		Metadata:   collection.Metadata{Name: "test", Version: "1.0.0"},
		Skills: []collection.SkillRef{
			{Source: "https://github.com/org/repo/tree/main/skill"},
		},
	}
	errs := collection.Validate(col)
	if len(errs) != 0 {
		t.Errorf("source without name should be valid, got errors: %v", errs)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/panni/work/skillimage && go test ./pkg/collection/ -run 'TestParseSourceField|TestValidateSource|TestValidateNeither' -v`

Expected: `TestParseSourceField` fails because `source` is an
unknown field (strict parsing). The validation tests fail
because the struct doesn't have a `Source` field.

- [ ] **Step 3: Update `SkillRef` struct**

In `pkg/collection/collection.go`, replace the `SkillRef`
struct:

```go
type SkillRef struct {
	Name   string `yaml:"name,omitempty"`
	Image  string `yaml:"image,omitempty"`
	Source string `yaml:"source,omitempty"`
}
```

- [ ] **Step 4: Update `Validate()` function**

In `pkg/collection/collection.go`, replace the validation loop
inside `Validate()`:

```go
seen := make(map[string]bool)
for i, s := range col.Skills {
	switch {
	case s.Image != "" && s.Source != "":
		errs = append(errs, fmt.Sprintf("skills[%d]: image and source are mutually exclusive", i))
	case s.Image == "" && s.Source == "":
		errs = append(errs, fmt.Sprintf("skills[%d]: image or source is required", i))
	case s.Image != "" && s.Name == "":
		errs = append(errs, fmt.Sprintf("skills[%d].name is required", i))
	}
	if s.Name != "" && seen[s.Name] {
		errs = append(errs, fmt.Sprintf("duplicate skill name %q", s.Name))
	}
	if s.Name != "" {
		seen[s.Name] = true
	}
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /Users/panni/work/skillimage && go test ./pkg/collection/ -v`

Expected: All tests pass, including existing tests. The
existing `TestValidateMissingFields` test validated that
`name == ""` and `image == ""` each produce errors. With the
new rules, `name == ""` with `image != ""` still produces the
name-required error. But `image == ""` with `name != ""` now
triggers the "image or source is required" error instead of the
old `skills[N].image is required`. We need to update that test.

- [ ] **Step 6: Fix `TestValidateMissingFields` if needed**

The existing test expects at least 2 errors for entries
`{Name: "", Image: "quay.io/org/s:1.0.0"}` and
`{Name: "s2", Image: ""}`. With the new logic:

- Entry 0: `name == ""`, `image != ""`, `source == ""` →
  triggers `skills[0].name is required` (correct)
- Entry 1: `name == "s2"`, `image == ""`, `source == ""` →
  triggers `skills[1]: image or source is required` (new msg)

The test checks `len(errs) < 2`, so it should still pass.
Verify by running all tests.

- [ ] **Step 7: Commit**

```bash
git add pkg/collection/collection.go pkg/collection/collection_test.go
git commit -s -m "feat(collection): add source field to SkillRef with validation

Add Source field to SkillRef struct for Git URL references.
Update Validate() to enforce mutual exclusivity between image
and source fields. Name is optional for source entries (derived
at install time from SKILL.md frontmatter).

Refs: #35

Assisted-By: Claude (Anthropic AI) <noreply@anthropic.com>"
```

---

### Task 2: Add `LsRemote()` to `pkg/source/`

**Files:**

- Create: `pkg/source/lsremote.go`
- Create: `pkg/source/lsremote_test.go`

- [ ] **Step 1: Write failing test for `LsRemote`**

Create `pkg/source/lsremote_test.go`:

```go
package source

import (
	"context"
	"testing"
)

func TestLsRemoteReturnsCommitSHA(t *testing.T) {
	if err := CheckGit(); err != nil {
		t.Skip("git not available")
	}

	// Use a well-known public repo with a known branch.
	sha, err := LsRemote(context.Background(), "https://github.com/octocat/Hello-World.git", "master")
	if err != nil {
		t.Fatalf("LsRemote: %v", err)
	}
	if len(sha) < 7 {
		t.Errorf("expected commit SHA, got %q", sha)
	}
	if !commitSHAPattern.MatchString(sha) {
		t.Errorf("SHA %q does not match commit pattern", sha)
	}
}

func TestLsRemoteBadRef(t *testing.T) {
	if err := CheckGit(); err != nil {
		t.Skip("git not available")
	}

	_, err := LsRemote(context.Background(), "https://github.com/octocat/Hello-World.git", "nonexistent-branch-xyz")
	if err == nil {
		t.Fatal("expected error for nonexistent ref")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/panni/work/skillimage && go test ./pkg/source/ -run TestLsRemote -v`

Expected: Compilation error — `LsRemote` not defined.

- [ ] **Step 3: Implement `LsRemote`**

Create `pkg/source/lsremote.go`:

```go
package source

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// LsRemote queries a remote Git repository for the commit SHA
// of the given ref without cloning. Returns the full commit SHA.
func LsRemote(ctx context.Context, cloneURL, ref string) (string, error) {
	if err := CheckGit(); err != nil {
		return "", err
	}

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "git", "ls-remote", cloneURL, ref)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git ls-remote %s %s: %s", cloneURL, ref, sanitizeGitOutput(stderr.String()))
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return "", fmt.Errorf("ref %q not found in %s", ref, cloneURL)
	}

	// Output format: "<sha>\t<refname>\n" — may have multiple lines.
	// Take the first line's SHA.
	firstLine := strings.SplitN(output, "\n", 2)[0]
	fields := strings.Fields(firstLine)
	if len(fields) < 1 {
		return "", fmt.Errorf("unexpected ls-remote output for %s %s", cloneURL, ref)
	}

	return fields[0], nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/panni/work/skillimage && go test ./pkg/source/ -run TestLsRemote -v`

Expected: Both tests pass.

- [ ] **Step 5: Commit**

```bash
git add pkg/source/lsremote.go pkg/source/lsremote_test.go
git commit -s -m "feat(source): add LsRemote for lightweight ref checking

LsRemote queries a remote Git repo for the commit SHA of a ref
without cloning. Used by collection install to skip unchanged
source entries.

Refs: #35

Assisted-By: Claude (Anthropic AI) <noreply@anthropic.com>"
```

---

### Task 3: Extract shared helpers from `install.go`

The `collection install` command needs the same output directory
resolution and provenance-writing logic that the individual
`install` command uses. Extract the reusable pieces.

**Files:**

- Modify: `internal/cli/install.go`

- [ ] **Step 1: Run existing install tests to establish baseline**

Run: `cd /Users/panni/work/skillimage && go test ./internal/cli/ -v`

Expected: All existing tests pass.

- [ ] **Step 2: Export `writeProvenance` and helpers**

In `internal/cli/install.go`, rename `writeProvenance` to
`WriteProvenance` (exported), and `newSkillCardFromRef` to
`NewSkillCardFromRef`:

```go
func WriteProvenance(ctx context.Context, client *oci.Client, ref, skillDir string) error {
```

```go
func NewSkillCardFromRef(ref string) *skillcard.SkillCard {
```

Update the call site in `runInstall` from `writeProvenance` to
`WriteProvenance` and `newSkillCardFromRef` to
`NewSkillCardFromRef`.

- [ ] **Step 3: Run tests to verify nothing broke**

Run: `cd /Users/panni/work/skillimage && go test ./internal/cli/ -v`

Expected: All tests pass.

- [ ] **Step 4: Commit**

```bash
git add internal/cli/install.go
git commit -s -m "refactor(cli): export WriteProvenance for reuse

Export WriteProvenance and NewSkillCardFromRef so the collection
install command can reuse them.

Refs: #35

Assisted-By: Claude (Anthropic AI) <noreply@anthropic.com>"
```

---

### Task 4: Implement `collection install` command

**Files:**

- Modify: `internal/cli/collection.go`

- [ ] **Step 1: Add `newCollectionInstallCmd` to the collection command**

In `internal/cli/collection.go`, add the install subcommand
registration inside `newCollectionCmd()`:

```go
func newCollectionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "collection",
		Short: "Manage skill collections",
	}
	cmd.AddCommand(newCollectionPushCmd())
	cmd.AddCommand(newCollectionPullCmd())
	cmd.AddCommand(newCollectionInstallCmd())
	cmd.AddCommand(newCollectionVolumeCmd())
	cmd.AddCommand(newCollectionGenerateCmd())
	return cmd
}
```

- [ ] **Step 2: Implement the command and install flow**

Add to the bottom of `internal/cli/collection.go`:

```go
func newCollectionInstallCmd() *cobra.Command {
	var file string
	var target string
	var outputDir string
	var force bool
	var ref string
	cmd := &cobra.Command{
		Use:   "install [-f <file> | <git-url>]",
		Short: "Install skills from a collection into an agent's skill directory",
		Long: `Install skills defined in a collection YAML into a directory
where an agent can find them. Skills can be referenced by OCI
image (image:) or Git source URL (source:).

Source entries clone the repo, build locally, and install.
Image entries pull from the registry and install.

Skills that haven't changed since last install are skipped
unless --force is set.

Examples:
  skillctl collection install -f ./collection.yaml --target claude
  skillctl collection install https://github.com/myorg/skills/tree/main/collection.yaml -t claude
  skillctl collection install -f ./collection.yaml -o ~/my-skills --force`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCollectionInstall(cmd, file, args, target, outputDir, force, ref)
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "path to local collection YAML file")
	cmd.Flags().StringVarP(&target, "target", "t", "", "agent name (claude, cursor, windsurf, opencode, openclaw)")
	cmd.Flags().StringVarP(&outputDir, "output", "o", "", "custom output directory")
	cmd.Flags().BoolVar(&force, "force", false, "reinstall even if skills are up to date")
	cmd.Flags().StringVar(&ref, "ref", "", "Git ref override (for collection YAML URL)")
	return cmd
}

func runCollectionInstall(cmd *cobra.Command, file string, args []string, target, outputDir string, force bool, ref string) error {
	col, cleanup, err := resolveCollectionInput(cmd.Context(), file, args, ref)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return err
	}

	if errs := collection.Validate(col); len(errs) > 0 {
		return fmt.Errorf("invalid collection:\n  %s", strings.Join(errs, "\n  "))
	}

	dirs, err := resolveTargetDirs(target, outputDir, false)
	if err != nil {
		return err
	}
	var destDir string
	for _, d := range dirs {
		destDir = d
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("creating directory %s: %w", destDir, err)
	}

	client, err := defaultClient()
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	w := cmd.OutOrStdout()
	errW := cmd.ErrOrStderr()

	fmt.Fprintf(w, "Installing collection %q (%d skills)\n", col.Metadata.Name, len(col.Skills))

	var installed, skipped, failed int
	for _, s := range col.Skills {
		switch {
		case s.Source != "":
			result, err := installFromSource(ctx, client, s, destDir, force, w, errW)
			switch {
			case err != nil:
				fmt.Fprintf(errW, "  %s (source)  error: %v\n", skillLabel(s), err)
				failed++
			case result == "skipped":
				skipped++
			default:
				installed++
			}
		case s.Image != "":
			result, err := installFromImage(ctx, client, s, destDir, force, w)
			switch {
			case err != nil:
				fmt.Fprintf(errW, "  %s (image)  error: %v\n", s.Name, err)
				failed++
			case result == "skipped":
				skipped++
			default:
				installed++
			}
		}
	}

	fmt.Fprintf(w, "Installed %d skills", installed)
	if skipped > 0 {
		fmt.Fprintf(w, ", %d up to date", skipped)
	}
	fmt.Fprintln(w)

	if failed > 0 {
		return fmt.Errorf("%d skill(s) failed to install", failed)
	}
	return nil
}

func resolveCollectionInput(ctx context.Context, file string, args []string, ref string) (*collection.SkillCollection, func(), error) {
	if file != "" && len(args) > 0 {
		return nil, nil, fmt.Errorf("specify -f <file> or a Git URL, not both")
	}
	if file != "" {
		col, err := collection.ParseFile(file)
		return col, nil, err
	}
	if len(args) == 0 {
		return nil, nil, fmt.Errorf("specify -f <file> or a Git URL")
	}

	rawURL := args[0]
	if !source.IsRemote(rawURL) {
		return nil, nil, fmt.Errorf("not a valid URL: %s\n\nUse -f for local files", rawURL)
	}

	src, err := source.ParseGitURL(rawURL)
	if err != nil {
		return nil, nil, err
	}

	cloneResult, err := source.Clone(ctx, src, source.CloneOptions{RefOverride: ref})
	if err != nil {
		return nil, nil, fmt.Errorf("cloning collection: %w", err)
	}

	yamlPath := cloneResult.Dir
	info, statErr := os.Stat(yamlPath)
	if statErr != nil {
		cloneResult.Cleanup()
		return nil, nil, fmt.Errorf("collection file not found at %s in repository", src.SubPath)
	}
	if info.IsDir() {
		cloneResult.Cleanup()
		return nil, nil, fmt.Errorf("URL must point to a collection YAML file, not a directory: %s", src.SubPath)
	}

	col, err := collection.ParseFile(yamlPath)
	if err != nil {
		cloneResult.Cleanup()
		return nil, nil, err
	}

	return col, cloneResult.Cleanup, nil
}

func installFromSource(ctx context.Context, client *oci.Client, s collection.SkillRef, destDir string, force bool, w io.Writer, errW io.Writer) (string, error) {
	label := skillLabel(s)

	src, err := source.ParseGitURL(s.Source)
	if err != nil {
		return "", err
	}

	// Skip check: compare stored provenance commit against remote HEAD.
	if !force && label != "" {
		installedSHA := readInstalledCommit(destDir, label)
		if installedSHA != "" {
			refToCheck := src.Ref
			if refToCheck == "" {
				refToCheck = "HEAD"
			}
			remoteSHA, lsErr := source.LsRemote(ctx, src.CloneURL, refToCheck)
			if lsErr == nil && remoteSHA == installedSHA {
				fmt.Fprintf(w, "  %s (source)  up to date\n", label)
				return "skipped", nil
			}
		}
	}

	fmt.Fprintf(w, "  %s (source)  cloning...", label)

	result, err := source.Resolve(ctx, s.Source, "", "")
	if err != nil {
		fmt.Fprintln(w)
		return "", err
	}
	defer result.Cleanup()

	if len(result.Skills) == 0 {
		fmt.Fprintln(w)
		return "", fmt.Errorf("no skills found at %s", s.Source)
	}

	skill := result.Skills[0]
	if label == "" {
		label = skill.Name
	}

	fmt.Fprintf(w, "  building...")

	desc, err := client.Build(ctx, skill.Dir, oci.BuildOptions{SkillCard: skill.SkillCard})
	if err != nil {
		fmt.Fprintln(w)
		return "", fmt.Errorf("building: %w", err)
	}

	ref := fmt.Sprintf("%s/%s:%s", skill.SkillCard.Metadata.Namespace, skill.SkillCard.Metadata.Name, skill.SkillCard.Metadata.Version)
	if err := client.Unpack(ctx, ref, destDir); err != nil {
		fmt.Fprintln(w)
		return "", fmt.Errorf("unpacking: %w", err)
	}

	skillDir := filepath.Join(destDir, skill.Name)
	writeSourceProvenance(skillDir, skill.SkillCard, desc.Digest.String())

	fmt.Fprintf(w, "  installed\n")
	return "installed", nil
}

func installFromImage(ctx context.Context, client *oci.Client, s collection.SkillRef, destDir string, force bool, w io.Writer) (string, error) {
	// Skip check: compare stored provenance digest against local store.
	if !force {
		installedDigest := readInstalledCommit(destDir, s.Name)
		if installedDigest != "" {
			localDigest, err := client.ResolveDigest(ctx, s.Image)
			if err == nil && localDigest == installedDigest {
				fmt.Fprintf(w, "  %s (image)  up to date\n", s.Name)
				return "skipped", nil
			}
		}
	}

	fmt.Fprintf(w, "  %s (image)  pulling...", s.Name)

	if !looksLocal(s.Image) {
		if _, err := client.ResolveDigest(ctx, s.Image); err != nil {
			if _, pullErr := client.Pull(ctx, s.Image, oci.PullOptions{}); pullErr != nil {
				fmt.Fprintln(w)
				return "", fmt.Errorf("pulling %s: %w", s.Image, pullErr)
			}
		}
	}

	if err := client.Unpack(ctx, s.Image, destDir); err != nil {
		fmt.Fprintln(w)
		return "", fmt.Errorf("unpacking %s: %w", s.Image, err)
	}

	skillDir := filepath.Join(destDir, oci.SkillNameFromRef(s.Image))
	if err := WriteProvenance(ctx, client, s.Image, skillDir); err != nil {
		fmt.Fprintf(os.Stderr, "  warning: provenance write failed: %v\n", err)
	}

	fmt.Fprintf(w, "  installed\n")
	return "installed", nil
}

func skillLabel(s collection.SkillRef) string {
	if s.Name != "" {
		return s.Name
	}
	return ""
}

// readInstalledCommit reads provenance.commit from skill.yaml
// in destDir/skillName, returning empty string if not found.
func readInstalledCommit(destDir, skillName string) string {
	skillPath := filepath.Join(destDir, skillName, "skill.yaml")
	f, err := os.Open(skillPath)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()

	sc, err := skillcard.Parse(f)
	if err != nil {
		return ""
	}
	if sc.Provenance == nil {
		return ""
	}
	return sc.Provenance.Commit
}

// writeSourceProvenance writes provenance data for a source-built
// skill into its skill.yaml.
func writeSourceProvenance(skillDir string, sc *skillcard.SkillCard, digest string) {
	if sc.Provenance == nil {
		sc.Provenance = &skillcard.Provenance{}
	}

	skillPath := filepath.Join(skillDir, "skill.yaml")
	wf, err := os.Create(skillPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  warning: could not write provenance: %v\n", err)
		return
	}
	defer func() { _ = wf.Close() }()
	if err := skillcard.Serialize(sc, wf); err != nil {
		fmt.Fprintf(os.Stderr, "  warning: could not serialize provenance: %v\n", err)
	}
}
```

- [ ] **Step 3: Add required imports**

Update the imports at the top of `internal/cli/collection.go`:

```go
import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/redhat-et/skillimage/pkg/collection"
	"github.com/redhat-et/skillimage/pkg/oci"
	"github.com/redhat-et/skillimage/pkg/skillcard"
	"github.com/redhat-et/skillimage/pkg/source"
	"github.com/spf13/cobra"
)
```

- [ ] **Step 4: Verify it compiles**

Run: `cd /Users/panni/work/skillimage && go build ./...`

Expected: Clean compilation.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/collection.go
git commit -s -m "feat(cli): add collection install subcommand

Implements skillctl collection install with support for both
image: (OCI registry) and source: (Git URL) entries. Includes
SHA/digest-based skip logic and --force flag to override.

Refs: #35

Assisted-By: Claude (Anthropic AI) <noreply@anthropic.com>"
```

---

### Task 5: Add CLI tests for `collection install`

**Files:**

- Create: `internal/cli/collection_install_test.go`

- [ ] **Step 1: Write tests for input validation and image-based install**

Create `internal/cli/collection_install_test.go`:

```go
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
```

- [ ] **Step 2: Run tests**

Run: `cd /Users/panni/work/skillimage && go test ./internal/cli/ -run TestCollectionInstall -v`

Expected: All tests pass.

- [ ] **Step 3: Commit**

```bash
git add internal/cli/collection_install_test.go
git commit -s -m "test(cli): add collection install validation tests

Tests input validation, target resolution, invalid YAML handling,
and mutual exclusivity validation for the collection install
command.

Refs: #35

Assisted-By: Claude (Anthropic AI) <noreply@anthropic.com>"
```

---

### Task 6: Add example collection YAML with mixed entries

**Files:**

- Create: `examples/dev-collection.yaml`

- [ ] **Step 1: Create the example file**

Create `examples/dev-collection.yaml`:

```yaml
apiVersion: skillimage.io/v1alpha1
kind: SkillCollection
metadata:
  name: dev-skills
  version: 0.1.0
  description: Development collection with Git source and OCI image entries
skills:
  - source: https://github.com/anthropics/courses/tree/master/prompt_engineering_interactive_tutorial/skills/code-review
  - name: document-summarizer
    image: quay.io/skillimage/business/document-summarizer:1.0.0-testing
```

- [ ] **Step 2: Verify the example parses**

Run: `cd /Users/panni/work/skillimage && go run ./cmd/skillctl/ validate examples/dev-collection.yaml 2>&1 || echo "(validate may not support collections — that's fine, parse test below is sufficient)"`

If `validate` doesn't support collections, add a quick parse
test instead:

```bash
cd /Users/panni/work/skillimage && go test ./pkg/collection/ -run TestParseFile -v
```

- [ ] **Step 3: Commit**

```bash
git add examples/dev-collection.yaml
git commit -s -m "docs: add example dev collection with source and image entries

Shows mixed source: (Git URL) and image: (OCI ref) entries in
a collection YAML for the development workflow.

Refs: #35

Assisted-By: Claude (Anthropic AI) <noreply@anthropic.com>"
```

---

### Task 7: Run full test suite and lint

**Files:** None (verification only)

- [ ] **Step 1: Run all tests**

Run: `cd /Users/panni/work/skillimage && make test`

Expected: All tests pass.

- [ ] **Step 2: Run linter**

Run: `cd /Users/panni/work/skillimage && make lint`

Expected: No lint errors. Fix any that appear.

- [ ] **Step 3: Run build**

Run: `cd /Users/panni/work/skillimage && make build`

Expected: Clean build.

- [ ] **Step 4: Smoke test the command**

Run: `cd /Users/panni/work/skillimage && ./bin/skillctl collection install --help`

Expected: Help text shows usage with `-f`, `--target`, `-o`,
`--force`, and `--ref` flags.

- [ ] **Step 5: Fix any issues found, commit if needed**
