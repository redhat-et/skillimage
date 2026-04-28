# Skill Collections Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace physical skill bundles with logical skill collections — YAML manifests stored as OCI artifacts that reference individual skill images from any registry.

**Architecture:** New `pkg/collection/` package for YAML parsing and validation. New `internal/cli/collection.go` registers a `collection` parent command with `push`, `pull`, `volume`, `generate` subcommands. Catalog server gets a `collections` table and two API endpoints. All existing bundle code is removed.

**Tech Stack:** Go 1.25+, oras-go, Cobra, chi, SQLite, gopkg.in/yaml.v3

**Spec:** `docs/superpowers/specs/2026-04-27-skill-collections-design.md`

---

### Task 1: Remove bundle code

**Files:**
- Delete: `pkg/oci/bundle.go`
- Delete: `pkg/oci/bundle_test.go`
- Modify: `pkg/oci/annotations.go` — remove bundle constants
- Modify: `internal/cli/build.go` — remove `--bundle` flag and `runBuildBundle`
- Modify: `internal/store/store.go` — remove bundle fields and columns
- Modify: `internal/store/sync.go` — remove bundle annotation check

- [ ] **Step 1: Delete bundle files**

```bash
rm pkg/oci/bundle.go pkg/oci/bundle_test.go
```

- [ ] **Step 2: Remove bundle constants from annotations.go**

In `pkg/oci/annotations.go`, remove lines 22-23:

```go
	AnnotationBundle       = "io.skillimage.bundle"
	AnnotationBundleSkills = "io.skillimage.bundle.skills"
```

- [ ] **Step 3: Remove --bundle flag from build.go**

Replace `internal/cli/build.go` with the bundle-free version:

```go
package cli

import (
	"context"
	"fmt"

	"github.com/redhat-et/skillimage/pkg/oci"
	"github.com/spf13/cobra"
)

func newBuildCmd() *cobra.Command {
	var tag string
	var mediaType string
	cmd := &cobra.Command{
		Use:   "build <dir>",
		Short: "Build a skill directory into a local OCI image",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBuild(cmd, args[0], tag, mediaType)
		},
	}
	cmd.Flags().StringVar(&tag, "tag", "", "override the image tag (default: <version>-draft)")
	cmd.Flags().StringVar(&mediaType, "media-type", "", `media type profile: "standard" (default) or "redhat" (for oc-mirror)`)
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
```

- [ ] **Step 4: Remove bundle fields from store.go**

In `internal/store/store.go`:

Remove from `Skill` struct (lines 38-39):
```go
	Bundle        bool   `json:"bundle"`
	BundleSkills  string `json:"bundle_skills"`
```

Remove from `createSchema()` SQL (lines 96-97):
```go
			bundle        INTEGER DEFAULT 0,
			bundle_skills TEXT,
```

Remove from `UpsertSkill()`:
- Remove `bundle, bundle_skills,` from INSERT column list
- Remove the two `?` placeholders for bundle values
- Remove `bundle=excluded.bundle, bundle_skills=excluded.bundle_skills,` from ON CONFLICT
- Remove `sk.Bundle, sk.BundleSkills,` from value args

Remove `bundle, bundle_skills,` from every SELECT query in `ListSkills`, `GetSkill`, `GetVersions`.

Remove `&sk.Bundle, &sk.BundleSkills,` from the `Scan()` call in `querySkills`.

- [ ] **Step 5: Remove bundle annotation check from sync.go**

In `internal/store/sync.go`, remove lines 144-147:

```go
	if ann[oci.AnnotationBundle] == "true" {
		sk.Bundle = true
		sk.BundleSkills = ann[oci.AnnotationBundleSkills]
	}
```

- [ ] **Step 6: Remove BundleBuildOptions from client.go**

In `pkg/oci/client.go`, remove the `BundleBuildOptions` type (if it exists there — it may be defined in the deleted bundle.go).

- [ ] **Step 7: Verify build and tests**

Run: `go build ./... && go test ./...`
Expected: All pass with no bundle references.

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -s -m "refactor: remove bundle code in favor of collections"
```

---

### Task 2: Add collection YAML parsing package

**Files:**
- Create: `pkg/collection/collection.go`
- Create: `pkg/collection/collection_test.go`

- [ ] **Step 1: Write the failing test**

Create `pkg/collection/collection_test.go`:

```go
package collection_test

import (
	"strings"
	"testing"

	"github.com/redhat-et/skillimage/pkg/collection"
)

func TestParseValid(t *testing.T) {
	input := `apiVersion: skillimage.io/v1alpha1
kind: SkillCollection
metadata:
  name: hr-skills
  version: 1.0.0
  description: Skills for HR document processing
skills:
  - name: document-summarizer
    image: quay.io/skillimage/business/document-summarizer:1.0.0
  - name: document-reviewer
    image: ghcr.io/acme/document-reviewer:2.1.0
`
	col, err := collection.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if col.Metadata.Name != "hr-skills" {
		t.Errorf("name = %q, want %q", col.Metadata.Name, "hr-skills")
	}
	if col.Metadata.Version != "1.0.0" {
		t.Errorf("version = %q, want %q", col.Metadata.Version, "1.0.0")
	}
	if len(col.Skills) != 2 {
		t.Fatalf("skills count = %d, want 2", len(col.Skills))
	}
	if col.Skills[0].Name != "document-summarizer" {
		t.Errorf("skill[0].name = %q, want %q", col.Skills[0].Name, "document-summarizer")
	}
	if col.Skills[1].Image != "ghcr.io/acme/document-reviewer:2.1.0" {
		t.Errorf("skill[1].image = %q", col.Skills[1].Image)
	}
}

