# Collection install with Git source support

## Summary

Add `skillctl collection install` to install skills from a
collection YAML directly into an agent's skill directory.
Extend the `SkillRef` struct to support a `source:` field
pointing to Git repository URLs, enabling a faster inner loop
for teams iterating on skills without the full
build-push-pull cycle.

Closes #35.

## Motivation

Teams collaborating on skills across Git branches currently
must build, push to a registry, and pull before testing
changes. This friction slows the development inner loop.
A `source:` field in collection YAML lets developers point
directly at Git branches and install skills with a single
command, while `image:` entries continue to work for
production/stable skills.

## Design

### Data model: `SkillRef` in `pkg/collection/`

Add `Source` field to `SkillRef`:

```go
type SkillRef struct {
    Name   string `yaml:"name,omitempty"`
    Image  string `yaml:"image,omitempty"`
    Source string `yaml:"source,omitempty"`
}
```

- `name` becomes optional for `source:` entries (derived from
  SKILL.md frontmatter via `source.Resolve()`)
- `image` and `source` are mutually exclusive per entry
- Each entry must have exactly one of `image` or `source`

### Collection YAML example

```yaml
apiVersion: skillimage.io/v1alpha1
kind: SkillCollection
metadata:
  name: team-skills
  version: 0.1.0
  description: Dev manifest for our team
skills:
  - source: https://github.com/myorg/skills/tree/feature-branch/code-reviewer
  - source: https://github.com/myorg/skills/tree/main/meeting-notes
  - name: stable-tool
    image: quay.io/myorg/stable-tool:1.0.0
```

### Validation changes

Update `Validate()` in `pkg/collection/collection.go`:

| Rule | Error message |
| ---- | ------------- |
| Neither `image` nor `source` set | `skills[N]: image or source is required` |
| Both `image` and `source` set | `skills[N]: image and source are mutually exclusive` |
| `image` entry without `name` | `skills[N].name is required` (unchanged) |
| `source` entry without `name` | Valid (name derived at install time) |
| Duplicate names (when known) | `duplicate skill name "X"` (unchanged; for `source:` entries without `name`, duplicates are detected at install time after name resolution) |

Strict YAML parsing (`KnownFields(true)`) already rejects
unknown fields, so the `source` field must be added to the
struct for YAML files that use it to parse successfully.

### Command: `collection install`

```text
skillctl collection install [-f <file> | <git-url>] \
    [--target <agent> | -o <dir>] \
    [--force] [--ref <git-ref>]
```

**Flags:**

| Flag | Short | Type | Default | Description |
| ---- | ----- | ---- | ------- | ----------- |
| `--file` | `-f` | string | `""` | Path to local collection YAML |
| `--target` | `-t` | string | `""` | Agent name (claude, cursor, etc.) |
| `--output` | `-o` | string | `""` | Custom output directory |
| `--force` | | bool | false | Reinstall even if up to date |
| `--ref` | | string | `""` | Git ref override for collection YAML URL |

**Input resolution:**

- `-f <file>` reads a local YAML file
- Positional Git URL: uses `source.ParseGitURL()` to get
  clone URL and subpath, clones with sparse checkout, reads
  the YAML file at the resolved subpath. The URL must point
  to the collection YAML file itself (e.g.,
  `https://github.com/myorg/skills/tree/main/collection.yaml`)
- Exactly one of `-f` or positional URL required

**Target resolution** reuses the existing `agentTargets` map
from `install.go`. One of `--target` or `-o` is required.

### Install flow

For each entry in `skills[]`:

**`source:` entries:**

1. Check provenance: read `skill.yaml` in target dir for
   matching skill name, extract stored commit SHA
1. Run `git ls-remote` against the source URL's ref to get
   current commit SHA
1. If SHAs match and `--force` is not set, skip ("up to date")
1. Call `source.Resolve()` to clone, discover SKILL.md,
   generate SkillCard with provenance
1. Call `oci.Build()` to build into local OCI store
1. Call `client.Unpack()` to install to target directory
1. Write provenance with Git commit SHA via `writeProvenance()`

**`image:` entries:**

1. Check provenance: read stored digest from `skill.yaml`
   in target dir
1. Call `client.ResolveDigest()` to get current remote digest
1. If digests match and `--force` is not set, skip
1. Call `client.Pull()` if not in local store
1. Call `client.Unpack()` to install to target directory
1. Write provenance with OCI digest

### Skip logic detail

For `source:` entries, the skip check uses `git ls-remote`:

```bash
git ls-remote <clone-url> <ref>
```

This returns the commit SHA for the ref without cloning.
Compare against `provenance.commit` in the installed
`skill.yaml`. This is a single lightweight network call
per source entry.

For `image:` entries, `client.ResolveDigest()` does a HEAD
request to the registry. Compare against `provenance.commit`
(which stores the digest for OCI-sourced skills).

### Error handling

| Scenario | Behavior |
| -------- | -------- |
| Bad YAML / validation failure | Fail fast, no partial install |
| Both `image` and `source` on entry | Validation error |
| Individual skill build/pull fails | Print error, continue |
| `git` not in PATH (source entries) | Error from `source.CheckGit()` |
| Network unreachable | Error with wrapped details |
| Neither `-f` nor URL provided | Error: usage message |
| Neither `--target` nor `-o` | Error |

Continue-on-error for individual skills: if one fails, the
rest still install. Exit with error if any failed.

### Output format

```text
Installing collection "team-skills" (3 skills)
  code-reviewer (source)  cloning...  building...  installed
  meeting-notes (source)  up to date
  stable-tool   (image)   pulling...  installed
Installed 2 skills, 1 up to date
```

### Files changed

| File | Change |
| ---- | ------ |
| `pkg/collection/collection.go` | Add `Source` to `SkillRef`, update `Validate()` |
| `pkg/collection/collection_test.go` | Tests for source/image mutual exclusivity, optional name |
| `internal/cli/collection.go` | New `newCollectionInstallCmd()` and `runCollectionInstall()` |
| `internal/cli/install.go` | Extract shared helpers (e.g., `resolveOutputDir()`) |
| `examples/dev-collection.yaml` | Example collection with mixed source and image entries |

### Testing

**`pkg/collection/` unit tests:**

- Parse YAML with `source:` field succeeds
- Parse YAML with both `image:` and `source:` on same entry
  fails strict parsing (or validation catches it)
- Validate: entry with only `source:` and no `name:` is valid
- Validate: entry with only `image:` and no `name:` is invalid
- Validate: entry with neither `image:` nor `source:` is invalid
- Validate: duplicate names still detected when names are set

**CLI tests:**

- `collection install -f` with local file containing
  `image:` entries installs to target dir
- `collection install -f` with `source:` entries (needs Git
  repo fixture or skip in CI)
- `--force` flag causes reinstall even when SHA matches
- Missing `--target`/`-o` produces error
- Invalid YAML produces error before any install attempt

## Out of scope

- `collection pull` handling `source:` entries (pull remains
  OCI-only)
- Authentication for private Git repositories
- Parallel cloning/building of source entries
- Lock file for pinning exact commit SHAs
- Auto-upgrade / watch mode
