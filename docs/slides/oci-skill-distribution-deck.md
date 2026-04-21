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
Context: This feature was implemented in PR #21 on redhat-et/docsclaw.
The design spec is at docs/dev/2026-04-12-oci-skill-distribution-design.md.
-->

---

## Your Agent Is Only as Good as Its <span class="accent">Skills</span>

Skills are the unit of specialization. Each skill lives in its own subdirectory and gives the agent domain expertise.

- **SKILL.md** — instructions in Agent Skills spec format
- **skill.yaml** — metadata: tools, resources, versioning

```
skills/
├── resume-screener/
│   ├── SKILL.md
│   └── skill.yaml
├── policy-comparator/
│   ├── SKILL.md
│   └── skill.yaml
└── checklist-auditor/
    ├── SKILL.md
    └── skill.yaml
```

<span class="muted small">Agent Skills Specification — agentskills.io</span>

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

The OCI Distribution Specification is **content-agnostic**. Any blob + a manifest + a media type = a valid OCI artifact.

The **ORAS** project (OCI Registry As Storage) makes this practical: push any content to any OCI registry with standard tooling.

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

<span class="muted small">ORAS — oras.land · OCI Distribution Spec</span>

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
Skill Directory  →  skillctl pack  →  OCI Layout  →  skillctl push  →  Registry
```

**What goes into the artifact:**
- `SKILL.md` — instructions
- `skill.yaml` — SkillCard metadata
- Any supporting files in the directory

**What the registry provides:**
- Immutable digest + mutable tags
- RBAC & pull secrets
- Cosign / sigstore signatures

<!--
Community alignment: This implementation aligns with the Agent Skills OCI Artifacts
Specification by Thomas Vitale.

Media types used:
- application/vnd.oci.image.layer.v1.tar+gzip (skill content layer)
- application/vnd.oci.image.config.v1+json (config with SkillCard metadata)

The custom annotations (io.docsclaw.skill.*) carry SkillCard metadata in the manifest
for fast inspection without pulling the full layer.
-->

---

## The Workflow: <span class="accent">Pack, Push, Mount</span>

```bash
# Pack skill into a local OCI layout
$ skillctl pack examples/skills/resume-screener
Packed skill → oci-layout · sha256:65af81ce... · 1226 bytes

# Push to registry (uses podman/docker credentials)
$ skillctl push examples/skills/resume-screener \
    quay.io/docsclaw/skill-resume-screener:1.0.0
Pushed → quay.io/docsclaw/skill-resume-screener:1.0.0

# Push as mountable image (for image volumes)
$ skillctl push --as-image examples/skills/resume-screener \
    quay.io/docsclaw/skill-resume-screener:1.0.0-image

# Inspect remotely (no download needed)
$ skillctl inspect quay.io/docsclaw/skill-resume-screener:1.0.0
Name: resume-screener · Version: 1.0.0 · Tools: [read_file]
```

<!--
The --as-image flag produces an OCI image (with rootfs layer) instead of an OCI artifact.
This is required for Kubernetes image volumes, which expect a proper container image that
the kubelet can pull via the container runtime.

Credential resolution is automatic: skillctl reads podman/docker auth configs, so if you've
done 'podman login quay.io', push/pull just works.
-->

---

## Every Skill Has a <span class="accent">Machine-Readable Identity</span>

```
$ skillctl inspect quay.io/docsclaw/skill-resume-screener:1.0.0

Name:          resume-screener
Namespace:     official
Version:       1.0.0
Description:   Screen resumes against a job description...
Author:        Red Hat ET
License:       Apache-2.0
Tools:         [read_file]
Memory:        32Mi
CPU:           100m
```

**What SkillCard enables:**
- 🔍 **Discovery** — search by name, namespace, category, author
- 📐 **Resource planning** — CPU and memory hints before deployment
- 🔗 **Compatibility** — required tools, dependencies, min agent version
- ✅ **Governance** — license, author, namespace for org-level trust policies

> **Future: Skill Catalog** — A UI that queries registries, reads SkillCards, and presents a searchable catalog — browse by category, check signing status, deploy with one click.

<!--
SkillCard schema: docsclaw.io/v1alpha1 kind: SkillCard. The metadata travels inside the
OCI manifest annotations, so 'skill inspect' reads it without pulling the full layer.

Future vision: a Skill Catalog UI that queries registries, reads SkillCard metadata, and
presents a searchable, browsable catalog of available skills — like a curated app store
for agent capabilities.

The SkillCard schema is intentionally extensible: additional fields (compatibility matrix,
test results, usage metrics) can be added without breaking existing skills.
-->

---

## Image Volumes: Mount Skills <span class="accent">Directly</span>

On OpenShift 4.20+ / Kubernetes 1.33+, the kubelet can mount an OCI image as a **read-only volume** — no init container needed.

- Kubelet pulls and caches skill images
- Read-only mount — immutable at runtime
- No emptyDir, no node storage consumed

```yaml
volumes:
  - name: skill-resume-screener
    image:
      reference: quay.io/docsclaw/skill-resume-screener:1.0.0
      pullPolicy: IfNotPresent

