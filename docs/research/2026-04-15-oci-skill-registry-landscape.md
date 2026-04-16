# OCI Skill Registry — Competitive Landscape Research

**Date:** 2026-04-15
**Audience:** OCTO internal (Red Hat)
**Purpose:** Inform the oci-skill-registry PRD with competitive
analysis and ecosystem context

## Executive summary

The AI agent skills ecosystem has exploded since Anthropic
published the Agent Skills (SKILL.md) spec in December 2025.
There are now 96,000+ skills on SkillsMP alone, 17,000+ MCP
servers, and dozens of registries. However, the space is
fragmented: most registries are simple directories (markdown
files in git repos or npm packages). Very few combine OCI-native
distribution, lifecycle management, and enterprise trust — which
is exactly our differentiator.

The most relevant projects to watch are Stacklok ToolHive
(OCI skills + registry server), Thomas Vitale's OCI Skills spec
(community standard), and JFrog AI Catalog (enterprise governance).
AWS Agent Registry launched in April 2026 as a cloud-native
alternative.

## Competitive matrix

| Project | Distribution | Lifecycle | OCI native | Signing | Enterprise | Open source |
|---|---|---|---|---|---|---|
| **Our project** | OCI artifacts | draft→testing→published→deprecated→archived | Yes (content) | Sigstore (planned) | K8s/OpenShift/Podman | Yes |
| **agent-registry** (azaalouk) | OCI refs (metadata only) | draft→evaluated→approved→published→deprecated→archived | Refs only | No | K8s/OpenShift | Yes |
| **ToolHive** (Stacklok) | OCI artifacts | active/deprecated/archived | Yes | Via OCI ecosystem | Registry server | Yes |
| **Thomas Vitale spec** | OCI artifacts | No | Yes | Cosign/SLSA | Registry-agnostic | Yes (spec) |
| **JFrog AI Catalog** | Artifactory | Discovery→governance→compliance→deploy | Via Artifactory | JFrog Xray | Full enterprise | Commercial |
| **AWS Agent Registry** | AWS-native | AWS-managed | No | AWS signing | AWS-only | No |
| **Tessl** | npm/GitHub | Quality/impact/security scoring | No | Snyk scanning | Enterprise tier | Commercial |
| **SkillRegistry.io** | npm CLI | Timestamps + downloads | No | No | No | Commercial |
| **skify** | Custom REST API | Publish/update only | No | RBAC tokens | Small teams | Yes |
| **zeroclaw-skills** | Git monorepo | Semver only | No | CI scans | No | Yes |
| **MCP Registry** (official) | npm/HTTP | Public/private sub-registries | No | No | Sub-registries | Yes |
| **Smithery** | Hosted/CLI | Hosted servers | No | No | No | Partial |
| **MS Agent Governance** | Plugin system | Ed25519 signing, trust-tiered | No | Ed25519 | OWASP compliance | Yes (MIT) |
| **OpenAI AgentKit** | Connector Registry | Admin-managed | No | No | Enterprise | No |

## Detailed analysis

### Direct competitors (OCI-based skill distribution)

#### Stacklok ToolHive

The closest competitor. ToolHive now supports building and
publishing skills as OCI artifacts using the Agent Skills spec.
Their registry server provides versioned publishing with
reverse-DNS namespacing, search/filtering, and lifecycle controls
(active, deprecated, archived).

**Overlap with our project:** High. Both use OCI for skill
content, both support the Agent Skills spec, both target
container registries.

**Our differentiators:**
- Richer lifecycle (5 states vs 3)
- Semver-aware promotion with different gates for major vs
  minor/patch
- Library-first architecture (importable Go library, not just CLI)
- K8s image volumes and Podman delivery
- Designed for non-technical authors (UI authoring support)
- Companion to agent-registry for metadata/governance

**Risk:** ToolHive is backed by Stacklok (Craig McLuckie,
Kubernetes co-founder). Strong credibility in cloud-native space.

#### Thomas Vitale's OCI Skills spec

A community specification for packaging Agent Skills as OCI
artifacts. Proposes two artifact types: skill artifacts (single
skills) and collections (OCI Image Index grouping related skills).
Introduces `skills.json` + `skills.lock.json` for dependency
management (npm-like model).

