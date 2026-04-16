# OCI Skill Registry

A framework-agnostic, OCI-based registry for AI agent skills.
Packages skills as standard OCI images, manages their lifecycle
(draft, testing, published, deprecated, archived), and works with
the container tools enterprises already use: `podman`, `skopeo`,
`crane`, and Kubernetes ImageVolumes.

Companion to
[agentoperations/agent-registry](https://github.com/agentoperations/agent-registry)
(metadata and governance layer).

## Why OCI images?

Skills are packaged as standard OCI images (`FROM scratch`), not
ORAS artifacts. This means:

- `podman pull` / `skopeo copy` / `crane pull` work out of the box
- Kubernetes ImageVolumes can mount skills directly into agent pods
- Standard registries (Quay, GHCR, Zot) index and serve them normally
- Signing and verification via cosign/sigstore (planned)

## Quick start

### Build

```bash
make build
```

### Create a skill

A skill is a directory with a `skill.yaml` (SkillCard metadata)
and a `SKILL.md` (prompt content). See `examples/hello-world/`
for a working example.

```yaml
apiVersion: skills.redhat.io/v1alpha1
kind: SkillCard
metadata:
  name: hello-world
  namespace: examples
  version: 1.0.0
  description: A simple example skill.
spec:
  prompt: SKILL.md
```

### Validate, pack, and inspect

```bash
# Validate the SkillCard against the JSON Schema
bin/skillctl validate examples/hello-world/

# Pack into a local OCI image (tagged as 1.0.0-draft)
bin/skillctl pack examples/hello-world/

# List local images
bin/skillctl images

# Inspect metadata and OCI details
bin/skillctl inspect examples/hello-world:1.0.0-draft
```

### Lifecycle promotion

```bash
# Promote draft -> testing (retagged as 1.0.0-rc)
bin/skillctl promote examples/hello-world:1.0.0-draft --to testing --local

# Promote testing -> published (retagged as 1.0.0 + latest)
bin/skillctl promote examples/hello-world:1.0.0-rc --to published --local
```

Promotion updates OCI manifest annotations and retags without
modifying image content. The layer digest stays the same from
draft through published.

### Push and pull (remote registry)

```bash
# Push to a remote registry
bin/skillctl push quay.io/myorg/hello-world:1.0.0-draft

# Pull from a remote registry
bin/skillctl pull quay.io/myorg/hello-world:1.0.0 -o ./skills/
```

Authentication uses your existing `~/.docker/config.json` or
Podman's `auth.json` -- no separate login needed.

## Testing

```bash
make test        # Run all tests
make lint        # Run golangci-lint
make fmt         # Format code
```

## Project structure

| Path | Description |
| ---- | ----------- |
| `cmd/skillctl/` | CLI entry point |
| `internal/cli/` | Cobra commands |
| `pkg/skillcard/` | SkillCard parse, validate, serialize |
| `pkg/oci/` | OCI image pack/push/pull/inspect/promote |
| `pkg/lifecycle/` | State machine, tag rules |
| `schemas/` | JSON Schema for SkillCard |
| `examples/` | Sample skills |
| `docs/` | Design specs and research |

## Architecture

Library-first: core logic lives in `pkg/` as importable Go
packages. The CLI is a thin consumer. Agent runtimes, CI/CD
pipelines, and other tools can import the library directly.

```text
consumers: skillctl CLI, agent runtimes, CI/CD
      |
  pkg/ (public Go API)
  +-- skillcard/  parse, validate, serialize
  +-- oci/        pack, push, pull, inspect, promote
  +-- lifecycle/  state machine, tag rules
      |
  OCI registries (quay.io, ghcr.io, Zot)
```

## Lifecycle states

```text
draft --> testing --> published --> deprecated --> archived
```

| State | OCI tag | Example |
| ----- | ------- | ------- |
| draft | `<ver>-draft` | `1.0.0-draft` |
| testing | `<ver>-rc` | `1.0.0-rc` |
| published | `<ver>` + `latest` | `1.0.0` |
| deprecated | `<ver>` | `1.0.0` |
| archived | tag removed | digest only |

Status is stored in OCI manifest annotations
(`io.skillregistry.status`), not inside the image. Image content
is immutable across promotions.

## License

Apache-2.0