containers:
  - name: docsclaw
    volumeMounts:
      - name: skill-resume-screener
        mountPath: /skills/resume-screener
```

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

For Kubernetes < 1.33 or OpenShift < 4.20, use skillctl as an init container to pull skills before the agent starts.

- Pull at pod startup, verify signatures
- Cache on a PVC across restarts
- Share the volume with the main container

```yaml
initContainers:
  - name: skill-puller
    image: ghcr.io/redhat-et/skillctl:latest
    command:
      - "skillctl"
      - "pull"
      - "--verify"
      - "-o"
      - "/skills"
      - "quay.io/docsclaw/skill-resume-screener:1.0.0"
```

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
| **Distribution** | git clone or manual copy | Push/pull with standard tooling |
| **Versioning** | Branch/tag only | Semver tags + immutable digests |
| **Signing** | None | Cosign / sigstore signing |
| **Audit** | No trail | Registry access logs |
| **Auth** | Repo level only | Per-namespace RBAC + pull secrets |
| **Size** | ConfigMap 1 MiB limit | No limits |
| **Runtime** | Mutable | Read-only image volume mount |

<!--
The before/after contrast highlights what OCI distribution adds to the picture. All the
"after" properties come for free from the OCI ecosystem — registries, sigstore, RBAC,
pull policies — we're just reusing existing infrastructure.
-->

---

## Disconnected and <span class="accent">Air-Gapped</span> Environments

Many enterprise OpenShift clusters operate in restricted networks with no external registry access. Skills must reach these environments reliably.

**oc-mirror** can mirror skill images alongside operator bundles to an internal registry — if the skill uses a recognizable MIME type.

```text
Connected                              Disconnected
┌──────────┐   oc-mirror   ┌──────────────────────────┐
│ Quay.io  │ ───────────── │ Internal OCP Registry    │
│          │   (mirrored)  │ image-registry.svc:5000  │
└──────────┘               └──────────────────────────┘
  Skills +                   Skills + Operators +
  Operators                  Signatures preserved
```

- **OLM integration** — skills as related images in operator bundles, mirrored automatically
- **Internal registry** — pull from the cluster's built-in image registry, no external access needed
- **Signature preservation** — cosign signatures survive the mirror, verified at admission

<!--
OCPSTRAT-3122 covers this from the platform side. The oc-mirror tool needs a
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

Not every consumer can mount OCI images. skillctl supports multiple ways to get skills where they need to go.

| Method | Command | Best for |
| ------ | ------- | -------- |
| **Image volume** | Pod spec `volumes.image` | OpenShift 4.20+ / K8s 1.33+ |
| **Init container** | `skillctl pull -o /skills` | Older clusters |
| **CLI pull** | `skillctl pull -o ~/.claude/skills` | Developer workstations |
| **Container extract** | `podman create` + `podman cp` | No skillctl installed |
| **Zip download** | Artifact repo or registry | Non-OCI-aware tools |

```bash
# No skillctl? Use podman to extract skill files directly:
$ podman create --name tmp quay.io/skills/resume-screener:1.0.0
$ podman cp tmp:/skills/resume-screener ./skills/
$ podman rm tmp
```

<!--
The podman/docker extraction path is important for users who don't want to install
skillctl. Since skills are standard OCI images, any container runtime can pull and
extract the content.

Zip format: skillctl can bundle a .zip alongside the OCI image for consumers that
can't work with OCI natively (e.g. some coding assistants). Low implementation
effort — just create a zip of the skill directory and push as an additional layer
or separate artifact.

Ann Marie Fred (OpenShift AI) noted that coding assistants can't natively load OCI
artifacts yet. Multiple consumption paths lower the barrier to adoption.
-->

---

## Try It <span class="accent">Today</span>

- 💻 **Hands-on:** spin up a local Zot registry, pack a skill, push, inspect, pull — 5 minutes end to end
- 🔀 **Review the PR:** design decisions, media types, and annotation schema are documented in the design spec — feedback welcome
- 🤝 **Community:** should we propose SkillCard alignment to the Agent Skills OCI Artifacts Specification?
- 🔏 **Next milestone:** full sigstore integration for keyless skill signing and verification
- 🏢 **OpenShift alignment:** OCPSTRAT-3122 is building platform-side support — oc-mirror, OLM integration, admission control

<span class="tag-accent">github.com/redhat-et/docsclaw</span>

<!--
Resources:
- OCPSTRAT-3122: Agent skill packaging for OpenShift (Jira)
- OCPSTRAT-3118: Shipping Agent skills for OLM-managed operators
- OCPSTRAT-3119: Automatic agent skill discovery and registration
- PR #21: github.com/redhat-et/docsclaw/pull/21
- Design spec: docs/dev/2026-04-12-oci-skill-distribution-design.md
- OCI skills guide: docs/oci-skills-guide.md
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
