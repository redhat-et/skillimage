# Demo: immutable skills via OCI image volumes

**Audience:** Product Managers, security/compliance stakeholders, anyone
who needs to see — not just be told — that a skill delivered through the
registry can't be silently changed underneath an agent.

**Runtime:** ~3 minutes narrated, ~20 seconds with `--auto`.

**What it proves:**

1. A skill is packaged once as a content-addressed OCI image (a digest).
2. It travels through a normal container registry, same as any container image.
3. If you mount it the naive way (no `:ro`), an agent — or a bug, or an
   attacker who compromises the agent — can silently rewrite the skill's
   instructions. This is the "prompt injection via mutable skill" risk.
4. If you mount it the way OpenShift/Kubernetes ImageVolumes always do
   (read-only), the same write attempt is rejected by the kernel, in
   front of the audience, with a real error message.
5. No matter what happens to a local copy, the *digest* of the artifact
   in the registry never changes. (A registry can still let someone
   move the `:1.0.0-draft` tag to point at a different digest later —
   that's a separate guarantee, provided by pinning deployments to a
   digest or by a registry's tag-immutability policy, not by anything
   in this demo.)
6. A real, minimal agent runtime ([DocsClaw](https://github.com/redhat-et/docsclaw))
   picks up the skill with zero code changes, proving this isn't just a
   filesystem trick — it's how agents actually consume skills in production.

## Prerequisites

Install once, on macOS or Linux:

```bash
brew install podman skopeo        # or your package manager of choice
brew install pavelanni/tap/skillctl
podman machine init && podman machine start   # macOS only
```

No LLM API key is required — the demo only exercises skill packaging,
distribution, and discovery, not chat responses.

Run from the repo root:

```bash
cd skillimage
./demo/immutable-volume/run.sh
```

Pass `--auto` to skip the "press Enter" pauses (useful for a dry run or
for recording a terminal cast):

```bash
./demo/immutable-volume/run.sh --auto
```

The script is idempotent and self-cleaning: it builds the skill, spins
up a throwaway local registry container, tears it down at the end, and
leaves your environment untouched otherwise. Nothing outside the
`hello-world` example skill is modified.

## Suggested talking track

Say these out loud as the corresponding step runs. The step numbers
match the script's output headers.

**1. Build the skill into an OCI image**
> "Every skill — the instructions we give an agent — gets packaged as a
> standard container image. Same format Docker and Kubernetes already
> use. This digest is a cryptographic fingerprint of the content."

**2. Push to a registry and pull it back**
> "This is exactly what happens with Quay, GHCR, or an internal Harbor
> instance in production. Nothing skill-specific about the plumbing —
> your existing registry, RBAC, and scanning tools all just work."

**3. The naive way: mount without `:ro`**
> "Here's the mistake that matters. If a skill is mounted as a normal,
> writable volume, anything running in that container — the agent
> itself, a compromised dependency, a bug — can rewrite its own
> instructions. Watch: I write a malicious instruction into the skill
> file, and it sticks."

**4. The production way: mount with `:ro`**
> "Now the same attack, but mounted read-only, which is what our
> deployment manifests always do. The kernel itself refuses the write.
> This isn't a policy an agent has to remember to respect — it's not
> *possible*."

**5. The published artifact itself never changed**
> "And critically — even in step 3, when a local copy got corrupted, the
> artifact sitting in the registry was never touched. We just asked the
> registry directly, not our local cache. The digest is identical to the
> one we recorded when we built it, and it holds regardless of what
> happens to any one running container. Two callouts for the compliance
> folks: this checks the digest, not the tag — someone could still
> repoint `:1.0.0-draft` at different content later, which is why real
> deployments pin by digest or rely on a registry's tag-immutability
> policy. And step 4's read-only guarantee is a property of the local
> mount, not the registry — it's what stops an agent from corrupting
> its own copy, which is a distinct protection from digest immutability."

**6. A real agent harness discovers the skill**
> "This isn't a synthetic demo — this is DocsClaw, an actual open-source
> agent runtime, discovering the skill exactly like it would in a
> production pod. No init container, no sidecar, no custom code to
> mount a skill safely."

**Closing line:**
> "On OpenShift and Kubernetes 1.33+, this read-only behavior isn't
> something a deployment engineer has to remember to configure — the
> ImageVolume API doesn't have a writable option. It's the only way to
> mount a skill image at all."

## Troubleshooting

- **`podman: command not found`** — install Podman Desktop or `brew
  install podman`, then `podman machine init && podman machine start`.
- **Port 5500 already in use** — set a different port:
  `REGISTRY_PORT=5555 ./demo/immutable-volume/run.sh`.
- **`image platform ... does not match`** warning during pull — harmless.
  Skill images are `FROM scratch` (no OS/binaries), so architecture
  doesn't matter; Podman just doesn't know that.
- **Step 6 fails to pull `ghcr.io/redhat-et/docsclaw`** — you're offline,
  or the image was moved. The rest of the demo (steps 1–5) already
  makes the immutability point without it; the script will note the
  skip and continue.
- **Re-running the demo** — safe. Each run rebuilds the skill (new
  digest only if the content changed), recreates the registry
  container, and cleans up after itself.

## Why this exists

See the main [README.md](../../README.md#using-skill-images) for the
non-interactive explanation of image-volume mounting, and
`docs/design/2026-04-16-implementation-spec.md` for the underlying
design decision ("Image-mounted skills (K8s ImageVolumes) are
guaranteed immutable by the kubelet"). This demo makes that written
guarantee something you can watch happen.
