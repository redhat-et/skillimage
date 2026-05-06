# Provenance tracking and installed skills listing

## Summary

Populate OCI provenance metadata during `skillctl install` so
installed skills know where they came from, then add
`list --installed` to show what's installed across agent target
directories.

This is sub-project 1 of issue #27. Sub-project 2 (upgrade
command) depends on this and will be designed separately.

Closes #27 (partially).

## Motivation

Once a user pulls and installs skills, there's no way to see
what's installed or where it came from. The `Provenance` struct
in `skill.yaml` already has the right fields (`source`, `commit`,
`path`) but they're not populated during install. This work
populates them and adds a listing command to surface the
information.

## Design

### Provenance tracking during install

**Current flow:** `runInstall` calls `client.Unpack(ctx, ref, dir)`
which extracts files to disk. No metadata is written about the
source image.

**New flow:** After `Unpack`, the install command:

1. Resolves the ref's digest via `client.ResolveDigest(ctx, ref)`
2. Reads `skill.yaml` from the unpacked skill directory
3. Sets `Provenance.source` to the OCI ref (e.g.,
   `quay.io/skills/summarize:2.1.0`)
4. Sets `Provenance.commit` to the image digest (e.g.,
   `sha256:abc123...`)
5. Writes the updated `skill.yaml` back to disk

**New client method:**

```go
// File: pkg/oci/resolve.go
func (c *Client) ResolveDigest(ctx context.Context, ref string) (string, error)
```

Returns the digest string for a ref in the local store. Calls
`c.store.Resolve(ctx, ref)` and returns `desc.Digest.String()`.

**Field mapping:**

| Provenance field | Value | Example |
|------------------|-------|---------|
| `source` | Full OCI reference | `quay.io/skills/summarize:2.1.0` |
| `commit` | Image digest | `sha256:abc123...` |
| `path` | Not set by install | (preserved if already set) |

The `path` field is populated by the Git source builder
(`pkg/source/`) and is unrelated to OCI installs.

### Installed skills listing

**New flags on `list` command:**

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--installed` | `-i` | bool | false | List installed skills |
| `--target` | `-t` | string | `""` | Filter to a specific agent |
| `--output` | `-o` | string | `""` | Scan a custom directory |

Flag interactions:

- Without `--installed`: current behavior (local OCI store),
  `--target` and `-o` are ignored
- `--installed` alone: scan all known agent target directories
- `--installed --target claude`: scan only Claude's skill dir
- `--installed -o ~/myskills`: scan a custom directory
- `--target` and `-o` are mutually exclusive (same as install)

**New package: `pkg/installed/`**

```go
// File: pkg/installed/installed.go

type InstalledSkill struct {
    Name    string // skill name from metadata
    Version string // version from metadata
    Source  string // OCI ref from provenance (empty if local)
    Digest  string // image digest from provenance
    Target  string // agent name or directory path
    Path    string // full filesystem path to skill directory
}

func Scan(targets map[string]string) ([]InstalledSkill, error)
```

`Scan` takes a map of `{targetName: dirPath}` and for each
directory, looks for `*/skill.yaml` one level deep. Parses each
`skill.yaml` and returns `InstalledSkill` entries. Directories
that don't exist are silently skipped (agent not installed).

**Output format:**

```text
NAME              VERSION  SOURCE                              TARGET
hello-world       1.0.0    test/hello-world:1.0.0-draft        claude
my-summarizer     2.1.0    quay.io/skills/summarize:2.1.0      cursor
local-skill       1.0.0    (local)                             ~/myskills
```

Skills without provenance source show `(local)`.

### Error handling

| Scenario | Behavior |
|----------|----------|
| skill.yaml missing after unpack | Error from install (should not happen) |
| skill.yaml unparseable after unpack | Error from install |
| Serialize fails writing back | Error from install |
| Target dir doesn't exist (listing) | Skip silently |
| Malformed skill.yaml in target dir | Skip, print warning to stderr |
| No installed skills found | Print "No installed skills found." |

### Testing

**`pkg/oci/resolve.go`** — unit test:
- Build an image, call `ResolveDigest`, verify non-empty digest
- Call `ResolveDigest` on non-existent ref, verify error

**`pkg/installed/installed.go`** — unit test:
- Create temp dirs with skill.yaml files (with and without
  provenance), call `Scan`, verify results
- Scan with non-existent directory, verify no error
- Scan with malformed skill.yaml, verify it's skipped

**Install provenance** — unit test:
- Use existing test patterns: build image, unpack, verify
  skill.yaml contains provenance after the install flow

## Out of scope

- `--upgradable` flag (sub-project 2)
- `upgrade` command (sub-project 2)
- Digest-based version comparison
- Remote registry queries during listing
