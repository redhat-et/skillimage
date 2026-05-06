# List Output Formatting Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task.
> Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `skillctl list` output match container tool
conventions by stripping digest prefixes and humanizing timestamps.

**Architecture:** All changes are in the CLI display layer
(`internal/cli/list.go`). The OCI data model is unchanged.
A `--no-trunc` flag preserves raw values for scripting.

**Tech Stack:** Go stdlib `time`, `strings`; `dustin/go-humanize`
(already an indirect dep via modernc/sqlite — promote to direct).

---

### Task 1: Add go-humanize as direct dependency

**Files:**
- Modify: `go.mod`

- [ ] **Step 1: Promote go-humanize to direct dependency**

Run:

```bash
go get github.com/dustin/go-humanize@v1.0.1
```

This moves `go-humanize` from `// indirect` to a direct
dependency in `go.mod`. No `go.sum` changes needed since
the module is already resolved.

- [ ] **Step 2: Verify**

Run:

```bash
grep 'go-humanize' go.mod
```

Expected: line without `// indirect` comment.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -s -m "build: promote go-humanize to direct dependency

Needed for humanized timestamps in skillctl list output.

Assisted-By: Claude (Anthropic AI) <noreply@anthropic.com>"
```

---

### Task 2: Write tests for digest and timestamp formatting

**Files:**
- Create: `internal/cli/list_test.go`

- [ ] **Step 1: Write tests for formatDigest helper**

Create `internal/cli/list_test.go`:

```go
package cli

import (
	"testing"
	"time"
)

func TestFormatDigest(t *testing.T) {
	tests := []struct {
		name    string
		digest  string
		noTrunc bool
		want    string
	}{
		{
			name:   "strips sha256 prefix and truncates",
			digest: "sha256:a593244d38f0e1b2c3d4e5f6a7b8c9d0e1f2a3b4",
			want:   "a593244d38f0",
		},
		{
			name:   "strips other algo prefix",
			digest: "sha512:abcdef123456789012345678",
			want:   "abcdef123456",
		},
		{
			name:   "handles digest shorter than 12 chars",
			digest: "sha256:abcd",
			want:   "abcd",
		},
		{
			name:   "handles digest with no prefix",
			digest: "a593244d38f0e1b2c3d4",
			want:   "a593244d38f0",
		},
		{
			name:    "no-trunc preserves full digest",
			digest:  "sha256:a593244d38f0e1b2c3d4e5f6a7b8c9d0e1f2a3b4",
			noTrunc: true,
			want:    "sha256:a593244d38f0e1b2c3d4e5f6a7b8c9d0e1f2a3b4",
		},
		{
			name:   "empty digest",
			digest: "",
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDigest(tt.digest, tt.noTrunc)
			if got != tt.want {
				t.Errorf("formatDigest(%q, %v) = %q, want %q",
					tt.digest, tt.noTrunc, got, tt.want)
			}
		})
	}
}

func TestFormatCreated(t *testing.T) {
	tests := []struct {
		name    string
		created string
		noTrunc bool
		want    string
	}{
		{
			name:    "no-trunc returns raw timestamp",
			created: "2026-04-29T22:07:29Z",
			noTrunc: true,
			want:    "2026-04-29T22:07:29Z",
		},
		{
			name:    "empty string returns empty",
			created: "",
			want:    "",
		},
		{
			name:    "unparseable falls back to raw string",
			created: "not-a-timestamp",
			want:    "not-a-timestamp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatCreated(tt.created, tt.noTrunc)
			if got != tt.want {
				t.Errorf("formatCreated(%q, %v) = %q, want %q",
					tt.created, tt.noTrunc, got, tt.want)
			}
		})
	}

	t.Run("recent timestamp shows relative time", func(t *testing.T) {
		recent := time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)
		got := formatCreated(recent, false)
		if got == recent {
			t.Errorf("expected humanized time, got raw timestamp %q", got)
		}
		if got == "" {
			t.Error("expected non-empty result")
		}
	})
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
go test ./internal/cli/ -run 'TestFormat' -v
```

Expected: compilation error — `formatDigest` and `formatCreated`
are undefined.

---

### Task 3: Implement formatting helpers

**Files:**
- Modify: `internal/cli/list.go`

- [ ] **Step 1: Add the two formatting functions to list.go**

Add these functions at the bottom of `internal/cli/list.go`:

```go
func formatDigest(digest string, noTrunc bool) string {
	if digest == "" {
		return ""
	}
	if noTrunc {
		return digest
	}
	if idx := strings.IndexByte(digest, ':'); idx >= 0 {
		digest = digest[idx+1:]
	}
	if len(digest) > 12 {
		digest = digest[:12]
	}
	return digest
}

