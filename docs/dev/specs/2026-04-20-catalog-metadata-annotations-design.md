# Catalog metadata annotations

## Problem

The catalog UI pulls the full tar.gz layer blob (up to 10MB) for every
skill image during listing, just to extract `skill.yaml` for a handful
of fields not present in manifest annotations. This makes catalog page
load slow and bandwidth-heavy.

The missing fields are: `tags`, `compatibility`, and a content
complexity indicator (word count of SKILL.md).

## Decision

Enrich existing OCI manifest annotations with three new custom keys.
The frontend can then build the full catalog card from the manifest
alone, eliminating all blob downloads during listing.

### Alternatives considered

| Approach | Verdict |
| -------- | ------- |
| ORAS referrer artifact | Requires Referrers API (OCI 1.1), two fetches, lifecycle sync overhead |
| Separate metadata layer | Still requires blob fetch, breaks single-layer design, complicates ImageVolume mounts |

Both are overkill for approximately 200 bytes of additional metadata.

## Design

### New annotations

Three new keys in the `io.skillimage.*` namespace, set during
`skillctl pack`:

| Annotation key | Value format | Source | Example |
| -------------- | ------------ | ------ | ------- |
| `io.skillimage.tags` | JSON string array | `metadata.tags` | `["kubernetes","debugging"]` |
| `io.skillimage.compatibility` | Plain string | `metadata.compatibility` | `"claude-3.5-sonnet"` |
| `io.skillimage.wordcount` | Integer as string | SKILL.md word count | `"342"` |

All three are optional. If the source field is empty or SKILL.md does
not exist, the annotation is omitted. This matches the existing
pattern for `license`, `source`, and `revision`.

### Complete annotation map after this change

| OCI annotation | Source |
| -------------- | ------ |
| `org.opencontainers.image.title` | `metadata.display-name` or `metadata.name` |
| `org.opencontainers.image.description` | `metadata.description` (first 256 chars) |
| `org.opencontainers.image.version` | `metadata.version` |
| `org.opencontainers.image.authors` | `metadata.authors[]` comma-separated |
| `org.opencontainers.image.licenses` | `metadata.license` |
| `org.opencontainers.image.vendor` | `metadata.namespace` |
| `org.opencontainers.image.created` | Pack timestamp (RFC 3339) |
| `org.opencontainers.image.source` | `provenance.source` |
| `org.opencontainers.image.revision` | `provenance.commit` |
| `io.skillimage.status` | Lifecycle state (set to `draft` at pack) |
| `io.skillimage.tags` | **New** -- `metadata.tags` as JSON array |
| `io.skillimage.compatibility` | **New** -- `metadata.compatibility` |
| `io.skillimage.wordcount` | **New** -- SKILL.md word count |

### Word count computation

Computed during `Pack()` in `pkg/oci/pack.go`, before calling
`createLayer()`. The pack function reads `SKILL.md` from the skill
directory (if it exists), counts words using `strings.Fields()` (which
splits on any whitespace and handles soft line wraps, multiple spaces,
tabs, and blank lines), and passes the count to `buildAnnotations()`.

The signature of `buildAnnotations()` expands to accept the count:

```go
func buildAnnotations(sc *skillcard.SkillCard, wordCount int) map[string]string
```

If SKILL.md does not exist or is empty, `wordCount` is 0 and the
`io.skillimage.wordcount` annotation is omitted.

The frontend owns the display mapping (e.g., word count to
"Simple"/"Medium"/"Complex" labels). The backend stores the raw
number.

## Files changed

| File | Change |
| ---- | ------ |
| `pkg/oci/annotations.go` | Add `io.skillimage.tags`, `io.skillimage.compatibility`, `io.skillimage.wordcount`; accept `wordCount int` parameter |
| `pkg/oci/pack.go` | Count SKILL.md words during directory walk; pass count to `buildAnnotations()` |
| `pkg/oci/inspect.go` | Read and return new annotations |
| `pkg/oci/client.go` | Add `Tags`, `Compatibility`, `WordCount` fields to `InspectResult` |
| `internal/cli/inspect.go` | Display new fields in `skillctl inspect` output |
| `pkg/oci/oci_test.go` | Test that new annotations are set correctly, including edge cases (no tags, no SKILL.md, empty SKILL.md) |

## What does not change

- **Promote flow** (`pkg/oci/promote.go`): copies all manifest
  annotations forward. New annotations come along automatically.
