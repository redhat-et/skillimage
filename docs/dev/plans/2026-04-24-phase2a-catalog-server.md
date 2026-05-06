# Phase 2a: Catalog Server and OCI Bundles — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task.
> Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a read-only catalog server that indexes skills from
an OCI registry into SQLite and serves them via REST API, plus a
CLI command for packing multi-skill OCI bundle images.

**Architecture:** The server is a new `skillctl serve` command that
starts an HTTP server (chi router), runs a background sync goroutine
to walk the OCI registry catalog API and index manifest annotations
into SQLite, and serves a REST API for listing, filtering, and
content retrieval. Bundle support adds a `--bundle` flag to the
existing `skillctl pack` command. Both features reuse existing
`pkg/oci/` and `pkg/lifecycle/` code.

**Tech Stack:** Go 1.26+, chi (HTTP router), modernc.org/sqlite
(pure-Go SQLite, no CGO), oras-go (OCI operations), Viper (config)

---

## File map

| Path | Responsibility | Task |
|---|---|---|
| `internal/store/store.go` | SQLite schema, open/close, skill CRUD queries | 1 |
| `internal/store/store_test.go` | Store unit tests | 1 |
| `internal/store/sync.go` | Sync engine: registry walk, index update, stale cleanup | 2 |
| `internal/store/sync_test.go` | Sync engine tests | 2 |
| `pkg/oci/catalog.go` | Registry catalog API: list repos, list tags, fetch manifest annotations | 2 |
| `pkg/oci/catalog_test.go` | Catalog API tests | 2 |
| `internal/handler/skills.go` | HTTP handlers: list, get, versions, content | 3 |
| `internal/handler/skills_test.go` | Handler tests | 3 |
| `internal/handler/sync.go` | HTTP handler: POST /sync trigger | 4 |
| `internal/handler/health.go` | HTTP handler: GET /healthz | 4 |
| `internal/server/server.go` | HTTP server setup, config, graceful shutdown | 4 |
| `internal/server/router.go` | chi router, middleware (request ID, JSON content type) | 4 |
| `internal/cli/serve.go` | `skillctl serve` command | 5 |
| `pkg/oci/bundle.go` | Bundle pack logic: walk subdirs, validate, build multi-skill image | 6 |
| `pkg/oci/bundle_test.go` | Bundle pack tests | 6 |
| `internal/cli/pack.go` | Modified: add `--bundle` flag | 7 |

---

### Task 1: SQLite store

**Files:**
- Create: `internal/store/store.go`
- Create: `internal/store/store_test.go`

New dependency: `modernc.org/sqlite` (pure-Go, no CGO — required
for cross-compilation and scratch containers).

- [ ] **Step 1: Add the sqlite dependency**

```bash
go get modernc.org/sqlite
go get github.com/jmoiron/sqlx
```

- [ ] **Step 2: Write the failing test for schema creation**

Create `internal/store/store_test.go`:

```go
package store_test

import (
	"testing"

	"github.com/redhat-et/skillimage/internal/store"
)

func TestNewStore(t *testing.T) {
	db, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer db.Close()

	// Schema should be created — verify the skills table exists.
	skills, err := db.ListSkills(store.ListFilter{})
	if err != nil {
		t.Fatalf("ListSkills on empty db: %v", err)
	}
	if len(skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(skills))
	}
}
```

Run: `go test ./internal/store/ -run TestNewStore -v`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Implement store with schema creation**

Create `internal/store/store.go`:

```go
package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type Skill struct {
	ID            int64  `json:"id"`
	Repository    string `json:"repository"`
	Tag           string `json:"tag"`
	Digest        string `json:"digest"`
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	Version       string `json:"version"`
	Status        string `json:"status"`
	DisplayName   string `json:"display_name"`
	Description   string `json:"description"`
	Authors       string `json:"authors"`
	License       string `json:"license"`
	TagsJSON      string `json:"tags_json"`
	Compatibility string `json:"compatibility"`
	WordCount     int    `json:"word_count"`
	Created       string `json:"created"`
	Bundle        bool   `json:"bundle"`
	BundleSkills  string `json:"bundle_skills"`
	SyncedAt      string `json:"synced_at"`
}

type ListFilter struct {
	Query         string
	Tags          []string
	Status        string
	Namespace     string
	Compatibility string
	Page          int
	PerPage       int
}

func New(dsn string) (*Store, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("pinging database: %w", err)
	}
	s := &Store{db: db}
	if err := s.createSchema(); err != nil {
		return nil, fmt.Errorf("creating schema: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) createSchema() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS skills (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			repository    TEXT NOT NULL,
			tag           TEXT NOT NULL,
			digest        TEXT NOT NULL,
			name          TEXT,
			namespace     TEXT,
			version       TEXT,
			status        TEXT,
			display_name  TEXT,
			description   TEXT,
			authors       TEXT,
			license       TEXT,
			tags_json     TEXT,
			compatibility TEXT,
			word_count    INTEGER DEFAULT 0,
			created       TEXT,
			bundle        INTEGER DEFAULT 0,
			bundle_skills TEXT,
			synced_at     TEXT NOT NULL,
			UNIQUE(repository, tag)
		);
		CREATE INDEX IF NOT EXISTS idx_skills_namespace ON skills(namespace);
		CREATE INDEX IF NOT EXISTS idx_skills_status ON skills(status);
		CREATE INDEX IF NOT EXISTS idx_skills_name ON skills(name);
	`)
	return err
}
```

- [ ] **Step 4: Run the test**

Run: `go test ./internal/store/ -run TestNewStore -v`
Expected: PASS

- [ ] **Step 5: Write failing test for UpsertSkill and ListSkills**

Add to `internal/store/store_test.go`:

```go
func TestUpsertAndList(t *testing.T) {
	db, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer db.Close()

	skill := store.Skill{
		Repository:  "team1/doc-reviewer",
		Tag:         "1.0.0-draft",
		Digest:      "sha256:abc123",
		Name:        "doc-reviewer",
		Namespace:   "team1",
		Version:     "1.0.0",
		Status:      "draft",
		DisplayName: "Document Reviewer",
		Description: "Reviews docs",
		TagsJSON:    `["review","docs"]`,
		WordCount:   100,
	}
	if err := db.UpsertSkill(skill); err != nil {
		t.Fatalf("UpsertSkill: %v", err)
	}

	skills, err := db.ListSkills(store.ListFilter{})
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	if skills[0].Name != "doc-reviewer" {
		t.Errorf("name = %q, want %q", skills[0].Name, "doc-reviewer")
	}
}

