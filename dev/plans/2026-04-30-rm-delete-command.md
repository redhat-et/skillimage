# rm/delete command implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task.
> Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `skillctl rm` (alias `delete`) to remove skill images
from the local OCI store by name:tag reference.

**Architecture:** A `Remove()` method on `pkg/oci.Client` calls
`store.Resolve()` then `store.Untag()`. A thin CLI command in
`internal/cli/rm.go` handles argument parsing, confirmation
prompts, and output formatting. Follows the same pattern as
the existing `prune` command.

**Tech Stack:** Go, cobra, oras-go (`content/oci.Store`)

---

### Task 1: Client.Remove method — test

**Files:**
- Create: `pkg/oci/remove_test.go`

- [ ] **Step 1: Write TestRemove — remove an existing image**

```go
package oci_test

import (
	"context"
	"testing"

	"github.com/redhat-et/skillimage/pkg/oci"
)

func TestRemove(t *testing.T) {
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

	err = client.Remove(ctx, "test/test-skill:1.0.0-draft")
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}

	images, err := client.ListLocal()
	if err != nil {
		t.Fatalf("ListLocal: %v", err)
	}
	if len(images) != 0 {
		t.Errorf("expected 0 images after remove, got %d", len(images))
	}
}
```

- [ ] **Step 2: Write TestRemoveNotFound — error on missing ref**

```go
func TestRemoveNotFound(t *testing.T) {
	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	err = client.Remove(context.Background(), "no/such-image:1.0.0")
	if err == nil {
		t.Fatal("expected error for non-existent image")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "not found")
	}
}
```

Add `"strings"` to the import block.

- [ ] **Step 3: Write TestRemoveMultipleImages — remove one, keep another**

```go
func TestRemoveMultipleImages(t *testing.T) {
	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()

	// Build first image
	skillDir1 := t.TempDir()
	writeTestSkill(t, skillDir1)
	_, err = client.Build(ctx, skillDir1, oci.BuildOptions{})
	if err != nil {
		t.Fatalf("Build first: %v", err)
	}

	// Build second image with different name
	skillDir2 := t.TempDir()
	writeTestSkillNamed(t, skillDir2, "other-skill")
	_, err = client.Build(ctx, skillDir2, oci.BuildOptions{})
	if err != nil {
		t.Fatalf("Build second: %v", err)
	}

	// Remove only the first
	err = client.Remove(ctx, "test/test-skill:1.0.0-draft")
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}

	images, err := client.ListLocal()
	if err != nil {
		t.Fatalf("ListLocal: %v", err)
	}
	if len(images) != 1 {
		t.Fatalf("expected 1 image after remove, got %d", len(images))
	}
	if images[0].Name != "test/other-skill" {
		t.Errorf("remaining image = %q, want %q", images[0].Name, "test/other-skill")
	}
}
```

This test uses a helper `writeTestSkillNamed` — add it to the
test file:

```go
func writeTestSkillNamed(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillYAML := []byte(fmt.Sprintf(`apiVersion: skillimage.io/v1alpha1
kind: SkillCard
metadata:
  name: %s
  namespace: test
  version: 1.0.0
  description: A test skill.
spec:
  prompt: SKILL.md
`, name))
	if err := os.WriteFile(filepath.Join(dir, "skill.yaml"), skillYAML, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("Test prompt."), 0o644); err != nil {
		t.Fatal(err)
	}
}
```

Add `"fmt"`, `"os"`, and `"path/filepath"` to the import block.

- [ ] **Step 4: Run tests to verify they fail**

Run: `go test ./pkg/oci/ -run "TestRemove" -v`

Expected: compilation failure — `client.Remove` undefined.

- [ ] **Step 5: Commit**

```bash
git add pkg/oci/remove_test.go
git commit -s -m "test: add failing tests for Client.Remove"
```

---

### Task 2: Client.Remove method — implementation

**Files:**
- Create: `pkg/oci/remove.go`

- [ ] **Step 1: Implement Remove**

```go
package oci

import (
	"context"
	"fmt"

	"oras.land/oras-go/v2/errdef"
)

// Remove removes a skill image from the local store by its tag
// reference (e.g., "test/test-skill:1.0.0-draft"). It does not
// clean up unreferenced blobs — use Prune for that.
func (c *Client) Remove(ctx context.Context, ref string) error {
	if _, err := c.store.Resolve(ctx, ref); err != nil {
		if errdef.IsNotFound(err) {
			return fmt.Errorf("image not found: %s", ref)
		}
		return fmt.Errorf("resolving %s: %w", ref, err)
	}

	if err := c.store.Untag(ctx, ref); err != nil {
		return fmt.Errorf("removing %s: %w", ref, err)
	}

	return nil
}
```

