---
marp: true
theme: default
paginate: true
size: 16:9
title: OCI-Based Skill Distribution
author: Pavel Anni
class: invert
style: |
  @import url('https://fonts.googleapis.com/css2?family=Red+Hat+Display:wght@400;500;700;900&family=Red+Hat+Text:wght@400;500;700&family=Red+Hat+Mono:wght@400;700&display=swap');
  :root {
    --color-background: #000000;
    --color-foreground: #ffffff;
    --color-highlight: #ee0000;
    --color-dimmed: #a3a3a3;
  }
  section {
    font-family: 'Red Hat Text', sans-serif;
    background: #000000;
    color: #ffffff;
  }
  section::after {
    color: #a3a3a3;
    font-family: 'Red Hat Mono', monospace;
    font-size: 0.7em;
  }
  h1, h2, h3 {
    font-family: 'Red Hat Display', sans-serif;
    color: #ffffff;
  }
  h1 { font-weight: 900; }
  h2 { font-weight: 700; }
  code {
    font-family: 'Red Hat Mono', monospace;
    background: #292929;
    border-radius: 3px;
  }
  pre {
    background: #1f1f1f;
    border: 1px solid #383838;
    border-radius: 3px;
  }
  a { color: #f56e6e; }
  strong { color: #ffffff; }
  em { color: #c7c7c7; }
  .accent { color: #ee0000; }
  .tag {
    display: inline-block;
    padding: 2px 14px;
    border: 1px solid #383838;
    border-radius: 64px;
    font-family: 'Red Hat Mono', monospace;
    font-size: 0.7em;
    letter-spacing: 0.05em;
    text-transform: uppercase;
    color: #c7c7c7;
    margin: 2px;
  }
  .tag-accent {
    display: inline-block;
    padding: 2px 14px;
    border: 1px solid #ee0000;
    border-radius: 64px;
    font-family: 'Red Hat Mono', monospace;
    font-size: 0.7em;
    letter-spacing: 0.05em;
    text-transform: uppercase;
    color: #ee0000;
    margin: 2px;
  }
  .muted { color: #a3a3a3; }
  .small { font-size: 0.75em; }
  section.lead h1 { font-size: 2.8em; }
  section.lead .subtitle {
    color: #a3a3a3;
    font-size: 1.1em;
    max-width: 680px;
  }
  table {
    font-size: 0.85em;
    background: transparent;
  }
  th {
    background: #1f1f1f;
    border-color: #383838;
  }
  td {
    border-color: #383838;
  }
  section.thankyou h1 {
    font-size: 5em;
    text-align: center;
  }
  section.thankyou {
    display: flex;
    flex-direction: column;
    justify-content: center;
    align-items: center;
    text-align: center;
  }
---

<!-- _class: lead -->

# Distributing AI Skills at <span class="accent">Enterprise Scale</span>

Package, sign, and distribute agent skills as OCI artifacts — using the same registries and trust chains you already run for container images.

<span class="tag-accent">OCI Artifacts</span> <span class="tag">ORAS</span> <span class="tag">Sigstore</span> <span class="tag">Image Volumes</span>

<span class="muted small">Pavel Anni · Office of CTO · Red Hat</span>

<!--
Tips: Arrow keys or click to navigate. Press N to toggle notes.
Project: github.com/redhat-et/skillimage
-->

---

## Your Agent Is Only as Good as Its <span class="accent">Skills</span>

Skills are the unit of specialization. Each skill gives the agent domain expertise.

- **SKILL.md** — instructions in Agent Skills spec format
- **skill.yaml** — metadata: versioning, compatibility, licensing

```
skills/
├── resume-screener/
│   ├── SKILL.md
│   └── skill.yaml
└── policy-comparator/
    ├── SKILL.md
    └── skill.yaml
```

<!--
Agent Skills Specification: agentskills.io/specification

Skills are the unit of specialization for an agent. Each skill is a self-contained
directory with instructions (SKILL.md) and optional metadata (skill.yaml). The agent
discovers and loads them at startup.

Think of skills like plugins: the agent provides the runtime, skills provide domain expertise.
-->

---

## Today's Skill Distribution Is <span class="accent">Ad-Hoc</span>

| Method | How it works | Drawback |
| ------ | ------------ | -------- |
| **Copy from a Friend** | Slack messages, email, shared drives | No versioning. No provenance. No audit trail. |
| **Clone from GitHub** | git clone the repo, copy the directory | No signing. No atomic versioning. Auth is coarse-grained. |
| **Mount a ConfigMap** | Embed skill text in a K8s ConfigMap | 1 MiB limit. No versioning. Mixes config with content. |

These get you started, but none provide the **versioning, signing, and auditability** that enterprise deployments require.

<!--
Each of these methods gets progressively closer to enterprise readiness —
ConfigMaps are already Kubernetes-native — but all three lack the supply chain
guarantees (signing, versioning, audit) that regulated environments require.

ConfigMaps have a 1 MiB size limit (etcd constraint), so larger skills with
examples or data files won't fit.
-->

---

## Enterprise Deployments Need <span class="accent">Trust Infrastructure</span>

- 🔒 **Signing** — Cryptographic proof of who published a skill and that it hasn't been tampered with
- 📋 **Auditability** — Registry logs show who pulled what, when, and where it was deployed
- 🔄 **Versioning** — Immutable tags, semantic versions, rollback to a known-good skill set
- 📌 **Reproducibility** — Pin skills by digest (sha256) for byte-identical deployments across environments
- 🔑 **Access control** — Registry RBAC, pull secrets, org-scoped namespaces — the same policies you use for images

<!--
This is the core motivation. When you deploy AI agents in a regulated enterprise,
you need the same supply chain guarantees you have for container images: signed
artifacts, vulnerability scanning, access control, and audit logs.

Reproducibility matters because you need to prove which exact skill version
produced an agent's output — especially in regulated industries (finance,
healthcare, government).
-->

---

## OCI Is Not Just for <span class="accent">Container Images</span>

The OCI Distribution Spec is **content-agnostic**. Any blob + manifest + media type = a valid artifact. **ORAS** makes this practical.

<span class="tag">Helm Charts</span> <span class="tag">WASM Modules</span> <span class="tag">ML Models</span> <span class="tag">Policy Bundles</span> <span class="tag-accent">Agent Skills</span>

```
┌────────────────────────────────────────────────────┐
│      OCI Registry (Quay / GHCR / Harbor / Zot)     │
└────────────────────────────────────────────────────┘
    ↓  Same protocol, same auth, same RBAC  ↓
┌──────────────┐  ┌──────────────┐  ┌──────────────┐
│  Container   │  │    Helm      │  │    Agent     │
│   Images     │  │   Charts     │  │   Skills ★   │
└──────────────┘  └──────────────┘  └──────────────┘
```

<!--
ORAS (OCI Registry As Storage): oras.land

The OCI Distribution Specification was designed to be content-agnostic. Media types
let registries and tools distinguish content without special handling. Helm charts
have been distributed as OCI artifacts since Helm 3.8 (2022). WASM modules, ML models,
and policy bundles are also distributed this way.

Key insight: your Quay/Harbor/GHCR registry can already store and serve non-image content.
No infrastructure changes needed.
-->

---

## A Skill Becomes an <span class="accent">OCI Artifact</span>

```
Skill Directory  →  skillctl build  →  OCI Image  →  skillctl push  →  Registry
```

**What goes into the image:**
- `SKILL.md` — instructions
- `skill.yaml` — SkillCard metadata
- Any supporting files in the directory

**What the registry provides:**
- Immutable digest + mutable tags
- RBAC & pull secrets
- Cosign / sigstore signatures

<!--
Skills are packaged as standard OCI images (FROM scratch), not ORAS artifacts.
This means podman pull, skopeo copy, and Kubernetes ImageVolumes all work natively.

Media types used:
- application/vnd.oci.image.layer.v1.tar+gzip (skill content layer)
- application/vnd.oci.image.config.v1+json (config)

Skill metadata is stored in OCI manifest annotations (io.skillimage.*) for fast
inspection without pulling the full layer.
-->

---

## The Workflow: <span class="accent">Build, Push, Install</span>

```bash
# Build skill into a local OCI image
$ skillctl build skills/resume-screener/
Built resume-screener:1.0.0-draft (sha256:65af...)

# Tag and push to registry
$ skillctl tag resume-screener:1.0.0-draft \
    quay.io/myorg/resume-screener:1.0.0-draft
$ skillctl push quay.io/myorg/resume-screener:1.0.0-draft

# Install directly from registry to Claude Code
$ skillctl install quay.io/myorg/resume-screener:1.0.0 \
    --target claude
Pulling quay.io/myorg/resume-screener:1.0.0...
Installed to ~/.claude/skills/resume-screener
```

<!--
skillctl build produces standard OCI images (FROM scratch). Since skills are OCI images,
podman pull, skopeo copy, and Kubernetes ImageVolumes all work natively.

Install auto-pulls from remote registries if the image isn't in the local store.
Provenance (source registry and digest) is recorded in the installed skill's skill.yaml
for later upgrade tracking.
-->

---

## Every Skill Has a <span class="accent">Machine-Readable Identity</span>

```
$ skillctl inspect quay.io/myorg/resume-screener:1.0.0
Name:     myorg/resume-screener    Version: 1.0.0
Status:   published                License: Apache-2.0
Authors:  Red Hat OCTO             Compat:  claude-3.5-sonnet
Tags:     ["hr","screening"]
```

**What SkillCard enables:**
- **Discovery** — search by name, namespace, tags, author
- **Lifecycle** — draft → testing → published → deprecated
- **Compatibility** — target model compatibility hints
- **Governance** — license, author, namespace for trust policies

<!--
SkillCard schema: skillimage.io/v1alpha1 kind: SkillCard. The metadata travels inside the
OCI manifest annotations, so 'skillctl inspect' reads it without pulling the full layer.

The SkillCard schema is intentionally extensible: additional fields (compatibility matrix,
test results, usage metrics) can be added without breaking existing skills.
-->

---

## Image Volumes: Mount Skills <span class="accent">Directly</span>

On OpenShift 4.20+ / K8s 1.33+, mount an OCI image as a **read-only volume** — no init container needed.

```yaml
volumes:
  - name: resume-screener
    image:
      reference: quay.io/myorg/resume-screener:1.0.0
      pullPolicy: IfNotPresent
containers:
  - name: agent
    volumeMounts:
      - name: resume-screener
        mountPath: /skills/resume-screener
```

- Kubelet pulls and caches skill images automatically
- Read-only mount — immutable at runtime
- Same pull policies, pull secrets, and mirrors as container images

<span class="muted small">KEP-4639 · Kubernetes 1.33+ · OpenShift 4.20+</span>

<!--
Image Volumes (KEP-4639): GA in Kubernetes 1.33 / OpenShift 4.20.

The kubelet pulls the image via the container runtime and mounts it read-only into the pod.
No init container, no PVC, no emptyDir. The image is cached in the node's container image
store — subsequent pods that use the same skill image don't need to pull again.

This is the exact same mechanism used for container images, so existing image pull policies
(IfNotPresent, Always), pull secrets, and registry mirrors all work.
-->

---

## Init Container for <span class="accent">Older Clusters</span>

For K8s < 1.33 / OpenShift < 4.20, use skillctl as an init container.

```yaml
initContainers:
  - name: skill-puller
    image: ghcr.io/redhat-et/skillctl:latest
    command: ["skillctl", "pull", "--verify",
              "-o", "/skills",
              "quay.io/myorg/resume-screener:1.0.0"]
    volumeMounts:
      - name: skills
        mountPath: /skills
```

- Pull and verify signatures at pod startup
- Cache on a PVC across restarts
- Share the volume with the agent container

<!--
For older clusters, the init container approach uses skillctl to pull skills from the
registry before the main container starts. Use a PVC (not emptyDir) to persist the skill
cache across pod restarts and avoid filling node ephemeral storage.

Signature verification happens at pull time: --verify + --key flags enforce cosign
verification before extracting the skill.
-->

---

## From Ad-Hoc to <span class="accent">Supply Chain</span>

| | Before | After (OCI) |
| --- | ------ | ----------- |
| **Distribution** | git clone or manual copy | `skillctl install` from registry |
| **Versioning** | Branch/tag only | Semver tags + immutable digests |
| **Upgrades** | Manual re-clone | `skillctl upgrade` with version check |
| **Signing** | None | Cosign / sigstore signing |
| **Audit** | No trail | Registry access logs |
| **Auth** | Repo level only | Per-namespace RBAC + pull secrets |
| **Runtime** | Mutable | Read-only image volume mount |

<!--
The before/after contrast highlights what OCI distribution adds to the picture. All the
"after" properties come for free from the OCI ecosystem — registries, sigstore, RBAC,
pull policies — we're just reusing existing infrastructure.
-->

---

## Disconnected and <span class="accent">Air-Gapped</span> Environments

**oc-mirror** mirrors skill images alongside operator bundles to internal registries.

```text
Connected                           Disconnected
┌──────────┐   oc-mirror   ┌────────────────────────┐
│ Quay.io  │ ────────────▸ │ Internal OCP Registry  │
└──────────┘               └────────────────────────┘
  Skills + Operators         Mirrored with signatures
```

- **OLM integration** — skills as related images, mirrored automatically
- **Internal registry** — same `skillctl pull` workflow, no external access
- **Signatures preserved** — cosign signatures survive the mirror

<!--
The OpenShift platform team is also building support for this. The oc-mirror tool needs a
recognizable MIME type (application/vnd.redhat.agentskill.layer.v1+tar) to
identify skill artifacts for mirroring. skillctl supports this as an optional
media type alongside the standard OCI types.

For air-gapped clusters, the internal OpenShift registry
(image-registry.openshift-image-registry.svc:5000) serves the same role as
Quay.io — skillctl pull works the same way.

OLM integration: operators can declare skills as related images in their
ClusterServiceVersion. When oc-mirror processes the operator catalog, it
automatically includes the skill images.
-->

---

## Multiple <span class="accent">Consumption</span> Paths

Skills are standard OCI images — any tool that pulls images can consume them.

| Method | Command | Best for |
| ------ | ------- | -------- |
| **skillctl install** | `skillctl install <ref> --target claude` | Developer workstations |
| **Image volume** | Pod spec `volumes.image` | K8s 1.33+ / OCP 4.20+ |
| **Init container** | `skillctl pull -o /skills` | Older clusters |
| **Container extract** | `podman create` + `podman cp` | No skillctl installed |

```bash
# One-command install (auto-pulls from registry):
$ skillctl install quay.io/myorg/resume-screener:1.0.0 --target claude
```

<!--
skillctl install is the simplest path for developers. Supports Claude Code, Cursor,
Windsurf, OpenCode, and OpenClaw. Since skills are standard OCI images, any container
runtime can also pull and extract the content.
-->

---

## Brew-Style <span class="accent">Skill Management</span>

Like `dnf` for AI skills — install, list, upgrade in one command each.

```bash
$ skillctl install quay.io/myorg/code-reviewer:1.0.0 --target claude
Installed to ~/.claude/skills/code-reviewer

$ skillctl list --installed --upgradable --target claude
NAME            VERSION  LATEST  SOURCE                             TARGET
code-reviewer   1.0.0    2.0.0   quay.io/myorg/code-reviewer:2.0.0  claude

$ skillctl upgrade code-reviewer --target claude
Upgraded code-reviewer 1.0.0 → 2.0.0 (claude)

$ skillctl upgrade --all --target claude
All skills are up to date.
```

<!--
skillctl tracks provenance (source registry and digest) in each installed skill's
skill.yaml. This lets it check for newer published versions and upgrade in place.

The upgrade command only considers published versions (no -draft or -testing suffixes)
and uses strict semver comparison. Local skills without provenance are skipped.
-->

---

## Try It <span class="accent">Today</span>

- 💻 **Install:** `brew install pavelanni/tap/skillctl` — build, push, install, upgrade in minutes
- 🔄 **Full lifecycle:** build → promote → push → install → upgrade → remove
- 🤝 **Community:** SkillCard aligns with the Agent Skills OCI Artifacts Specification
- 🔏 **Next milestone:** sigstore integration for keyless skill signing and verification
- 🏢 **OpenShift alignment:** oc-mirror, OLM integration, and admission control

<span class="tag-accent">github.com/redhat-et/skillimage</span>

<!--
Resources:
- Project: github.com/redhat-et/skillimage
- Agent Skills OCI Spec: github.com/ThomasVitale/agents-skills-oci-artifacts-spec
- ORAS project: oras.land
- Zot registry: zotregistry.dev
-->

---

<!-- _class: thankyou -->

# Thank <span class="accent">You</span>

Pavel Anni · Office of CTO · Red Hat

<!--
Thank you for your time. Questions and feedback welcome.
-->
