# Design: Improve list output formatting

**Issue:** #23
**Date:** 2026-05-01

## Summary

Update `skillctl list` output to match container tool conventions
by stripping the `sha256:` prefix from digests and humanizing
timestamps. Add `--no-trunc` flag to preserve access to raw values
for scripting.

## Current output

```text
NAME                        TAG            STATUS   DIGEST               CREATED
examples/hello-world        1.0.0-draft    draft    sha256:a593244d38f0  2026-04-29T22:07:29Z
```

## Proposed output

```text
NAME                        TAG            STATUS   DIGEST         CREATED
examples/hello-world        1.0.0-draft    draft    a593244d38f0   2 days ago
```

With `--no-trunc`:

```text
NAME                        TAG            STATUS   DIGEST                                                              CREATED
examples/hello-world        1.0.0-draft    draft    sha256:a593244d38f0e1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4  2026-04-29T22:07:29Z
```

## Changes

| Aspect | Current | New |
| ------ | ------- | --- |
| Digest display | `sha256:a593244d38f0` (19 chars, includes prefix) | `a593244d38f0` (12 hex chars, no prefix) |
| Timestamp display | `2026-04-29T22:07:29Z` (RFC 3339) | `2 days ago` (relative) |
| Column header | DIGEST | DIGEST (unchanged) |
| `--no-trunc` flag | Does not exist | Shows full digest with prefix + raw RFC 3339 |

## Scope

- Only the local store listing (`skillctl list` without `--installed`)
  is affected. The `--installed` and `--upgradable` views do not
  show digest or timestamp columns.
- No SIZE column. Skills are small text files; the value would not
  be actionable.
- No `--format` flag in this iteration.

## Implementation

### Dependency

Add `github.com/dustin/go-humanize` for relative time formatting.

### File changes

**`internal/cli/list.go`**

1. Add `--no-trunc` bool flag to the `list` command.
2. In `runList()`, pass `noTrunc` to the rendering logic.
3. Digest formatting:
   - Default: strip `sha256:` prefix (or any `algo:` prefix),
     truncate to 12 hex characters.
   - `--no-trunc`: show the full digest string as-is.
4. Timestamp formatting:
   - Default: parse RFC 3339 string with `time.Parse`, format with
     `humanize.Time(t)`. On parse failure, fall back to the raw
     string.
   - `--no-trunc`: show the raw RFC 3339 string.

### No other file changes

The `LocalImage` struct and OCI layer remain unchanged. Formatting
is a display concern handled entirely in the CLI layer.

## Testing

- Update or add tests in `internal/cli/list_test.go` to verify:
  - Default output strips prefix and shows relative time
  - `--no-trunc` output preserves full digest and raw timestamp
  - Graceful fallback when `Created` is empty or unparseable