Note: `errdef.IsNotFound` is from `oras.land/oras-go/v2/errdef`,
already imported elsewhere in the package (see `build.go` line 19).

- [ ] **Step 2: Run tests to verify they pass**

Run: `go test ./pkg/oci/ -run "TestRemove" -v`

Expected: all three tests pass.

- [ ] **Step 3: Run full test suite**

Run: `go test ./...`

Expected: all tests pass (no regressions).

- [ ] **Step 4: Commit**

```bash
git add pkg/oci/remove.go
git commit -s -m "feat: add Client.Remove to untag local images"
```

---

### Task 3: CLI rm command

**Files:**
- Create: `internal/cli/rm.go`
- Modify: `internal/cli/root.go:29` (add `newRmCmd()`)

- [ ] **Step 1: Create the rm command**

```go
package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func newRmCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:     "rm <ref> [<ref>...]",
		Aliases: []string{"delete"},
		Short:   "Remove skill images from local store",
		Long: `Remove one or more skill images from the local store by
tag reference (e.g., test/hello-world:1.0.0-draft).

Does not clean up unreferenced blobs — run 'skillctl prune'
after removal to reclaim disk space.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRm(cmd, args, force)
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "skip confirmation prompt")
	return cmd
}

func runRm(cmd *cobra.Command, refs []string, force bool) error {
	client, err := defaultClient()
	if err != nil {
		return err
	}

	ctx := context.Background()

	// Resolve all refs first to report errors before confirming.
	images, err := client.ListLocal()
	if err != nil {
		return fmt.Errorf("listing images: %w", err)
	}
	knownRefs := make(map[string]bool, len(images))
	for _, img := range images {
		knownRefs[img.Name+":"+img.Tag] = true
	}

	var valid []string
	var hadErrors bool
	for _, ref := range refs {
		if !knownRefs[ref] {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: image not found: %s\n", ref)
			hadErrors = true
			continue
		}
		valid = append(valid, ref)
	}

	if len(valid) == 0 {
		if hadErrors {
			return fmt.Errorf("no valid images to remove")
		}
		return nil
	}

	// Confirm unless --force.
	if !force {
		if len(valid) == 1 {
			fmt.Fprintf(cmd.OutOrStdout(), "Remove %s? [y/N] ", valid[0])
		} else {
			for _, ref := range valid {
				fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", ref)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Remove %d image(s)? [y/N] ", len(valid))
		}

		scanner := bufio.NewScanner(os.Stdin)
		if !scanner.Scan() {
			return nil
		}
		answer := strings.TrimSpace(scanner.Text())
		if answer != "y" && answer != "Y" {
			fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
			return nil
		}
	}

	// Remove each valid ref.
	for _, ref := range valid {
		if err := client.Remove(ctx, ref); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)
			hadErrors = true
			continue
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Removed %s\n", ref)
	}

	if hadErrors {
		return fmt.Errorf("some images could not be removed")
	}
	return nil
}
```

- [ ] **Step 2: Register the command in root.go**

Add this line after the `newPruneCmd()` registration (line 29
in `internal/cli/root.go`):

```go
cmd.AddCommand(newRmCmd())
```

- [ ] **Step 3: Verify it compiles**

Run: `go build ./cmd/skillctl/`

Expected: clean build.

- [ ] **Step 4: Manual smoke test**

Run:

```bash
# Build a test image
./bin/skillctl build testdata/skills/hello-world

# List it
./bin/skillctl list

# Remove it (with confirmation)
./bin/skillctl rm examples/hello-world:1.0.0-draft

# Verify it's gone
./bin/skillctl list
```

Alternatively, if no testdata exists, build from the test
fixtures in the test suite.

- [ ] **Step 5: Run linter**

Run: `make lint`

Expected: no new warnings.

- [ ] **Step 6: Run full test suite**

Run: `go test ./...`

Expected: all tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/cli/rm.go internal/cli/root.go
git commit -s -m "feat: add skillctl rm command to remove local images

Closes #22"
```
