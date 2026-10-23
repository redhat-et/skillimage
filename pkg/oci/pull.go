package oci

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
)

// Pull copies an image from a remote registry into the local store.
// If opts.OutputDir is set, the image is also unpacked into that directory.
func (c *Client) Pull(ctx context.Context, ref string, opts PullOptions) (ocispec.Descriptor, error) {
	desc, _, err := c.PullWithUnpack(ctx, ref, opts)
	return desc, err
}

// PullWithUnpack pulls an image into the local store and returns the directory
// created when OutputDir is set. The image type determines its unpacking rules.
func (c *Client) PullWithUnpack(ctx context.Context, ref string, opts PullOptions) (ocispec.Descriptor, string, error) {
	repo, err := newRemoteRepository(ref, opts.SkipTLSVerify)
	if err != nil {
		return ocispec.Descriptor{}, "", fmt.Errorf("creating remote repository: %w", err)
	}

	_, tag := splitRefTag(ref)
	desc, err := oras.Copy(ctx, repo, tag, c.store, ref, oras.DefaultCopyOptions)
	if err != nil {
		return ocispec.Descriptor{}, "", fmt.Errorf("pulling %s: %w", ref, err)
	}

	var unpackedPath string
	if opts.OutputDir != "" {
		unpackedPath, err = c.UnpackPackage(ctx, ref, opts.OutputDir)
		if err != nil {
			return desc, "", fmt.Errorf("unpacking after pull: %w", err)
		}
	}
	return desc, unpackedPath, nil
}

// Unpack extracts a locally stored skill or Agent Plugin package.
func (c *Client) Unpack(ctx context.Context, ref string, outputDir string) error {
	_, err := c.UnpackPackage(ctx, ref, outputDir)
	return err
}

// UnpackPackage extracts a package using the rules for its detected OCI type
// and returns the created package directory.
func (c *Client) UnpackPackage(ctx context.Context, ref, outputDir string) (string, error) {
	desc, err := c.store.Resolve(ctx, ref)
	if err != nil {
		return "", fmt.Errorf("resolving %s: %w", ref, err)
	}

	rc, err := c.store.Fetch(ctx, desc)
	if err != nil {
		return "", fmt.Errorf("fetching manifest: %w", err)
	}
	defer func() { _ = rc.Close() }()
	manifestBytes, err := readManifestBytes(rc)
	if err != nil {
		return "", fmt.Errorf("reading manifest: %w", err)
	}

	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return "", fmt.Errorf("parsing manifest: %w", err)
	}
	if manifest.Annotations[AnnotationPluginType] == PluginType {
		return c.UnpackPlugin(ctx, ref, outputDir)
	}

	skillName := SkillNameFromRef(ref)
	targetDir := filepath.Join(outputDir, skillName)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", fmt.Errorf("creating output directory: %w", err)
	}

	for _, layer := range manifest.Layers {
		if err := extractLayer(ctx, c, layer, targetDir); err != nil {
			return "", fmt.Errorf("extracting layer %s: %w", layer.Digest, err)
		}
	}

	return targetDir, nil
}

// SkillNameFromRef extracts the skill name from a reference like
// "namespace/name:tag" — it returns "name". Handles both tag
// (name:tag) and digest (name@sha256:...) references.
func SkillNameFromRef(ref string) string {
	name := ref
	if idx := strings.Index(ref, "@"); idx >= 0 {
		name = ref[:idx]
	} else if idx := strings.LastIndex(ref, ":"); idx >= 0 {
		name = ref[:idx]
	}
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}
	return name
}

// sanitizeTarPath validates that a tar entry name stays within the target
// directory, preventing path traversal (Zip Slip) attacks.
func sanitizeTarPath(name string, targetDir string) (string, error) {
	target := filepath.Join(targetDir, filepath.Clean(name))
	rel, err := filepath.Rel(targetDir, target)
	if err != nil {
		return "", fmt.Errorf("tar entry %q: %w", name, err)
	}
	if strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || rel == ".." {
		return "", fmt.Errorf("tar entry %q escapes target directory", name)
	}
	return target, nil
}

func clampDirMode(mode int64) os.FileMode {
	m := os.FileMode(mode) & 0o755
	if m == 0 {
		m = 0o755
	}
	return m
}

func clampFileMode(mode int64) os.FileMode {
	m := os.FileMode(mode) & 0o644
	if m == 0 {
		m = 0o644
	}
	return m
}

func extractFile(target string, r io.Reader, mode os.FileMode) error {
	f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("creating file %s: %w", target, err)
	}
	defer func() { _ = f.Close() }()
	if _, err := io.Copy(f, r); err != nil {
		return fmt.Errorf("writing file %s: %w", target, err)
	}
	return nil
}

// extractLayer fetches a layer from the store, decompresses it, and extracts
// tar entries into targetDir with path traversal protection.
func extractLayer(ctx context.Context, c *Client, layer ocispec.Descriptor, targetDir string) error {
	rc, err := c.store.Fetch(ctx, layer)
	if err != nil {
		return fmt.Errorf("fetching layer: %w", err)
	}
	defer func() { _ = rc.Close() }()

	gz, err := gzip.NewReader(rc)
	if err != nil {
		return fmt.Errorf("creating gzip reader: %w", err)
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar entry: %w", err)
		}

		target, err := sanitizeTarPath(header.Name, targetDir)
		if err != nil {
			return err
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, clampDirMode(header.Mode)); err != nil {
				return fmt.Errorf("creating directory %s: %w", target, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("creating parent directory for %s: %w", target, err)
			}
			if err := extractFile(target, tr, clampFileMode(header.Mode)); err != nil {
				return err
			}
		}
	}

	return nil
}
