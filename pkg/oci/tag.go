package oci

import (
	"context"
	"fmt"
)

// Tag creates an additional reference for an existing image in the local store.
// This is equivalent to "docker tag" or "podman tag".
func (c *Client) Tag(ctx context.Context, src, dst string) error {
	desc, err := c.store.Resolve(ctx, src)
	if err != nil {
		return fmt.Errorf("resolving %s: %w", src, err)
	}

	if err := c.store.Tag(ctx, desc, dst); err != nil {
		return fmt.Errorf("tagging %s as %s: %w", src, dst, err)
	}

	return nil
}
