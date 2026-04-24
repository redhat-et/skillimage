# Phase 2a: Catalog Server and OCI Bundles

**Date:** 2026-04-24
**Status:** Draft
**Prerequisite:** `2026-04-16-implementation-spec.md` (Phase 1)

## Goal

Deliver a read-only catalog server that indexes skills from an OCI
registry into SQLite and serves them via a REST API, plus a CLI
command for packing multi-skill OCI bundle images.

## Problem

The CLI provides per-image operations (inspect, list, pull), but
consumers that need to browse, filter, and search across many skills
have no fast path. Walking the registry catalog API and fetching
manifests one by one is slow and impractical for UIs, dashboards,
or higher-level governance systems like MLflow. Enterprises also
need to distribute curated sets of skills as a single deployable
unit.

## Decisions made during design

| Decision | Choice | Rationale |
|---|---|---|
| Server role | General-purpose OpenShift AI component | Not tied to any single frontend (RHDH plugin, MLflow, custom) |
| Registry scope | Single registry (Phase 2a) | Multi-registry is an additive change later |
| Data architecture | SQLite index with periodic sync | Fast filtered queries without hammering the registry |
| Bundle type | OCI images (multi-skill) | Deployable via ImageVolumes; fits distribution vehicle model |
| Search | SQL-based filtering on indexed annotations | No external dependencies; sufficient for <10K skills |
| Auth | Reuse existing `credentialStore()` | Works with all registries via `~/.docker/config.json` or SA tokens |
| Server writes | None in Phase 2a | Pack, push, promote stay in the CLI |

## Architecture

```text
OCI Registry (OpenShift internal, Quay, GHCR, Zot, Harbor)
     |
     | periodic sync (configurable interval, default 60s)
     |
     v
Catalog Server (skillctl serve)
  +-- sync engine: walks /v2/_catalog, fetches manifests, indexes annotations
  +-- SQLite: indexed skill metadata from manifest annotations
  +-- REST API: list, search, filter, get content
     |
     | GET /api/v1/skills?q=...&tags=...&status=...
     |
     v
Consumers: RHDH plugin, MLflow sync, custom UIs, scripts
```

The server is read-only. It reads from the registry and serves
from the index. The CLI remains the write path (pack, push,
promote).

## Sync engine

### Startup sync

On startup, the server performs a full sync:

1. Call `/v2/_catalog` to list all repositories
2. For each repo (optionally filtered by configured namespace
   prefix), call `/v2/<repo>/tags/list`
3. For each tag, fetch the manifest and extract annotations
4. Skip images without `io.skillimage.status` annotation (not a skill)
5. Insert or update the SQLite index

### Skill image detection

Not every image in the registry is a skill. The sync engine
filters by checking for the `io.skillimage.status` annotation
in the manifest. Every image packed by `skillctl pack` has this
annotation (set to `draft` at pack time). Images without it are
skipped during sync.

This detection heuristic is intentionally simple and expected
to change. A CNCF interest group is working on a common standard
for OCI images containing AI agent skills. Once a standard
annotation or media type is defined, the sync engine should
adopt it. The filter logic is isolated in the sync engine to
make this swap straightforward.

### Incremental sync

A background goroutine runs every N seconds (configurable via
`syncInterval`, default `60s`):

1. Walk the same catalog/tags/manifest path
2. Skip manifests whose digest has not changed since last sync
3. Update changed entries, insert new ones
4. Remove entries whose tags no longer exist in the registry
   (stale cleanup)

### Content retrieval

SKILL.md content is NOT indexed. It lives inside the OCI layer
blob (tar.gz). When a client requests content via the API, the
server fetches the layer on demand from the registry, extracts
SKILL.md, and returns it. This avoids storing potentially large
text blobs in SQLite while keeping the common listing path fast
(manifest annotations only).

## SQLite schema

Single table for the index. One row per skill version (tag).

