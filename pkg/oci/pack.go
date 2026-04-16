package oci

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	_ "crypto/sha256" // Register SHA256 algorithm for go-digest.
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	godigest "github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/redhat-et/oci-skill-registry/pkg/lifecycle"
	"github.com/redhat-et/oci-skill-registry/pkg/skillcard"
)

// Pack reads a skill directory, validates the SkillCard, creates an OCI image,
// and stores it in the local OCI layout. It returns the manifest descriptor.
func (c *Client) Pack(ctx context.Context, skillDir string, opts PackOptions) (ocispec.Descriptor, error) {
	// 1. Read and parse skill.yaml.
	skillPath := filepath.Join(skillDir, "skill.yaml")
	f, err := os.Open(skillPath)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("opening skill.yaml: %w", err)
	}
	defer f.Close()

	sc, err := skillcard.Parse(f)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("parsing skill.yaml: %w", err)
	}

	// 2. Validate the SkillCard.
	validationErrors, err := skillcard.Validate(sc)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("validating skill.yaml: %w", err)
	}
	if len(validationErrors) > 0 {
		var msgs []string
		for _, ve := range validationErrors {
			msgs = append(msgs, ve.String())
		}
		return ocispec.Descriptor{}, fmt.Errorf("skill.yaml validation failed: %s", strings.Join(msgs, "; "))
	}

	// 3. Create a tar.gz layer of all files in the directory.
	layerBuf, uncompressedDigest, err := createLayer(skillDir)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("creating layer: %w", err)
	}

	// 4. Push the layer blob to the store.
	layerBytes := layerBuf.Bytes()
	layerDigest := godigest.FromBytes(layerBytes)
	layerDesc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageLayerGzip,
		Digest:    layerDigest,
		Size:      int64(len(layerBytes)),
	}
	if err := c.store.Push(ctx, layerDesc, bytes.NewReader(layerBytes)); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("pushing layer: %w", err)
	}

	// 5. Create and push the OCI image config.
	configBytes, err := buildImageConfig(uncompressedDigest)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("building image config: %w", err)
	}
	configDigest := godigest.FromBytes(configBytes)
	configDesc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageConfig,
		Digest:    configDigest,
		Size:      int64(len(configBytes)),
	}
	if err := c.store.Push(ctx, configDesc, bytes.NewReader(configBytes)); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("pushing config: %w", err)
	}

	// 6. Build annotations from SkillCard.
	annotations := buildAnnotations(sc)

	// 7. Create and push the OCI manifest.
	manifest := ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    configDesc,
		Layers:    []ocispec.Descriptor{layerDesc},
		Annotations: annotations,
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

	if err := c.store.Push(ctx, manifestDesc, bytes.NewReader(manifestBytes)); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("pushing manifest: %w", err)
	}

	// 8. Tag as namespace/name:tag.
	tag := opts.Tag
	if tag == "" {
		tag = lifecycle.TagForState(sc.Metadata.Version, lifecycle.Draft)
	}
	ref := fmt.Sprintf("%s/%s:%s", sc.Metadata.Namespace, sc.Metadata.Name, tag)
	if err := c.store.Tag(ctx, manifestDesc, ref); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("tagging image: %w", err)
	}

	return manifestDesc, nil
}

// ListLocal reads the store's tags and returns image metadata from manifest annotations.
func (c *Client) ListLocal() ([]LocalImage, error) {
	ctx := context.Background()
	var images []LocalImage

	err := c.store.Tags(ctx, "", func(tags []string) error {
		for _, tag := range tags {
			desc, err := c.store.Resolve(ctx, tag)
			if err != nil {
				continue
			}

			// Fetch and parse the manifest to get annotations.
			rc, err := c.store.Fetch(ctx, desc)
			if err != nil {
				continue
			}
			manifestBytes, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				continue
			}

			var manifest ocispec.Manifest
			if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
				continue
			}

			ann := manifest.Annotations
			if ann == nil {
				continue
			}

			// Extract name from vendor/title or the tag reference itself.
			name := parseNameFromTag(tag)
			version := ann[ocispec.AnnotationVersion]
			status := ann[lifecycle.StatusAnnotation]
			created := ann[ocispec.AnnotationCreated]

			// Extract just the tag portion.
			tagPart := ""
			if idx := strings.LastIndex(tag, ":"); idx >= 0 {
				tagPart = tag[idx+1:]
			}

			images = append(images, LocalImage{
				Name:    name,
				Version: version,
				Tag:     tagPart,
				Digest:  desc.Digest.String(),
				Status:  status,
				Created: created,
			})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing tags: %w", err)
	}

	return images, nil
}

// parseNameFromTag extracts the "namespace/name" portion from a tag reference
// like "namespace/name:tag".
func parseNameFromTag(ref string) string {
	if idx := strings.LastIndex(ref, ":"); idx >= 0 {
		return ref[:idx]
	}
	return ref
}

// createLayer builds a tar.gz archive of all files in dir, skipping hidden
// directories. It returns the compressed buffer and the digest of the
// uncompressed tar (for diff_ids in the image config).
func createLayer(dir string) (*bytes.Buffer, godigest.Digest, error) {
	// First, build the uncompressed tar to compute its digest.
	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}

		// Skip the root directory entry itself.
		if rel == "." {
			return nil
		}

		// Skip hidden directories.
		if info.IsDir() && strings.HasPrefix(info.Name(), ".") {
			return filepath.SkipDir
		}

		// Skip hidden files.
		if strings.HasPrefix(info.Name(), ".") {
			return nil
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return fmt.Errorf("creating tar header for %s: %w", rel, err)
		}
		// Use forward-slash relative paths at root of archive.
		header.Name = filepath.ToSlash(rel)

		if err := tw.WriteHeader(header); err != nil {
			return fmt.Errorf("writing tar header for %s: %w", rel, err)
		}

		if !info.IsDir() {
			f, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("opening %s: %w", rel, err)
			}
			defer f.Close()
			if _, err := io.Copy(tw, f); err != nil {
				return fmt.Errorf("copying %s: %w", rel, err)
			}
		}

		return nil
	})
	if err != nil {
		return nil, "", fmt.Errorf("walking skill directory: %w", err)
	}

	if err := tw.Close(); err != nil {
		return nil, "", fmt.Errorf("closing tar writer: %w", err)
	}

	// Compute digest of uncompressed tar for diff_ids.
	uncompressedDigest := godigest.FromBytes(tarBuf.Bytes())

	// Now gzip the tar.
	var gzBuf bytes.Buffer
	gw := gzip.NewWriter(&gzBuf)
	if _, err := io.Copy(gw, &tarBuf); err != nil {
		return nil, "", fmt.Errorf("compressing layer: %w", err)
	}
	if err := gw.Close(); err != nil {
		return nil, "", fmt.Errorf("closing gzip writer: %w", err)
	}

	return &gzBuf, uncompressedDigest, nil
}

// buildImageConfig creates the OCI image config JSON (FROM scratch equivalent).
func buildImageConfig(diffID godigest.Digest) ([]byte, error) {
	config := ocispec.Image{
		Platform: ocispec.Platform{
			Architecture: "amd64",
			OS:           "linux",
		},
		RootFS: ocispec.RootFS{
			Type:    "layers",
			DiffIDs: []godigest.Digest{diffID},
		},
	}
	return json.Marshal(config)
}
