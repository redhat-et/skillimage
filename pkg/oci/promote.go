package oci

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	godigest "github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/errdef"

	"github.com/redhat-et/oci-skill-registry/pkg/lifecycle"
)

// promotableStore is the minimal interface required for promoting a skill
// image. Both oci.Store and remote.Repository satisfy this interface.
type promotableStore interface {
	Resolve(ctx context.Context, ref string) (ocispec.Descriptor, error)
	Fetch(ctx context.Context, desc ocispec.Descriptor) (io.ReadCloser, error)
	Push(ctx context.Context, desc ocispec.Descriptor, r io.Reader) error
	Tag(ctx context.Context, desc ocispec.Descriptor, ref string) error
}

// PromoteLocal promotes a skill image within the local store by
// transitioning it from its current lifecycle state to the target state.
// Image layers remain unchanged; only the manifest annotation and tags
// are updated.
func (c *Client) PromoteLocal(ctx context.Context, ref string, to lifecycle.State) error {
	return promote(ctx, c.store, ref, to)
}

// Promote promotes a skill on a remote registry by transitioning it
// from its current lifecycle state to the target state.
func (c *Client) Promote(ctx context.Context, ref string, to lifecycle.State, _ PromoteOptions) error {
	repo, err := newRemoteRepository(ref)
	if err != nil {
		return fmt.Errorf("creating remote repository: %w", err)
	}
	return promote(ctx, repo, ref, to)
}

// promote performs the lifecycle state transition on any promotable store.
func promote(ctx context.Context, store promotableStore, ref string, to lifecycle.State) error {
	// 1. Resolve the reference to get the manifest descriptor.
	desc, err := store.Resolve(ctx, ref)
	if err != nil {
		return fmt.Errorf("resolving %s: %w", ref, err)
	}

	// 2. Fetch the manifest bytes.
	rc, err := store.Fetch(ctx, desc)
	if err != nil {
		return fmt.Errorf("fetching manifest for %s: %w", ref, err)
	}
	manifestBytes, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		return fmt.Errorf("reading manifest for %s: %w", ref, err)
	}

	// 3. Unmarshal the manifest.
	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return fmt.Errorf("parsing manifest for %s: %w", ref, err)
	}

	// 4. Read the current status and validate the transition.
	if manifest.Annotations == nil {
		return fmt.Errorf("manifest has no annotations")
	}

	currentStatus := manifest.Annotations[lifecycle.StatusAnnotation]
	from, err := lifecycle.ParseState(currentStatus)
	if err != nil {
		return fmt.Errorf("parsing current state %q: %w", currentStatus, err)
	}

	if !lifecycle.ValidTransition(from, to) {
		return fmt.Errorf("invalid transition: %s -> %s", from, to)
	}

	// 5. Update the status annotation.
	manifest.Annotations[lifecycle.StatusAnnotation] = string(to)

	// 6. Marshal the updated manifest and compute the new digest.
	updatedBytes, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("marshaling updated manifest: %w", err)
	}

	newDigest := godigest.FromBytes(updatedBytes)
	newDesc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
		Digest:    newDigest,
		Size:      int64(len(updatedBytes)),
	}

	// 7. Push the new manifest to the store.
	if err := store.Push(ctx, newDesc, bytes.NewReader(updatedBytes)); err != nil && !errors.Is(err, errdef.ErrAlreadyExists) {
		return fmt.Errorf("pushing updated manifest: %w", err)
	}

	// 8. Compute the new tag and tag the manifest.
	namespaceName := parseNameFromTag(ref)
	version := manifest.Annotations[ocispec.AnnotationVersion]
	newTag := lifecycle.TagForState(version, to)

	if newTag != "" {
		tagRef := fmt.Sprintf("%s:%s", namespaceName, newTag)
		if err := store.Tag(ctx, newDesc, tagRef); err != nil {
			return fmt.Errorf("tagging as %s: %w", tagRef, err)
		}
	}

	// 9. For published state, also tag as "latest".
	if to == lifecycle.Published {
		latestRef := fmt.Sprintf("%s:latest", namespaceName)
		if err := store.Tag(ctx, newDesc, latestRef); err != nil {
			return fmt.Errorf("tagging as %s: %w", latestRef, err)
		}
	}

	return nil
}
