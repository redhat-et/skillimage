package oci

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	godigest "github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/errdef"

	"github.com/redhat-et/skillimage/pkg/lifecycle"
	"github.com/redhat-et/skillimage/pkg/skillcard"
)

// BundlePackOptions configures the PackBundle operation.
type BundlePackOptions struct {
	Tag       string
	MediaType MediaTypeProfile
}

// PackBundle reads a directory containing multiple skill subdirectories,
// validates each SkillCard, creates a single OCI image with all skills,
// and stores it in the local OCI layout.
func (c *Client) PackBundle(ctx context.Context, bundleDir string, opts BundlePackOptions) (ocispec.Descriptor, error) {
	if opts.Tag == "" {
		return ocispec.Descriptor{}, fmt.Errorf("--tag is required for bundles")
	}

	entries, err := os.ReadDir(bundleDir)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("reading bundle directory: %w", err)
	}

	var skillNames []string
	var namespace string
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		skillDir := filepath.Join(bundleDir, entry.Name())
		skillPath := filepath.Join(skillDir, "skill.yaml")
		if _, err := os.Stat(skillPath); err != nil {
			continue
		}

		f, err := os.Open(skillPath)
		if err != nil {
			return ocispec.Descriptor{}, fmt.Errorf("opening %s: %w", skillPath, err)
		}
		sc, err := skillcard.Parse(f)
		_ = f.Close()
		if err != nil {
			return ocispec.Descriptor{}, fmt.Errorf("parsing %s: %w", skillPath, err)
		}

		validationErrors, err := skillcard.Validate(sc)
		if err != nil {
			return ocispec.Descriptor{}, fmt.Errorf("validating %s: %w", skillPath, err)
		}
		if len(validationErrors) > 0 {
			var msgs []string
			for _, ve := range validationErrors {
				msgs = append(msgs, ve.String())
			}
			return ocispec.Descriptor{}, fmt.Errorf("%s validation failed: %s", skillPath, strings.Join(msgs, "; "))
		}

		skillNames = append(skillNames, sc.Metadata.Name)
		if namespace == "" {
			namespace = sc.Metadata.Namespace
		} else if sc.Metadata.Namespace != namespace {
			return ocispec.Descriptor{}, fmt.Errorf(
				"namespace mismatch: skill %s has namespace %q, expected %q (all skills in a bundle must share the same namespace)",
				sc.Metadata.Name, sc.Metadata.Namespace, namespace,
			)
		}
	}

	if len(skillNames) == 0 {
		return ocispec.Descriptor{}, fmt.Errorf("no valid skill subdirectories found in %s", bundleDir)
	}

	layerMediaType, configMediaType := resolveMediaTypes(opts.MediaType)

	layerBuf, uncompressedDigest, err := createLayer(bundleDir)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("creating layer: %w", err)
	}

	layerBytes := layerBuf.Bytes()
	layerDigest := godigest.FromBytes(layerBytes)
	layerDesc := ocispec.Descriptor{
		MediaType: layerMediaType,
		Digest:    layerDigest,
		Size:      int64(len(layerBytes)),
	}
	if err := c.store.Push(ctx, layerDesc, bytes.NewReader(layerBytes)); err != nil && !errors.Is(err, errdef.ErrAlreadyExists) {
		return ocispec.Descriptor{}, fmt.Errorf("pushing layer: %w", err)
	}

	configBytes, err := buildImageConfig(uncompressedDigest)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("building image config: %w", err)
	}
	configDigest := godigest.FromBytes(configBytes)
	configDesc := ocispec.Descriptor{
		MediaType: configMediaType,
		Digest:    configDigest,
		Size:      int64(len(configBytes)),
	}
	if err := c.store.Push(ctx, configDesc, bytes.NewReader(configBytes)); err != nil && !errors.Is(err, errdef.ErrAlreadyExists) {
		return ocispec.Descriptor{}, fmt.Errorf("pushing config: %w", err)
	}

	skillsJSON, err := json.Marshal(skillNames)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("marshaling skill names: %w", err)
	}
	ann := map[string]string{
		AnnotationBundle:            "true",
		AnnotationBundleSkills:      string(skillsJSON),
		AnnotationStatus:            string(lifecycle.Draft),
		ocispec.AnnotationVersion:   opts.Tag,
		ocispec.AnnotationCreated:   time.Now().UTC().Format(time.RFC3339),
	}
	if namespace != "" {
		ann[ocispec.AnnotationVendor] = namespace
	}

	bundleName := filepath.Base(bundleDir)
	ann[ocispec.AnnotationTitle] = bundleName

	manifest := ocispec.Manifest{
		Versioned:   specs.Versioned{SchemaVersion: 2},
		MediaType:   ocispec.MediaTypeImageManifest,
		Config:      configDesc,
		Layers:      []ocispec.Descriptor{layerDesc},
		Annotations: ann,
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
		Annotations: ann,
	}

	if err := c.store.Push(ctx, manifestDesc, bytes.NewReader(manifestBytes)); err != nil && !errors.Is(err, errdef.ErrAlreadyExists) {
		return ocispec.Descriptor{}, fmt.Errorf("pushing manifest: %w", err)
	}

	ref := fmt.Sprintf("%s/%s:%s", namespace, bundleName, opts.Tag)
	if err := c.store.Tag(ctx, manifestDesc, ref); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("tagging bundle image: %w", err)
	}

	return manifestDesc, nil
}