func TestParseInvalidKind(t *testing.T) {
	input := `apiVersion: skillimage.io/v1alpha1
kind: SkillCard
metadata:
  name: test
  version: 1.0.0
skills:
  - name: s1
    image: quay.io/org/s1:1.0.0
`
	_, err := collection.Parse(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for wrong kind")
	}
}

func TestValidateDuplicateNames(t *testing.T) {
	col := &collection.SkillCollection{
		APIVersion: "skillimage.io/v1alpha1",
		Kind:       "SkillCollection",
		Metadata:   collection.Metadata{Name: "test", Version: "1.0.0"},
		Skills: []collection.SkillRef{
			{Name: "skill-a", Image: "quay.io/org/skill-a:1.0.0"},
			{Name: "skill-a", Image: "ghcr.io/org/skill-a:2.0.0"},
		},
	}
	errs := collection.Validate(col)
	if len(errs) == 0 {
		t.Fatal("expected validation error for duplicate names")
	}
}

func TestValidateMissingFields(t *testing.T) {
	col := &collection.SkillCollection{
		APIVersion: "skillimage.io/v1alpha1",
		Kind:       "SkillCollection",
		Metadata:   collection.Metadata{Name: "test", Version: "1.0.0"},
		Skills: []collection.SkillRef{
			{Name: "", Image: "quay.io/org/s:1.0.0"},
			{Name: "s2", Image: ""},
		},
	}
	errs := collection.Validate(col)
	if len(errs) < 2 {
		t.Errorf("expected at least 2 errors, got %d", len(errs))
	}
}

func TestValidateEmptySkills(t *testing.T) {
	col := &collection.SkillCollection{
		APIVersion: "skillimage.io/v1alpha1",
		Kind:       "SkillCollection",
		Metadata:   collection.Metadata{Name: "test", Version: "1.0.0"},
		Skills:     nil,
	}
	errs := collection.Validate(col)
	if len(errs) == 0 {
		t.Fatal("expected error for empty skills list")
	}
}

