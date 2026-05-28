package oci

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"
)

// Push copies an image from the local OCI store to a remote registry.
func (c *Client) Push(ctx context.Context, ref string, opts PushOptions) error {
	repo, err := newRemoteRepository(ref, opts.SkipTLSVerify)
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
func newRemoteRepository(ref string, skipTLSVerify bool) (*remote.Repository, error) {
	repoRef, _ := splitRefTag(ref)
	repo, err := remote.NewRepository(repoRef)
	if err != nil {
		return nil, err
	}

	store, err := credentialStore()
	if err != nil {
		return nil, fmt.Errorf("loading credentials: %w", err)
	}

	repo.Client = newAuthClient(store, skipTLSVerify)
	return repo, nil
}

func insecureHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}, //nolint:gosec // user-requested via --tls-verify=false
		},
	}
}

func newAuthClient(store credentials.Store, skipTLSVerify bool) *auth.Client {
	c := &auth.Client{
		Credential: credentials.Credential(store),
	}
	if skipTLSVerify {
		c.Client = insecureHTTPClient()
	}
	return c
}

// credentialStore returns a credential store that checks Docker config first,
// then falls back to Podman auth files found via podmanAuthPaths.
func credentialStore() (credentials.Store, error) {
	dockerStore, err := credentials.NewStoreFromDocker(credentials.StoreOptions{})
	if err != nil {
		return nil, err
	}

	var fallbacks []credentials.Store
	for _, p := range podmanAuthPaths() {
		if _, err := os.Stat(p); err != nil {
			slog.Debug("podman auth file not found", "path", p)
			continue
		}
		store, err := credentials.NewStore(p, credentials.StoreOptions{})
		if err != nil {
			slog.Debug("skipping unreadable podman auth file", "path", p, "error", err)
			continue
		}
		fallbacks = append(fallbacks, store)
	}

	if len(fallbacks) == 0 {
		return dockerStore, nil
	}
	return credentials.NewStoreWithFallbacks(dockerStore, fallbacks...), nil
}

// podmanAuthPaths returns Podman/containers auth file locations in search
// order per the containers-auth.json(5) spec:
//  1. $XDG_RUNTIME_DIR/containers/auth.json  (Linux primary)
//  2. $XDG_CONFIG_HOME/containers/auth.json   (fallback; defaults to ~/.config)
func podmanAuthPaths() []string {
	var paths []string

	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		paths = append(paths, filepath.Join(xdg, "containers", "auth.json"))
	}

	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		if home, err := os.UserHomeDir(); err == nil {
			configHome = filepath.Join(home, ".config")
		}
	}
	if configHome != "" {
		p := filepath.Join(configHome, "containers", "auth.json")
		if len(paths) == 0 || paths[0] != p {
			paths = append(paths, p)
		}
	}

	return paths
}

// splitRefTag splits an OCI reference into repository and tag/digest.
// Handles registry ports correctly: localhost:5000/ns/name:tag splits
// at the tag colon (after the last /), not the port colon.
// Digest references (name@sha256:abc) split at the @ and return the
// full digest (sha256:abc) as the tag.
func splitRefTag(ref string) (repo, tag string) {
	// Find the last slash to isolate the name portion.
	lastSlash := strings.LastIndex(ref, "/")
	if lastSlash < 0 {
		if idx := strings.Index(ref, "@"); idx >= 0 {
			return ref[:idx], ref[idx+1:]
		}
		if idx := strings.LastIndex(ref, ":"); idx >= 0 {
			return ref[:idx], ref[idx+1:]
		}
		return ref, ""
	}
	tail := ref[lastSlash+1:]
	// Digest reference: name@sha256:abc...
	if idx := strings.Index(tail, "@"); idx >= 0 {
		return ref[:lastSlash+1+idx], tail[idx+1:]
	}
	// Tag reference: name:tag
	if idx := strings.LastIndex(tail, ":"); idx >= 0 {
		return ref[:lastSlash+1+idx], tail[idx+1:]
	}
	return ref, ""
}
