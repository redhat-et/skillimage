package oci

import (
	"context"
	"errors"
	"fmt"

	"oras.land/oras-go/v2/errdef"
)

// Remove removes a skill image from the local store by its tag reference
// (e.g., "test/test-skill:1.0.0-draft"). Unreferenced blobs are not
// cleaned up; use Prune for that.
func (c *Client) Remove(ctx context.Context, ref string) error {
	if _, err := c.store.Resolve(ctx, ref); err != nil {
		if errors.Is(err, errdef.ErrNotFound) {
			return fmt.Errorf("image not found: %s", ref)
		}
		return fmt.Errorf("resolving %s: %w", ref, err)
	}

	if err := c.store.Untag(ctx, ref); err != nil {
		return fmt.Errorf("removing %s: %w", ref, err)
	}

	return nil
}
