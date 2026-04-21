# Catalog metadata annotations implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task.
> Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add three new OCI manifest annotations (`io.skillimage.tags`,
`io.skillimage.compatibility`, `io.skillimage.wordcount`) so the
catalog UI can build skill cards from manifest metadata without
downloading layer blobs.

**Architecture:** Extend `buildAnnotations()` to accept a word count
and emit three new custom annotations. Compute SKILL.md word count in
`Pack()` before creating the layer. Extend `InspectResult` and the
CLI inspect command to surface the new fields.

**Tech Stack:** Go, oras-go, OCI image spec, Cobra

**Spec:**
`docs/superpowers/specs/2026-04-20-catalog-metadata-annotations-design.md`

---

## File structure

| File | Role |
| ---- | ---- |
| `pkg/oci/annotations.go` | Annotation constants + `buildAnnotations()` |
| `pkg/oci/pack.go` | SKILL.md word count, passes count to annotations |
| `pkg/oci/client.go` | `InspectResult` struct |
| `pkg/oci/inspect.go` | Reads new annotations into `InspectResult` |
| `internal/cli/inspect.go` | Displays new fields |
| `pkg/oci/oci_test.go` | Tests for new annotations and edge cases |

No new files are created. All changes are to existing files.

---

## Task 1: Add annotation constants and extend buildAnnotations

**Files:**
- Modify: `pkg/oci/annotations.go`

- [ ] **Step 1: Write the failing test**

Add to `pkg/oci/oci_test.go`:

```go
func TestAnnotationsIncludeTags(t *testing.T) {
	skillDir := t.TempDir()
	skillYAML := []byte(`apiVersion: skillimage.io/v1alpha1
kind: SkillCard
metadata:
  name: annotated-skill
  namespace: test
  version: 2.0.0
  description: Skill with tags and compat.
  tags:
    - kubernetes
    - debugging
  compatibility: claude-3.5-sonnet
spec:
  prompt: SKILL.md
`)
	if err := os.WriteFile(filepath.Join(skillDir, "skill.yaml"), skillYAML, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("one two three four five"), 0o644); err != nil {
		t.Fatal(err)
	}

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	_, err = client.Pack(ctx, skillDir, oci.PackOptions{})
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}

	result, err := client.Inspect(ctx, "test/annotated-skill:2.0.0-draft")
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}

	if result.Tags != `["kubernetes","debugging"]` {
		t.Errorf("tags = %q, want %q", result.Tags, `["kubernetes","debugging"]`)
	}
	if result.Compatibility != "claude-3.5-sonnet" {
		t.Errorf("compatibility = %q, want %q", result.Compatibility, "claude-3.5-sonnet")
	}
	if result.WordCount != "5" {
		t.Errorf("wordcount = %q, want %q", result.WordCount, "5")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./pkg/oci/ -run TestAnnotationsIncludeTags -v`

Expected: compilation error — `InspectResult` has no fields `Tags`,
`Compatibility`, `WordCount`.

- [ ] **Step 3: Add annotation key constants to annotations.go**

Add below the existing imports in `pkg/oci/annotations.go`:

```go
const (
	AnnotationTags          = "io.skillimage.tags"
	AnnotationCompatibility = "io.skillimage.compatibility"
	AnnotationWordCount     = "io.skillimage.wordcount"
)
```

- [ ] **Step 4: Extend buildAnnotations signature and add new annotations**

Change the `buildAnnotations` function in `pkg/oci/annotations.go` to
accept `wordCount int` and emit the three new annotations:

```go
func buildAnnotations(sc *skillcard.SkillCard, wordCount int) map[string]string {
	ann := make(map[string]string)

	// Title: use display-name if set, otherwise name.
	title := sc.Metadata.DisplayName
	if title == "" {
		title = sc.Metadata.Name
	}
	ann[ocispec.AnnotationTitle] = title

	// Description: first 256 characters.
	desc := sc.Metadata.Description
	if len(desc) > 256 {
		desc = desc[:256]
		for len(desc) > 0 && !utf8.ValidString(desc) {
			desc = desc[:len(desc)-1]
		}
	}
	ann[ocispec.AnnotationDescription] = desc

	// Version.
	ann[ocispec.AnnotationVersion] = sc.Metadata.Version

	// Authors: comma-separated "name <email>".
	if len(sc.Metadata.Authors) > 0 {
		var parts []string
		for _, a := range sc.Metadata.Authors {
			if a.Email != "" {
				parts = append(parts, fmt.Sprintf("%s <%s>", a.Name, a.Email))
			} else {
				parts = append(parts, a.Name)
			}
		}
		ann[ocispec.AnnotationAuthors] = strings.Join(parts, ", ")
	}

	// License.
	if sc.Metadata.License != "" {
		ann[ocispec.AnnotationLicenses] = sc.Metadata.License
	}

	// Vendor: namespace.
	ann[ocispec.AnnotationVendor] = sc.Metadata.Namespace

	// Created: RFC 3339 timestamp.
	ann[ocispec.AnnotationCreated] = time.Now().UTC().Format(time.RFC3339)

	// Provenance fields.
	if sc.Provenance != nil {
		if sc.Provenance.Source != "" {
			ann[ocispec.AnnotationSource] = sc.Provenance.Source
		}
		if sc.Provenance.Commit != "" {
			ann[ocispec.AnnotationRevision] = sc.Provenance.Commit
		}
	}

	// Lifecycle status: initial state is always draft.
	ann[lifecycle.StatusAnnotation] = string(lifecycle.Draft)

	// Tags: JSON-encoded string array.
	if len(sc.Metadata.Tags) > 0 {
		tagsJSON, err := json.Marshal(sc.Metadata.Tags)
		if err == nil {
			ann[AnnotationTags] = string(tagsJSON)
		}
	}

	// Compatibility.
	if sc.Metadata.Compatibility != "" {
		ann[AnnotationCompatibility] = sc.Metadata.Compatibility
	}

	// Word count of SKILL.md.
	if wordCount > 0 {
		ann[AnnotationWordCount] = strconv.Itoa(wordCount)
	}

	return ann
}
```

