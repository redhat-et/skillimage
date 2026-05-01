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

## Install

### Homebrew (macOS / Linux)

```bash
brew install pavelanni/tap/skillctl
```

### Install script

```bash
curl -fsSL https://raw.githubusercontent.com/redhat-et/skillimage/main/install.sh | sh
```

To install a specific version or to a custom directory:

```bash
curl -fsSL https://raw.githubusercontent.com/redhat-et/skillimage/main/install.sh \
  | VERSION=0.1.0 INSTALL_DIR=~/.local/bin sh
```

### Go install

```bash
go install github.com/redhat-et/skillimage/cmd/skillctl@latest
```

### Container image

```bash
podman run --rm ghcr.io/redhat-et/skillctl:latest version
```

### From source

```bash
make build    # produces bin/skillctl
```

### GitHub releases

Pre-built binaries for Linux, macOS, and Windows (amd64/arm64)
are available on the
[releases page](https://github.com/redhat-et/skillimage/releases).

## Quick start

### Create a skill

A skill is a directory with a `skill.yaml` (SkillCard metadata)
and a `SKILL.md` (prompt content). See `examples/hello-world/`
for a working example.

```yaml
apiVersion: skillimage.io/v1alpha1
kind: SkillCard
metadata:
  name: hello-world
  namespace: examples
  version: 1.0.0
  description: A simple example skill.
spec:
  prompt: SKILL.md
```

The `namespace` field groups skills locally. When you run
`skillctl list`, skills display as `namespace/name` (e.g.,
`examples/hello-world`). The namespace is a logical grouping
within the skill card and is independent of the remote registry
path.

### Validate, build, and inspect

```bash
# Validate the SkillCard against the JSON Schema
bin/skillctl validate examples/hello-world/

# Build into a local OCI image (tagged as 1.0.0-draft)
bin/skillctl build examples/hello-world/

# List local images
bin/skillctl list

# Inspect metadata and OCI details
bin/skillctl inspect examples/hello-world:1.0.0-draft
```

### Lifecycle promotion

```bash
# Promote draft -> testing (retagged as 1.0.0-testing)
bin/skillctl promote examples/hello-world:1.0.0-draft --to testing --local

# Promote testing -> published (retagged as 1.0.0 + latest)
bin/skillctl promote examples/hello-world:1.0.0-testing --to published --local
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

We recommend aligning the remote registry path with the skill's
`namespace` field. For example, a skill with `namespace: business`
would push to `quay.io/myorg/business/hello-world:1.0.0-draft`.
This is a convention, not enforced by skillctl, but it makes it
easier to find skills in both local and remote listings.

Authentication uses your existing `~/.docker/config.json` or
Podman's `auth.json` -- no separate login needed.

### Install skills for AI agents

Install skills directly into an agent's skill directory. If the
image isn't in the local store, skillctl pulls it automatically.

```bash
# Install to Claude Code's skill directory
skillctl install quay.io/myorg/hello-world:1.0.0 --target claude

# Install to another agent
skillctl install quay.io/myorg/hello-world:1.0.0 --target opencode

# Install to a custom directory
skillctl install quay.io/myorg/hello-world:1.0.0 -o ~/custom/skills/
```

Supported targets: `claude`, `cursor`, `windsurf`, `opencode`,
`openclaw`.

After installing, skillctl records provenance (source registry
and digest) in the skill's `skill.yaml` so upgrades can find
the original source.

### List and upgrade installed skills

```bash
# List installed skills across all agent directories
skillctl list --installed

# List installed skills for a specific agent
skillctl list --installed --target claude

# Check which skills have newer published versions
skillctl list --installed --upgradable

# Upgrade a specific skill
skillctl upgrade hello-world --target claude

# Upgrade all skills for an agent
skillctl upgrade --all --target claude
```

### Remove local images

```bash
# Remove a skill image from the local store
skillctl rm examples/hello-world:1.0.0-draft

# Remove multiple images
skillctl rm ref1:tag ref2:tag

# Skip confirmation prompt
skillctl rm examples/hello-world:1.0.0-draft --force
```

### Inspect with standard tools

Skill images are standard OCI images. You can inspect metadata
from any registry without downloading the image:

```bash
# View all metadata annotations (no image download)
skopeo inspect docker://quay.io/myorg/hello-world:1.0.0

# Get just the skill tags
skopeo inspect docker://quay.io/myorg/hello-world:1.0.0 \
  | jq -r '.Annotations["io.skillimage.tags"]'
# → ["example","getting-started"]

# Get lifecycle status
skopeo inspect docker://quay.io/myorg/hello-world:1.0.0 \
  | jq -r '.Annotations["io.skillimage.status"]'
# → published
```

This works because all skill metadata is stored in OCI manifest
annotations, not inside the image layers. A catalog UI or CI
pipeline can read skill metadata with a single manifest fetch.

### Using skill images

Since skills are standard OCI images, you can mount them directly
into running containers with `--mount type=image`:

```bash
podman pull quay.io/myorg/hello-world:1.0.0
podman run --rm \
  --mount type=image,source=quay.io/myorg/hello-world:1.0.0,destination=/skills \
  my-agent:latest
```

This works with both Podman and Docker on Linux. On macOS, the
remote client does not support `--mount type=image`, but you
can create an image-backed volume instead:

```bash
podman pull quay.io/myorg/hello-world:1.0.0
podman volume create --driver image \
  --opt image=quay.io/myorg/hello-world:1.0.0 hello-world-skill
podman run --rm -v hello-world-skill:/skills:ro my-agent:latest
```

For Kubernetes / OpenShift, use
[ImageVolumes](https://kubernetes.io/docs/tasks/configure-pod-container/image-volumes/)
(beta in K8s 1.33) to mount skills directly into pods:

```yaml
volumes:
  - name: skill
    image:
      reference: quay.io/myorg/hello-world:1.0.0
      pullPolicy: IfNotPresent
containers:
  - name: agent
    volumeMounts:
      - name: skill
        mountPath: /skills
        readOnly: true
```

#### Example: mounting a skill into an agent runtime

This example pulls a document-summarizer skill from the registry
and mounts it into [OpenCode](https://github.com/anomalyco/opencode),
an open-source AI coding agent with a terminal UI:

```bash
podman pull quay.io/skillimage/business/document-summarizer:1.0.0-testing
podman volume create --driver image \
  --opt image=quay.io/skillimage/business/document-summarizer:1.0.0-testing \
  summarizer-skill
podman run -it --rm \
  -v summarizer-skill:/root/.config/opencode/skills/document-summarizer \
  ghcr.io/anomalyco/opencode
```

OpenCode discovers the skill automatically. Running `/skills`
in the TUI shows the document-summarizer, and the agent uses it
when asked to summarize a document or web page.

### Running skillctl on OpenShift

You can run skillctl directly on an OpenShift cluster to inspect
images in the internal registry. First, create a secret with your
registry credentials:

```bash
oc create secret docker-registry skillctl-auth \
  --docker-server=image-registry.openshift-image-registry.svc:5000 \
  --docker-username=unused \
  --docker-password="$(oc whoami -t)"
```

Then run skillctl as a pod with the secret mounted:

```bash
oc run skillctl --rm -i --restart=Never \
  --image=ghcr.io/redhat-et/skillctl:latest \
  --overrides='{"spec":{"containers":[{"name":"skillctl","image":"ghcr.io/redhat-et/skillctl:latest","args":["inspect","--tls-verify=false","image-registry.openshift-image-registry.svc:5000/NAMESPACE/SKILL@sha256:DIGEST"],"volumeMounts":[{"name":"auth","mountPath":"/home/skillctl/.docker"}]}],"volumes":[{"name":"auth","secret":{"secretName":"skillctl-auth","items":[{"key":".dockerconfigjson","path":"config.json"}]}}]}}'
```

Use `--tls-verify=false` for the internal registry (self-signed
certificate). The `-i` flag ensures `--rm` cleans up the pod
after it completes. The token from `oc whoami -t` is typically
valid for 24 hours; recreate the secret when it expires.

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
| `pkg/oci/` | OCI image build/push/pull/inspect/promote |
| `pkg/installed/` | Installed skill discovery and upgrade checking |
| `pkg/lifecycle/` | State machine, tag rules |
| `pkg/source/` | Remote Git source resolution |
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
  +-- skillcard/   parse, validate, serialize
  +-- oci/         build, push, pull, inspect, promote
  +-- installed/   scan, upgrade checking
  +-- lifecycle/   state machine, tag rules
  +-- source/      remote Git source resolution
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
| testing | `<ver>-testing` | `1.0.0-testing` |
| published | `<ver>` + `latest` | `1.0.0` |
| deprecated | `<ver>` | `1.0.0` |
| archived | tag removed | digest only |

Status is stored in OCI manifest annotations
(`io.skillimage.status`), not inside the image. Image content
is immutable across promotions.

## Similar projects

Several projects explore packaging AI agent skills as OCI
artifacts. We share the same vision and welcome collaboration.

| Project | Author | Approach |
| ------- | ------ | -------- |
| [Agent Skills OCI Artifacts Spec](https://github.com/ThomasVitale/agents-skills-oci-artifacts-spec) | Thomas Vitale | Specification for skills as ORAS artifacts with Arconia CLI |
| [skills-oci](https://github.com/salaboy/skills-oci) | Mauricio Salatino | CLI for skills as OCI artifacts with SLSA provenance and SBOMs |
| **skillctl** (this project) | Red Hat OCTO | Skills as OCI images for multi-user OpenShift/K8s with lifecycle management, read-only ImageVolume mounting, and standard container tooling |

## License

Apache-2.0
