package oci

import (
	"context"
	"fmt"
	"strings"

	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/registry/remote"
)

// Push copies an image from the local OCI store to a remote registry.
func (c *Client) Push(ctx context.Context, ref string, _ PushOptions) error {
	repo, err := newRemoteRepository(ref)
	if err != nil {
		return fmt.Errorf("creating remote repository: %w", err)
	}

	tag := tagFromRef(ref)
	_, err = oras.Copy(ctx, c.store, ref, repo, tag, oras.DefaultCopyOptions)
	if err != nil {
		return fmt.Errorf("pushing %s: %w", ref, err)
	}

	return nil
}

// CopyTo copies an image from this client's store to another client's store.
// This is useful for testing without a real remote registry.
func (c *Client) CopyTo(ctx context.Context, ref string, dst *Client) error {
	tag := tagFromRef(ref)
	_, err := oras.Copy(ctx, c.store, ref, dst.store, tag, oras.DefaultCopyOptions)
	if err != nil {
		return fmt.Errorf("copying %s: %w", ref, err)
	}

	return nil
}

// newRemoteRepository creates a remote.Repository from a full reference string
// like "registry.example.com/namespace/name:tag".
func newRemoteRepository(ref string) (*remote.Repository, error) {
	// Strip tag for repository creation.
	repoRef := ref
	if idx := strings.LastIndex(ref, ":"); idx >= 0 {
		repoRef = ref[:idx]
	}
	return remote.NewRepository(repoRef)
}

// tagFromRef extracts the tag portion after the last ":" in a reference.
func tagFromRef(ref string) string {
	if idx := strings.LastIndex(ref, ":"); idx >= 0 {
		return ref[idx+1:]
	}
	return ref
}
