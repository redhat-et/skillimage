# OCI Skill Registry — Implementation spec (milestone 1)

**Date:** 2026-04-16
**Status:** Draft
**Prerequisite:** `2026-04-15-oci-skill-registry-design.md`

## Goal

Deliver a working `skillctl` CLI with lifecycle management,
proving the core value proposition: skills as OCI images with
lifecycle governance on top of standard container tooling.

## Scope

### In scope

- `pkg/skillcard/` — parse, validate, serialize SkillCard YAML
- `pkg/oci/` — pack, push, pull, inspect, promote + local store
- `pkg/lifecycle/` — state machine, tag rules, status annotations
- `cmd/skillctl/` + `internal/cli/` — CLI commands
- `schemas/skillcard-v1.json` — JSON Schema for SkillCard
- `examples/hello-world/` — sample skill for testing
- Testing with oras-go in-memory registry

### Deferred (phase 2+)

| Feature | Reason |
| ------- | ------ |
| REST API server + SQLite | Not needed for CLI-only milestone |
| Signing/verification (RHTAS) | Requires infrastructure setup |
| Multi-skill bundles (`pack --bundle`) | Single-skill images prove the model |
| Dependency resolution (`skills.json` / `skills.lock.json`) | Adds significant complexity |
| Import/export (Agent Skills format) | Nice-to-have, not core |
| `skillctl diff` | Needs `pkg/diff/`, not core |
| `skillctl config` | Env vars sufficient for now |
| Deploy artifacts (Dockerfile, Kustomize) | No server to deploy |
| `skillctl rmi` (remove local images) | Easy follow-on to `skillctl images` |
| Eval signals, search, UI support | Server features |

## Key design decisions

### Lifecycle status lives in OCI annotations

The SkillCard YAML inside the image has no `status` field.
Status is stored as an OCI manifest annotation
(`io.skillregistry.status`). This keeps image content immutable
across promotions — the same layer digest from draft through
published. Promotion updates annotations and retags without
repacking.

**Security implication:** Image-mounted skills (K8s ImageVolumes)
are guaranteed immutable by the kubelet. Skills pulled to writable
volumes can be modified — consumers should verify the digest
against the registry to confirm approval status.

### OCI images, not ORAS artifacts

Skills are standard OCI images (`FROM scratch` equivalent) so
they work with `podman`, `skopeo`, `crane`, and Kubernetes
ImageVolumes. See the design doc for the full rationale.

### apiVersion

`skillimage.io/v1alpha1` — project-owned domain, vendor-neutral.

## SkillCard schema

**File:** `schemas/skillcard-v1.json`

### Required fields

| Field | Constraints |
| ----- | ----------- |
| `apiVersion` | Must be `skillimage.io/v1alpha1` |
| `kind` | Must be `SkillCard` |
| `metadata.name` | 1-64 chars, `[a-z0-9-]`, no leading/trailing/consecutive hyphens |
| `metadata.namespace` | 1-128 chars, `[a-z0-9-/]`, each segment follows name rules |
| `metadata.version` | Valid semver |
| `metadata.description` | Non-empty string |

### Optional fields

`metadata.display-name`, `metadata.license`, `metadata.compatibility`,
`metadata.tags`, `metadata.authors`, `metadata.allowed-tools`,
`provenance.*`, `spec.*`

### Field naming

Kebab-case in YAML (SkillCard files), snake_case in JSON (future
API responses). Go structs use explicit tags:

```go
type Metadata struct {
    Name        string `yaml:"name" json:"name"`
    DisplayName string `yaml:"display-name" json:"display_name"`
}
```

### OCI annotation mapping

Populated at pack time from SkillCard fields:

| OCI annotation | Source |
| -------------- | ------ |
| `org.opencontainers.image.title` | `metadata.display-name` |
| `org.opencontainers.image.description` | `metadata.description` (first 256 chars) |
| `org.opencontainers.image.version` | `metadata.version` |
| `org.opencontainers.image.authors` | `metadata.authors[]` (comma-separated) |
| `org.opencontainers.image.licenses` | `metadata.license` |
| `org.opencontainers.image.vendor` | `metadata.namespace` |
| `org.opencontainers.image.created` | RFC 3339 timestamp at pack time |
| `org.opencontainers.image.source` | `provenance.source` |
| `org.opencontainers.image.revision` | `provenance.commit` |
| `io.skillregistry.status` | Lifecycle state (custom annotation) |

## Package design

### `pkg/skillcard/`

```go
// Parse deserializes a SkillCard from YAML.
func Parse(r io.Reader) (*SkillCard, error)

// Validate checks a SkillCard against the embedded JSON Schema.
// Returns structured errors with field paths.
func Validate(sc *SkillCard) ([]ValidationError, error)

// Serialize writes a SkillCard as YAML.
func Serialize(sc *SkillCard, w io.Writer) error
```

JSON Schema is embedded via `go:embed` from `schemas/`.
Validation converts YAML to JSON in memory, then validates
against the schema using `santhosh-tekuri/jsonschema/v6`.
`ValidationError` includes field path and message.

### `pkg/lifecycle/`

```go
type State string

const (
    Draft      State = "draft"
    Testing    State = "testing"
    Published  State = "published"
    Deprecated State = "deprecated"
    Archived   State = "archived"
)

// ValidTransition returns true if from→to is allowed.
func ValidTransition(from, to State) bool

// TagForState returns the OCI tag for a version in a given state.
func TagForState(version string, state State) string
```

State machine: `draft → testing → published → deprecated → archived`

Transition gates:

| Transition | Gate |
| ---------- | ---- |
| `draft → testing` | `pkg/skillcard.Validate()` passes (JSON Schema) |
| `testing → published` | No automated gate in milestone 1 (manual decision) |
| `published → deprecated` | No gate (author decision) |
| `deprecated → archived` | No gate (author decision) |

Tag rules:

| State | Tag pattern | Example |
| ----- | ----------- | ------- |
| Draft | `<version>-draft` | `1.2.0-draft` |
| Testing | `<version>-testing` | `1.2.0-testing` |
| Published | `<version>` + `latest` | `1.2.0`, `latest` |
| Deprecated | `<version>` (no `latest`) | `1.2.0` |
| Archived | Tag removed, digest-only | n/a |

### `pkg/oci/`

```go
// Pack builds an OCI image from a skill directory.
// Validates the SkillCard first. Stores result in local store.
func Pack(dir string, opts PackOptions) (ocispec.Descriptor, error)

// Push copies an image from local store to a remote registry.
func Push(ctx context.Context, ref string, opts PushOptions) error

// Pull copies an image from a remote registry to local store.
// If Unpack is set in opts, extracts content to a directory.
func Pull(ctx context.Context, ref string, opts PullOptions) error

// Inspect returns SkillCard metadata and OCI manifest details.
// Works on both local and remote references.
func Inspect(ctx context.Context, ref string, opts InspectOptions) (*InspectResult, error)

// Promote transitions a skill to a new lifecycle state on the
// remote registry. Updates annotations and retags without
// pulling layer data.
func Promote(ctx context.Context, ref string, to lifecycle.State, opts PromoteOptions) error

// ListLocal returns all images in the local store.
func ListLocal(opts ListOptions) ([]LocalImage, error)
```

**Image structure:** single tar+gzip layer containing skill files
at root. Config media type is
`application/vnd.oci.image.config.v1+json`. Platform defaults to
`linux/amd64`.

**Local store:** OCI layout at `~/.local/share/skillctl/store/`,
managed via oras-go's `oci.Store`.

**Pull with auto-naming:** When `-o` points to an existing
directory, `Pull` creates a subdirectory named after
`metadata.name` from the SkillCard. When `-o` points to a
non-existent path, it uses that path as-is.