**Reference implementations:** Arconia CLI (Java/ORAS), skills-oci
(Go/ORAS).

**Overlap:** High conceptual overlap. The spec is
framework-agnostic and registry-agnostic.

**Our differentiators:**
- Lifecycle management (the spec has none)
- Server component with REST API
- UI authoring support for non-technical users
- Signing/verification built in

**Opportunity:** Align our SkillCard schema with this spec. Thomas
is a known contact (mentioned in OCI implementation notes). We
should engage before diverging.

### Enterprise platforms

#### JFrog AI Catalog

A commercial governance platform for enterprise AI assets. Covers
models, agent skills, MCP servers, and AI-generated code.
Leverages Artifactory for distribution. Key features: shadow AI
detection, MCP governance, compliance enforcement.

**Overlap:** Low at the technical level (Artifactory vs OCI
registries), high at the use case level (enterprise governance).

**Our differentiators:**
- Open source
- OCI-native (works with any registry, not just Artifactory)
- Focused scope (skills, not everything)

**Lesson learned:** The "shadow AI" detection concept is
interesting — enterprises need visibility into what skills agents
are actually using vs. what's approved.

#### AWS Agent Registry

Part of AWS AgentCore, launched April 2026. Helps enterprises
discover, share, and reuse AI agents, tools, and skills.
Framework-agnostic but AWS-native.

**Overlap:** Use case overlap but cloud-locked.

**Our differentiator:** Cloud-agnostic, runs on any K8s/OpenShift.

#### Microsoft Agent Governance Toolkit

Open source (MIT). Runtime security for AI agents. Includes
plugin lifecycle management with Ed25519 signing, verification,
trust-tiered capability gating. First toolkit to address all 10
OWASP agentic AI risks.

**Overlap:** The plugin lifecycle and trust model are relevant.

**Lesson learned:** Ed25519 signing for skills is simpler than
full sigstore. Their trust-tiered gating (different capabilities
based on trust level) is worth considering.

### Community registries

#### SkillsMP, ClawHub, SkillHub

Large-scale community directories (96k+, 5.7k+, 7k+ skills
respectively). Mostly unvetted collections with search.

**Security concern:** The "ClawHavoc" incident (February 2026)
saw 341 malicious skills on ClawHub distributing macOS malware.
7.1% of one major registry had credential leaks.

**Lesson learned:** This validates our emphasis on signing and
lifecycle gates. A registry without trust verification is a
supply chain risk.

#### skify

Self-hosted private skill registry. TypeScript, Cloudflare
Workers or Docker deployment. No OCI, no lifecycle, no signing.

**Overlap:** Low. Different tech stack, different scale.

**Lesson learned:** Their AGENTS.md generation pattern (manifest
of available skills for agent consumption) is a nice UX idea.

#### zeroclaw-skills

Git monorepo of skills with TOML manifests. 3 days old, very
early. CI validation with security scans.

**Overlap:** Minimal. Git-based, no OCI, no lifecycle.

### Standards and working groups

#### CNCF ecosystem

- **ModelPack:** CNCF Sandbox project for packaging ML artifacts
  as OCI objects. Contributors include Red Hat.
- **KitOps:** CNCF Sandbox, Docker-like CLI for AI/ML artifacts
  using Kitfile manifests.
- **ORAS:** OCI Registry as Storage — the library we use.
- **Harbor:** VMware is positioning Harbor as an AI model
  registry.
- **Dragonfly:** Graduated January 2026, distributes AI model
  artifacts at scale using ModelPack spec.
- **WG Artifacts:** CNCF working group gathering stakeholders
  for packaging, distribution, and deployment mechanisms.
- **Cloud Native AI WG:** Under TAG Runtime, focused on AI
  workloads on Kubernetes.

**Opportunity:** Our project sits at the intersection of CNCF's
OCI artifact work and the Agent Skills ecosystem. We could
propose a CNCF sandbox submission or contribute to WG Artifacts.

#### Official MCP Registry

Launched September 2025, API frozen at v0.1. Supports public
and private sub-registries. Cross-company working group
(Anthropic, GitHub, PulseMCP, Block, Microsoft). Focused on
MCP server discovery, not skills.