func TestParseFile(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/collection.yaml"
	content := []byte(`apiVersion: skillimage.io/v1alpha1
kind: SkillCollection
metadata:
  name: test-col
  version: 1.0.0
skills:
  - name: s1
    image: quay.io/org/s1:1.0.0
`)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	col, err := collection.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if col.Metadata.Name != "test-col" {
		t.Errorf("name = %q, want %q", col.Metadata.Name, "test-col")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/collection/...`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Write the implementation**

Create `pkg/collection/collection.go`:

```go
package collection

import (
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

// SkillCollection is a YAML manifest listing skill images.
type SkillCollection struct {
	APIVersion string    `yaml:"apiVersion"`
	Kind       string    `yaml:"kind"`
	Metadata   Metadata  `yaml:"metadata"`
	Skills     []SkillRef `yaml:"skills"`
}

// Metadata holds collection-level fields.
type Metadata struct {
	Name        string `yaml:"name"`
	Version     string `yaml:"version"`
	Description string `yaml:"description,omitempty"`
}

// SkillRef is a reference to a skill image in a registry.
type SkillRef struct {
	Name  string `yaml:"name"`
	Image string `yaml:"image"`
}

// Parse reads a SkillCollection from an io.Reader.
func Parse(r io.Reader) (*SkillCollection, error) {
	var col SkillCollection
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true)
	if err := dec.Decode(&col); err != nil {
		return nil, fmt.Errorf("parsing collection YAML: %w", err)
	}
	if col.Kind != "SkillCollection" {
		return nil, fmt.Errorf("expected kind SkillCollection, got %q", col.Kind)
	}
	return &col, nil
}

// ParseFile reads a SkillCollection from a file path.
func ParseFile(path string) (*SkillCollection, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening collection file: %w", err)
	}
	defer func() { _ = f.Close() }()
	return Parse(f)
}

// Validate checks a parsed SkillCollection for errors.
func Validate(col *SkillCollection) []string {
	var errs []string

	if col.Metadata.Name == "" {
		errs = append(errs, "metadata.name is required")
	}
	if col.Metadata.Version == "" {
		errs = append(errs, "metadata.version is required")
	}
	if len(col.Skills) == 0 {
		errs = append(errs, "at least one skill is required")
		return errs
	}

	seen := make(map[string]bool)
	for i, s := range col.Skills {
		if s.Name == "" {
			errs = append(errs, fmt.Sprintf("skills[%d].name is required", i))
		}
		if s.Image == "" {
			errs = append(errs, fmt.Sprintf("skills[%d].image is required", i))
		}
		if s.Name != "" && seen[s.Name] {
			errs = append(errs, fmt.Sprintf("duplicate skill name %q", s.Name))
		}
		seen[s.Name] = true
	}

	return errs
}
```

- [ ] **Step 4: Add missing import to test file**

Add `"os"` to the imports in `collection_test.go`.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./pkg/collection/...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add pkg/collection/
git commit -s -m "feat: add collection YAML parsing package"
```

---

### Task 3: Add collection push command

**Files:**
- Create: `pkg/oci/collection.go`
- Create: `pkg/oci/collection_test.go`
- Create: `internal/cli/collection.go`
- Modify: `internal/cli/root.go` — register collection command

- [ ] **Step 1: Write the failing test for OCI push**

Create `pkg/oci/collection_test.go`:

```go
package oci_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/redhat-et/skillimage/pkg/oci"
)

func writeTestCollection(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "collection.yaml")
	content := []byte(`apiVersion: skillimage.io/v1alpha1
kind: SkillCollection
metadata:
  name: test-collection
  version: 1.0.0
  description: Test collection
skills:
  - name: skill-a
    image: quay.io/org/skill-a:1.0.0
  - name: skill-b
    image: ghcr.io/org/skill-b:2.0.0
`)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBuildCollectionArtifact(t *testing.T) {
	dir := t.TempDir()
	yamlPath := writeTestCollection(t, dir)

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	desc, err := client.BuildCollectionArtifact(ctx, yamlPath, "test/test-collection:1.0.0")
	if err != nil {
		t.Fatalf("BuildCollectionArtifact: %v", err)
	}
	if desc.Digest.String() == "" {
		t.Error("expected non-empty digest")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/oci/ -run TestBuildCollection`
Expected: FAIL — method not defined.

- [ ] **Step 3: Write the OCI collection implementation**

Create `pkg/oci/collection.go`:

```go
package oci

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	godigest "github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/errdef"

	"github.com/redhat-et/skillimage/pkg/collection"
)

const (
	CollectionArtifactType = "application/vnd.skillimage.collection.v1+yaml"
	CollectionMediaType    = "application/vnd.skillimage.collection.v1+yaml"
	AnnotationCollectionName = "io.skillimage.collection.name"
)

// BuildCollectionArtifact reads a collection YAML file, validates it,
// and stores it as an OCI artifact in the local store.
func (c *Client) BuildCollectionArtifact(ctx context.Context, yamlPath, ref string) (ocispec.Descriptor, error) {
	col, err := collection.ParseFile(yamlPath)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	if errs := collection.Validate(col); len(errs) > 0 {
		return ocispec.Descriptor{}, fmt.Errorf("invalid collection: %s", errs[0])
	}

	yamlBytes, err := os.ReadFile(yamlPath)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("reading collection file: %w", err)
	}

	layerDigest := godigest.FromBytes(yamlBytes)
	layerDesc := ocispec.Descriptor{
		MediaType: CollectionMediaType,
		Digest:    layerDigest,
		Size:      int64(len(yamlBytes)),
	}
	if err := c.store.Push(ctx, layerDesc, bytes.NewReader(yamlBytes)); err != nil && !errors.Is(err, errdef.ErrAlreadyExists) {
		return ocispec.Descriptor{}, fmt.Errorf("pushing collection layer: %w", err)
	}

	configBytes := []byte("{}")
	configDigest := godigest.FromBytes(configBytes)
	configDesc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeEmptyJSON,
		Digest:    configDigest,
		Size:      int64(len(configBytes)),
	}
	if err := c.store.Push(ctx, configDesc, bytes.NewReader(configBytes)); err != nil && !errors.Is(err, errdef.ErrAlreadyExists) {
		return ocispec.Descriptor{}, fmt.Errorf("pushing config: %w", err)
	}

	ann := map[string]string{
		ocispec.AnnotationTitle:       col.Metadata.Name,
		ocispec.AnnotationVersion:     col.Metadata.Version,
		ocispec.AnnotationCreated:     time.Now().UTC().Format(time.RFC3339),
		AnnotationCollectionName:      col.Metadata.Name,
	}
	if col.Metadata.Description != "" {
		ann[ocispec.AnnotationDescription] = col.Metadata.Description
	}

	return c.buildAndTagManifest(ctx, configDesc, []ocispec.Descriptor{layerDesc}, ann, CollectionArtifactType, ref)
}

// PushCollection pushes a collection artifact from the local store to a remote registry.
func (c *Client) PushCollection(ctx context.Context, ref string, opts PushOptions) error {
	return c.Push(ctx, ref, opts)
}
```

- [ ] **Step 4: Add buildAndTagManifest helper**

This helper is extracted from the common pattern in `build.go`. Add to `pkg/oci/collection.go`:

```go
func (c *Client) buildAndTagManifest(ctx context.Context, configDesc ocispec.Descriptor, layers []ocispec.Descriptor, ann map[string]string, artifactType, ref string) (ocispec.Descriptor, error) {
	manifest := ocispec.Manifest{
		Versioned:   specs.Versioned{SchemaVersion: 2},
		MediaType:   ocispec.MediaTypeImageManifest,
		Config:      configDesc,
		Layers:      layers,
		Annotations: ann,
	}
	if artifactType != "" {
		manifest.ArtifactType = artifactType
	}

	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("marshaling manifest: %w", err)
	}

	manifestDigest := godigest.FromBytes(manifestBytes)
	manifestDesc := ocispec.Descriptor{
		MediaType:    ocispec.MediaTypeImageManifest,
		Digest:       manifestDigest,
		Size:         int64(len(manifestBytes)),
		Annotations:  ann,
		ArtifactType: artifactType,
	}

	if err := c.store.Push(ctx, manifestDesc, bytes.NewReader(manifestBytes)); err != nil && !errors.Is(err, errdef.ErrAlreadyExists) {
		return ocispec.Descriptor{}, fmt.Errorf("pushing manifest: %w", err)
	}

	if err := c.store.Tag(ctx, manifestDesc, ref); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("tagging: %w", err)
	}

	return manifestDesc, nil
}
```

Add the required imports to `collection.go`:

```go
import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	godigest "github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/errdef"

	"github.com/redhat-et/skillimage/pkg/collection"
)
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./pkg/oci/ -run TestBuildCollection`
Expected: PASS

- [ ] **Step 6: Write the CLI collection command**

Create `internal/cli/collection.go`:

```go
package cli

import (
	"context"
	"fmt"

	"github.com/redhat-et/skillimage/pkg/oci"
	"github.com/spf13/cobra"
)

func newCollectionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "collection",
		Short: "Manage skill collections",
	}
	cmd.AddCommand(newCollectionPushCmd())
	return cmd
}

func newCollectionPushCmd() *cobra.Command {
	var file string
	var tlsVerify bool
	cmd := &cobra.Command{
		Use:   "push <ref>",
		Short: "Push a collection YAML to a registry as an OCI artifact",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCollectionPush(cmd, args[0], file, !tlsVerify)
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "path to collection YAML file (required)")
	_ = cmd.MarkFlagRequired("file")
	cmd.Flags().BoolVar(&tlsVerify, "tls-verify", true, "require HTTPS and verify certificates")
	return cmd
}

func runCollectionPush(cmd *cobra.Command, ref, file string, skipTLSVerify bool) error {
	client, err := defaultClient()
	if err != nil {
		return err
	}

	ctx := context.Background()
	desc, err := client.BuildCollectionArtifact(ctx, file, ref)
	if err != nil {
		return fmt.Errorf("building collection artifact: %w", err)
	}

	if err := client.PushCollection(ctx, ref, oci.PushOptions{SkipTLSVerify: skipTLSVerify}); err != nil {
		return fmt.Errorf("pushing collection: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Pushed collection %s\nDigest: %s\n", ref, desc.Digest)
	return nil
}
```

- [ ] **Step 7: Register collection command in root.go**

In `internal/cli/root.go`, add:

```go
	cmd.AddCommand(newCollectionCmd())
```

- [ ] **Step 8: Verify build**

Run: `go build ./...`
Expected: Compiles cleanly.

- [ ] **Step 9: Commit**

```bash
git add pkg/oci/collection.go pkg/oci/collection_test.go internal/cli/collection.go internal/cli/root.go
git commit -s -m "feat: add collection push command and OCI artifact support"
```

---

### Task 4: Add collection pull command

**Files:**
- Modify: `internal/cli/collection.go` — add pull subcommand
- Modify: `pkg/oci/collection.go` — add pull logic

- [ ] **Step 1: Write the pull method in pkg/oci**

Add to `pkg/oci/collection.go`:

```go
// PullCollection fetches a collection artifact from a remote registry,
// parses the YAML, and pulls each referenced skill image into outputDir.
func (c *Client) PullCollection(ctx context.Context, ref string, outputDir string, opts PullOptions) (*collection.SkillCollection, error) {
	if err := c.Pull(ctx, ref, oci.PullOptions{SkipTLSVerify: opts.SkipTLSVerify}); err != nil {
		return nil, fmt.Errorf("pulling collection artifact: %w", err)
	}

	col, err := c.extractCollectionYAML(ctx, ref)
	if err != nil {
		return nil, err
	}

	if outputDir != "" {
		for _, skill := range col.Skills {
			if err := c.Pull(ctx, skill.Image, PullOptions{
				OutputDir:     outputDir,
				SkipTLSVerify: opts.SkipTLSVerify,
			}); err != nil {
				return nil, fmt.Errorf("pulling skill %s: %w", skill.Name, err)
			}
		}
	}

	return col, nil
}

func (c *Client) extractCollectionYAML(ctx context.Context, ref string) (*collection.SkillCollection, error) {
	desc, err := c.store.Resolve(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("resolving %s: %w", ref, err)
	}

	rc, err := c.store.Fetch(ctx, desc)
	if err != nil {
		return nil, fmt.Errorf("fetching manifest: %w", err)
	}
	defer func() { _ = rc.Close() }()

	manifestBytes, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}

	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}

	if len(manifest.Layers) == 0 {
		return nil, fmt.Errorf("collection manifest has no layers")
	}

	layerRC, err := c.store.Fetch(ctx, manifest.Layers[0])
	if err != nil {
		return nil, fmt.Errorf("fetching collection layer: %w", err)
	}
	defer func() { _ = layerRC.Close() }()

	return collection.Parse(layerRC)
}
```

Add `"io"` to the imports.

- [ ] **Step 2: Add the CLI pull subcommand**

Add to `internal/cli/collection.go`:

```go
func newCollectionPullCmd() *cobra.Command {
	var outputDir string
	var tlsVerify bool
	cmd := &cobra.Command{
		Use:   "pull <ref>",
		Short: "Pull a collection and all its skills from a registry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCollectionPull(cmd, args[0], outputDir, !tlsVerify)
		},
	}
	cmd.Flags().StringVarP(&outputDir, "output", "o", "", "directory to extract skills into")
	cmd.Flags().BoolVar(&tlsVerify, "tls-verify", true, "require HTTPS and verify certificates")
	return cmd
}

func runCollectionPull(cmd *cobra.Command, ref, outputDir string, skipTLSVerify bool) error {
	client, err := defaultClient()
	if err != nil {
		return err
	}

	col, err := client.PullCollection(context.Background(), ref, outputDir, oci.PullOptions{
		OutputDir:     outputDir,
		SkipTLSVerify: skipTLSVerify,
	})
	if err != nil {
		return fmt.Errorf("pulling collection: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Pulled collection %s (%d skills)\n", col.Metadata.Name, len(col.Skills))
	for _, s := range col.Skills {
		fmt.Fprintf(cmd.OutOrStdout(), "  - %s (%s)\n", s.Name, s.Image)
	}
	return nil
}
```

- [ ] **Step 3: Register pull subcommand**

In `newCollectionCmd()`, add:

```go
	cmd.AddCommand(newCollectionPullCmd())
```

- [ ] **Step 4: Verify build**

Run: `go build ./...`
Expected: Compiles cleanly.

- [ ] **Step 5: Commit**

```bash
git add pkg/oci/collection.go internal/cli/collection.go
git commit -s -m "feat: add collection pull command"
```

---

### Task 5: Add collection volume command

**Files:**
- Modify: `internal/cli/collection.go` — add volume subcommand
- Modify: `pkg/collection/collection.go` — add GenerateVolume helper

- [ ] **Step 1: Write the failing test**

Add to `pkg/collection/collection_test.go`:

```go
func TestGeneratePodmanVolumes(t *testing.T) {
	col := &collection.SkillCollection{
		Skills: []collection.SkillRef{
			{Name: "skill-a", Image: "quay.io/org/skill-a:1.0.0"},
			{Name: "skill-b", Image: "ghcr.io/org/skill-b:2.0.0"},
		},
	}
	var buf bytes.Buffer
	collection.GeneratePodmanVolumes(&buf, col, "/skills")
	output := buf.String()
	if !strings.Contains(output, "podman pull quay.io/org/skill-a:1.0.0") {
		t.Errorf("missing pull command for skill-a")
	}
	if !strings.Contains(output, "podman volume create --driver image") {
		t.Errorf("missing volume create command")
	}
	if !strings.Contains(output, "-v skill-a:/skills/skill-a:ro") {
		t.Errorf("missing mount hint for skill-a in output:\n%s", output)
	}
}
```

Add `"bytes"` to the test imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/collection/ -run TestGeneratePodman`
Expected: FAIL — function not defined.

- [ ] **Step 3: Write the implementation**

Add to `pkg/collection/collection.go`:

```go
// GeneratePodmanVolumes writes Podman volume creation commands to w.
func GeneratePodmanVolumes(w io.Writer, col *SkillCollection, mountRoot string) {
	for _, s := range col.Skills {
		fmt.Fprintf(w, "podman pull %s\n", s.Image)
		fmt.Fprintf(w, "podman volume create --driver image \\\n  --opt image=%s \\\n  %s\n\n", s.Image, s.Name)
	}
	fmt.Fprintf(w, "# Run with:\n# podman run --rm \\\n")
	for _, s := range col.Skills {
		fmt.Fprintf(w, "#   -v %s:%s/%s:ro \\\n", s.Name, mountRoot, s.Name)
	}
	fmt.Fprintf(w, "#   my-agent:latest\n")
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/collection/ -run TestGeneratePodman`
Expected: PASS

- [ ] **Step 5: Add the CLI volume subcommand**

Add to `internal/cli/collection.go`:

```go
func newCollectionVolumeCmd() *cobra.Command {
	var file string
	var mountRoot string
	var execute bool
	cmd := &cobra.Command{
		Use:   "volume [-f <file> | <ref>]",
		Short: "Generate Podman volume commands from a collection",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			col, err := resolveCollection(file, args)
			if err != nil {
				return err
			}
			if execute {
				return runCollectionVolumeExecute(cmd, col, mountRoot)
			}
			collection.GeneratePodmanVolumes(cmd.OutOrStdout(), col, mountRoot)
			return nil
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "path to collection YAML file")
	cmd.Flags().StringVar(&mountRoot, "mount-root", "/skills", "root mount path for volumes")
	cmd.Flags().BoolVar(&execute, "execute", false, "run the commands instead of printing them")
	return cmd
}

func resolveCollection(file string, args []string) (*collection.SkillCollection, error) {
	if file != "" {
		return collection.ParseFile(file)
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("specify -f <file> or a registry reference")
	}
	// TODO: pull from registry and parse — will be wired up after collection pull works end-to-end
	return nil, fmt.Errorf("pulling collections from registry is not yet supported in volume command; use -f <file>")
}

func runCollectionVolumeExecute(cmd *cobra.Command, col *collection.SkillCollection, mountRoot string) error {
	// For now, just print — --execute will shell out in a future iteration
	fmt.Fprintf(cmd.OutOrStdout(), "# --execute is not yet implemented; printing commands instead:\n\n")
	collection.GeneratePodmanVolumes(cmd.OutOrStdout(), col, mountRoot)
	return nil
}
```

Add `"github.com/redhat-et/skillimage/pkg/collection"` to the imports.

- [ ] **Step 6: Register volume subcommand**

In `newCollectionCmd()`, add:

```go
	cmd.AddCommand(newCollectionVolumeCmd())
```

- [ ] **Step 7: Verify build and tests**

Run: `go build ./... && go test ./pkg/collection/...`
Expected: All pass.

- [ ] **Step 8: Commit**

```bash
git add pkg/collection/collection.go pkg/collection/collection_test.go internal/cli/collection.go
git commit -s -m "feat: add collection volume command for Podman"
```

---

### Task 6: Add collection generate command

**Files:**
- Modify: `pkg/collection/collection.go` — add GenerateKubeYAML helper
- Modify: `pkg/collection/collection_test.go` — add test
- Modify: `internal/cli/collection.go` — add generate subcommand

- [ ] **Step 1: Write the failing test**

Add to `pkg/collection/collection_test.go`:

```go
func TestGenerateKubeYAML(t *testing.T) {
	col := &collection.SkillCollection{
		Skills: []collection.SkillRef{
			{Name: "skill-a", Image: "quay.io/org/skill-a:1.0.0"},
			{Name: "skill-b", Image: "ghcr.io/org/skill-b:2.0.0"},
		},
	}
	var buf bytes.Buffer
	collection.GenerateKubeYAML(&buf, col, "/skills")
	output := buf.String()
	if !strings.Contains(output, "reference: quay.io/org/skill-a:1.0.0") {
		t.Errorf("missing image reference for skill-a")
	}
	if !strings.Contains(output, "mountPath: /skills/skill-a") {
		t.Errorf("missing mountPath for skill-a")
	}
	if !strings.Contains(output, "readOnly: true") {
		t.Errorf("missing readOnly")
	}
}

func TestGenerateKubeYAMLCustomMountRoot(t *testing.T) {
	col := &collection.SkillCollection{
		Skills: []collection.SkillRef{
			{Name: "skill-a", Image: "quay.io/org/skill-a:1.0.0"},
		},
	}
	var buf bytes.Buffer
	collection.GenerateKubeYAML(&buf, col, "/agent/skills")
	output := buf.String()
	if !strings.Contains(output, "mountPath: /agent/skills/skill-a") {
		t.Errorf("expected custom mount root, got:\n%s", output)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/collection/ -run TestGenerateKube`
Expected: FAIL — function not defined.

- [ ] **Step 3: Write the implementation**

Add to `pkg/collection/collection.go`:

```go
// GenerateKubeYAML writes Kubernetes partial pod spec (volumes + volumeMounts) to w.
func GenerateKubeYAML(w io.Writer, col *SkillCollection, mountRoot string) {
	fmt.Fprintln(w, "volumes:")
	for _, s := range col.Skills {
		fmt.Fprintf(w, "  - name: %s\n", s.Name)
		fmt.Fprintf(w, "    image:\n")
		fmt.Fprintf(w, "      reference: %s\n", s.Image)
		fmt.Fprintf(w, "      pullPolicy: IfNotPresent\n")
	}
	fmt.Fprintln(w, "containers:")
	fmt.Fprintln(w, "  - name: agent")
	fmt.Fprintln(w, "    volumeMounts:")
	for _, s := range col.Skills {
		fmt.Fprintf(w, "      - name: %s\n", s.Name)
		fmt.Fprintf(w, "        mountPath: %s/%s\n", mountRoot, s.Name)
		fmt.Fprintf(w, "        readOnly: true\n")
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/collection/ -run TestGenerateKube`
Expected: PASS

- [ ] **Step 5: Add the CLI generate subcommand**

Add to `internal/cli/collection.go`:

```go
func newCollectionGenerateCmd() *cobra.Command {
	var file string
	var mountRoot string
	cmd := &cobra.Command{
		Use:   "generate [-f <file> | <ref>]",
		Short: "Generate Kubernetes volume YAML from a collection",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			col, err := resolveCollection(file, args)
			if err != nil {
				return err
			}
			collection.GenerateKubeYAML(cmd.OutOrStdout(), col, mountRoot)
			return nil
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "path to collection YAML file")
	cmd.Flags().StringVar(&mountRoot, "mount-root", "/skills", "root mount path for volumes")
	return cmd
}
```

- [ ] **Step 6: Register generate subcommand**

In `newCollectionCmd()`, add:

```go
	cmd.AddCommand(newCollectionGenerateCmd())
```

- [ ] **Step 7: Verify build and all tests**

Run: `go build ./... && go test ./...`
Expected: All pass.

- [ ] **Step 8: Commit**

```bash
git add pkg/collection/ internal/cli/collection.go
git commit -s -m "feat: add collection generate command for Kubernetes"
```

---

### Task 7: Add collections table and store methods

**Files:**
- Modify: `internal/store/store.go` — add Collection type, schema, CRUD methods

- [ ] **Step 1: Write the failing test**

Add to `internal/store/store_test.go`:

```go
func TestUpsertAndListCollections(t *testing.T) {
	db, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer db.Close()

	col := store.Collection{
		Repository:  "quay.io/myorg/collections/hr-skills",
		Tag:         "1.0.0",
		Digest:      "sha256:abc123",
		Name:        "hr-skills",
		Version:     "1.0.0",
		Description: "HR skills collection",
		SkillsJSON:  `[{"name":"doc-summarizer","image":"quay.io/org/doc-summarizer:1.0.0"}]`,
		Created:     "2026-04-27T10:00:00Z",
	}

	if err := db.UpsertCollection(col); err != nil {
		t.Fatalf("UpsertCollection: %v", err)
	}

	collections, err := db.ListCollections()
	if err != nil {
		t.Fatalf("ListCollections: %v", err)
	}
	if len(collections) != 1 {
		t.Fatalf("expected 1 collection, got %d", len(collections))
	}
	if collections[0].Name != "hr-skills" {
		t.Errorf("name = %q, want %q", collections[0].Name, "hr-skills")
	}
}

func TestGetCollection(t *testing.T) {
	db, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer db.Close()

	col := store.Collection{
		Repository:  "quay.io/myorg/collections/hr-skills",
		Tag:         "1.0.0",
		Digest:      "sha256:abc123",
		Name:        "hr-skills",
		Version:     "1.0.0",
		Description: "HR skills",
		SkillsJSON:  `[{"name":"s1","image":"quay.io/org/s1:1.0.0"}]`,
		Created:     "2026-04-27T10:00:00Z",
	}
	if err := db.UpsertCollection(col); err != nil {
		t.Fatalf("UpsertCollection: %v", err)
	}

	got, err := db.GetCollection("hr-skills")
	if err != nil {
		t.Fatalf("GetCollection: %v", err)
	}
	if got.Version != "1.0.0" {
		t.Errorf("version = %q, want %q", got.Version, "1.0.0")
	}
}

func TestGetCollectionNotFound(t *testing.T) {
	db, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer db.Close()

	_, err = db.GetCollection("nonexistent")
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
```

Add `"errors"` to the test imports if not already present.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestUpsertAndListCollections`
Expected: FAIL — type and methods not defined.

- [ ] **Step 3: Write the implementation**

Add to `internal/store/store.go`:

```go
// Collection represents a skill collection indexed from the OCI registry.
type Collection struct {
	ID          int64  `json:"-"`
	Repository  string `json:"repository"`
	Tag         string `json:"tag"`
	Digest      string `json:"digest"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	SkillsJSON  string `json:"skills_json"`
	Created     string `json:"created"`
	SyncedAt    string `json:"synced_at"`
}
```

Add to `createSchema()`, after the skills table:

```go
		CREATE TABLE IF NOT EXISTS collections (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			repository  TEXT NOT NULL,
			tag         TEXT NOT NULL,
			digest      TEXT NOT NULL,
			name        TEXT NOT NULL,
			version     TEXT,
			description TEXT,
			skills_json TEXT NOT NULL,
			created     TEXT,
			synced_at   TEXT NOT NULL,
			UNIQUE(repository, tag)
		);
```

Add the CRUD methods:

```go
// UpsertCollection inserts or updates a collection.
func (s *Store) UpsertCollection(col Collection) error {
	col.SyncedAt = time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		INSERT INTO collections (repository, tag, digest, name, version,
			description, skills_json, created, synced_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(repository, tag) DO UPDATE SET
			digest=excluded.digest, name=excluded.name,
			version=excluded.version, description=excluded.description,
			skills_json=excluded.skills_json, created=excluded.created,
			synced_at=excluded.synced_at
	`, col.Repository, col.Tag, col.Digest, col.Name, col.Version,
		col.Description, col.SkillsJSON, col.Created, col.SyncedAt)
	return err
}

// ListCollections returns all collections.
func (s *Store) ListCollections() ([]Collection, error) {
	rows, err := s.db.Query(
		"SELECT id, repository, tag, digest, name, version, description, skills_json, created, synced_at FROM collections ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var collections []Collection
	for rows.Next() {
		var col Collection
		if err := rows.Scan(&col.ID, &col.Repository, &col.Tag, &col.Digest,
			&col.Name, &col.Version, &col.Description, &col.SkillsJSON,
			&col.Created, &col.SyncedAt); err != nil {
			return nil, err
		}
		collections = append(collections, col)
	}
	return collections, rows.Err()
}

// GetCollection returns a collection by name.
func (s *Store) GetCollection(name string) (*Collection, error) {
	var col Collection
	err := s.db.QueryRow(
		"SELECT id, repository, tag, digest, name, version, description, skills_json, created, synced_at FROM collections WHERE name = ?",
		name,
	).Scan(&col.ID, &col.Repository, &col.Tag, &col.Digest,
		&col.Name, &col.Version, &col.Description, &col.SkillsJSON,
		&col.Created, &col.SyncedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &col, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/store/...`
Expected: All pass.

- [ ] **Step 5: Commit**

```bash
git add internal/store/store.go internal/store/store_test.go
git commit -s -m "feat: add collections table and store methods"
```

---

### Task 8: Add collections API handler and routes

**Files:**
- Create: `internal/handler/collections.go`
- Create: `internal/handler/collections_test.go`
- Modify: `internal/server/router.go` — add collection routes

- [ ] **Step 1: Write the failing test**

Create `internal/handler/collections_test.go`:

```go
package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/redhat-et/skillimage/internal/handler"
	"github.com/redhat-et/skillimage/internal/store"
)

func TestCollectionsList(t *testing.T) {
	db, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	col := store.Collection{
		Repository:  "quay.io/org/collections/hr",
		Tag:         "1.0.0",
		Digest:      "sha256:abc",
		Name:        "hr-skills",
		Version:     "1.0.0",
		Description: "HR collection",
		SkillsJSON:  `[{"name":"s1","image":"quay.io/org/s1:1.0.0"}]`,
		Created:     "2026-04-27T10:00:00Z",
	}
	if err := db.UpsertCollection(col); err != nil {
		t.Fatal(err)
	}

	h := handler.NewCollectionsHandler(db)
	req := httptest.NewRequest("GET", "/api/v1/collections", nil)
	w := httptest.NewRecorder()
	h.List(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp struct {
		Data []store.Collection `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 collection, got %d", len(resp.Data))
	}
	if resp.Data[0].Name != "hr-skills" {
		t.Errorf("name = %q, want %q", resp.Data[0].Name, "hr-skills")
	}
}

func TestCollectionsGet(t *testing.T) {
	db, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	col := store.Collection{
		Repository:  "quay.io/org/collections/hr",
		Tag:         "1.0.0",
		Digest:      "sha256:abc",
		Name:        "hr-skills",
		Version:     "1.0.0",
		Description: "HR collection",
		SkillsJSON:  `[{"name":"s1","image":"quay.io/org/s1:1.0.0"}]`,
		Created:     "2026-04-27T10:00:00Z",
	}
	if err := db.UpsertCollection(col); err != nil {
		t.Fatal(err)
	}

	h := handler.NewCollectionsHandler(db)
	req := httptest.NewRequest("GET", "/api/v1/collections/hr-skills", nil)
	w := httptest.NewRecorder()
	h.Get(w, req, "hr-skills")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestCollectionsGetNotFound(t *testing.T) {
	db, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	h := handler.NewCollectionsHandler(db)
	req := httptest.NewRequest("GET", "/api/v1/collections/missing", nil)
	w := httptest.NewRecorder()
	h.Get(w, req, "missing")

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/handler/ -run TestCollections`
Expected: FAIL — type not defined.

- [ ] **Step 3: Write the handler implementation**

Create `internal/handler/collections.go`:

```go
package handler

import (
	"errors"
	"net/http"

	"github.com/redhat-et/skillimage/internal/store"
)

// CollectionsHandler provides HTTP handlers for skill collections.
type CollectionsHandler struct {
	store *store.Store
}

// NewCollectionsHandler creates a handler backed by the given store.
func NewCollectionsHandler(s *store.Store) *CollectionsHandler {
	return &CollectionsHandler{store: s}
}

// List handles GET /api/v1/collections.
func (h *CollectionsHandler) List(w http.ResponseWriter, r *http.Request) {
	collections, err := h.store.ListCollections()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "listing collections", err)
		return
	}
	if collections == nil {
		collections = []store.Collection{}
	}
	writeJSON(w, http.StatusOK, envelope{Data: collections})
}

// Get handles GET /api/v1/collections/{name}.
func (h *CollectionsHandler) Get(w http.ResponseWriter, r *http.Request, name string) {
	col, err := h.store.GetCollection(name)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "collection not found", err)
			return
		}
		writeError(w, http.StatusInternalServerError, "getting collection", err)
		return
	}
	writeJSON(w, http.StatusOK, col)
}
```

- [ ] **Step 4: Add routes to router.go**

In `internal/server/router.go`, add the collections handler and routes:

After the `skills` handler initialization:

```go
	collections := handler.NewCollectionsHandler(db)
```

Inside the `r.Route("/api/v1", ...)` block, after the sync route:

```go
		r.Get("/collections", collections.List)
		r.Get("/collections/{name}", func(w http.ResponseWriter, r *http.Request) {
			collections.Get(w, r, chi.URLParam(r, "name"))
		})
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/handler/ -run TestCollections`
Expected: All pass.

Run: `go build ./...`
Expected: Compiles cleanly.

- [ ] **Step 6: Commit**

```bash
git add internal/handler/collections.go internal/handler/collections_test.go internal/server/router.go
git commit -s -m "feat: add collections API endpoints"
```

---

### Task 9: Run full test suite and lint

**Files:** None (verification only)

- [ ] **Step 1: Run all tests**

Run: `go test ./...`
Expected: All pass.

- [ ] **Step 2: Run linter**

Run: `make lint`
Expected: Zero issues.

- [ ] **Step 3: Verify CLI help**

Run: `go build -o bin/skillctl ./cmd/skillctl && bin/skillctl collection --help`

Expected output should show subcommands: push, pull, volume, generate.

Run: `bin/skillctl build --help`

Expected: No `--bundle` flag.

- [ ] **Step 4: Commit any lint fixes if needed**

```bash
git add -A
git commit -s -m "fix: address lint findings"
```

---

### Task 10: Create PR

- [ ] **Step 1: Push branch and create draft PR**

```bash
git push -u origin <branch-name>
gh pr create --draft --title "feat: add skill collections, remove bundles" --body "$(cat <<'EOF'
## Summary

- Add `skillctl collection` command with push, pull, volume, generate subcommands
- Add `pkg/collection/` package for YAML parsing and validation
- Add OCI artifact support for collection storage
- Add collections table and API endpoints to catalog server
- Remove all bundle code (replaced by collections)

Design spec: `docs/superpowers/specs/2026-04-27-skill-collections-design.md`

## Test plan

- [ ] `go test ./...` passes
- [ ] `make lint` zero issues
- [ ] `skillctl collection volume -f collection.yaml` generates valid Podman commands
- [ ] `skillctl collection generate -f collection.yaml` generates valid K8s YAML
- [ ] `skillctl build --help` shows no --bundle flag
- [ ] Collection API endpoints return correct responses

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```
