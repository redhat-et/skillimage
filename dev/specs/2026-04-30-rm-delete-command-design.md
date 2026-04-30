# rm/delete command for local skill images

## Summary

Add `skillctl rm` (alias `delete`) to remove skill images from the
local OCI store by name:tag reference. Supports multiple refs in a
single invocation, confirms before removal unless `--force` is set,
and defers blob cleanup to `skillctl prune`.

Closes #22.

## Motivation

After building or pulling images, they accumulate in the local store
(`~/.local/share/skillctl/store/`) with no targeted cleanup
mechanism. The existing `prune` command only removes images
superseded by promotion — it cannot remove a specific image by
reference. Users need a way to remove individual images.

## Design

### CLI command

**File:** `internal/cli/rm.go`

```
skillctl rm <ref> [<ref>...] [flags]
```

| Field | Value |
| ----- | ----- |
| Use | `rm <ref> [<ref>...]` |
| Aliases | `delete` |
| Short | Remove skill images from local store |
| Args | One or more `name:tag` references |

**Flags:**

| Flag | Short | Type | Default | Description |
| ---- | ----- | ---- | ------- | ----------- |
| `--force` | `-f` | bool | false | Skip confirmation prompt |

**Behavior:**

1. Create OCI client via `defaultClient()`
2. Resolve each ref against the local store to verify it exists
3. Print the list of images to be removed
4. Unless `--force`, prompt for confirmation:
   - Single image: `Remove examples/hello-world:1.0.0-draft? [y/N]`
   - Multiple images: list all, then `Remove 3 image(s)? [y/N]`
5. Call `client.Remove(ctx, ref)` for each ref
6. Print each removed image on success
7. If a ref doesn't exist, print an error for that ref but
   continue with the rest
8. Exit code 1 if any removal failed

**Not in scope (v1):**

- Digest-based removal (`sha256:abc...`) — tag-based only
- Glob/wildcard patterns (`examples/*`)
- Automatic blob garbage collection (use `prune` separately)

### Client method

**File:** `pkg/oci/remove.go`

```go
func (c *Client) Remove(ctx context.Context, ref string) error
```

1. Call `c.store.Resolve(ctx, ref)` to verify the ref exists
2. If not found, return `"image not found: <ref>"`
3. Call `c.store.Untag(ctx, ref)` to remove the tag
4. Return wrapped error on failure

No blob cleanup — unreferenced blobs are cleaned up by
`skillctl prune`.

### Confirmation prompt

Read from `os.Stdin` using `bufio.Scanner`. Accept `y` or `Y` as
confirmation; anything else (including empty input) is treated as
decline. This matches the `[y/N]` convention where N is the default.

### Error handling

| Scenario | Behavior |
| -------- | -------- |
| Ref not found | Print error, continue with remaining refs |
| Untag failure | Print error, continue with remaining refs |
| All refs fail | Exit code 1 |
| Some refs fail | Print successes and errors, exit code 1 |
| User declines confirmation | Exit code 0, no action |

### Testing

**Unit tests** in `pkg/oci/remove_test.go`:

- Build an image, remove by ref, verify absent from `ListLocal()`
- Remove a non-existent ref, verify error message
- Remove multiple refs (mix of valid and invalid), verify partial
  success behavior

Tests follow the existing pattern in `pkg/oci/` — create a temp
store, build test images, then exercise removal.

## Alternatives considered

**Direct store access in CLI:** Skip the `Client.Remove()` method
and call `store.Untag()` from the CLI. Rejected because it breaks
the established pattern where CLI calls client methods.

**Service layer abstraction:** Create a `RemoveService` in
`internal/service/`. Rejected as over-engineered for a simple
untag operation.

**Auto GC after removal:** Run garbage collection automatically
after untagging. Rejected to keep `rm` fast and predictable,
consistent with container tooling conventions.