```sql
CREATE TABLE skills (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    repository    TEXT NOT NULL,      -- e.g. "team1/document-reviewer"
    tag           TEXT NOT NULL,      -- e.g. "1.0.0-draft"
    digest        TEXT NOT NULL,      -- manifest digest for change detection
    name          TEXT,               -- from annotation: image.title or parsed from repo
    namespace     TEXT,               -- from annotation: image.vendor
    version       TEXT,               -- from annotation: image.version
    status        TEXT,               -- from annotation: io.skillimage.status
    display_name  TEXT,               -- from annotation: image.title
    description   TEXT,               -- from annotation: image.description
    authors       TEXT,               -- from annotation: image.authors
    license       TEXT,               -- from annotation: image.licenses
    tags_json     TEXT,               -- from annotation: io.skillimage.tags (JSON array)
    compatibility TEXT,               -- from annotation: io.skillimage.compatibility
    word_count    INTEGER,            -- from annotation: io.skillimage.wordcount
    created       TEXT,               -- from annotation: image.created (RFC 3339)
    synced_at     TEXT NOT NULL,      -- last sync timestamp
    UNIQUE(repository, tag)
);

CREATE INDEX idx_skills_namespace ON skills(namespace);
CREATE INDEX idx_skills_status ON skills(status);
CREATE INDEX idx_skills_name ON skills(name);
```

## REST API

**Base path:** `/api/v1`

### Endpoints

| Method | Path | Description |
|---|---|---|
| `GET` | `/skills` | List/search skills with filtering |
| `GET` | `/skills/{ns}/{name}` | Get skill metadata (latest version) |
| `GET` | `/skills/{ns}/{name}/versions` | List all versions of a skill |
| `GET` | `/skills/{ns}/{name}/versions/{ver}/content` | Get SKILL.md content (on-demand layer fetch) |
| `POST` | `/sync` | Trigger immediate re-sync from registry |
| `GET` | `/healthz` | Health check |

### Query parameters for `GET /skills`

| Parameter | Example | Behavior |
|---|---|---|
| `q` | `q=kubernetes` | Free-text search against name, display name, description |
| `tags` | `tags=kubernetes,debugging` | Match skills containing any of the specified tags |
| `status` | `status=published` | Exact match on lifecycle status |
| `namespace` | `namespace=acme` | Exact match on namespace |
| `compatibility` | `compatibility=claude-3.5-sonnet` | Exact match |
| `page` | `page=2` | Page number (default 1) |
| `per_page` | `per_page=50` | Results per page (default 20, max 100) |

No parameters returns all skills. Parameters can be combined.

### Response format

Follows the envelope pattern from the original design doc:

```json
{
  "data": [ ... ],
  "_meta": { "request_id": "..." },
  "pagination": { "total": 42, "page": 1, "per_page": 20 }
}
```

Errors follow RFC 7807:

```json
{
  "type": "about:blank",
  "title": "Not Found",
  "status": 404,
  "detail": "skill acme/nonexistent not found"
}
```

### Skill object

```json
{
  "repository": "team1/document-reviewer",
  "name": "document-reviewer",
  "namespace": "team1",
  "display_name": "Document Reviewer",
  "version": "1.0.0",
  "status": "published",
  "description": "Reviews technical documents for clarity...",
  "authors": "OCTO Team <octo@redhat.com>",
  "license": "Apache-2.0",
  "tags": ["review", "documents"],
  "compatibility": "claude-3.5-sonnet",
  "word_count": 342,
  "digest": "sha256:ffa608d3...",
  "created": "2026-04-19T20:50:49Z"
}
```

## Bundle support (CLI)

### Pack command

`skillctl pack --bundle <dir>` packs a directory containing
multiple skill subdirectories into a single OCI image:

```text
hr-skills/
+-- document-reviewer/
|   +-- skill.yaml
|   +-- SKILL.md
+-- document-summarizer/
    +-- skill.yaml
    +-- SKILL.md
```

The resulting image has skills at subdirectory roots:

```text
/
+-- document-reviewer/
|   +-- skill.yaml
|   +-- SKILL.md
+-- document-summarizer/
    +-- skill.yaml
    +-- SKILL.md
```

### Bundle metadata

The bundle image uses a synthetic SkillCard-like annotation set:

| Annotation | Value |
|---|---|
| `io.skillimage.bundle` | `"true"` |
| `io.skillimage.bundle.skills` | JSON array of skill names |
| `io.skillimage.status` | `"draft"` (initial) |
| `org.opencontainers.image.version` | Required via `--tag` flag (no default — bundles have no single version) |

### Bundle discovery

The catalog server detects bundles via the `io.skillimage.bundle`
annotation. Bundle listing could be a separate endpoint in the
future, but for Phase 2a they appear in the regular skills listing
with a `bundle: true` field.

### K8s deployment

A bundle mounts as a single ImageVolume:

