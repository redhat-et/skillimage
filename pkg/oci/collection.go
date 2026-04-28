package oci

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	godigest "github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/errdef"

	"github.com/redhat-et/skillimage/pkg/collection"
)

// OCI artifact and media types for skill collections.
const (
	CollectionArtifactType = "application/vnd.skillimage.collection.v1+yaml"
	CollectionMediaType    = "application/vnd.skillimage.collection.v1+yaml"
	AnnotationCollectionName = "io.skillimage.collection.name"
)

// BuildCollectionArtifact reads a collection YAML file, validates it, and builds
// an OCI artifact manifest in the local store. Returns the manifest descriptor.
func (c *Client) BuildCollectionArtifact(ctx context.Context, yamlPath, ref string) (ocispec.Descriptor, error) {
	// 1. Parse and validate the collection YAML.
	col, err := collection.ParseFile(yamlPath)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("parsing collection: %w", err)
	}

	validationErrors := collection.Validate(col)
	if len(validationErrors) > 0 {
		return ocispec.Descriptor{}, fmt.Errorf("validation failed: %s", strings.Join(validationErrors, "; "))
	}

	// 2. Read the YAML file bytes.
	yamlBytes, err := os.ReadFile(yamlPath)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("reading YAML file: %w", err)
	}

	// 3. Push YAML bytes as a single layer.
	layerDigest := godigest.FromBytes(yamlBytes)
	layerDesc := ocispec.Descriptor{
		MediaType: CollectionMediaType,
		Digest:    layerDigest,
		Size:      int64(len(yamlBytes)),
	}
	if err := c.store.Push(ctx, layerDesc, bytes.NewReader(yamlBytes)); err != nil && !errors.Is(err, errdef.ErrAlreadyExists) {
		return ocispec.Descriptor{}, fmt.Errorf("pushing layer: %w", err)
	}

	// 4. Push an empty config.
	configBytes := []byte("{}")
	configDigest := godigest.FromBytes(configBytes)
	configDesc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeEmptyJSON,
		Digest:    configDigest,
		Size:      int64(len(configBytes)),
	}
	if err := c.store.Push(ctx, configDesc, bytes.NewReader(configBytes)); err != nil && !errors.Is(err, errdef.ErrAlreadyExists) {
		return ocispec.Descriptor{}, fmt.Errorf("pushing config: %w", err)
	}

	// 5. Build annotations from collection metadata.
	annotations := buildCollectionAnnotations(col)

	// 6. Build and push the OCI manifest.
	return c.buildAndTagManifest(
		ctx,
		configDesc,
		[]ocispec.Descriptor{layerDesc},
		annotations,
		CollectionArtifactType,
		ref,
	)
}

// PushCollection pushes a collection artifact to a remote registry.
// The artifact must already be built and stored locally.
func (c *Client) PushCollection(ctx context.Context, ref string, opts PushOptions) error {
	return c.Push(ctx, ref, opts)
}

// PullCollection fetches a collection artifact from a remote registry,
// parses the YAML, and pulls each referenced skill image into opts.OutputDir.
func (c *Client) PullCollection(ctx context.Context, ref string, opts PullOptions) (*collection.SkillCollection, error) {
	if _, err := c.Pull(ctx, ref, PullOptions{SkipTLSVerify: opts.SkipTLSVerify}); err != nil {
		return nil, fmt.Errorf("pulling collection artifact: %w", err)
	}

	col, err := c.extractCollectionYAML(ctx, ref)
	if err != nil {
		return nil, err
	}

	if opts.OutputDir != "" {
		for _, skill := range col.Skills {
			if _, err := c.Pull(ctx, skill.Image, PullOptions{
				OutputDir:     opts.OutputDir,
				SkipTLSVerify: opts.SkipTLSVerify,
			}); err != nil {
				return nil, fmt.Errorf("pulling skill %s: %w", skill.Name, err)
			}
		}
	}

	return col, nil
}

func (c *Client) extractCollectionYAML(ctx context.Context, ref string) (*collection.SkillCollection, error) {
	desc, err := c.store.Resolve(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("resolving %s: %w", ref, err)
	}

	rc, err := c.store.Fetch(ctx, desc)
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

	if manifest.ArtifactType != CollectionArtifactType {
		return nil, fmt.Errorf("ref %s is not a collection artifact (artifactType=%s)", ref, manifest.ArtifactType)
	}

	if len(manifest.Layers) == 0 {
		return nil, fmt.Errorf("collection manifest has no layers")
	}

	if manifest.Layers[0].MediaType != CollectionMediaType {
		return nil, fmt.Errorf("unexpected layer media type %s, expected %s", manifest.Layers[0].MediaType, CollectionMediaType)
	}

	layerRC, err := c.store.Fetch(ctx, manifest.Layers[0])
	if err != nil {
		return nil, fmt.Errorf("fetching collection layer: %w", err)
	}
	defer func() { _ = layerRC.Close() }()

	return collection.Parse(layerRC)
}

// buildAndTagManifest creates an OCI manifest with the given config, layers,
// and annotations, pushes it to the store, tags it, and returns the descriptor.
func (c *Client) buildAndTagManifest(
	ctx context.Context,
	configDesc ocispec.Descriptor,
	layers []ocispec.Descriptor,
	annotations map[string]string,
	artifactType, ref string,
) (ocispec.Descriptor, error) {
	manifest := ocispec.Manifest{
		Versioned:    specs.Versioned{SchemaVersion: 2},
		MediaType:    ocispec.MediaTypeImageManifest,
		ArtifactType: artifactType,
		Config:       configDesc,
		Layers:       layers,
		Annotations:  annotations,
	}

	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("marshaling manifest: %w", err)
	}

	manifestDigest := godigest.FromBytes(manifestBytes)
	manifestDesc := ocispec.Descriptor{
		MediaType:   ocispec.MediaTypeImageManifest,
		Digest:      manifestDigest,
		Size:        int64(len(manifestBytes)),
		Annotations: annotations,
	}

	if err := c.store.Push(ctx, manifestDesc, bytes.NewReader(manifestBytes)); err != nil && !errors.Is(err, errdef.ErrAlreadyExists) {
		return ocispec.Descriptor{}, fmt.Errorf("pushing manifest: %w", err)
	}

	// Tag the manifest.
	if err := c.store.Tag(ctx, manifestDesc, ref); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("tagging manifest: %w", err)
	}

	return manifestDesc, nil
}

// buildCollectionAnnotations creates OCI annotations from collection metadata.
func buildCollectionAnnotations(col *collection.SkillCollection) map[string]string {
	ann := make(map[string]string)

	// Standard OCI annotations.
	ann[ocispec.AnnotationTitle] = col.Metadata.Name
	ann[ocispec.AnnotationVersion] = col.Metadata.Version
	ann[ocispec.AnnotationCreated] = time.Now().UTC().Format(time.RFC3339)

	if col.Metadata.Description != "" {
		ann[ocispec.AnnotationDescription] = col.Metadata.Description
	}

	// Custom annotation for collection name.
	ann[AnnotationCollectionName] = col.Metadata.Name

	return ann
}
