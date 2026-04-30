package oci

import (
	"context"
	"errors"
	"fmt"

	"oras.land/oras-go/v2/errdef"
)

// ResolveDigest returns the digest of a locally stored image
// identified by its tag reference.
func (c *Client) ResolveDigest(ctx context.Context, ref string) (string, error) {
	desc, err := c.store.Resolve(ctx, ref)
	if err != nil {
		if errors.Is(err, errdef.ErrNotFound) {
			return "", fmt.Errorf("image not found: %s", ref)
		}
		return "", fmt.Errorf("resolving %s: %w", ref, err)
	}
	return desc.Digest.String(), nil
}
