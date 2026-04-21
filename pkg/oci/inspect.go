package oci

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/redhat-et/skillimage/pkg/lifecycle"
)

// inspectableStore is the minimal interface for reading a manifest.
type inspectableStore interface {
	Resolve(ctx context.Context, ref string) (ocispec.Descriptor, error)
	Fetch(ctx context.Context, desc ocispec.Descriptor) (io.ReadCloser, error)
}

// Inspect retrieves detailed metadata for a skill image in the local store.
func (c *Client) Inspect(ctx context.Context, ref string) (*InspectResult, error) {
	return inspect(ctx, c.store, ref)
}

// InspectRemote retrieves detailed metadata for a skill image on a remote registry.
func (c *Client) InspectRemote(ctx context.Context, ref string) (*InspectResult, error) {
	repo, err := newRemoteRepository(ref)
	if err != nil {
		return nil, fmt.Errorf("creating remote repository: %w", err)
	}
	return inspect(ctx, repo, ref)
}

func inspect(ctx context.Context, store inspectableStore, ref string) (*InspectResult, error) {
	desc, err := store.Resolve(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("resolving %s: %w", ref, err)
	}

	rc, err := store.Fetch(ctx, desc)
	if err != nil {
		return nil, fmt.Errorf("fetching manifest: %w", err)
	}
	defer func() { _ = rc.Close() }()

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

	name := parseNameFromTag(ref)

	version := ann[ocispec.AnnotationVersion]
	status := ann[lifecycle.StatusAnnotation]
	description := ann[ocispec.AnnotationDescription]
	displayName := ann[ocispec.AnnotationTitle]
	authors := ann[ocispec.AnnotationAuthors]
	license := ann[ocispec.AnnotationLicenses]
	created := ann[ocispec.AnnotationCreated]
	tags := ann[AnnotationTags]
	compatibility := ann[AnnotationCompatibility]
	wordCount := ann[AnnotationWordCount]

	var totalSize int64
	for _, layer := range manifest.Layers {
		totalSize += layer.Size
	}

	return &InspectResult{
		Name:          name,
		DisplayName:   displayName,
		Version:       version,
		Status:        status,
		Description:   description,
		Authors:       authors,
		License:       license,
		Tags:          tags,
		Compatibility: compatibility,
		WordCount:     wordCount,
		Digest:        desc.Digest.String(),
		Created:       created,
		MediaType:     desc.MediaType,
		Size:          totalSize,
		LayerCount:    len(manifest.Layers),
	}, nil
}
