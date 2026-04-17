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

	_, tag := splitRefTag(ref)
	_, err = oras.Copy(ctx, c.store, ref, repo, tag, oras.DefaultCopyOptions)
	if err != nil {
		return fmt.Errorf("pushing %s: %w", ref, err)
	}

	return nil
}

// CopyTo copies an image from this client's store to another client's store.
// This is useful for testing without a real remote registry.
func (c *Client) CopyTo(ctx context.Context, ref string, dst *Client) error {
	_, tag := splitRefTag(ref)
	_, err := oras.Copy(ctx, c.store, ref, dst.store, tag, oras.DefaultCopyOptions)
	if err != nil {
		return fmt.Errorf("copying %s: %w", ref, err)
	}

	return nil
}

// newRemoteRepository creates a remote.Repository from a full reference string
// like "registry.example.com/namespace/name:tag".
func newRemoteRepository(ref string) (*remote.Repository, error) {
	repoRef, _ := splitRefTag(ref)
	return remote.NewRepository(repoRef)
}

// splitRefTag splits an OCI reference into repository and tag.
// Handles registry ports correctly: localhost:5000/ns/name:tag splits
// at the tag colon (after the last /), not the port colon.
func splitRefTag(ref string) (repo, tag string) {
	// Find the last slash to isolate the name:tag portion.
	lastSlash := strings.LastIndex(ref, "/")
	if lastSlash < 0 {
		// No slash: the whole ref might be name:tag.
		if idx := strings.LastIndex(ref, ":"); idx >= 0 {
			return ref[:idx], ref[idx+1:]
		}
		return ref, ""
	}
	// Look for a colon only after the last slash.
	tail := ref[lastSlash+1:]
	if idx := strings.LastIndex(tail, ":"); idx >= 0 {
		return ref[:lastSlash+1+idx], tail[idx+1:]
	}
	return ref, ""
}