**Overlap:** Low (different artifact type), but the sub-registry
model for enterprises is worth studying.

## Key takeaways for our PRD

### Validated decisions

1. **OCI-native storage is the right bet.** ToolHive, Thomas
   Vitale's spec, and CNCF projects all converge on OCI.
2. **Lifecycle management is a differentiator.** Most registries
   have none or minimal (active/deprecated). Our 5-state machine
   with semver-aware gates is unique.
3. **Non-technical authoring matters.** No competitor offers a
   good UI authoring experience for skills.
4. **Signing is non-negotiable.** ClawHavoc proved that
   unsigned skill registries are supply chain risks.

### Gaps to consider

1. **Align with Thomas Vitale's spec.** His `skills.json` /
   `skills.lock.json` dependency model is more mature than ours.
   We should adopt or converge rather than compete.
2. **Trust tiers.** Microsoft's trust-tiered capability gating
   is worth adopting — skills at different trust levels get
   different permissions.
3. **Shadow skill detection.** JFrog's concept of detecting
   unapproved AI usage could apply to skills: what skills are
   agents actually loading vs. what's in the registry?
4. **Security scanning integration.** Tessl's Snyk integration
   and ToolHive's Stacklok heritage suggest that vulnerability
   scanning of skill content should be on our roadmap.
5. **Collections/bundles.** Thomas Vitale's "collections" concept
   (OCI Image Index grouping related skills) is useful for
   distributing skill packs.

### Competitive positioning

Our project occupies a unique position:

```
                    Enterprise governance
                           ↑
              JFrog ●      |     ● AWS
                           |
  Content storage ←────────┼────────→ Metadata only
                           |
   ToolHive ●    ● US      |     ● agent-registry
                           |
              Tessl ●      |
                           ↓
                    Community directory
```

We are the only open-source project that combines OCI content
storage with enterprise lifecycle management and non-technical
authoring support. ToolHive is closest but lacks lifecycle depth
and UI authoring.

## Sources

- [Stacklok ToolHive: Reusable agent skills across CLI and Registry](https://docs.stacklok.com/toolhive/updates/2026/04/06/updates)
- [Thomas Vitale: Agent Skills as OCI Artifacts](https://www.thomasvitale.com/agent-skills-as-oci-artifacts/)
- [Every AI Agent Skills Platform You Need to Know in 2026](https://dev.to/haoyang_pang_a9f08cdb0b6c/every-ai-agent-skills-platform-you-need-to-know-in-2026-4alg)
- [The State of OCI Artifacts for AI/ML](https://www.gorkem-ercan.com/p/the-state-of-oci-artifacts-for-aiml)
- [CNCF: How OCI Artifacts will drive future AI use cases](https://www.cncf.io/blog/2025/08/27/how-oci-artifacts-will-drive-future-ai-use-cases/)
- [CNCF Cloud Native AI Working Group](https://tag-runtime.cncf.io/wgs/cnaiwg/)
- [CNCF WG Artifacts](https://github.com/cncf-tags/wg-artifacts)
- [Introducing the MCP Registry](https://blog.modelcontextprotocol.io/posts/2025-09-08-mcp-registry-preview/)
- [Microsoft Agent Governance Toolkit](https://opensource.microsoft.com/blog/2026/04/02/introducing-the-agent-governance-toolkit-open-source-runtime-security-for-ai-agents/)
- [AWS Agent Registry](https://thenewstack.io/aws-wants-to-register-your-ai-agents/)
- [OpenAI AgentKit](https://openai.com/index/introducing-agentkit/)
- [JFrog AI Catalog](https://jfrog.com/ai-catalog/)
- [Tessl Registry](https://tessl.io/registry)
- [SkillRegistry.io](https://skillregistry.io/)
- [skify](https://github.com/lynnzc/skify)
- [zeroclaw-skills](https://github.com/zeroclaw-labs/zeroclaw-skills)
- [agentregistry-dev](https://github.com/agentregistry-dev/agentregistry)
- [Using Harbor as an AI Model Registry](https://blogs.vmware.com/cloud-foundation/2026/03/03/using-harbor-as-an-ai-model-registry/)