**Auth:** oras-go reads credentials from
`~/.docker/config.json` and
`${XDG_RUNTIME_DIR}/containers/auth.json`. No custom auth
implementation.

**Promote mechanics:** Fetches manifest from registry, creates
a new manifest with updated `io.skillregistry.status` annotation
referencing the same layer digests, pushes new manifest with new
tag, removes old tag. No layer data crosses the wire.

## CLI commands

**Root:** `skillctl [--format text|json] [--verbose]`

| Command | Description |
| ------- | ----------- |
| `skillctl validate <dir\|file>` | Validate SkillCard against JSON Schema |
| `skillctl pack <dir> [--tag <tag>]` | Pack into OCI image in local store |
| `skillctl push <ref>` | Push from local store to remote registry |
| `skillctl pull <ref> [-o <dir>]` | Pull from registry; optionally unpack |
| `skillctl inspect <ref>` | Show SkillCard + OCI metadata |
| `skillctl images` | List images in local store |
| `skillctl promote <ref> --to <state>` | Promote on remote registry |

### Exit codes

| Code | Meaning |
| ---- | ------- |
| 0 | Success |
| 1 | Validation or business logic error |
| 2 | File not found, parse error, or usage error |

### Default registry

Set via `SKILLCTL_REGISTRY` env var. When a reference has no
registry prefix, this value is prepended. Defaults to
`localhost:5000` for development.

## Dependencies

| Module | Version | Purpose |
| ------ | ------- | ------- |
| `oras.land/oras-go/v2` | v2.6.0 | OCI operations, local store, remote |
| `github.com/spf13/cobra` | v1.10.2 | CLI framework |
| `github.com/spf13/viper` | v1.21.0 | Config management |
| `github.com/santhosh-tekuri/jsonschema/v6` | v6.0.2 | JSON Schema validation |
| `github.com/Masterminds/semver/v3` | v3.4.0 | Semver parsing |
| `gopkg.in/yaml.v3` | v3.0.1 | YAML parsing |

## Testing strategy

- **Unit tests:** table-driven, per package
- **Integration tests:** oras-go `registry.NewInMemory()` for
  `pkg/oci/` tests (no external registry needed)
- **End-to-end tests:** optional `make test-e2e` against local Zot
- **Test fixture:** `examples/hello-world/` with valid `skill.yaml`
  and `SKILL.md`

## Project structure

```text
oci-skill-registry/
├── cmd/skillctl/
│   └── main.go
├── internal/cli/
│   ├── root.go
│   ├── validate.go
│   ├── pack.go
│   ├── push.go
│   ├── pull.go
│   ├── inspect.go
│   ├── images.go
│   └── promote.go
├── pkg/
│   ├── skillcard/
│   │   ├── skillcard.go
│   │   ├── validate.go
│   │   └── skillcard_test.go
│   ├── oci/
│   │   ├── pack.go
│   │   ├── push.go
│   │   ├── pull.go
│   │   ├── inspect.go
│   │   ├── promote.go
│   │   ├── store.go
│   │   └── oci_test.go
│   └── lifecycle/
│       ├── lifecycle.go
│       └── lifecycle_test.go
├── schemas/
│   └── skillcard-v1.json
├── examples/
│   └── hello-world/
│       ├── skill.yaml
│       └── SKILL.md
├── go.mod
├── Makefile
└── CLAUDE.md
```

## Build order

Approach C: schema-first, then vertical slices.

1. JSON Schema + `pkg/skillcard/` (parse, validate, serialize)
1. `skillctl validate` (first working command, CLI framework setup)
1. `pkg/oci/` pack + `skillctl pack` + `skillctl images`
1. `skillctl push` + `skillctl pull`
1. `skillctl inspect`
1. `pkg/lifecycle/` + `skillctl promote`

Each slice produces a working, testable increment.
