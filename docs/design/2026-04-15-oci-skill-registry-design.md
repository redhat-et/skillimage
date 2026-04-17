# OCI Skill Registry — Design Spec

**Date:** 2026-04-15
**Status:** Draft
**Audience:** OCTO internal (Red Hat)
**Working project name:** oci-skill-registry

## Problem

AI agent skills need a distribution and lifecycle management layer
built on open standards. Existing solutions either store only
metadata (agent-registry) or are tied to specific agent frameworks
(DocsClaw OCI skills). Enterprises require content distribution
with provenance guarantees, versioning, and lifecycle governance
accessible to both technical and non-technical users.

## Vision

Skills are applications. Apply the full SDLC: develop, test,
version, distribute, update, deprecate. OCI is the distribution
layer because it already has signing, verification, and registry
infrastructure that enterprises trust.

## Scope

An OCI-based registry that **stores actual skill content** as OCI
artifacts and manages their lifecycle. This is a companion to
[agentoperations/agent-registry](https://github.com/agentoperations/agent-registry),
which handles metadata and governance for agents, skills, and MCP
servers. Integration is via shared OCI registries (loosely coupled).

### In scope

- Go library (`pkg/`) for OCI skill operations
- `skillctl` CLI for developers and CI/CD
- REST API server with OpenAPI 3.1 spec
- Skill authoring UI support (API endpoints for visual editor)
- SkillCard schema compatible with
  [Agent Skills spec](https://agentskills.io/specification)
- Lifecycle state machine with semver-aware promotion
- Signature verification (cosign/sigstore)
- Podman, Kubernetes, and OpenShift delivery targets

### Out of scope (planned for future phases)

- RBAC and customizable approval workflows
- Trust score computation (separate concern)
- gRPC interface (deferred until real consumer exists)
- UI implementation (separate project by UI developer)

### Not in scope

- Agent or MCP server registry (handled by agent-registry)
- Runtime execution of skills (handled by agent runtimes)

## Architecture

### Standard OCI tooling compatibility

Users should be able to interact with skill images using the
tools they already know: `podman`, `skopeo`, `crane`, `oras`,
and standard registry APIs. `skillctl` adds lifecycle and
quality features on top, but must not be the only way to push,
pull, or inspect skills.

#### OCI images vs ORAS artifacts

This distinction is critical and drove a key design decision.
The OCI ecosystem has two manifest formats:

| Format | Config media type | Mountable as ImageVolume? | Standard tool support |
| ------ | ----------------- | ------------------------- | --------------------- |
| OCI Image Manifest | `application/vnd.oci.image.config.v1+json` | Yes — kubelet unpacks rootfs layers | podman, skopeo, crane, oras, docker |
| ORAS Artifact Manifest | `application/vnd.oci.empty.v1+json` | No — kubelet rejects: "mismatched image rootfs and manifest layers" | oras only |

Kubernetes ImageVolumes (beta in K8s 1.33+) require a valid
OCI container image with rootfs layers. An artifact pushed
via `oras push file.txt` creates an ORAS artifact manifest
that the kubelet cannot mount. This was confirmed in
prototyping: pushing a skill with `oras push` and attempting
to mount it as an ImageVolume fails with
`ImageVolumeMountFailed`. In earlier prototyping
(see `~/work/docsclaw`), this tension led to a `--as-image`
flag that produced both formats — a workaround, not a
solution.

**Decision:** Skills are packaged as standard OCI images
(built from `FROM scratch` Dockerfiles or equivalent). This
gives us:

- Kubernetes ImageVolumes work — kubelet mounts the rootfs
  as a read-only filesystem
- `podman pull / skopeo copy / crane pull` work out of the
  box
- `oras` can still push/pull (it handles OCI images, not
  just ORAS artifacts)
- Standard registry tooling (Quay, Zot, GHCR) indexes and
  garbage-collects them normally

#### Role of ORAS

ORAS remains valuable for **registry metadata** — artifacts
that live alongside the skill image as separate manifests,
discoverable via the OCI Referrers API:

| Attached artifact | Media type | Purpose |
| ----------------- | ---------- | ------- |
| Cosign signature | `application/vnd.cncf.cosign.signature.v1+json` | Provenance proof |
| SBOM | `application/vnd.cyclonedx+json` | Supply chain transparency |
| JSON Schema | `application/json` | SkillCard validation schema |

These are attached with `oras attach --subject <digest>` and
discovered with `oras discover`. They are **not** part of the
image rootfs and cannot be mounted as volumes. The agent does
not see them on the filesystem — they are consumed by tooling
(`skillctl verify`, CI scanners, registry UIs).

The `oras` artifact format can be supported as an import path
(`skillctl import --from-oras-artifact`) for ecosystems that
already publish skills that way, but the canonical format is
an OCI image.

Beyond format compatibility, `skillctl` and the `skill-puller`
sidecar should honor the standard container configuration files
that enterprises already manage:

| File               | Purpose                                        |
| ------------------ | ---------------------------------------------- |
| `registries.conf`  | Registry mirrors, search order, blocked registries |
| `policy.json`      | Image trust policy (signature requirements per registry/namespace) |
| `auth.json`        | Registry credentials (same format as `docker config.json`) |

The ORAS community is actively working on supporting these
files, which are already used by Podman, Skopeo, and CRI-O.
Reusing them means `skillctl` inherits the enterprise's
existing registry mirrors, trust policies, and credential
management without requiring separate configuration. For
example, an enterprise that mirrors `quay.io` to an internal
registry via `registries.conf` should see `skillctl pull`
follow the same mirror rules automatically, and a
`policy.json` that requires signatures from a specific Fulcio
issuer should be enforced by `skillctl verify` without
additional flags.

### Library-first design

The core is a Go library (`pkg/`). CLI and server are thin
consumers. This allows agent runtimes (DocsClaw, OpenClaw, any
framework), CI/CD pipelines, and agent-registry to import the
library directly.

```text
┌─────────────────────────────────────────────────┐
│  Consumers: skillctl CLI, REST API server,      │
│  agent runtimes, CI/CD pipelines                │
├─────────────────────────────────────────────────┤
│  pkg/ — Go library (public API)                 │
│  ├── skillcard/   SkillCard parse/validate      │
│  ├── oci/         pack/push/pull/inspect        │
│  ├── verify/      signature verification        │
│  ├── lifecycle/   state machine, versioning     │
│  └── diff/        version comparison            │
├─────────────────────────────────────────────────┤
│  Storage: OCI registries (quay.io, ghcr.io,     │
│  Zot) + local SQLite/Postgres for metadata      │
└─────────────────────────────────────────────────┘
```

### Two operating modes

| Operation         | CLI + OCI registry (no server) | Server + UI                                                |
| ----------------- | ------------------------------ | ---------------------------------------------------------- |
| Create/edit skill | Edit files locally             | Visual editor: metadata form + prompt textarea + templates |
| Pack + push       | `skillctl pack && push`        | "Create skill" / "Save" button                             |
| Pull              | `skillctl pull`                | Download button                                            |
| Inspect           | `skillctl inspect`             | Skill detail page                                          |
| Diff              | `skillctl diff`                | Side-by-side visual diff                                   |
| Verify signature  | `skillctl verify`              | Trust badge                                                |
| Promote           | —                              | Promote button with confirmation                           |
| Search/browse     | —                              | Search, filters, categories                                |
| Eval results      | —                              | Eval dashboard                                             |
| Dependency graph  | —                              | Interactive visualization                                  |

The server uses the same `pkg/` library. CLI and UI execute
identical code paths.

## Skill lifecycle

### State machine

```text
draft → testing → published → deprecated → archived
```

### Transitions

| Transition             | Gate                                                                    |
| ---------------------- | ----------------------------------------------------------------------- |
| draft → testing        | Schema validation passes, required SkillCard fields present             |
| testing → published    | Major version: full review. Minor/patch: lightweight review (diff-only) |
| published → deprecated | Author or admin decision. Skill still pullable, consumers warned        |
| deprecated → archived  | Retention policy or manual. Skill no longer pullable, metadata retained |

### Updates

When a published skill gets an update, a new version enters as
`draft` and proceeds through the pipeline independently. The
previous version stays `published` until the new one is ready.
This mirrors standard software release management.

### OCI tags by state

| State      | Tag pattern               | Example           |
| ---------- | ------------------------- | ----------------- |
| draft      | `<version>-draft`         | `1.2.0-draft`     |
| testing    | `<version>-testing`       | `1.2.0-testing`   |
| published  | `<version>` + `latest`    | `1.2.0`, `latest` |
| deprecated | `<version>` (no `latest`) | `1.2.0`           |
| archived   | tag removed, digest-only  | —                 |

The SkillCard `status` field is the authoritative source of truth.
Tags are a convenience. `skillctl promote` updates both atomically.

## SkillCard schema

Compatible with the
[Agent Skills spec](https://agentskills.io/specification),
extended for OCI distribution and lifecycle management.

```yaml
apiVersion: skills.octo.ai/v1
kind: SkillCard
metadata:
  name: hr-onboarding
  display-name: "HR Onboarding Guide"
  namespace: acme
  version: 1.2.0
  status: published
  description: >
    Guides new employees through onboarding steps.
    Use when a new hire asks about first-day procedures.
  license: Apache-2.0
  compatibility: "Requires network access"
  tags:
    - hr
    - onboarding
  authors:
    - name: Jane Smith
      email: jsmith@acme.com
  allowed-tools: "exec webfetch"
provenance:
  source: https://github.com/acme/hr-skills
  commit: a1b2c3d
  path: skills/onboarding/
spec:
  prompt: system-prompt.txt
  examples:
    - input: "I'm starting next Monday"
      output: "Welcome! Let me walk you through..."
  dependencies:
    - name: acme/company-policies
      version: ">=1.0.0"
```

### Field mapping from Agent Skills spec

| Agent Skills spec  | SkillCard field          | Notes                                   |
| ------------------ | ------------------------ | --------------------------------------- |
| `name`             | `metadata.name`          | Same constraints                        |
| `description`      | `metadata.description`   | Same constraints                        |
| `license`          | `metadata.license`       | Direct adoption                         |
| `compatibility`    | `metadata.compatibility` | Direct adoption                         |
| `metadata.author`  | `metadata.authors[]`     | Structured list                         |
| `metadata.version` | `metadata.version`       | Required, semver                        |
| `allowed-tools`    | `metadata.allowed-tools` | Space-separated per spec (experimental) |

### Extensions beyond the spec

| Field                 | Purpose                              |
| --------------------- | ------------------------------------ |
| `apiVersion` + `kind` | Schema versioning, K8s-style         |
| `display-name`        | Human-readable name for UI           |
| `namespace`           | Multi-tenant registry scoping        |
| `status`              | Lifecycle state                      |
| `tags`                | Search and filtering                 |
| `provenance`          | Optional source linkage              |
| `spec.prompt`         | File reference for prompt content    |
| `spec.examples`       | Structured example interactions      |
| `spec.dependencies`   | Skill composition with semver ranges |

### OCI annotation mapping

The OCI image spec defines
[pre-defined annotation keys](https://specs.opencontainers.org/image-spec/annotations/)
that standard tools use to display image metadata. `skillctl pack`
populates these annotations on the OCI manifest so that `podman
inspect`, `skopeo inspect`, `crane manifest`, and registry UIs
show meaningful metadata without any skill-specific tooling.

| OCI annotation                            | Source in SkillCard             | Notes                                         |
| ----------------------------------------- | ------------------------------- | --------------------------------------------- |
| `org.opencontainers.image.title`          | `metadata.display-name`        | Human-readable title                          |
| `org.opencontainers.image.description`    | `metadata.description`         | First 256 chars (registries may truncate)     |
| `org.opencontainers.image.version`        | `metadata.version`             | Semver string                                 |
| `org.opencontainers.image.authors`        | `metadata.authors[]`           | Comma-separated `name <email>` list           |
| `org.opencontainers.image.licenses`       | `metadata.license`             | SPDX expression                               |
| `org.opencontainers.image.vendor`         | `metadata.namespace`           | Org or team that published the skill          |
| `org.opencontainers.image.created`        | Build time                     | RFC 3339 timestamp, set at pack time          |
| `org.opencontainers.image.source`         | `provenance.source`            | Source repo URL                               |
| `org.opencontainers.image.revision`       | `provenance.commit`            | Git commit SHA                                |
| `org.opencontainers.image.url`            | Server URL or registry page    | Link to skill detail page, if server is configured |
| `org.opencontainers.image.documentation`  | —                              | Optional; link to skill usage docs            |

The following OCI annotations are not used:

| OCI annotation                            | Reason excluded                                    |
| ----------------------------------------- | -------------------------------------------------- |
| `org.opencontainers.image.ref.name`       | Used for image layout references, not relevant here |
| `org.opencontainers.image.base.digest`    | Skills are not layered on base images               |
| `org.opencontainers.image.base.name`      | Skills are not layered on base images               |

This mapping is one-directional: SkillCard fields are the source
of truth, and OCI annotations are derived from them at pack time.
`skillctl inspect` reads the SkillCard, not the annotations. The
annotations exist so that the broader OCI ecosystem — registries,
scanners, dashboards — can display skill metadata without
understanding the SkillCard schema.

### OCI artifact contents

```text
skill.yaml          # The SkillCard
system-prompt.txt   # Main prompt content
examples/           # Optional example files
assets/             # Optional supporting files
```

### Skill image packaging

Skills are built as OCI images from `FROM scratch` Dockerfiles.
The skill files are placed at the image root so that mounting
the image at a target path produces a clean directory layout
with no nested subdirectories.

#### Single-skill image (one skill per image)

This is the standard packaging model. Each skill is its own
OCI image with files at the root:

```dockerfile
FROM scratch
COPY skill.yaml SKILL.md /
COPY examples/ /examples/
```

Image filesystem:

```text
/
├── skill.yaml
├── SKILL.md
└── examples/
```

The agent pod mounts each skill image at a named subdirectory
under the skills path:

```yaml
spec:
  containers:
  - name: agent
    image: acme/my-agent:latest
    volumeMounts:
    - name: resume-reviewer
      mountPath: /agent/skills/resume-reviewer
      readOnly: true
    - name: blog-writer
      mountPath: /agent/skills/blog-writer
      readOnly: true
  volumes:
  - name: resume-reviewer
    image:
      reference: ghcr.io/acme/skill-resume-reviewer:1.0.0
      pullPolicy: IfNotPresent
  - name: blog-writer
    image:
      reference: ghcr.io/acme/skill-blog-writer:1.0.0
      pullPolicy: IfNotPresent
```

The agent sees:

```text
/agent/skills/
├── resume-reviewer/
│   ├── skill.yaml
│   ├── SKILL.md
│   └── examples/
└── blog-writer/
    ├── skill.yaml
    └── SKILL.md
```

This model gives independent versioning and lifecycle per
skill. Adding or removing a skill changes the pod spec
(one volume + one volumeMount).

#### Multi-skill image (skill bundle)

For enterprise distribution of curated skill sets, multiple
skills can be packaged in a single image. Each skill gets its
own subdirectory:

```dockerfile
FROM scratch
COPY resume-reviewer/ /resume-reviewer/
COPY blog-writer/     /blog-writer/
COPY policy-lookup/   /policy-lookup/
```

Image filesystem:

```text
/
├── resume-reviewer/
│   ├── skill.yaml
│   └── SKILL.md
├── blog-writer/
│   ├── skill.yaml
│   └── SKILL.md
└── policy-lookup/
    ├── skill.yaml
    └── SKILL.md
```

The agent pod mounts the entire bundle at the skills path
with a single volume:

```yaml
volumes:
- name: hr-skills-pack
  image:
    reference: ghcr.io/acme/hr-skills-pack:2.0.0
    pullPolicy: IfNotPresent
```

```yaml
volumeMounts:
- name: hr-skills-pack
  mountPath: /agent/skills
  readOnly: true
```

This maps to the "Skill collections and bundles" concept in
the landscape findings. A multi-skill image is a simple
alternative to OCI Image Indexes when the target platform
supports ImageVolumes and all skills in the bundle share the
same lifecycle.

#### Which packaging model to use

| Scenario | Recommended model |
| -------- | ----------------- |
| Independent skill development and versioning | Single-skill images |
| Curated enterprise skill packs | Multi-skill image |
| Mixed: some stable, some frequently updated | Single-skill images |
| Offline / air-gapped deployment with minimal image count | Multi-skill image |

`skillctl pack` produces single-skill images by default.
`skillctl pack --bundle <dir>` produces multi-skill images
from a directory containing multiple skill subdirectories.

### Interoperability

`skillctl import --from-skill ./SKILL.md` converts Agent Skills
format to SkillCard. `skillctl export --format skill-md` produces
the reverse. This keeps the project compatible without being locked
to the Agent Skills schema.

## Naming model

Three layers of naming support both technical and non-technical
users.

| Layer            | Rules                                              | Example                                     |
| ---------------- | -------------------------------------------------- | ------------------------------------------- |
| Display name     | Free-form UTF-8, max 128 chars                     | `Resume Reviewer (Strict)`                  |
| Skill identifier | `namespace/name`, lowercase + hyphens, max 64 each | `docsclaw/hr-resume-reviewer`               |
| OCI reference    | `registry/namespace/name:version`                  | `quay.io/docsclaw/hr-resume-reviewer:1.2.0` |

### Auto-conversion

The UI auto-generates identifiers from display names: lowercase,
spaces to hyphens, strip special characters, collapse consecutive
hyphens. Users can override before saving.

### In agent configs

```yaml
skills:
  - docsclaw/hr-resume-reviewer:1.2.0          # short form
  - quay.io/docsclaw/hr-resume-reviewer:1.2.0  # full OCI ref
```

The default registry is configurable via `skillctl config`.

### Nested namespaces

OCI registries support multi-level paths. The namespace is
everything between the registry host and the skill name.
Recommended: keep it shallow (org/team at most).

### Validation

| Field          | Constraint                                                       |
| -------------- | ---------------------------------------------------------------- |
| `name`         | 1-64 chars, `[a-z0-9-]`, no leading/trailing/consecutive hyphens |
| `namespace`    | 1-128 chars, `[a-z0-9-/]`, each segment follows name rules       |
| `display-name` | 1-128 chars, UTF-8                                               |
| `version`      | Valid semver                                                     |

### Agent runtime integration

How skills surface to end users (slash commands, menus, etc.) is
the agent runtime's responsibility. The SkillCard provides `name`,
`display-name`, `description`, and `tags` — sufficient metadata for
any runtime to present skills in its own style.

## REST API

**Base path:** `/api/v1`

### Skill CRUD and content

| Method | Path                                         | Description                              |
| ------ | -------------------------------------------- | ---------------------------------------- |
| POST   | `/skills`                                    | Create skill (SkillCard + content)       |
| GET    | `/skills`                                    | List (filter by namespace, tags, status) |
| GET    | `/skills/{ns}/{name}/versions`               | List versions                            |
| GET    | `/skills/{ns}/{name}/versions/{ver}`         | Get SkillCard                            |
| PUT    | `/skills/{ns}/{name}/versions/{ver}`         | Update draft                             |
| DELETE | `/skills/{ns}/{name}/versions/{ver}`         | Delete draft only                        |
| GET    | `/skills/{ns}/{name}/versions/{ver}/content` | Get prompt content                       |
| PUT    | `/skills/{ns}/{name}/versions/{ver}/content` | Update prompt content                    |

### Lifecycle

| Method | Path                                         | Description       |
| ------ | -------------------------------------------- | ----------------- |
| POST   | `/skills/{ns}/{name}/versions/{ver}/promote` | Promote           |
| GET    | `/skills/{ns}/{name}/versions/{ver}/history` | Promotion history |

### Discovery

| Method | Path                                              | Description      |
| ------ | ------------------------------------------------- | ---------------- |
| GET    | `/search?q=...&status=...&tags=...`               | Full-text search |
| GET    | `/skills/{ns}/{name}/versions/{ver}/diff/{ver2}`  | Diff             |
| GET    | `/skills/{ns}/{name}/versions/{ver}/dependencies` | Dependencies     |

### Eval signals

| Method | Path                                       | Description |
| ------ | ------------------------------------------ | ----------- |
| POST   | `/skills/{ns}/{name}/versions/{ver}/evals` | Attach eval |
| GET    | `/skills/{ns}/{name}/versions/{ver}/evals` | List evals  |

### System

| Method | Path            | Description  |
| ------ | --------------- | ------------ |
| GET    | `/healthz`      | Health check |
| GET    | `/openapi.yaml` | OpenAPI spec |

### Response format

```json
{
  "data": { },
  "_meta": { "request_id": "..." },
  "pagination": { "total": 42, "page": 1, "per_page": 20 }
}
```

Errors follow RFC 7807. OpenAPI spec served for UI client
generation.

## CLI (`skillctl`)

### Standalone (no server needed)

```shell
skillctl pack <dir>                          Pack into OCI artifact
skillctl push <oci-ref>                      Push to registry
skillctl pull <oci-ref> -o <dir>             Pull to local directory
skillctl inspect <oci-ref>                   Show SkillCard + manifest
skillctl verify <oci-ref>                    Verify cosign signature
skillctl diff <oci-ref> <ver1> <ver2>        Diff two versions
skillctl diff <oci-ref> --local <dir>        Diff against local
skillctl validate <dir>                      Validate SkillCard schema
skillctl import --from-skill <SKILL.md>      Import Agent Skills format
skillctl export <oci-ref> --format skill-md  Export as SKILL.md
```

### Server commands

```shell
skillctl serve --port 8080                   Start API server
skillctl search <query> --status published
skillctl promote <ns/name> <ver> --to <state>
skillctl history <ns/name> <ver>
skillctl eval attach <ns/name> <ver> --category <cat> --score <n>
skillctl eval list <ns/name> <ver>
```

### Config

```shell
skillctl config init                         Interactive setup
skillctl config set registry quay.io
skillctl config set server http://localhost:8080

Config path: ~/.config/skillctl/config.yaml
```

## Project structure

```text
oci-skill-registry/
├── cmd/skillctl/           Entry point
├── internal/
│   ├── cli/                Cobra commands
│   ├── handler/            HTTP handlers
│   ├── service/            Business logic
│   ├── store/              Storage interface + SQLite
│   └── server/             Router, middleware
├── pkg/
│   ├── skillcard/          Parse, validate, serialize
│   ├── oci/                Pack/push/pull (oras-go)
│   ├── verify/             Sigstore verification
│   ├── lifecycle/          State machine
│   └── diff/               Version comparison
├── schemas/                JSON Schema for SkillCard
├── api/openapi.yaml        OpenAPI 3.1 spec
├── deploy/
│   ├── Dockerfile
│   └── k8s/                Kustomize overlays (k8s, openshift)
├── examples/               Sample skills
├── docs/                   Architecture, ADRs
└── Makefile
```

## Skill delivery to agents

Skills reach running agents through two complementary modes.
Both use the same OCI image format (see "Standard OCI tooling
compatibility" above), so the choice is a deployment decision,
not a packaging decision.

### Mode 1: Image volume mount (static)

The skill image is mounted as a read-only volume directly into
the agent pod. The kubelet pulls and unpacks the image; the
agent reads skill files from the mount path.

| Platform               | Mechanism                                      |
| ---------------------- | ---------------------------------------------- |
| K8s 1.33+ / OCP 4.20+ | Native image volumes (`spec.volumes[].image`)  |
| Older K8s / OCP        | Init container copies skill to emptyDir        |
| Local dev (Podman)     | `podman run --mount type=image,...`             |

**Advantages:** Read-only filesystem — the agent cannot modify
skill content at runtime. No additional components needed beyond
the kubelet. Signature verification can be enforced at the
admission level via Policy Controller.

**Limitations:** Adding or updating a skill requires a pod
configuration change and a restart. Acceptable for stable
production deployments, but not for iterative development or
agents that need to adapt their skill set at runtime.

### Mode 2: Pull into agent storage (dynamic)

A sidecar or in-agent component pulls skill images from the
registry into a writable volume. The agent watches the volume
for new or updated skills.

| Storage backend | Characteristics                               |
| --------------- | --------------------------------------------- |
| PVC             | Survives pod restarts; shared across replicas |
| emptyDir        | Ephemeral; skills re-pulled on restart        |

The skill delivery layer must be agent-agnostic. The registry
serves skills to any agent engine or framework — LangGraph,
CrewAI, AutoGen, Bee, custom agents — not just our own. This
rules out designs that require agents to integrate a specific
library or adopt a proprietary protocol. The sidecar approach
is preferred precisely because it is transparent: the agent
sees a directory of skill files appear on a volume mount, with
no awareness of how they got there.

The pull component is either:

- **Sidecar container** (`skill-puller`): runs alongside the
  agent, watches a skill manifest (e.g., `skills.lock.json`)
  or an API endpoint for changes, pulls new versions, and
  writes them to the shared volume. The agent detects changes
  via filesystem watch or polling. This is the recommended
  approach because it works with any agent runtime — the agent
  only needs to read files from a directory.
- **In-agent library call**: the agent imports `pkg/oci` and
  pulls skills directly. Simpler architecture, but couples the
  agent to our Go library. Appropriate only when building
  agents on top of our own `pkg/` stack.

**Advantages:** Skills can be added, updated, or removed
without restarting the agent. Supports dynamic skill assignment
(e.g., an orchestrator grants a skill to an agent mid-session).
PVC storage means skills survive pod restarts without re-pulling.

**Limitations:** The skill files are writable. Mitigations:

- The sidecar can set file permissions to read-only after
  writing and run as a different user than the agent.
- A `ReadOnlyMany` PVC mode prevents the agent from writing,
  though this requires a CSI driver that supports it.
- Signature re-verification on load (the agent calls
  `pkg/verify` before trusting skill content) catches
  tampering regardless of filesystem permissions.

### Which mode to use

| Scenario                                | Recommended mode  |
| --------------------------------------- | ----------------- |
| Production, stable skill set            | Image mount       |
| Development and testing                 | Pull (emptyDir)   |
| Agent needs dynamic skill assignment    | Pull (PVC)        |
| Compliance requires immutable workloads | Image mount       |
| Multi-agent cluster sharing a skill set | Pull (shared PVC) |

Both modes are first-class. `skillctl` and the server API
support both: image mount requires no special support (standard
`podman pull` or K8s image reference), and pull mode is served
by `skillctl pull` or the sidecar watching the registry.

## Delivery targets (summary)

| Target                       | Mechanism                                    |
| ---------------------------- | -------------------------------------------- |
| Local dev (Podman)           | `podman pull` + mount, or `skillctl pull`    |
| K8s 1.33+ / OpenShift 4.20+ | Image volumes (static mode)                  |
| Older K8s                    | Init container or skill-puller sidecar       |
| Dynamic skill management     | skill-puller sidecar + PVC                   |
| CI/CD                        | `skillctl pack && skillctl push` in pipeline |
| UI developer                 | OpenAPI spec → generated typed client        |

## Signing with Red Hat Trusted Artifact Signer

### Overview

Skill signing provides cryptographic proof that a specific OCI image
was reviewed and approved by an authorized person within the
organization. It ties the lifecycle governance model — the
`testing → published` promotion — to a verifiable identity claim,
giving skill consumers a machine-checkable answer to the question:
*who approved this, and has the image been tampered with since?*

The signing infrastructure is provided by
[Red Hat Trusted Artifact Signer](https://developers.redhat.com/products/trusted-artifact-signer/overview)
(RHTAS), which is the enterprise-supported, self-managed deployment
of the [Sigstore](https://sigstore.dev/) project. RHTAS runs on
OpenShift via an Operator and requires no external network access —
signatures and transparency log entries stay inside the cluster.

### Components used

| Component                        | Role                                                                                 |
| -------------------------------- | ------------------------------------------------------------------------------------ |
| Fulcio                           | Certificate authority; issues short-lived signing certificates tied to OIDC identity |
| Rekor                            | Append-only transparency log; records every signing event for audit                  |
| Timestamp Authority (TSA)        | RFC 3161 timestamps; proves the certificate was valid at signing time                |
| TUF repository                   | Distributes trust roots (Fulcio cert, Rekor public key, TSA cert chain) to clients   |
| Red Hat build of Keycloak (RHBK) | OIDC provider; the source of approver identity for Fulcio                            |
| `cosign` / `sigstore-go`         | Client library; used server-side inside the `oci-skill-registry` API                 |

RHTAS manages all of these as Kubernetes custom resources via the
`SecureSign` operator. The `pkg/verify` package uses the
`sigstore-go` library rather than shelling out to the `cosign`
binary, so the dependency is a Go library, not a CLI tool.

### When signing happens

Signing is part of the `testing → published` promotion, not a
separate step. The approver clicks a single **Publish** button (UI)
or runs `skillctl promote <ns/name> <ver> --to published` (CLI).
The server executes the following sequence atomically:

1. Verify the caller has the `skill:publish` role in their OIDC
   token claims.
1. Resolve the OCI digest of the image at the `<version>-testing` tag.
1. Exchange the caller's OIDC token for a short-lived Fulcio
   certificate. The certificate subject is the approver's email
   address (or a workload identity for CI-initiated promotions).
1. Sign the digest with the ephemeral key. The private key is
   discarded immediately after use.
1. Upload the `.sig` artifact to the OCI registry alongside the
   image.
1. Record the signing event in Rekor. Receive a transparency log
   entry URL.
1. Transition the skill state to `published`, update OCI tags, and
   persist the signing metadata in the SkillCard and the database.

If any step fails (Fulcio unreachable, authorization denied, OCI
push error), the promotion is rolled back and the skill remains in
`testing`. There is no partially-signed state.

### SkillCard provenance extension

The `provenance` block is extended to capture the signing result:

```yaml
provenance:
  source: https://github.com/acme/hr-skills
  commit: a1b2c3d
  path: skills/onboarding/
  signing:
    signer: jsmith@acme.com
    issuer: https://keycloak.acme.internal/realms/skills
    signed-at: "2026-04-15T14:32:00Z"
    digest: "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a4"
    rekor-entry: "https://rekor.acme.internal/api/v1/log/entries/abc123"
```

This makes signing metadata visible in `skillctl inspect` and in the
UI skill detail page without a live Rekor query on every page load.

### API changes

One new endpoint and one extension to the promote endpoint:

| Method | Path                                           | Description                                                   |
| ------ | ---------------------------------------------- | ------------------------------------------------------------- |
| `GET`  | `/skills/{ns}/{name}/versions/{ver}/signature` | Return signature status and Rekor entry URL                   |
| `POST` | `/skills/{ns}/{name}/versions/{ver}/verify`    | Re-verify signature against RHTAS Rekor (for background jobs) |

The existing `POST /skills/{ns}/{name}/versions/{ver}/promote` gains
a `signed` field in its response:

```json
{
  "data": {
    "status": "published",
    "signed": true,
    "signer": "jsmith@acme.com",
    "rekor_entry": "https://rekor.acme.internal/api/v1/log/entries/abc123"
  }
}
```

### CLI changes

```shell
skillctl sign <oci-ref>      Sign explicitly (CI use case, keyless via OIDC)
skillctl verify <oci-ref>    Verify signature; exits non-zero if unsigned or invalid
```

`skillctl verify` is already planned. The `--show-rekor` flag adds
the Rekor entry URL to the output.

For `skillctl sign` and `skillctl verify` to use the internal RHTAS
instance rather than the public Sigstore infrastructure, clients must
initialize TUF from the enterprise trust root once:

```shell
cosign initialize \
  --mirror=$RHTAS_TUF_URL \
  --root=$RHTAS_TUF_URL/root.json
```

`skillctl config init` should offer to do this automatically when an
RHTAS endpoint is configured.

### UI trust badge

The skill detail page shows a trust badge derived from the signature
status:

| Badge             | Condition                                                              |
| ----------------- | ---------------------------------------------------------------------- |
| ✅ Signed          | Signature valid, Rekor entry confirmed, signer matches expected issuer |
| ⚠️ External signer | Signed, but issuer is outside the configured trusted issuers list      |
| ❌ Unsigned        | No signature found; skill is in draft or testing state                 |
| ❌ Invalid         | Signature present but verification failed                              |

Clicking the badge opens a panel showing: signer email, issuer,
signing timestamp, and a link to the Rekor entry. This gives
non-technical users a clear signal without exposing the underlying
cryptography.

### RHTAS deployment notes

RHTAS is installed on OpenShift via the Operator Lifecycle Manager.
The `SecureSign` custom resource declares all components. Key
configuration points for this project:

- **OIDC issuer:** configure Fulcio to trust the same Keycloak realm
  used by the `oci-skill-registry` API. Multiple issuers can be
  declared in `spec.fulcio.config.OIDCIssuers`, which supports
  federating external IdPs if needed.
- **Supported OCP versions:** RHTAS 1.x supports OpenShift 4.14 and
  later. The current release is 1.3 (as of late 2025).
- **High availability:** RHTAS 1.3 introduced pod affinity rules,
  replica configuration, and external Redis support for Rekor search,
  which makes it suitable for production deployments.
- **Transparency log monitoring:** RHTAS 1.3 runs a Rekor log monitor
  alongside the deployment to continuously validate that the log
  remains append-only and untampered.
- **Air-gapped environments:** all signing and verification happens
  against internal endpoints; no external network calls are required
  at signing time. Clients initialize from the internal TUF mirror.

Detailed installation steps are in the
[RHTAS Deployment Guide](https://access.redhat.com/documentation/en-us/red_hat_trusted_artifact_signer/1).

### Go library integration

The `pkg/verify` package wraps `sigstore-go` and exposes:

```go
// Sign signs the OCI image at digest using the caller's OIDC token.
// It contacts Fulcio for a certificate and records the event in Rekor.
func Sign(ctx context.Context, digest string, token string, opts SignOptions) (SignResult, error)

// Verify verifies the signature on the OCI image at ref.
// It checks the Rekor entry and validates the certificate against the
// configured RHTAS trust root.
func Verify(ctx context.Context, ref string, opts VerifyOptions) (VerifyResult, error)
```

`SignOptions` and `VerifyOptions` carry the RHTAS endpoint URLs
(Fulcio, Rekor, TUF mirror), which are read from the server
configuration at startup. This keeps the library transport-agnostic:
the same code works against public Sigstore for local development and
against RHTAS for production.

### Authorization model

Signing is gated by the `skill:publish` OIDC role claim, the same
claim that gates the `testing → published` state transition. There
is no separate signing permission to manage. This means:

- A user who can promote a skill also signs it — these are the same
  action.
- CI/CD pipelines that promote skills use a service account OIDC
  token; the Fulcio certificate subject is the service account
  identity (a URI, not an email address).
- The `signing.signer` field in the SkillCard records whichever
  identity performed the promotion.

RBAC customization (separate approver and publisher roles) is planned
as a future enhancement alongside configurable approval workflows.

### Relationship to future work items

This section replaces the "Full sigstore integration" bullet in the
future work list. What remains deferred:

- **Trust tiers:** using the signing identity to gate runtime tool
  permissions (e.g., a skill signed by a security-team account gets
  broader `allowed-tools` than one signed by a line-of-business
  approver).
- **Policy Controller integration:** enforcing `ClusterImagePolicy`
  rules at the Kubernetes admission level so that only
  RHTAS-signed skill images can be pulled into agent workloads.
- **SLSA provenance attestations:** attaching a signed SLSA
  provenance attestation (built-by, builder identity, source repo)
  as a cosign attestation alongside the signature.

## Key dependencies

| Dependency  | Purpose                               |
| ----------- | ------------------------------------- |
| oras-go     | OCI artifact operations               |
| cosign      | Signature verification                |
| cobra/viper | CLI and config                        |
| chi         | HTTP router                           |
| SQLite      | Metadata storage (Postgres swap path) |

## Integration with agent-registry

Both projects share OCI registries as the common layer.
agent-registry stores metadata and governance signals;
oci-skill-registry stores actual skill content. Integration
is loosely coupled — no direct API dependency.

Future convergence: `skillctl` commands may become
`agentctl push skills`, `agentctl promote skills`, etc.
This depends on collaboration with azaalouk (TBD).

## Findings from landscape research

Competitive analysis (see companion document
`2026-04-15-oci-skill-registry-landscape.md`) identified five
gaps worth addressing. Items marked with priority are recommended
for inclusion in the first release; the rest are future work.

### Trust tiers (future)

Microsoft's Agent Governance Toolkit uses trust-tiered capability
gating: skills at different trust levels get different runtime
permissions. A "draft" skill from an unknown author should not
get the same tool access as a "published" skill signed by a
trusted org. This extends our lifecycle states with a
permission model.

### Shadow skill detection (future)

JFrog AI Catalog detects unauthorized AI usage across an
enterprise. Applied to skills: what skills are agents actually
loading at runtime vs. what's approved in the registry? This
requires runtime telemetry integration and is a future concern,
but the API should support querying "who is using this skill."

### Skill collections and bundles (priority)

Thomas Vitale's OCI Skills spec proposes "collections" — OCI
Image Indexes that group related skills into discoverable
bundles. Example: an "HR Skills Pack" containing onboarding,
resume-review, and policy-lookup skills. This is useful for
enterprise distribution (install a curated set, not individual
skills) and aligns with our OCI-native approach.

**Action:** Support collections as a first-class concept.
A collection is an OCI Image Index referencing multiple skill
artifacts. `skillctl` should support `pack --collection`,
`push`, and `pull` for collections.

### Skill quality validation (future)

[SkillCheck-Free](https://github.com/olgasafonova/SkillCheck-Free)
is an MIT-licensed validator for AI agent skills that checks
compliance with the agentskills.io specification. The free tier
performs 22 checks across structural (frontmatter format, field
constraints), semantic (contradictions, ambiguity, routing
clarity), naming (vague names, gerunds, length), and quality
pattern recognition (examples, error handling, triggers). A
commercial Pro tier adds security scanning, anti-slop detection,
token budget analysis, WCAG accessibility, enterprise readiness
checks, and a standalone CI/CD binary.

SkillCheck is implemented as a skill itself — pure markdown
rules interpreted by an LLM at runtime, no compiled code. This
is elegant for interactive use but means automated validation
in a CI pipeline requires either an LLM call or a reimplementation
of the rules as traditional code.

**Relevance:** Our registry should validate skill quality at
submission time, not just schema correctness. A skill that passes
JSON Schema validation can still have vague descriptions, naming
violations, contradictory instructions, or hollow content that
makes it ineffective. Quality gating during lifecycle transitions
(draft → testing) would catch these issues before skills reach
consumers.

**Action (future):** Add a `skillctl lint` command and a
server-side quality gate. Phase 1: implement structural and
naming checks as deterministic Go code (no LLM dependency).
Phase 2: integrate LLM-based semantic checks (contradiction
detection, ambiguity scoring, knowledge density) as an optional
eval signal provider, reusing the eval attachment API. Evaluate
SkillCheck's rule set as a starting point for both phases.

### Security scanning integration (future)

Tessl integrates Snyk for vulnerability scanning of skill
content. The ClawHavoc incident (341 malicious skills on ClawHub
in February 2026) validates that skill content should be scanned.
Our registry should support pluggable security scanners as eval
signal providers, reusing the eval attachment API.

### Dependency resolution with lock files (priority)

Thomas Vitale's spec introduces `skills.json` (declarative
dependencies) and `skills.lock.json` (resolved digests), mirroring
npm's package management model. This is more mature than our
current `spec.dependencies` with semver ranges.

**Action:** Adopt a similar two-file model. `skills.json` declares
what skills an agent needs with version ranges. `skills.lock.json`
pins exact OCI digests for reproducible deployments. `skillctl`
resolves and locks.

### Alignment with Thomas Vitale's OCI Skills spec (priority)

Vitale's spec is gaining community traction and has reference
implementations (Arconia CLI in Java, skills-oci in Go). He is
a known contact. We should align our SkillCard schema and OCI
artifact layout with his spec rather than diverge.

**Action:** Engage Thomas Vitale early. Align on artifact type
identifiers, media types, and layer layout. Contribute our
lifecycle and signing extensions back to the community spec.

## Future work

- **RBAC and workflow customization:** configurable policies for
  who can author, approve, and deprecate skills
- **gRPC interface:** when programmatic consumers justify it
- **skill-puller sidecar:** reference implementation for dynamic
  mode (see "Skill delivery to agents")
- **Trust score computation:** separate service, potentially
  integrated with agent-registry eval signals
- **Trust tiers:** permission gating based on skill trust level
- **Shadow skill detection:** runtime telemetry for skill usage
  visibility
- **Security scanning:** pluggable scanners as eval signal
  providers
- **Skill quality validation:** `skillctl lint` for structural
  and naming checks (deterministic), LLM-based semantic checks
  (contradictions, ambiguity, knowledge density) as optional eval
  signals; informed by SkillCheck-Free rule set
