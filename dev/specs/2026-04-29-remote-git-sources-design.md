# Remote Git sources for skillctl build

## Summary

Extend `skillctl build` to accept Git repository URLs as sources,
enabling users to build OCI skill images directly from remote
repositories without manual cloning. Skills are discovered by
`SKILL.md` presence, and SkillCards are generated on the fly from
frontmatter metadata when no `skill.yaml` exists.

Closes #10 and #13.

## Motivation

Currently `skillctl build` only works with local directories. The
distribution vehicle workflow — pull skills from upstream sources,
curate, sign, and publish as OCI images — requires manual cloning.
Many skill repositories (Anthropic, zeroclaw-skills, superpowers
plugins) don't include `skill.yaml` files, using `SKILL.md`
frontmatter instead. Supporting remote Git sources with automatic
SkillCard generation removes both friction points.

## Design

### Source detection

The `build` command accepts both local paths and Git URLs:

```bash
skillctl build <source> [flags]
```

Detection heuristic: if `source` starts with `https://` or
`http://`, treat as Git URL. Otherwise, treat as local path.

### URL parsing

A `ParseGitURL()` function extracts clone URL, ref, and subpath
from repository URLs.

| Input | CloneURL | Ref | SubPath |
| ----- | -------- | --- | ------- |
| `https://github.com/anthropics/skills` | `https://github.com/anthropics/skills.git` | `""` | `""` |
| `https://github.com/anthropics/skills/tree/main/skills` | `https://github.com/anthropics/skills.git` | `main` | `skills` |
| `https://github.com/anthropics/skills/tree/v1.0/skills/comms` | `https://github.com/anthropics/skills.git` | `v1.0` | `skills/comms` |
| `https://gitlab.com/org/repo/-/tree/main/path` | `https://gitlab.com/org/repo.git` | `main` | `path` |
| `https://unknown-host.com/org/repo` | `https://unknown-host.com/org/repo` | `""` | `""` |

GitHub uses `/tree/<ref>/path`, GitLab uses `/-/tree/<ref>/path`.
For unknown hosts, the base URL is used as the clone target with
no ref/subpath extraction.

The `--ref` flag overrides whatever ref was parsed from the URL.

### Git operations

A `Clone()` function handles cloning with a `git` availability
check up front.

**Prerequisites:**

- `exec.LookPath("git")` — returns actionable error if missing:
  "git is required for remote sources — install it from
  https://git-scm.com"

**Clone strategy:**

- When `SubPath` is set: sparse checkout for efficiency

  ```bash
  git clone --depth 1 --filter=blob:none --sparse --branch <ref> <url> <tmpdir>
  git -C <tmpdir> sparse-checkout set <subpath>
  ```

- When `SubPath` is empty: regular shallow clone

  ```bash
  git clone --depth 1 [--branch <ref>] <url> <tmpdir>
  ```

- If sparse checkout fails (unsupported server): fall back to
  full shallow clone with a warning

**Returns:** resolved directory path (tmpdir + SubPath), a cleanup
function, the HEAD commit SHA (via `git rev-parse HEAD`), and any
error.

**Auth scope:** public repos only (unauthenticated HTTPS). Private
repo support (tokens, SSH) deferred to a future iteration.

### Skill discovery

A `Discover()` function walks a directory and returns skill paths.

- Walk recursively using `filepath.WalkDir`
- A directory containing `SKILL.md` is a skill
- Skip hidden directories (`.git`, `.github`, etc.)
- Do not descend into a skill directory's subdirectories
- If the target directory itself contains `SKILL.md`, treat as
  single skill (no walk)
- `--filter` matches against resolved skill name via
  `filepath.Match` glob
- Return sorted list; error if no skills found

### SkillCard generation

When a skill directory has `SKILL.md` but no `skill.yaml`, generate
a SkillCard in memory from frontmatter metadata.

**Field mapping:**

| SKILL.md frontmatter | SkillCard field | Fallback |
| -------------------- | --------------- | -------- |
| `name` | `metadata.name` | directory name |
| `description` | `metadata.description` | text up to first period or newline in SKILL.md body |
| `metadata.version` | `metadata.version` | `0.1.0` |
| `metadata.author` | `metadata.authors[0].name` | GitHub org from clone URL |
| `license` | `metadata.license` | — |
| `compatibility` | `metadata.compatibility` | — |
| — | `metadata.namespace` | GitHub org from clone URL |
| — | `apiVersion` | `skillimage.io/v1alpha1` (always) |
| — | `kind` | `SkillCard` (always) |
| — | `spec.prompt` | `SKILL.md` (always) |

**Precedence:** if `skill.yaml` exists alongside `SKILL.md`, use
it as-is. Generated SkillCards are not written to disk.

### Provenance injection