Add `"encoding/json"` and `"strconv"` to the import block.

- [ ] **Step 5: Verify annotations.go compiles**

Run: `go build ./pkg/oci/`

Expected: compilation error in `pack.go` because `buildAnnotations`
now requires two arguments. That is expected — we fix it next.

- [ ] **Step 6: Add word count computation to Pack**

In `pkg/oci/pack.go`, in the `Pack` method, add SKILL.md word count
logic between step 2 (validation) and step 3 (createLayer). Then
update the `buildAnnotations` call to pass `wordCount`:

After the validation block (after the `if len(validationErrors) > 0`
block), add:

```go
	// 2b. Count words in SKILL.md if present.
	var wordCount int
	skillMDPath := filepath.Join(skillDir, "SKILL.md")
	if data, err := os.ReadFile(skillMDPath); err == nil {
		wordCount = len(strings.Fields(string(data)))
	}
```

Then change line 89 from:

```go
	annotations := buildAnnotations(sc)
```

to:

```go
	annotations := buildAnnotations(sc, wordCount)
```

- [ ] **Step 7: Verify the package compiles**

Run: `go build ./pkg/oci/`

Expected: clean build, no errors.

- [ ] **Step 8: Commit**

```bash
git add pkg/oci/annotations.go pkg/oci/pack.go pkg/oci/oci_test.go
git commit -s -m "feat(oci): add catalog metadata annotations

Add io.skillimage.tags, io.skillimage.compatibility, and
io.skillimage.wordcount annotations. Compute SKILL.md word count
during pack using strings.Fields()."
```

---

## Task 2: Extend InspectResult and inspect logic

**Files:**
- Modify: `pkg/oci/client.go`
- Modify: `pkg/oci/inspect.go`

- [ ] **Step 1: Add fields to InspectResult**

In `pkg/oci/client.go`, add three fields to `InspectResult`:

```go
type InspectResult struct {
	Name          string
	DisplayName   string
	Version       string
	Status        string
	Description   string
	Authors       string
	License       string
	Tags          string
	Compatibility string
	WordCount     string
	Digest        string
	Created       string
	MediaType     string
	Size          int64
	LayerCount    int
}
```

- [ ] **Step 2: Read new annotations in Inspect**

In `pkg/oci/inspect.go`, in the `Inspect` method, add three lines
after the existing annotation reads (after the `created` line):

```go
	tags := ann[AnnotationTags]
	compatibility := ann[AnnotationCompatibility]
	wordCount := ann[AnnotationWordCount]
```

And add them to the returned `InspectResult`:

```go
	return &InspectResult{
		Name:          name,
		DisplayName:   displayName,
		Version:       version,
		Status:        status,
		Description:   description,
		Authors:       authors,
		License:       license,
		Tags:          tags,
		Compatibility: compatibility,
		WordCount:     wordCount,
		Digest:        desc.Digest.String(),
		Created:       created,
		MediaType:     desc.MediaType,
		Size:          totalSize,
		LayerCount:    len(manifest.Layers),
	}, nil
```

- [ ] **Step 3: Run the failing test from Task 1**

Run: `go test ./pkg/oci/ -run TestAnnotationsIncludeTags -v`

Expected: PASS — tags, compatibility, and wordcount all match.

- [ ] **Step 4: Run full test suite**

Run: `make test`