func TestListSkillsFilters(t *testing.T) {
	db, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer db.Close()

	skills := []store.Skill{
		{Repository: "team1/skill-a", Tag: "1.0.0", Digest: "sha256:aaa", Name: "skill-a", Namespace: "team1", Status: "published", Description: "Kubernetes debugging", TagsJSON: `["kubernetes"]`},
		{Repository: "team1/skill-b", Tag: "2.0.0-draft", Digest: "sha256:bbb", Name: "skill-b", Namespace: "team1", Status: "draft", Description: "Python linting", TagsJSON: `["python"]`},
		{Repository: "team2/skill-c", Tag: "1.0.0", Digest: "sha256:ccc", Name: "skill-c", Namespace: "team2", Status: "published", Description: "Go testing", TagsJSON: `["go","testing"]`},
	}
	for _, sk := range skills {
		if err := db.UpsertSkill(sk); err != nil {
			t.Fatalf("UpsertSkill %s: %v", sk.Name, err)
		}
	}

	tests := []struct {
		name   string
		filter store.ListFilter
		want   int
	}{
		{"no filter", store.ListFilter{}, 3},
		{"by status", store.ListFilter{Status: "published"}, 2},
		{"by namespace", store.ListFilter{Namespace: "team2"}, 1},
		{"by query", store.ListFilter{Query: "kubernetes"}, 1},
		{"by tags", store.ListFilter{Tags: []string{"go"}}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := db.ListSkills(tt.filter)
			if err != nil {
				t.Fatalf("ListSkills: %v", err)
			}
			if len(result) != tt.want {
				t.Errorf("got %d skills, want %d", len(result), tt.want)
			}
		})
	}
}
```

Run: `go test ./internal/store/ -run TestUpsert -v`
Expected: FAIL — `UpsertSkill` not defined.

- [ ] **Step 6: Implement UpsertSkill, ListSkills, GetSkill, DeleteStale**

Add to `internal/store/store.go`:

```go
func (s *Store) UpsertSkill(sk Skill) error {
	sk.SyncedAt = time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		INSERT INTO skills (repository, tag, digest, name, namespace, version,
			status, display_name, description, authors, license, tags_json,
			compatibility, word_count, created, bundle, bundle_skills, synced_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(repository, tag) DO UPDATE SET
			digest=excluded.digest, name=excluded.name, namespace=excluded.namespace,
			version=excluded.version, status=excluded.status,
			display_name=excluded.display_name, description=excluded.description,
			authors=excluded.authors, license=excluded.license,
			tags_json=excluded.tags_json, compatibility=excluded.compatibility,
			word_count=excluded.word_count, created=excluded.created,
			bundle=excluded.bundle, bundle_skills=excluded.bundle_skills,
			synced_at=excluded.synced_at
	`, sk.Repository, sk.Tag, sk.Digest, sk.Name, sk.Namespace, sk.Version,
		sk.Status, sk.DisplayName, sk.Description, sk.Authors, sk.License,
		sk.TagsJSON, sk.Compatibility, sk.WordCount, sk.Created,
		sk.Bundle, sk.BundleSkills, sk.SyncedAt)
	return err
}

func (s *Store) ListSkills(f ListFilter) ([]Skill, error) {
	query := "SELECT * FROM skills WHERE 1=1"
	var args []any

	if f.Status != "" {
		query += " AND status = ?"
		args = append(args, f.Status)
	}
	if f.Namespace != "" {
		query += " AND namespace = ?"
		args = append(args, f.Namespace)
	}
	if f.Compatibility != "" {
		query += " AND compatibility = ?"
		args = append(args, f.Compatibility)
	}
	if f.Query != "" {
		query += " AND (name LIKE ? OR display_name LIKE ? OR description LIKE ?)"
		q := "%" + f.Query + "%"
		args = append(args, q, q, q)
	}
	if len(f.Tags) > 0 {
		for _, tag := range f.Tags {
			query += " AND tags_json LIKE ?"
			args = append(args, "%"+tag+"%")
		}
	}

	query += " ORDER BY namespace, name, version"

	if f.PerPage > 0 {
		offset := 0
		if f.Page > 1 {
			offset = (f.Page - 1) * f.PerPage
		}
		query += fmt.Sprintf(" LIMIT %d OFFSET %d", f.PerPage, offset)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var skills []Skill
	for rows.Next() {
		var sk Skill
		if err := rows.Scan(&sk.ID, &sk.Repository, &sk.Tag, &sk.Digest,
			&sk.Name, &sk.Namespace, &sk.Version, &sk.Status,
			&sk.DisplayName, &sk.Description, &sk.Authors, &sk.License,
			&sk.TagsJSON, &sk.Compatibility, &sk.WordCount, &sk.Created,
			&sk.Bundle, &sk.BundleSkills, &sk.SyncedAt); err != nil {
			return nil, err
		}
		skills = append(skills, sk)
	}
	return skills, rows.Err()
}

func (s *Store) GetSkill(namespace, name string) (*Skill, error) {
	row := s.db.QueryRow(`
		SELECT * FROM skills WHERE namespace = ? AND name = ?
		ORDER BY created DESC LIMIT 1
	`, namespace, name)

	var sk Skill
	err := row.Scan(&sk.ID, &sk.Repository, &sk.Tag, &sk.Digest,
		&sk.Name, &sk.Namespace, &sk.Version, &sk.Status,
		&sk.DisplayName, &sk.Description, &sk.Authors, &sk.License,
		&sk.TagsJSON, &sk.Compatibility, &sk.WordCount, &sk.Created,
		&sk.Bundle, &sk.BundleSkills, &sk.SyncedAt)
	if err != nil {
		return nil, err
	}
	return &sk, nil
}

func (s *Store) GetVersions(namespace, name string) ([]Skill, error) {
	rows, err := s.db.Query(`
		SELECT * FROM skills WHERE namespace = ? AND name = ?
		ORDER BY created DESC
	`, namespace, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var skills []Skill
	for rows.Next() {
		var sk Skill
		if err := rows.Scan(&sk.ID, &sk.Repository, &sk.Tag, &sk.Digest,
			&sk.Name, &sk.Namespace, &sk.Version, &sk.Status,
			&sk.DisplayName, &sk.Description, &sk.Authors, &sk.License,
			&sk.TagsJSON, &sk.Compatibility, &sk.WordCount, &sk.Created,
			&sk.Bundle, &sk.BundleSkills, &sk.SyncedAt); err != nil {
			return nil, err
		}
		skills = append(skills, sk)
	}
	return skills, rows.Err()
}

func (s *Store) CountSkills(f ListFilter) (int, error) {
	query := "SELECT COUNT(*) FROM skills WHERE 1=1"
	var args []any

	if f.Status != "" {
		query += " AND status = ?"
		args = append(args, f.Status)
	}
	if f.Namespace != "" {
		query += " AND namespace = ?"
		args = append(args, f.Namespace)
	}
	if f.Compatibility != "" {
		query += " AND compatibility = ?"
		args = append(args, f.Compatibility)
	}
	if f.Query != "" {
		query += " AND (name LIKE ? OR display_name LIKE ? OR description LIKE ?)"
		q := "%" + f.Query + "%"
		args = append(args, q, q, q)
	}
	if len(f.Tags) > 0 {
		for _, tag := range f.Tags {
			query += " AND tags_json LIKE ?"
			args = append(args, "%"+tag+"%")
		}
	}

	var count int
	err := s.db.QueryRow(query, args...).Scan(&count)
	return count, err
}

func (s *Store) DeleteStale(before time.Time) (int64, error) {
	result, err := s.db.Exec("DELETE FROM skills WHERE synced_at < ?",
		before.Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
```

- [ ] **Step 7: Run all store tests**

Run: `go test ./internal/store/ -v`
Expected: all PASS

- [ ] **Step 8: Commit**

```bash
git add internal/store/ go.mod go.sum
git commit -s -m "feat: add SQLite store for skill catalog index"
```

---

### Task 2: Registry catalog walker and sync engine

**Files:**
- Create: `pkg/oci/catalog.go`
- Create: `pkg/oci/catalog_test.go`
- Create: `internal/store/sync.go`
- Create: `internal/store/sync_test.go`

- [ ] **Step 1: Write failing test for catalog repository listing**

Create `pkg/oci/catalog_test.go`:

```go
package oci_test

import (
	"context"
	"testing"

	"github.com/redhat-et/skillimage/pkg/oci"
)

func TestListRepositories(t *testing.T) {
	// Pack a skill into a local store, then list repos.
	skillDir := t.TempDir()
	writeTestSkill(t, skillDir)

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

	repos, err := client.ListRepositories()
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if len(repos) == 0 {
		t.Fatal("expected at least 1 repository")
	}
}
```

Run: `go test ./pkg/oci/ -run TestListRepositories -v`
Expected: FAIL — `ListRepositories` not defined.

- [ ] **Step 2: Implement ListRepositories and ListTags**

Create `pkg/oci/catalog.go`:

```go
package oci

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/registry/remote"
)

// SkillManifest holds the metadata extracted from a manifest's annotations.
type SkillManifest struct {
	Repository  string
	Tag         string
	Digest      string
	Annotations map[string]string
}

// ListRepositories returns all repository names (tags) in the local store.
func (c *Client) ListRepositories() ([]string, error) {
	tags, err := c.store.Tags(context.Background())
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	var repos []string
	for _, tag := range tags {
		repo, _ := splitRefTag(tag)
		if !seen[repo] {
			seen[repo] = true
			repos = append(repos, repo)
		}
	}
	return repos, nil
}

// ListRemoteRepositories lists repository names from a remote registry.
// If prefix is non-empty, only repos starting with prefix are returned.
func ListRemoteRepositories(ctx context.Context, registryURL string, prefix string, skipTLSVerify bool) ([]string, error) {
	reg, err := remote.NewRegistry(registryURL)
	if err != nil {
		return nil, fmt.Errorf("creating registry client: %w", err)
	}
	if skipTLSVerify {
		reg.Client = insecureHTTPClient()
	}

	store, err := credentialStore()
	if err == nil {
		reg.Client = authClient(store, skipTLSVerify)
	}

	var repos []string
	err = reg.Repositories(ctx, "", func(repoNames []string) error {
		for _, name := range repoNames {
			if prefix == "" || strings.HasPrefix(name, prefix) {
				repos = append(repos, name)
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing repositories: %w", err)
	}
	return repos, nil
}

// ListRemoteTags lists all tags for a repository on a remote registry.
func ListRemoteTags(ctx context.Context, registryURL, repoName string, skipTLSVerify bool) ([]string, error) {
	ref := registryURL + "/" + repoName
	repo, err := newRemoteRepository(ref, skipTLSVerify)
	if err != nil {
		return nil, err
	}

	var tags []string
	err = repo.Tags(ctx, "", func(tagList []string) error {
		tags = append(tags, tagList...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing tags for %s: %w", repoName, err)
	}
	return tags, nil
}

// FetchManifestAnnotations fetches a manifest from a remote registry
// and returns its annotations and digest. Returns nil annotations
// if the manifest has no io.skillimage.status annotation (not a skill).
func FetchManifestAnnotations(ctx context.Context, registryURL, repoName, tag string, skipTLSVerify bool) (*SkillManifest, error) {
	ref := fmt.Sprintf("%s/%s:%s", registryURL, repoName, tag)
	repo, err := newRemoteRepository(ref, skipTLSVerify)
	if err != nil {
		return nil, err
	}

	desc, err := repo.Resolve(ctx, tag)
	if err != nil {
		return nil, fmt.Errorf("resolving %s:%s: %w", repoName, tag, err)
	}

	rc, err := repo.Fetch(ctx, desc)
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

	if manifest.Annotations == nil {
		return nil, nil
	}
	if _, ok := manifest.Annotations["io.skillimage.status"]; !ok {
		return nil, nil
	}

	return &SkillManifest{
		Repository:  repoName,
		Tag:         tag,
		Digest:      desc.Digest.String(),
		Annotations: manifest.Annotations,
	}, nil
}
```

Note: `insecureHTTPClient()` and `authClient()` are helper functions
that extract the TLS and auth setup from the existing
`newRemoteRepository`. You will need to refactor `newRemoteRepository`
in `pkg/oci/push.go` to expose these helpers. Extract the
`http.Client` creation and `auth.Client` creation into package-level
functions that both `newRemoteRepository` and the catalog functions
can use.

- [ ] **Step 3: Run catalog test**

Run: `go test ./pkg/oci/ -run TestListRepositories -v`
Expected: PASS

- [ ] **Step 4: Write the sync engine**

Create `internal/store/sync.go`:

```go
package store

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/redhat-et/skillimage/pkg/lifecycle"
	"github.com/redhat-et/skillimage/pkg/oci"
)

type SyncConfig struct {
	RegistryURL   string
	Namespace     string
	SkipTLSVerify bool
}

// Sync performs a full sync from the registry into the store.
// It lists all repositories, fetches manifests, filters for skill
// images, and upserts into the database.
func (s *Store) Sync(ctx context.Context, cfg SyncConfig) error {
	syncStart := time.Now()

	repos, err := oci.ListRemoteRepositories(ctx, cfg.RegistryURL, cfg.Namespace, cfg.SkipTLSVerify)
	if err != nil {
		return err
	}

	for _, repo := range repos {
		tags, err := oci.ListRemoteTags(ctx, cfg.RegistryURL, repo, cfg.SkipTLSVerify)
		if err != nil {
			slog.Warn("listing tags failed, skipping repo", "repo", repo, "error", err)
			continue
		}

		for _, tag := range tags {
			sm, err := oci.FetchManifestAnnotations(ctx, cfg.RegistryURL, repo, tag, cfg.SkipTLSVerify)
			if err != nil {
				slog.Warn("fetching manifest failed, skipping", "repo", repo, "tag", tag, "error", err)
				continue
			}
			if sm == nil {
				continue
			}

			sk := manifestToSkill(sm)
			if err := s.UpsertSkill(sk); err != nil {
				slog.Warn("upserting skill failed", "repo", repo, "tag", tag, "error", err)
			}
		}
	}

	deleted, err := s.DeleteStale(syncStart)
	if err != nil {
		slog.Warn("stale cleanup failed", "error", err)
	} else if deleted > 0 {
		slog.Info("cleaned up stale entries", "count", deleted)
	}

	return nil
}

func manifestToSkill(sm *oci.SkillManifest) Skill {
	ann := sm.Annotations
	wc, _ := strconv.Atoi(ann[oci.AnnotationWordCount])

	sk := Skill{
		Repository:    sm.Repository,
		Tag:           sm.Tag,
		Digest:        sm.Digest,
		Name:          parseName(ann, sm.Repository),
		Namespace:     ann[ocispec.AnnotationVendor],
		Version:       ann[ocispec.AnnotationVersion],
		Status:        ann[lifecycle.StatusAnnotation],
		DisplayName:   ann[ocispec.AnnotationTitle],
		Description:   ann[ocispec.AnnotationDescription],
		Authors:       ann[ocispec.AnnotationAuthors],
		License:       ann[ocispec.AnnotationLicenses],
		TagsJSON:      ann[oci.AnnotationTags],
		Compatibility: ann[oci.AnnotationCompatibility],
		WordCount:     wc,
		Created:       ann[ocispec.AnnotationCreated],
	}

	if ann[oci.AnnotationBundle] == "true" {
		sk.Bundle = true
		sk.BundleSkills = ann[oci.AnnotationBundleSkills]
	}

	return sk
}

func parseName(ann map[string]string, repo string) string {
	if title := ann[ocispec.AnnotationTitle]; title != "" {
		return title
	}
	// Fall back to last segment of repository path.
	if idx := len(repo) - 1; idx >= 0 {
		for i := idx; i >= 0; i-- {
			if repo[i] == '/' {
				return repo[i+1:]
			}
		}
	}
	return repo
}
```

Note: `oci.AnnotationBundle` and `oci.AnnotationBundleSkills` are
new constants you will add in Task 6 (bundle support). For now,
define them early in `pkg/oci/annotations.go`:

```go
AnnotationBundle       = "io.skillimage.bundle"
AnnotationBundleSkills = "io.skillimage.bundle.skills"
```

- [ ] **Step 5: Run all tests**

Run: `go test ./internal/store/ ./pkg/oci/ -v`
Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add pkg/oci/catalog.go pkg/oci/annotations.go internal/store/sync.go
git commit -s -m "feat: add registry catalog walker and sync engine"
```

---

### Task 3: HTTP handlers for skill listing and content

**Files:**
- Create: `internal/handler/skills.go`
- Create: `internal/handler/skills_test.go`

- [ ] **Step 1: Write failing test for list handler**

Create `internal/handler/skills_test.go`:

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

func setupStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestListSkillsHandler(t *testing.T) {
	db := setupStore(t)
	_ = db.UpsertSkill(store.Skill{
		Repository: "team1/doc-reviewer", Tag: "1.0.0",
		Digest: "sha256:abc", Name: "doc-reviewer",
		Namespace: "team1", Status: "published",
		Version: "1.0.0", Description: "Reviews docs",
	})

	h := handler.NewSkillsHandler(db)
	req := httptest.NewRequest("GET", "/api/v1/skills", nil)
	w := httptest.NewRecorder()

	h.List(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Errorf("got %d skills, want 1", len(resp.Data))
	}
}
```

Run: `go test ./internal/handler/ -run TestListSkillsHandler -v`
Expected: FAIL — package does not exist.

- [ ] **Step 2: Implement the skills handler**

Create `internal/handler/skills.go`:

```go
package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/redhat-et/skillimage/internal/store"
)

type SkillsHandler struct {
	store *store.Store
}

func NewSkillsHandler(s *store.Store) *SkillsHandler {
	return &SkillsHandler{store: s}
}

type envelope struct {
	Data       any            `json:"data"`
	Meta       map[string]any `json:"_meta,omitempty"`
	Pagination *pagination    `json:"pagination,omitempty"`
}

type pagination struct {
	Total   int `json:"total"`
	Page    int `json:"page"`
	PerPage int `json:"per_page"`
}

func (h *SkillsHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(q.Get("per_page"))
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	var tags []string
	if t := q.Get("tags"); t != "" {
		tags = strings.Split(t, ",")
	}

	filter := store.ListFilter{
		Query:         q.Get("q"),
		Tags:          tags,
		Status:        q.Get("status"),
		Namespace:     q.Get("namespace"),
		Compatibility: q.Get("compatibility"),
		Page:          page,
		PerPage:       perPage,
	}

	skills, err := h.store.ListSkills(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "listing skills", err)
		return
	}

	total, err := h.store.CountSkills(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "counting skills", err)
		return
	}

	writeJSON(w, http.StatusOK, envelope{
		Data: skills,
		Pagination: &pagination{
			Total:   total,
			Page:    page,
			PerPage: perPage,
		},
	})
}

func (h *SkillsHandler) Get(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("ns")
	name := r.PathValue("name")

	skill, err := h.store.GetSkill(ns, name)
	if err != nil {
		writeError(w, http.StatusNotFound, "skill not found", err)
		return
	}
	writeJSON(w, http.StatusOK, envelope{Data: skill})
}

func (h *SkillsHandler) Versions(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("ns")
	name := r.PathValue("name")

	versions, err := h.store.GetVersions(ns, name)
	if err != nil {
		writeError(w, http.StatusNotFound, "skill not found", err)
		return
	}
	writeJSON(w, http.StatusOK, envelope{Data: versions})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, title string, err error) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":   "about:blank",
		"title":  title,
		"status": status,
		"detail": err.Error(),
	})
}
```

Note: This uses Go 1.22+ `r.PathValue()` for path parameters, which
works with `net/http.ServeMux` or chi's `{param}` patterns. Since the
project uses chi, you will wire these in Task 4 using chi's URL
parameters via `chi.URLParam(r, "ns")` instead of `r.PathValue()`.
Update `Get` and `Versions` methods accordingly when wiring the router.

- [ ] **Step 3: Run handler tests**

Run: `go test ./internal/handler/ -run TestListSkillsHandler -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/handler/
git commit -s -m "feat: add HTTP handlers for skill listing and detail"
```

---

### Task 4: HTTP server, router, and remaining handlers

**Files:**
- Create: `internal/server/server.go`
- Create: `internal/server/router.go`
- Create: `internal/handler/sync.go`
- Create: `internal/handler/health.go`

- [ ] **Step 1: Add chi dependency**

```bash
go get github.com/go-chi/chi/v5
```

- [ ] **Step 2: Create the router**

Create `internal/server/router.go`:

```go
package server

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/redhat-et/skillimage/internal/handler"
	"github.com/redhat-et/skillimage/internal/store"
)

func NewRouter(db *store.Store, syncFn func()) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.SetHeader("Content-Type", "application/json"))

	skills := handler.NewSkillsHandler(db)
	syncH := handler.NewSyncHandler(syncFn)
	healthH := handler.NewHealthHandler()

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/skills", skills.List)
		r.Get("/skills/{ns}/{name}", skills.Get)
		r.Get("/skills/{ns}/{name}/versions", skills.Versions)
		r.Get("/skills/{ns}/{name}/versions/{ver}/content", skills.Content)
		r.Post("/sync", syncH.Trigger)
	})

	r.Get("/healthz", healthH.Check)

	return r
}
```

Update `internal/handler/skills.go` to use `chi.URLParam`:

Add `"github.com/go-chi/chi/v5"` to imports and change path
parameter access:

```go
func (h *SkillsHandler) Get(w http.ResponseWriter, r *http.Request) {
	ns := chi.URLParam(r, "ns")
	name := chi.URLParam(r, "name")
	// ... rest unchanged
}

func (h *SkillsHandler) Versions(w http.ResponseWriter, r *http.Request) {
	ns := chi.URLParam(r, "ns")
	name := chi.URLParam(r, "name")
	// ... rest unchanged
}
```

- [ ] **Step 3: Create sync and health handlers**

Create `internal/handler/sync.go`:

```go
package handler

import "net/http"

type SyncHandler struct {
	triggerSync func()
}

func NewSyncHandler(triggerSync func()) *SyncHandler {
	return &SyncHandler{triggerSync: triggerSync}
}

func (h *SyncHandler) Trigger(w http.ResponseWriter, r *http.Request) {
	go h.triggerSync()
	writeJSON(w, http.StatusAccepted, envelope{
		Data: map[string]string{"message": "sync triggered"},
	})
}
```

Create `internal/handler/health.go`:

```go
package handler

import "net/http"

type HealthHandler struct{}

func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

func (h *HealthHandler) Check(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
```

- [ ] **Step 4: Create the server with graceful shutdown**

Create `internal/server/server.go`:

```go
package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/redhat-et/skillimage/internal/store"
)

type Config struct {
	Port          int
	DBPath        string
	RegistryURL   string
	Namespace     string
	SkipTLSVerify bool
	SyncInterval  time.Duration
}

func Run(ctx context.Context, cfg Config) error {
	db, err := store.New(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer db.Close()

	syncCfg := store.SyncConfig{
		RegistryURL:   cfg.RegistryURL,
		Namespace:     cfg.Namespace,
		SkipTLSVerify: cfg.SkipTLSVerify,
	}

	slog.Info("running initial sync", "registry", cfg.RegistryURL)
	if err := db.Sync(ctx, syncCfg); err != nil {
		slog.Error("initial sync failed", "error", err)
	}

	triggerSync := func() {
		slog.Info("sync triggered")
		if err := db.Sync(context.Background(), syncCfg); err != nil {
			slog.Error("sync failed", "error", err)
		}
		slog.Info("sync complete")
	}

	go func() {
		ticker := time.NewTicker(cfg.SyncInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				triggerSync()
			}
		}
	}()

	router := NewRouter(db, triggerSync)
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: router,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	slog.Info("server listening", "port", cfg.Port)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}
```

- [ ] **Step 5: Verify it compiles**

Run: `go vet ./internal/server/ ./internal/handler/`
Expected: no errors

- [ ] **Step 6: Commit**

```bash
git add internal/server/ internal/handler/ go.mod go.sum
git commit -s -m "feat: add HTTP server with chi router and sync/health handlers"
```

---

### Task 5: `skillctl serve` command

**Files:**
- Create: `internal/cli/serve.go`
- Modify: `internal/cli/root.go`

- [ ] **Step 1: Create the serve command**

Create `internal/cli/serve.go`:

```go
package cli

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/redhat-et/skillimage/internal/server"
)

func newServeCmd() *cobra.Command {
	var (
		port         int
		dbPath       string
		registryURL  string
		namespace    string
		syncInterval string
		tlsVerify    bool
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the skill catalog server",
		Long: `Start an HTTP server that indexes skills from an OCI registry
and serves them via a REST API.

The server syncs skill metadata from the configured registry into
a local SQLite database and serves it for fast listing, filtering,
and search.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			interval, err := time.ParseDuration(syncInterval)
			if err != nil {
				return fmt.Errorf("invalid sync interval: %w", err)
			}

			if registryURL == "" {
				return fmt.Errorf("--registry is required")
			}

			ctx, cancel := signal.NotifyContext(
				context.Background(), syscall.SIGINT, syscall.SIGTERM,
			)
			defer cancel()

			return server.Run(ctx, server.Config{
				Port:          port,
				DBPath:        dbPath,
				RegistryURL:   registryURL,
				Namespace:     namespace,
				SkipTLSVerify: !tlsVerify,
				SyncInterval:  interval,
			})
		},
	}

	cmd.Flags().IntVar(&port, "port", 8080, "HTTP listen port")
	cmd.Flags().StringVar(&dbPath, "db", "skillctl.db", "SQLite database path")
	cmd.Flags().StringVar(&registryURL, "registry", "", "OCI registry URL (required)")
	cmd.Flags().StringVar(&namespace, "namespace", "", "limit sync to a namespace prefix")
	cmd.Flags().StringVar(&syncInterval, "sync-interval", "60s", "background sync interval")
	cmd.Flags().BoolVar(&tlsVerify, "tls-verify", true, "require HTTPS and verify certificates")

	return cmd
}
```

- [ ] **Step 2: Register the command in root.go**

Add to `internal/cli/root.go` in the `NewRootCmd` function:

```go
cmd.AddCommand(newServeCmd())
```

- [ ] **Step 3: Verify it compiles and shows help**

Run: `go build -o bin/skillctl ./cmd/skillctl && bin/skillctl serve --help`
Expected: prints usage with `--registry`, `--port`, etc.

- [ ] **Step 4: Commit**

```bash
git add internal/cli/serve.go internal/cli/root.go
git commit -s -m "feat: add skillctl serve command"
```

---

### Task 6: Bundle pack support

**Files:**
- Create: `pkg/oci/bundle.go`
- Create: `pkg/oci/bundle_test.go`
- Modify: `pkg/oci/annotations.go` (already has constants from Task 2)

- [ ] **Step 1: Write failing test for bundle packing**

Create `pkg/oci/bundle_test.go`:

```go
package oci_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/redhat-et/skillimage/pkg/oci"
)

