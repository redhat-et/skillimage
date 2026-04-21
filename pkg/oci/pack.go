package oci

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	_ "crypto/sha256" // Register SHA256 algorithm for go-digest.
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	godigest "github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/errdef"

	"github.com/redhat-et/skillimage/pkg/lifecycle"
	"github.com/redhat-et/skillimage/pkg/skillcard"
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
	defer func() { _ = f.Close() }()

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

	// 2b. Count words in SKILL.md if present, excluding YAML frontmatter.
	var wordCount int
	skillMDPath := filepath.Join(skillDir, "SKILL.md")
	if data, err := os.ReadFile(skillMDPath); err == nil {
		wordCount = len(strings.Fields(stripFrontmatter(string(data))))
	}

	// 3. Resolve media types for the selected profile.
	layerMediaType, configMediaType, err := resolveMediaTypes(opts.MediaType)
	if err != nil {
		return ocispec.Descriptor{}, err
	}

	// 4. Create a tar.gz layer of all files in the directory.
	layerBuf, uncompressedDigest, err := createLayer(skillDir)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("creating layer: %w", err)
	}

	// 5. Push the layer blob to the store.
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

	// 6. Create and push the OCI image config.
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

	// 7. Build annotations from SkillCard.
	annotations := buildAnnotations(sc, wordCount)

	// 8. Create and push the OCI manifest.
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

	if err := c.store.Push(ctx, manifestDesc, bytes.NewReader(manifestBytes)); err != nil && !errors.Is(err, errdef.ErrAlreadyExists) {
		return ocispec.Descriptor{}, fmt.Errorf("pushing manifest: %w", err)
	}

	// 9. Tag as namespace/name:tag.
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
	images, err := c.listLocal(context.Background())
	if err != nil {
		return nil, fmt.Errorf("listing tags: %w", err)
	}
	return images, nil
}

func (c *Client) listLocal(ctx context.Context) ([]LocalImage, error) {
	var images []LocalImage

	err := c.store.Tags(ctx, "", func(tags []string) error {
		for _, tag := range tags {
			desc, err := c.store.Resolve(ctx, tag)
			if err != nil {
				continue
			}

			img, err := c.imageFromManifest(ctx, tag, desc)
			if err != nil {
				continue
			}
			images = append(images, *img)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return images, nil
}

// imageFromManifest fetches a manifest and extracts LocalImage metadata.
func (c *Client) imageFromManifest(ctx context.Context, tag string, desc ocispec.Descriptor) (*LocalImage, error) {
	rc, err := c.store.Fetch(ctx, desc)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	manifestBytes, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}

	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, err
	}

	ann := manifest.Annotations
	if ann == nil {
		return nil, fmt.Errorf("no annotations")
	}

	name := parseNameFromTag(tag)
	tagPart := ""
	if idx := strings.LastIndex(tag, ":"); idx >= 0 {
		tagPart = tag[idx+1:]
	}

	return &LocalImage{
		Name:    name,
		Version: ann[ocispec.AnnotationVersion],
		Tag:     tagPart,
		Digest:  desc.Digest.String(),
		Status:  ann[lifecycle.StatusAnnotation],
		Created: ann[ocispec.AnnotationCreated],
	}, nil
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

		if !info.Mode().IsRegular() && !info.IsDir() {
			return nil
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return fmt.Errorf("creating tar header for %s: %w", rel, err)
		}
		header.Name = filepath.ToSlash(rel)

		if err := tw.WriteHeader(header); err != nil {
			return fmt.Errorf("writing tar header for %s: %w", rel, err)
		}

		if info.Mode().IsRegular() {
			if err := copyFileToTar(tw, path, rel); err != nil {
				return err
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

func copyFileToTar(tw *tar.Writer, path, rel string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("opening %s: %w", rel, err)
	}
	defer func() { _ = f.Close() }()
	if _, err := io.Copy(tw, f); err != nil {
		return fmt.Errorf("copying %s: %w", rel, err)
	}
	return nil
}

// stripFrontmatter removes YAML frontmatter (delimited by "---") from
// SKILL.md content so that metadata fields don't inflate the word count.
func stripFrontmatter(s string) string {
	if !strings.HasPrefix(s, "---") {
		return s
	}
	end := strings.Index(s[3:], "\n---")
	if end < 0 {
		return s
	}
	// Skip past the closing "---" and its newline.
	body := s[3+end+4:]
	return body
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