- **Image format**: single tar.gz layer, same media types, same config.
- **SkillCard schema**: no changes to `skill.yaml` or `schemas/skillcard-v1.json`.
- **Backwards compatibility**: images packed before this change lack the
  three new annotations. Consumers treat missing annotations as
  absent/unknown, which is the existing pattern.

## Frontend impact

After this change, the catalog listing flow becomes:

1. `GET /v2/_catalog` -- repository names
2. Per repo: `GET /v2/<repo>/tags/list` + `GET /v2/<repo>/manifests/<tag>` -- all catalog card data
3. No blob download needed for listing

The individual skill detail page (showing full SKILL.md content) still
requires a layer pull, but that is a single on-demand fetch, not N
fetches at page load.

## Frontend integration guide

After this change, the catalog listing no longer needs blob
downloads. The manifest annotations contain everything needed for
the skill card.

### Annotation keys and value formats

| Annotation key | Type | Parse as | Example value |
| -------------- | ---- | -------- | ------------- |
| `org.opencontainers.image.title` | string | display name | `"Kubernetes Troubleshooter"` |
| `org.opencontainers.image.description` | string | truncated to 256 chars | `"Diagnoses pod failures..."` |
| `org.opencontainers.image.version` | string | semver | `"1.2.0"` |
| `org.opencontainers.image.authors` | string | comma-separated `name <email>` | `"Jane <j@x.com>, Bob"` |
| `org.opencontainers.image.licenses` | string | SPDX expression | `"Apache-2.0"` |
| `org.opencontainers.image.vendor` | string | namespace | `"acme/skills"` |
| `org.opencontainers.image.created` | string | RFC 3339 timestamp | `"2026-04-20T14:30:00Z"` |
| `io.skillimage.status` | string | lifecycle state enum | `"published"` |
| `io.skillimage.tags` | string | `JSON.parse()` to `string[]` | `'["kubernetes","debugging"]'` |
| `io.skillimage.compatibility` | string | plain string | `"claude-3.5-sonnet"` |
| `io.skillimage.wordcount` | string | `parseInt()` to number | `"342"` |

### Handling missing annotations

All `io.skillimage.*` annotations are optional. Images packed before
this change will not have `tags`, `compatibility`, or `wordcount`.
Treat missing keys as absent/unknown:

```
tags:          missing → show no tags (empty array)
compatibility: missing → show no badge or "Any"
wordcount:     missing → hide complexity indicator
```

### Recommended catalog listing flow

```
1. GET /v2/_catalog                          → repo names
2. GET /v2/<repo>/tags/list                  → tag names
3. GET /v2/<repo>/manifests/<tag>            → manifest JSON
   Accept: application/vnd.oci.image.manifest.v1+json
4. Read manifest.annotations                → all catalog card data
```

No blob downloads needed. For the skill detail page (full SKILL.md
content), continue fetching the layer blob on demand for the
individual skill.

## Standards compliance: building with podman/buildah

The image `skillctl pack` produces is a standard OCI image,
reproducible with `podman build` or `buildah` without any custom
tooling.

**Containerfile + podman:**

```dockerfile
FROM scratch
COPY skill.yaml SKILL.md /
```

```bash
podman build \
  --annotation "org.opencontainers.image.title=K8s Troubleshooter" \
  --annotation "org.opencontainers.image.version=1.0.0" \
  --annotation "io.skillimage.status=draft" \
  --annotation 'io.skillimage.tags=["kubernetes","debugging"]' \
  -t registry.example.com/skills/k8s-troubleshoot:1.0.0-draft .
```

**buildah (no Containerfile):**

```bash
ctr=$(buildah from scratch)
buildah copy "$ctr" ./my-skill/ /
buildah config \
  --annotation "org.opencontainers.image.title=K8s Troubleshooter" \
  --annotation "io.skillimage.status=draft" \
  "$ctr"
buildah commit "$ctr" registry.example.com/skills/k8s-troubleshoot:1.0.0-draft
```

**What `skillctl pack` adds on top of standard tools:**

| Concern | podman/buildah (manual) | skillctl (automatic) |
| ------- | ---------------------- | -------------------- |
| Containerfile | Write yourself | Not needed |
| Word count | `wc -w SKILL.md` | Computed during pack |
| Annotations | Type each flag | Auto-mapped from `skill.yaml` |
| SkillCard validation | None | JSON Schema validation |
| Lifecycle state | Manual | Enforces `draft` initial state |
| Version format | Unchecked | Validates semver |
