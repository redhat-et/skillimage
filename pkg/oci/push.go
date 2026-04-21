package oci

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"
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
// like "registry.example.com/namespace/name:tag", configured with credentials
// from Docker and Podman auth files.
func newRemoteRepository(ref string) (*remote.Repository, error) {
	repoRef, _ := splitRefTag(ref)
	repo, err := remote.NewRepository(repoRef)
	if err != nil {
		return nil, err
	}

	store, err := credentialStore()
	if err != nil {
		return nil, fmt.Errorf("loading credentials: %w", err)
	}

	repo.Client = &auth.Client{
		Credential: credentials.Credential(store),
	}

	return repo, nil
}

// credentialStore returns a credential store that checks Docker config first,
// then falls back to Podman's auth.json.
func credentialStore() (credentials.Store, error) {
	dockerStore, err := credentials.NewStoreFromDocker(credentials.StoreOptions{})
	if err != nil {
		return nil, err
	}

	podmanPath := podmanAuthPath()
	if podmanPath == "" {
		return dockerStore, nil
	}

	if _, err := os.Stat(podmanPath); err != nil {
		return dockerStore, nil
	}

	podmanStore, err := credentials.NewStore(podmanPath, credentials.StoreOptions{})
	if err != nil {
		// Podman config unreadable but Docker config works — fall back gracefully.
		return dockerStore, nil
	}

	return credentials.NewStoreWithFallbacks(dockerStore, podmanStore), nil
}

func podmanAuthPath() string {
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		return filepath.Join(xdg, "containers", "auth.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "containers", "auth.json")
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