func writeTestBundle(t *testing.T, dir string) {
	t.Helper()

	// Create two skill subdirectories.
	for _, name := range []string{"skill-a", "skill-b"} {
		skillDir := filepath.Join(dir, name)
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatal(err)
		}
		skillYAML := []byte(`apiVersion: skillimage.io/v1alpha1
kind: SkillCard
metadata:
  name: ` + name + `
  namespace: test
  version: 1.0.0
  description: Test skill ` + name + `.
spec:
  prompt: SKILL.md
`)
		if err := os.WriteFile(filepath.Join(skillDir, "skill.yaml"), skillYAML, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("Prompt for "+name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPackBundle(t *testing.T) {
	bundleDir := t.TempDir()
	writeTestBundle(t, bundleDir)

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	desc, err := client.PackBundle(ctx, bundleDir, oci.BundlePackOptions{
		Tag: "1.0.0-draft",
	})
	if err != nil {
		t.Fatalf("PackBundle: %v", err)
	}
	if desc.Digest.String() == "" {
		t.Error("expected non-empty digest")
	}

	// Inspect the bundle — it should have the bundle annotation.
	result, err := client.Inspect(ctx, "test/test-bundle:1.0.0-draft")
	if err != nil {
		// The tag format for bundles needs to be determined by the
		// implementation. This test may need adjustment.
		t.Logf("inspect by name failed (expected for bundles): %v", err)
	}

	// List local — should find the bundle.
	images, err := client.ListLocal()
	if err != nil {
		t.Fatalf("ListLocal: %v", err)
	}
	if len(images) == 0 {
		t.Fatal("expected at least 1 image after PackBundle")
	}
}
```

Run: `go test ./pkg/oci/ -run TestPackBundle -v`
Expected: FAIL — `PackBundle` not defined.

- [ ] **Step 2: Implement PackBundle**

Create `pkg/oci/bundle.go`:

```go
package oci

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	godigest "github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/errdef"

	"github.com/redhat-et/skillimage/pkg/lifecycle"
	"github.com/redhat-et/skillimage/pkg/skillcard"
)

type BundlePackOptions struct {
	Tag       string
	MediaType MediaTypeProfile
}

// PackBundle reads a directory containing multiple skill
// subdirectories, validates each SkillCard, creates a single
// OCI image with all skills, and stores it in the local OCI
// layout.
func (c *Client) PackBundle(ctx context.Context, bundleDir string, opts BundlePackOptions) (ocispec.Descriptor, error) {
	if opts.Tag == "" {
		return ocispec.Descriptor{}, fmt.Errorf("--tag is required for bundles")
	}

	entries, err := os.ReadDir(bundleDir)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("reading bundle directory: %w", err)
	}

	var skillNames []string
	var namespace string
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		skillDir := filepath.Join(bundleDir, entry.Name())
		skillPath := filepath.Join(skillDir, "skill.yaml")
		if _, err := os.Stat(skillPath); err != nil {
			continue
		}

		f, err := os.Open(skillPath)
		if err != nil {
			return ocispec.Descriptor{}, fmt.Errorf("opening %s: %w", skillPath, err)
		}
		sc, err := skillcard.Parse(f)
		_ = f.Close()
		if err != nil {
			return ocispec.Descriptor{}, fmt.Errorf("parsing %s: %w", skillPath, err)
		}

		validationErrors, err := skillcard.Validate(sc)
		if err != nil {
			return ocispec.Descriptor{}, fmt.Errorf("validating %s: %w", skillPath, err)
		}
		if len(validationErrors) > 0 {
			var msgs []string
			for _, ve := range validationErrors {
				msgs = append(msgs, ve.String())
			}
			return ocispec.Descriptor{}, fmt.Errorf("%s validation failed: %s", skillPath, strings.Join(msgs, "; "))
		}

		skillNames = append(skillNames, sc.Metadata.Name)
		if namespace == "" {
			namespace = sc.Metadata.Namespace
		}
	}

	if len(skillNames) == 0 {
		return ocispec.Descriptor{}, fmt.Errorf("no valid skill subdirectories found in %s", bundleDir)
	}

	layerMediaType, configMediaType := resolveMediaTypes(opts.MediaType)

	layerBuf, uncompressedDigest, err := createLayer(bundleDir)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("creating layer: %w", err)
	}

	layerBytes := layerBuf.Bytes()
	layerDigest := godigest.FromBytes(layerBytes)
	layerDesc := ocispec.Descriptor{
		MediaType: layerMediaType,
		Digest:    layerDigest,
		Size:      int64(len(layerBytes)),
	}
	if err := c.store.Push(ctx, layerDesc, bytes.NewReader(layerBytes)); err != nil && !errors.Is(err, errdef.ErrAlreadyExists) {
		return ocispec.Descriptor{}, fmt.Errorf("pushing layer: %w", err)
	}

	configBytes, err := buildImageConfig(uncompressedDigest)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("building image config: %w", err)
	}
	configDigest := godigest.FromBytes(configBytes)
	configDesc := ocispec.Descriptor{
		MediaType: configMediaType,
		Digest:    configDigest,
		Size:      int64(len(configBytes)),
	}
	if err := c.store.Push(ctx, configDesc, bytes.NewReader(configBytes)); err != nil && !errors.Is(err, errdef.ErrAlreadyExists) {
		return ocispec.Descriptor{}, fmt.Errorf("pushing config: %w", err)
	}

	skillsJSON, _ := json.Marshal(skillNames)
	ann := map[string]string{
		AnnotationBundle:                "true",
		AnnotationBundleSkills:          string(skillsJSON),
		lifecycle.StatusAnnotation:      string(lifecycle.Draft),
		ocispec.AnnotationVersion:       opts.Tag,
		ocispec.AnnotationCreated:       time.Now().UTC().Format(time.RFC3339),
	}
	if namespace != "" {
		ann[ocispec.AnnotationVendor] = namespace
	}

	manifest := ocispec.Manifest{
		Versioned:   specs.Versioned{SchemaVersion: 2},
		MediaType:   ocispec.MediaTypeImageManifest,
		Config:      configDesc,
		Layers:      []ocispec.Descriptor{layerDesc},
		Annotations: ann,
	}

	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("marshaling manifest: %w", err)
	}

	manifestDigest := godigest.FromBytes(manifestBytes)
	manifestDesc := ocispec.Descriptor{
		MediaType:   ocispec.MediaTypeImageManifest,
		Digest:      manifestDigest,
		Size:        int64(len(manifestBytes)),
		Annotations: ann,
	}

	if err := c.store.Push(ctx, manifestDesc, bytes.NewReader(manifestBytes)); err != nil && !errors.Is(err, errdef.ErrAlreadyExists) {
		return ocispec.Descriptor{}, fmt.Errorf("pushing manifest: %w", err)
	}

	bundleName := filepath.Base(bundleDir)
	ref := fmt.Sprintf("%s/%s:%s", namespace, bundleName, opts.Tag)
	if err := c.store.Tag(ctx, manifestDesc, ref); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("tagging bundle image: %w", err)
	}

	return manifestDesc, nil
}
```

- [ ] **Step 3: Run bundle test**

Run: `go test ./pkg/oci/ -run TestPackBundle -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add pkg/oci/bundle.go pkg/oci/bundle_test.go
git commit -s -m "feat: add bundle pack support for multi-skill OCI images"
```

---

### Task 7: Wire `--bundle` flag into CLI

**Files:**
- Modify: `internal/cli/pack.go`

- [ ] **Step 1: Add the `--bundle` flag**

Update `internal/cli/pack.go`:

```go
func newPackCmd() *cobra.Command {
	var tag string
	var mediaType string
	var bundle bool
	cmd := &cobra.Command{
		Use:   "pack <dir>",
		Short: "Pack a skill directory into a local OCI image",
		Long: `Pack a skill directory into a local OCI image.

Use --bundle to pack a directory containing multiple skill
subdirectories into a single OCI image.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if bundle {
				return runPackBundle(cmd, args[0], tag, mediaType)
			}
			return runPack(cmd, args[0], tag, mediaType)
		},
	}
	cmd.Flags().StringVar(&tag, "tag", "", "override the image tag (default: <version>-draft)")
	cmd.Flags().StringVar(&mediaType, "media-type", "", `media type profile: "standard" (default) or "redhat" (for oc-mirror)`)
	cmd.Flags().BoolVar(&bundle, "bundle", false, "pack multiple skill subdirectories as a single image")
	return cmd
}

func runPackBundle(cmd *cobra.Command, dir, tag, mediaType string) error {
	profile, err := oci.ParseMediaTypeProfile(mediaType)
	if err != nil {
		return err
	}

	client, err := defaultClient()
	if err != nil {
		return err
	}

	desc, err := client.PackBundle(context.Background(), dir, oci.BundlePackOptions{
		Tag:       tag,
		MediaType: profile,
	})
	if err != nil {
		return fmt.Errorf("packing bundle %s: %w", dir, err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Packed bundle %s\nDigest: %s\n", dir, desc.Digest)
	return nil
}
```

- [ ] **Step 2: Verify it compiles and shows help**

Run: `go build -o bin/skillctl ./cmd/skillctl && bin/skillctl pack --help`
Expected: shows `--bundle` flag in help output.

- [ ] **Step 3: End-to-end test**

```bash
mkdir -p /tmp/test-bundle/skill-a /tmp/test-bundle/skill-b

cat > /tmp/test-bundle/skill-a/skill.yaml <<'EOF'
apiVersion: skillimage.io/v1alpha1
kind: SkillCard
metadata:
  name: skill-a
  namespace: test
  version: 1.0.0
  description: Test skill A.
spec:
  prompt: SKILL.md
EOF
echo "Prompt A" > /tmp/test-bundle/skill-a/SKILL.md

cat > /tmp/test-bundle/skill-b/skill.yaml <<'EOF'
apiVersion: skillimage.io/v1alpha1
kind: SkillCard
metadata:
  name: skill-b
  namespace: test
  version: 1.0.0
  description: Test skill B.
spec:
  prompt: SKILL.md
EOF
echo "Prompt B" > /tmp/test-bundle/skill-b/SKILL.md

bin/skillctl pack --bundle --tag 1.0.0-draft /tmp/test-bundle
bin/skillctl list
```

Expected: `Packed bundle /tmp/test-bundle` and the bundle appears
in `list` output.

- [ ] **Step 4: Clean up and commit**

```bash
rm -rf /tmp/test-bundle
git add internal/cli/pack.go
git commit -s -m "feat: add --bundle flag to skillctl pack"
```

---

### Task 8: Content retrieval handler

**Files:**
- Modify: `internal/handler/skills.go`

The content endpoint fetches SKILL.md from the OCI registry on
demand. This requires the handler to know the registry URL and
TLS settings.

- [ ] **Step 1: Add registry config to SkillsHandler**

Update `internal/handler/skills.go`:

Add a `ContentConfig` field to `SkillsHandler`:

```go
type ContentConfig struct {
	RegistryURL   string
	SkipTLSVerify bool
}

type SkillsHandler struct {
	store     *store.Store
	contentCfg ContentConfig
}

func NewSkillsHandler(s *store.Store, cfg ContentConfig) *SkillsHandler {
	return &SkillsHandler{store: s, contentCfg: cfg}
}
```

Update all callers of `NewSkillsHandler` (router, tests) to pass
the config.

- [ ] **Step 2: Implement the Content handler**

Add the `Content` method to `SkillsHandler` in
`internal/handler/skills.go`:

```go
func (h *SkillsHandler) Content(w http.ResponseWriter, r *http.Request) {
	ns := chi.URLParam(r, "ns")
	name := chi.URLParam(r, "name")
	ver := chi.URLParam(r, "ver")

	versions, err := h.store.GetVersions(ns, name)
	if err != nil || len(versions) == 0 {
		writeError(w, http.StatusNotFound, "skill not found", fmt.Errorf("no versions for %s/%s", ns, name))
		return
	}

	var skill *store.Skill
	for i := range versions {
		if versions[i].Version == ver {
			skill = &versions[i]
			break
		}
	}
	if skill == nil {
		writeError(w, http.StatusNotFound, "version not found", fmt.Errorf("version %s not found", ver))
		return
	}

	ref := fmt.Sprintf("%s/%s:%s", h.contentCfg.RegistryURL, skill.Repository, skill.Tag)
	client, err := oci.NewClient(os.TempDir())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}

	result, err := client.InspectRemote(r.Context(), ref, oci.InspectOptions{
		SkipTLSVerify: h.contentCfg.SkipTLSVerify,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, "registry error", err)
		return
	}

	// Pull and extract SKILL.md content.
	tmpDir := filepath.Join(os.TempDir(), "skillctl-content-"+skill.Digest)
	defer os.RemoveAll(tmpDir)

	_, err = client.Pull(r.Context(), ref, oci.PullOptions{
		OutputDir:     tmpDir,
		SkipTLSVerify: h.contentCfg.SkipTLSVerify,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, "pull failed", err)
		return
	}

	// Find SKILL.md in the unpacked directory.
	skillMD, err := findSkillMD(tmpDir)
	if err != nil {
		writeError(w, http.StatusNotFound, "SKILL.md not found", err)
		return
	}

	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(skillMD)
}

func findSkillMD(dir string) ([]byte, error) {
	var content []byte
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Name() == "SKILL.md" && !info.IsDir() {
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			content = data
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if content == nil {
		return nil, fmt.Errorf("SKILL.md not found in extracted content")
	}
	return content, nil
}
```

Note: This implementation pulls the full layer to a temp directory
to extract SKILL.md. For Phase 2a this is acceptable since content
requests are infrequent (individual skill detail pages). A future
optimization could cache extracted content or use streaming tar
extraction.

- [ ] **Step 3: Verify it compiles**

Run: `go vet ./internal/handler/ ./internal/server/`
Expected: no errors

- [ ] **Step 4: Commit**

```bash
git add internal/handler/skills.go internal/server/router.go
git commit -s -m "feat: add content retrieval handler for SKILL.md"
```

---

### Task 9: Integration test and final verification

**Files:**
- Create: `internal/server/server_test.go`

- [ ] **Step 1: Write an integration test**

Create `internal/server/server_test.go`:

```go
package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/redhat-et/skillimage/internal/handler"
	"github.com/redhat-et/skillimage/internal/server"
	"github.com/redhat-et/skillimage/internal/store"
)

func TestRouterIntegration(t *testing.T) {
	db, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer db.Close()

	_ = db.UpsertSkill(store.Skill{
		Repository: "team1/doc-reviewer", Tag: "1.0.0",
		Digest: "sha256:abc", Name: "doc-reviewer",
		Namespace: "team1", Status: "published",
		Version: "1.0.0", DisplayName: "Document Reviewer",
		Description: "Reviews docs", TagsJSON: `["review"]`,
	})

	router := server.NewRouter(db, func() {})

	tests := []struct {
		method string
		path   string
		status int
	}{
		{"GET", "/healthz", 200},
		{"GET", "/api/v1/skills", 200},
		{"GET", "/api/v1/skills?status=published", 200},
		{"GET", "/api/v1/skills?q=review", 200},
		{"GET", "/api/v1/skills/team1/doc-reviewer", 200},
		{"GET", "/api/v1/skills/team1/doc-reviewer/versions", 200},
		{"GET", "/api/v1/skills/team1/nonexistent", 404},
		{"POST", "/api/v1/sync", 202},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != tt.status {
				t.Errorf("status = %d, want %d; body: %s",
					w.Code, tt.status, w.Body.String())
			}
		})
	}

	// Verify the skills response has pagination.
	req := httptest.NewRequest("GET", "/api/v1/skills", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp struct {
		Data       []json.RawMessage `json:"data"`
		Pagination struct {
			Total   int `json:"total"`
			Page    int `json:"page"`
			PerPage int `json:"per_page"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Pagination.Total != 1 {
		t.Errorf("total = %d, want 1", resp.Pagination.Total)
	}
}
```

Note: Update `NewRouter` signature and `NewSkillsHandler` call to
pass `handler.ContentConfig{}` — for tests without a real registry
the content endpoint won't be tested here.

- [ ] **Step 2: Run all tests**

Run: `go test ./... -count=1`
Expected: all PASS

- [ ] **Step 3: Run linter**

Run: `make lint`
Expected: no errors (or only pre-existing warnings)

- [ ] **Step 4: Final commit**

```bash
git add internal/server/server_test.go
git commit -s -m "test: add integration test for catalog server routes"
```