func formatCreated(created string, noTrunc bool) string {
	if created == "" {
		return ""
	}
	if noTrunc {
		return created
	}
	t, err := time.Parse(time.RFC3339, created)
	if err != nil {
		return created
	}
	return humanize.Time(t)
}
```

- [ ] **Step 2: Update the imports**

Replace the import block at the top of `list.go` with:

```go
import (
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	humanize "github.com/dustin/go-humanize"
	"github.com/spf13/cobra"

	"github.com/redhat-et/skillimage/pkg/installed"
	"github.com/redhat-et/skillimage/pkg/oci"
)
```

- [ ] **Step 3: Run tests to verify they pass**

Run:

```bash
go test ./internal/cli/ -run 'TestFormat' -v
```

Expected: all tests pass.

- [ ] **Step 4: Commit**

```bash
git add internal/cli/list.go internal/cli/list_test.go
git commit -s -m "feat: add digest and timestamp formatting helpers

formatDigest strips the algo: prefix and truncates to 12 chars.
formatCreated renders relative time using go-humanize.
Both accept a noTrunc flag to preserve raw values.

Closes #23

Assisted-By: Claude (Anthropic AI) <noreply@anthropic.com>"
```

---

### Task 4: Wire formatting into list output and add --no-trunc flag

**Files:**
- Modify: `internal/cli/list.go`

- [ ] **Step 1: Add --no-trunc flag to newListCmd**

In `newListCmd()`, add a new variable and flag registration.
After the existing `var upgradable bool` line, add:

```go
var noTrunc bool
```

After the existing `cmd.Flags().BoolVarP(&upgradable, ...)` line,
add:

```go
cmd.Flags().BoolVar(&noTrunc, "no-trunc", false, "show full digest and raw timestamps")
```

- [ ] **Step 2: Pass noTrunc to runList**

Change the `runList` call in the `RunE` closure from:

```go
return runList(cmd)
```

to:

```go
return runList(cmd, noTrunc)
```

- [ ] **Step 3: Update runList signature and formatting**

Change `runList` to accept the `noTrunc` parameter and use
the formatting helpers. Replace the entire `runList` function:

```go
func runList(cmd *cobra.Command, noTrunc bool) error {
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
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			img.Name, img.Tag, img.Status,
			formatDigest(img.Digest, noTrunc),
			formatCreated(img.Created, noTrunc))
	}
	return w.Flush()
}
```

- [ ] **Step 4: Run full test suite**

Run:

```bash
go test ./internal/cli/ -v
```

Expected: all tests pass.

- [ ] **Step 5: Run linter**

Run:

```bash
make lint
```

Expected: no new warnings.

- [ ] **Step 6: Build and smoke test**

Run:

```bash
make build && bin/skillctl list
```

Verify output shows short digests (no `sha256:` prefix) and
relative timestamps. Then:

```bash
bin/skillctl list --no-trunc
```

Verify output shows full digests with `sha256:` prefix and
raw RFC 3339 timestamps.

- [ ] **Step 7: Commit**

```bash
git add internal/cli/list.go
git commit -s -m "feat: wire formatting into list output with --no-trunc flag

Default output now strips the sha256: prefix from digests and
shows relative timestamps. Use --no-trunc for raw values.

Assisted-By: Claude (Anthropic AI) <noreply@anthropic.com>"
```