Expected: all tests pass, including the existing ones (which use
`writeTestSkill` which has no tags/compatibility — those annotations
are simply absent, and existing tests don't check for them).

- [ ] **Step 5: Commit**

```bash
git add pkg/oci/client.go pkg/oci/inspect.go
git commit -s -m "feat(oci): surface tags, compatibility, wordcount in inspect

Extend InspectResult with three new fields and read them from
manifest annotations."
```

---

## Task 3: Display new fields in CLI inspect command

**Files:**
- Modify: `internal/cli/inspect.go`

- [ ] **Step 1: Add display lines for new fields**

In `internal/cli/inspect.go`, in `runInspect`, add after the
`License` block (before the blank line `fmt.Fprintln(out)`):

```go
	if result.Tags != "" {
		fmt.Fprintf(out, "Tags:         %s\n", result.Tags)
	}
	if result.Compatibility != "" {
		fmt.Fprintf(out, "Compat:       %s\n", result.Compatibility)
	}
	if result.WordCount != "" {
		fmt.Fprintf(out, "Word Count:   %s\n", result.WordCount)
	}
```

- [ ] **Step 2: Build and manually verify**

Run: `make build`

Then test with the hello-world example:

```bash
bin/skillctl pack examples/hello-world
bin/skillctl inspect examples/hello-world:1.0.0-draft
```

Expected output includes:

```
Tags:         ["example","getting-started"]
Word Count:   18
```

(No `Compat:` line since hello-world has no `compatibility` field.)

- [ ] **Step 3: Commit**

```bash
git add internal/cli/inspect.go
git commit -s -m "feat(cli): display tags, compatibility, word count in inspect"
```

---

## Task 4: Edge case tests

**Files:**
- Modify: `pkg/oci/oci_test.go`

- [ ] **Step 1: Test — no tags, no compatibility, no SKILL.md**

```go
func TestAnnotationsOmittedWhenEmpty(t *testing.T) {
	skillDir := t.TempDir()
	skillYAML := []byte(`apiVersion: skillimage.io/v1alpha1
kind: SkillCard
metadata:
  name: minimal-skill
  namespace: test
  version: 1.0.0
  description: Minimal skill with no optional fields.
spec:
  prompt: SKILL.md
`)
	if err := os.WriteFile(filepath.Join(skillDir, "skill.yaml"), skillYAML, 0o644); err != nil {
		t.Fatal(err)
	}

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	_, err = client.Pack(ctx, skillDir, oci.PackOptions{})
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}

	result, err := client.Inspect(ctx, "test/minimal-skill:1.0.0-draft")
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}

	if result.Tags != "" {
		t.Errorf("tags should be empty, got %q", result.Tags)
	}
	if result.Compatibility != "" {
		t.Errorf("compatibility should be empty, got %q", result.Compatibility)
	}
	if result.WordCount != "" {
		t.Errorf("wordcount should be empty, got %q", result.WordCount)
	}
}
```

- [ ] **Step 2: Test — empty SKILL.md**

```go
func TestAnnotationsEmptySKILLmd(t *testing.T) {
	skillDir := t.TempDir()
	skillYAML := []byte(`apiVersion: skillimage.io/v1alpha1
kind: SkillCard
metadata:
  name: empty-md-skill
  namespace: test
  version: 1.0.0
  description: Skill with empty SKILL.md.
spec:
  prompt: SKILL.md
`)
	if err := os.WriteFile(filepath.Join(skillDir, "skill.yaml"), skillYAML, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	_, err = client.Pack(ctx, skillDir, oci.PackOptions{})
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}

	result, err := client.Inspect(ctx, "test/empty-md-skill:1.0.0-draft")
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}

	if result.WordCount != "" {
		t.Errorf("wordcount should be empty for empty SKILL.md, got %q", result.WordCount)
	}
}
```

- [ ] **Step 3: Run all edge case tests**

Run: `go test ./pkg/oci/ -run "TestAnnotations" -v`

Expected: all three annotation tests pass (`TestAnnotationsIncludeTags`,
`TestAnnotationsOmittedWhenEmpty`, `TestAnnotationsEmptySKILLmd`).

- [ ] **Step 4: Run full suite and lint**

Run: `make test && make lint`

Expected: all pass, no lint issues.

- [ ] **Step 5: Commit**

```bash
git add pkg/oci/oci_test.go
git commit -s -m "test(oci): add edge case tests for catalog annotations

Cover: no tags/compat/SKILL.md, empty SKILL.md produces no wordcount."
```

---

## Task 5: Update example skill and final verification

**Files:**
- Modify: `examples/hello-world/skill.yaml`

- [ ] **Step 1: Add compatibility field to example skill**

In `examples/hello-world/skill.yaml`, add after `tags`:

```yaml
  compatibility: claude-3.5-sonnet
```

- [ ] **Step 2: Full end-to-end test**

```bash
make build && make test && make lint
```

Expected: all pass.

- [ ] **Step 3: Manual smoke test**

```bash
bin/skillctl pack examples/hello-world
bin/skillctl inspect examples/hello-world:1.0.0-draft
```

Expected output includes all three new fields:

```
Tags:         ["example","getting-started"]
Compat:       claude-3.5-sonnet
Word Count:   18
```

- [ ] **Step 4: Commit**

```bash
git add examples/hello-world/skill.yaml
git commit -s -m "docs: add compatibility field to hello-world example"
```
