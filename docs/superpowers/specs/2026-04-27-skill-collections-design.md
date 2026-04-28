# Skill Collections

**Date:** 2026-04-27
**Status:** Draft
**Prerequisite:** Phase 2a catalog server

## Goal

Replace physical skill bundles (multi-skill OCI images) with
skill collections — YAML manifests that reference individual
skill images by OCI ref. Collections are stored as OCI artifacts
in registries and integrated into the catalog server.

## Problem

Enterprises need to distribute curated sets of skills for
specific tasks (e.g., "HR document processing," "incident
response"). The existing `build --bundle` approach packages
multiple skills into a single OCI image, coupling their
lifecycles and requiring all skills to come from the same
source directory and namespace.

Collections solve this by grouping skills logically rather
than physically:

- Each skill stays independently versioned and mountable
- Skills can come from different registries (Quay, GHCR,
  internal registries)
- Updating one skill doesn't require rebuilding the bundle
- `skillctl` generates deployment artifacts (Podman volumes,
  Kubernetes YAML) from the collection manifest

## Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Grouping | Logical (YAML manifest) | No repackaging, independent lifecycles, cross-registry |
| Storage format | OCI artifact | Not mountable content; only `skillctl` reads it |
| Bundle removal | Delete all bundle code | Collections replace bundles; pre-1.0, no compatibility burden |
| Catalog integration | New `collections` table + API | Collections are first-class alongside skills |
| Namespace field | None | Collections span registries and namespaces |

## Collection format

```yaml
apiVersion: skillimage.io/v1alpha1
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
  - name: resume-screener
    image: registry.internal:5000/hr/resume-screener:1.0.0
```

| Field | Required | Description |
|-------|----------|-------------|
| `apiVersion` | Yes | `skillimage.io/v1alpha1` |
| `kind` | Yes | `SkillCollection` |
| `metadata.name` | Yes | Collection identifier |
| `metadata.version` | Yes | Collection version (independent of skill versions) |
| `metadata.description` | No | Short description |
| `skills[].name` | Yes | Local name (volume name, mount directory) |
| `skills[].image` | Yes | Full OCI image reference including registry |

**Validation rules:**

- Duplicate `skills[].name` values are rejected
- `skills[].image` must include a registry hostname
- Digest pinning is optional: `image: quay.io/.../skill:1.0.0@sha256:abc`

## OCI artifact storage

Collections are pushed to registries as OCI artifacts, not
OCI images. Only `skillctl` reads them — there is no need
for `podman pull` or Kubernetes ImageVolume compatibility.

| Property | Value |
|----------|-------|
| Artifact type | `application/vnd.skillimage.collection.v1+yaml` |
| Layer count | 1 (the collection YAML file) |
| Config | Empty OCI descriptor |

**Annotations on the artifact manifest:**

| Key | Source |
|-----|--------|
| `org.opencontainers.image.title` | `metadata.name` |
| `org.opencontainers.image.version` | `metadata.version` |
| `org.opencontainers.image.description` | `metadata.description` |
| `org.opencontainers.image.created` | Build timestamp |
| `io.skillimage.collection.name` | `metadata.name` |

Compatible with `oras pull`, `podman artifact pull` (5.5+),
and `skopeo`.

## CLI commands

### collection push

Push a local collection YAML to a registry as an OCI artifact.

```
skillctl collection push -f collection.yaml \
  quay.io/myorg/collections/hr-skills:1.0.0
```

### collection pull

Pull a collection and all its skills. Supports the same
output directory and agent target options as `skillctl pull`.

```
skillctl collection pull quay.io/myorg/collections/hr-skills:1.0.0
skillctl collection pull -o ~/.claude/skills \
  quay.io/myorg/collections/hr-skills:1.0.0
```

This fetches the collection artifact, parses the YAML, and
pulls each skill image, extracting it into
`<output-dir>/<skill-name>/`.

### collection volume

Generate (or execute) Podman volume creation commands from
a collection.

```
skillctl collection volume -f collection.yaml
skillctl collection volume quay.io/myorg/collections/hr-skills:1.0.0
skillctl collection volume -f collection.yaml --mount-root /agent/skills
skillctl collection volume -f collection.yaml --execute
```

Default `--mount-root` is `/skills`. Output includes volume
creation and a `podman run` example with mount paths:

```bash
podman pull quay.io/skillimage/business/document-summarizer:1.0.0
podman volume create --driver image \
  --opt image=quay.io/skillimage/business/document-summarizer:1.0.0 \
  document-summarizer
podman pull ghcr.io/acme/document-reviewer:2.1.0
podman volume create --driver image \
  --opt image=ghcr.io/acme/document-reviewer:2.1.0 \
  document-reviewer

# Run with:
# podman run --rm \
#   -v document-summarizer:/skills/document-summarizer:ro \
#   -v document-reviewer:/skills/document-reviewer:ro \
#   my-agent:latest
```

`--mount-root` changes the mount paths in the suggested
`podman run` command (e.g., `/agent/skills/document-summarizer`).

With `--execute`, runs the volume creation commands directly
instead of printing them.

### collection generate

Emit a Kubernetes partial pod spec (volumes + volumeMounts)
from a collection.

```
skillctl collection generate -f collection.yaml
skillctl collection generate -f collection.yaml --mount-root /agent/skills
skillctl collection generate quay.io/myorg/collections/hr-skills:1.0.0
```

Default `--mount-root` is `/skills`. Output:

```yaml
volumes:
  - name: document-summarizer
    image:
      reference: quay.io/skillimage/business/document-summarizer:1.0.0
      pullPolicy: IfNotPresent
  - name: document-reviewer
    image:
      reference: ghcr.io/acme/document-reviewer:2.1.0
      pullPolicy: IfNotPresent
containers:
  - name: agent
    volumeMounts:
      - name: document-summarizer
        mountPath: /skills/document-summarizer
        readOnly: true
      - name: document-reviewer
        mountPath: /skills/document-reviewer
        readOnly: true
```

This is a partial spec that users paste into their deployment
YAML. Not a full pod — that would be too opinionated about the
agent container.

## Catalog server integration

### SQLite schema

New `collections` table (the `skills` table is unchanged):

```sql
CREATE TABLE collections (
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

`skills_json` stores the full `skills` array from the
collection YAML as JSON.

### Sync engine

During registry sync, the engine detects OCI artifacts with
`artifactType: application/vnd.skillimage.collection.v1+yaml`.
For each:

1. Fetch the manifest and check artifact type
2. Pull the single layer (YAML content)
3. Parse the YAML and validate the collection format
4. Upsert into the `collections` table

Collections from registries that the catalog server doesn't
index are not resolved — the catalog stores the skill refs
as-is from the YAML.

### API endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/collections` | List all collections |
| GET | `/api/v1/collections/{name}` | Collection detail with skill list |

**List response:**

```json
{
  "data": [
    {
      "name": "hr-skills",
      "version": "1.0.0",
      "description": "Skills for HR document processing",
      "skills": ["document-summarizer", "document-reviewer", "resume-screener"],
      "digest": "sha256:...",
      "created": "2026-04-27T10:00:00Z"
    }
  ]
}
```

**Detail response** includes full skill image refs:

```json
{
  "name": "hr-skills",
  "version": "1.0.0",
  "description": "Skills for HR document processing",
  "skills": [
    {"name": "document-summarizer", "image": "quay.io/skillimage/business/document-summarizer:1.0.0"},
    {"name": "document-reviewer", "image": "ghcr.io/acme/document-reviewer:2.1.0"},
    {"name": "resume-screener", "image": "registry.internal:5000/hr/resume-screener:1.0.0"}
  ],
  "repository": "quay.io/myorg/collections/hr-skills",
  "tag": "1.0.0",
  "digest": "sha256:...",
  "created": "2026-04-27T10:00:00Z"
}
```

## Bundle removal

All existing bundle code is removed:

| What | File |
|------|------|
| `--bundle` flag | `internal/cli/build.go` |
| `BuildBundle`, `BundleBuildOptions` | `pkg/oci/bundle.go` (delete) |
| `AnnotationBundle`, `AnnotationBundleSkills` | `pkg/oci/annotations.go` |
| `Bundle`, `BundleSkills` struct fields | `internal/store/store.go` |
| `bundle`, `bundle_skills` columns | SQLite schema (recreate) |
| Bundle tests | `pkg/oci/bundle_test.go` (delete) |

Since this is pre-1.0, the SQLite schema change uses
`DROP TABLE + CREATE TABLE` rather than `ALTER TABLE`.

## Relationship to Thomas Vitale's spec

Thomas Vitale's Agent Skills OCI Artifacts Spec defines
collections as OCI Image Indexes referencing ORAS artifacts.
We diverge intentionally:

| Aspect | Vitale spec | skillctl |
|--------|-------------|----------|
| Individual skills | ORAS artifacts | OCI images (mountable) |
| Collections | OCI Image Index | OCI artifact (YAML manifest) |
| Kubernetes support | Not addressed | ImageVolumes, `generate` command |
| Cross-registry refs | Not supported | Skills from any registry |
| CLI generation | Not supported | Podman volumes, K8s YAML |

The OCI Image Index approach doesn't work for Kubernetes
because container runtimes interpret indexes as multi-platform
manifest lists and select a single entry by platform, rather
than mounting all entries. Our YAML manifest approach avoids
this limitation entirely.

## Verification

1. Create a `collection.yaml` with skills from Quay
2. `skillctl collection push -f collection.yaml quay.io/.../test-collection:1.0.0`
3. `skillctl collection pull quay.io/.../test-collection:1.0.0` — verify all skills are pulled
4. `skillctl collection volume -f collection.yaml` — verify Podman commands are correct
5. `skillctl collection generate -f collection.yaml` — verify K8s YAML is valid
6. Start catalog server, sync, verify collection appears in `/api/v1/collections`
7. `curl /api/v1/collections/test-collection` — verify detail response
