package oci

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/redhat-et/oci-skill-registry/pkg/lifecycle"
)

// Inspect retrieves detailed metadata for a skill image stored in the local OCI layout.
func (c *Client) Inspect(ctx context.Context, ref string) (*InspectResult, error) {
	// Resolve the reference to get the manifest descriptor.
	desc, err := c.store.Resolve(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("resolving %s: %w", ref, err)
	}

	// Fetch and parse the manifest.
	rc, err := c.store.Fetch(ctx, desc)
	if err != nil {
		return nil, fmt.Errorf("fetching manifest: %w", err)
	}
	defer rc.Close()

	manifestBytes, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}

	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}

	ann := manifest.Annotations
	if ann == nil {
		ann = make(map[string]string)
	}

	// Extract name from ref (everything before the last colon).
	name := parseNameFromTag(ref)

	// Extract annotations.
	version := ann[ocispec.AnnotationVersion]
	status := ann[lifecycle.StatusAnnotation]
	description := ann[ocispec.AnnotationDescription]
	displayName := ann[ocispec.AnnotationTitle]
	authors := ann[ocispec.AnnotationAuthors]
	license := ann[ocispec.AnnotationLicenses]
	created := ann[ocispec.AnnotationCreated]

	// Compute total size from layers.
	var totalSize int64
	for _, layer := range manifest.Layers {
		totalSize += layer.Size
	}

	return &InspectResult{
		Name:        name,
		DisplayName: displayName,
		Version:     version,
		Status:      status,
		Description: description,
		Authors:     authors,
		License:     license,
		Digest:      desc.Digest.String(),
		Created:     created,
		MediaType:   desc.MediaType,
		Size:        totalSize,
		LayerCount:  len(manifest.Layers),
	}, nil
}