```yaml
volumes:
- name: hr-skills
  image:
    reference: quay.io/acme/hr-skills:1.0.0
    pullPolicy: IfNotPresent
volumeMounts:
- name: hr-skills
  mountPath: /agent/skills
  readOnly: true
```

The agent sees:

```text
/agent/skills/
+-- document-reviewer/
|   +-- skill.yaml
|   +-- SKILL.md
+-- document-summarizer/
    +-- skill.yaml
    +-- SKILL.md
```

## Registry authentication

The server reuses the existing `credentialStore()` from
`pkg/oci/push.go`, which reads credentials from Docker and
Podman auth files. No new auth code is needed.

| Registry | Auth for server pod |
|---|---|
| OpenShift Internal | ServiceAccount with `system:image-puller` role — no secret needed |
| Quay.io / self-hosted | Robot account, mounted as `dockerconfigjson` Secret |
| GHCR | Fine-grained PAT with `read:packages`, mounted as Secret |
| Zot | htpasswd service user, mounted as Secret |
| Harbor | Robot account (v2) with `pull` permission, mounted as Secret |

For OpenShift internal registry, the server pod's ServiceAccount
token is auto-mounted. Bind the SA to the target namespace:

```bash
oc policy add-role-to-user system:image-puller \
  system:serviceaccount:<server-ns>:<server-sa> -n <skills-ns>
```

## Configuration

Via `config.yaml` or environment variables (Viper):

```yaml
registry:
  url: image-registry.openshift-image-registry.svc:5000
  namespace: team1
  tlsVerify: true
  syncInterval: 60s
server:
  port: 8080
  dbPath: /data/skillctl.db
```

| Config key | Env var | Default | Description |
|---|---|---|---|
| `registry.url` | `SKILLCTL_REGISTRY_URL` | (required) | OCI registry URL |
| `registry.namespace` | `SKILLCTL_REGISTRY_NAMESPACE` | (none — all repos) | Limit sync to a namespace prefix |
| `registry.tlsVerify` | `SKILLCTL_REGISTRY_TLS_VERIFY` | `true` | TLS certificate verification |
| `registry.syncInterval` | `SKILLCTL_REGISTRY_SYNC_INTERVAL` | `60s` | Background sync interval |
| `server.port` | `SKILLCTL_SERVER_PORT` | `8080` | HTTP listen port |
| `server.dbPath` | `SKILLCTL_SERVER_DB_PATH` | `skillctl.db` | SQLite database path |

## Project structure (new and changed files)

| Path | Description |
|---|---|
| `internal/server/server.go` | HTTP server setup, graceful shutdown |
| `internal/server/router.go` | chi router, middleware (request ID, logging) |
| `internal/handler/skills.go` | Skill listing, search, detail handlers |
| `internal/handler/sync.go` | Manual sync trigger handler |
| `internal/handler/health.go` | Health check handler |
| `internal/store/sqlite.go` | SQLite schema creation, CRUD queries |
| `internal/store/sync.go` | Sync engine (full and incremental) |
| `internal/cli/serve.go` | `skillctl serve` command |
| `pkg/oci/pack.go` | Updated: `--bundle` flag for multi-skill images |
| `pkg/oci/catalog.go` | New: registry catalog walking (list repos, tags) |

## Compatibility with consumers

### RHDH Skills Marketplace plugin

The plugin can call our API instead of implementing its own
`OciRegistryService`. The `/skills` endpoint returns the same
metadata the plugin currently extracts from manifests directly.

### MLflow Skill Registry

MLflow can sync from our API to auto-register OCI skills:
`GET /skills?status=published` returns all published skills with
their OCI refs, which map to MLflow's `source_type: "oci"` and
`source_url` fields.

### Custom frontends

The OpenAPI spec (served at `/api/v1/openapi.yaml`) enables
typed client generation for any language.

## What is NOT in Phase 2a

| Feature | Planned phase |
|---|---|
| Multi-registry support | Phase 2b |
| Write operations via API (pack, push, promote) | Phase 2b+ |
| Semantic/vector search | Out of scope (consumer responsibility) |
| RBAC and user management | Phase 2b+ |
| Signing and verification via server | Phase 2b+ |
| Dependency resolution (`skills.json` / `skills.lock.json`) | Phase 3 |
| Remote Git source for pack (issue #10) | Future |
| OpenAPI spec file | Phase 2a (generated from code or hand-written) |
| Webhook-triggered sync | Future optimization |