When building from Git, provenance fields are populated
automatically on every SkillCard:

- `provenance.source` — the clone URL
- `provenance.commit` — HEAD commit SHA from the cloned repo
- `provenance.path` — path within the repo (SubPath + skill dir)

### CLI changes

**New flags:**

| Flag | Description |
| ---- | ----------- |
| `--ref` | Override Git ref (branch, tag, or commit SHA) |
| `--filter` | Glob pattern to select skills by name |

**Existing flags** (`--tag`, `--media-type`) work unchanged, applied
to each built image. `--tag` is rejected when building multiple
skills (ambiguous).

**Output for multi-skill builds:**

```text
Cloning https://github.com/anthropics/skills (ref: main)...
Building internal-comms (1/3)...
  Digest: sha256:abc123...
Building code-review (2/3)...
  Digest: sha256:def456...
Building email-drafter (3/3)...
  Digest: sha256:789ghi...

Built 3 skills from https://github.com/anthropics/skills
```

### Changes to existing code

- `internal/cli/build.go` — `runBuild()` gains the remote path:
  detect URL, resolve via `pkg/source`, loop over discovered skills.
  Local single-skill path stays unchanged.
- `pkg/oci/build.go` — `Build()` gets a new option to accept a
  pre-built SkillCard instead of always reading `skill.yaml` from
  disk.
- `pkg/skillcard/` — no changes needed.

## Package structure

New package: `pkg/source/`

| File | Responsibility |
| ---- | -------------- |
| `source.go` | `IsRemote()` detection, top-level `Resolve()` orchestrator |
| `giturl.go` | `ParseGitURL()` — URL parsing for GitHub, GitLab, generic |
| `clone.go` | `Clone()` — git check, shallow clone, sparse checkout, cleanup |
| `discover.go` | `Discover()` — walk for SKILL.md, filter by glob |
| `generate.go` | `GenerateSkillCard()` — SKILL.md frontmatter to in-memory SkillCard |

## Data flow

```text
skillctl build https://github.com/anthropics/skills/tree/main/skills
    │
    ├─ source.IsRemote(input) → true
    ├─ source.ParseGitURL(input) → GitSource{CloneURL, Ref:"main", SubPath:"skills"}
    ├─ source.Clone(ctx, gitSource, opts)
    │       ├─ exec.LookPath("git")
    │       ├─ sparse checkout (SubPath is set)
    │       ├─ git rev-parse HEAD → commitSHA
    │       └─ returns (tmpDir/skills/, cleanup, commitSHA)
    ├─ source.Discover(dir, filter) → [{path, name}, ...]
    │       ├─ walk for SKILL.md files
    │       └─ apply --filter glob
    ├─ For each discovered skill:
    │       ├─ Has skill.yaml? → skillcard.Parse() as today
    │       ├─ No skill.yaml? → source.GenerateSkillCard(skillDir, frontmatter, gitSource)
    │       ├─ Inject provenance (source URL, commitSHA, path-in-repo)
    │       └─ oci.Build(ctx, skillDir, opts) → Descriptor
    └─ cleanup() removes tmpDir
```

## Error handling

| Scenario | Behavior |
| -------- | -------- |
| `git` not installed | "git is required for remote sources — install it from https://git-scm.com" |
| Clone failure | Wrap git stderr: "failed to clone %s: %s" |
| Ref doesn't exist | Git's own error ("Remote branch X not found") |
| SubPath missing after clone | "path %q not found in repository %s" |
| No SKILL.md found | "no skills found in %s" |
| No skills match `--filter` | "no skills matching %q in %s" |
| SKILL.md has no frontmatter | Generate SkillCard from fallbacks (dir name, default version, org from URL) |
| Malformed frontmatter YAML | Warning, fall back to directory-name-based generation |
| `--tag` with multiple skills | "--tag cannot be used when building multiple skills" |
| `skill.yaml` exists with `SKILL.md` | Use `skill.yaml`, don't regenerate |
| Sparse checkout fails | Fall back to full shallow clone, print warning |
| Temp dir cleanup fails | Log warning, don't fail the build |
| One skill in batch fails | Report error, continue building the rest, exit non-zero |

## Scope boundaries

**In scope:**

- Git URL parsing (GitHub, GitLab, generic hosts)
- Shallow clone with sparse checkout optimization
- Skill discovery by SKILL.md
- SkillCard generation from SKILL.md frontmatter
- Provenance auto-population
- Batch builds with `--filter`
- `--ref` flag for ref override

**Out of scope (future work):**

- Private repo authentication (SSH, tokens, credential helpers)
- `manifest.toml` parsing (issue #13 field mapping table)
- Interactive fallback for missing required fields
- `skillctl generate` as a standalone command
- Non-Git sources (S3, HTTP tarballs)
