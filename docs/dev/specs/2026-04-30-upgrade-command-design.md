# Upgrade command for installed skills

## Summary

Add `skillctl upgrade` to pull newer published versions of
installed skills from their source registry, and add
`list --installed --upgradable` to preview available upgrades.

This is sub-project 2 of issue #27, building on provenance
tracking from sub-project 1.

Closes #27.

## Motivation

Users who have installed skills from OCI registries need to know
when newer versions are available and upgrade to them. Without
this, the only workflow is manually checking registries, pulling,
and re-installing.

## Design

### Version checking (`pkg/installed/`)

**New function:**

```go
func CheckUpgrades(ctx context.Context, skills []InstalledSkill,
    opts CheckOptions) ([]UpgradeCandidate, error)
```

For each skill with a non-empty `Source`:

1. Extract the registry repo from `Source` (e.g.,
   `quay.io/skills/summarize` from
   `quay.io/skills/summarize:2.1.0`) using `splitRefTag`-style
   parsing
2. List remote tags via `oci.ListRemoteTags`
3. Filter to published-only tags (no `-draft`, `-testing` suffix)
4. Parse each as semver using `semver.StrictNewVersion`,
   find the highest
5. Compare against the installed version
6. If higher exists, include in result as `UpgradeCandidate`

Skills without provenance (`Source == ""`) or with non-semver
versions are silently skipped.

**Types:**

```go
type CheckOptions struct {
    SkipTLSVerify bool
}

type UpgradeCandidate struct {
    Installed     InstalledSkill
    LatestVersion string // e.g., "2.0.0"
    LatestRef     string // e.g., "quay.io/skills/summarize:2.0.0"
}
```

Version comparison uses semver ordering only — no date checking.
If an author publishes 1.3 after 1.5, the system correctly
treats 1.5 as the latest.

### `list --installed --upgradable`

**New flag on `list` command:**

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--upgradable` | `-u` | bool | false | Show only upgradable skills |

Requires `--installed` — error if used without it.

**Behavior:** Calls `installed.Scan()` then
`installed.CheckUpgrades()`, displays only skills with
available upgrades:

```text
NAME                VERSION  LATEST  SOURCE                             TARGET
red-hat-quick-deck  0.1.0    1.0.0   toddward/red-hat-quick-deck:1.0.0  opencode
summarizer          1.0.0    2.1.0   quay.io/skills/summarize:2.1.0     claude
```

Adds a LATEST column showing the available version.

### `upgrade` command

```text
skillctl upgrade <skill-name> --target <agent> [flags]
skillctl upgrade --all --target <agent> [flags]
```

**Flags:**

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--target` | `-t` | string | `""` | Agent target (required) |
| `--output` | `-o` | string | `""` | Custom directory |
| `--all` | `-a` | bool | false | Upgrade all skills |
| `--tls-verify` | | bool | true | TLS verification |

`--target` and `-o` are mutually exclusive, same as install.
One of them is required.

**Single skill flow:**

1. Scan target directory for the named skill
2. Read provenance — error if no `Source` (local skill)
3. Call `CheckUpgrades` for that skill
4. If no upgrade available, print "already at latest version",
   exit 0
5. Pull new version to local OCI store
6. Unpack to same directory (overwrites)
7. Update provenance in skill.yaml with new ref and digest
8. Print: `Upgraded red-hat-quick-deck 0.1.0 → 1.0.0 (opencode)`

**Batch flow (`--all`):**

1. Scan target directory for all installed skills
2. Call `CheckUpgrades` for all
3. If none upgradable, print "all skills up to date", exit 0
4. For each candidate: pull, unpack, update provenance
5. Print each upgrade, then: `Upgraded 3 skill(s).`

### Error handling

| Scenario | Behavior |
|----------|----------|
| Skill not found in target | Error |
| No provenance (local) | Error: "no source registry" |
| Registry unreachable | Error with wrapped network error |
| Already at latest | Print message, exit 0 |
| `--all` without `--target`/`-o` | Error |
| Skill name with `--all` | Error |
| Neither name nor `--all` | Error |
| `--upgradable` without `--installed` | Error |

### Testing

**`pkg/installed/check.go`** — unit tests:
- Mock tag listing (inject a function or use interface) to
  test version comparison without a real registry
- Test: installed 1.0.0, remote has 1.0.0 and 2.0.0 →
  candidate with 2.0.0
- Test: installed 2.0.0, remote has 1.0.0 and 2.0.0 →
  no candidate (already latest)
- Test: installed skill with no provenance → skipped
- Test: remote has only draft/testing tags → no candidate
- Test: installed version is not valid semver → skipped

**CLI tests** are integration-level and harder to unit test
without a real registry. The core logic in `CheckUpgrades`
covers the important cases.

## Out of scope

- Pinning to specific versions
- Rollback to previous version
- Dependency resolution
- Auto-upgrade on schedule
- Upgrading from draft/testing channels
