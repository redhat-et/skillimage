package oci

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	_ "crypto/sha256" // Register SHA256 for go-digest.
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	godigest "github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/errdef"

	"github.com/redhat-et/skillimage/pkg/agentplugin"
	"github.com/redhat-et/skillimage/pkg/lifecycle"
)

const (
	// PluginType identifies a portable Agent Plugins package in OCI annotations.
	PluginType = "agent-plugin"

	AnnotationPluginType       = "io.skillimage.type"
	AnnotationPluginSchema     = "io.skillimage.plugin.schema"
	AnnotationPluginSkills     = "io.skillimage.plugin.skills"
	AnnotationPluginMCP        = "io.skillimage.plugin.mcp-servers"
	AnnotationPluginExtensions = "io.skillimage.plugin.extensions"
	AnnotationPluginKeywords   = "io.skillimage.plugin.keywords"
	AnnotationPluginAuthorURL  = "io.skillimage.plugin.author-url"
)

// PluginInspectResult describes a packaged Agent Plugin and its OCI image.
type PluginInspectResult struct {
	Name        string
	Version     string
	Description string
	Author      string
	AuthorURL   string
	Homepage    string
	License     string
	Repository  string
	Keywords    []string
	SpecVersion string
	Skills      []string
	MCPServers  []string
	Extensions  []string
	Digest      string
	Created     string
	MediaType   string
	LayerType   string
	Size        int64
	LayerCount  int
}

// BuildPlugin validates and packages a complete Agent Plugins directory into
// an OCI image in the local store. The image contains one complete plugin
// filesystem layer, including dotfiles and executable files.
func (c *Client) BuildPlugin(ctx context.Context, pluginDir string, opts PluginBuildOptions) (PluginBuildResult, error) {
	profile, err := ParseMediaTypeProfile(string(opts.MediaType))
	if err != nil {
		return PluginBuildResult{}, err
	}
	if profile != MediaTypeStandard {
		return PluginBuildResult{}, fmt.Errorf("agent plugin packages require standard OCI media types")
	}

	info, err := agentplugin.ValidateDir(pluginDir)
	if err != nil {
		return PluginBuildResult{}, err
	}
	ref, err := resolvePluginImageRef(info.Manifest, opts)
	if err != nil {
		return PluginBuildResult{}, err
	}

	layerBuf, diffID, err := createPluginLayer(info.Root)
	if err != nil {
		return PluginBuildResult{}, fmt.Errorf("creating plugin layer: %w", err)
	}
	layerBytes := layerBuf.Bytes()
	layerDesc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageLayerGzip,
		Digest:    godigest.FromBytes(layerBytes),
		Size:      int64(len(layerBytes)),
	}
	if err := c.store.Push(ctx, layerDesc, bytes.NewReader(layerBytes)); err != nil && !errors.Is(err, errdef.ErrAlreadyExists) {
		return PluginBuildResult{}, fmt.Errorf("pushing plugin layer: %w", err)
	}

	configBytes, err := buildImageConfig(diffID)
	if err != nil {
		return PluginBuildResult{}, fmt.Errorf("building plugin image config: %w", err)
	}
	configDesc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageConfig,
		Digest:    godigest.FromBytes(configBytes),
		Size:      int64(len(configBytes)),
	}
	if err := c.store.Push(ctx, configDesc, bytes.NewReader(configBytes)); err != nil && !errors.Is(err, errdef.ErrAlreadyExists) {
		return PluginBuildResult{}, fmt.Errorf("pushing plugin config: %w", err)
	}

	annotations, err := pluginAnnotations(info)
	if err != nil {
		return PluginBuildResult{}, err
	}
	desc, err := c.buildAndTagManifest(ctx, configDesc, []ocispec.Descriptor{layerDesc}, annotations, "", ref)
	if err != nil {
		return PluginBuildResult{}, err
	}
	return PluginBuildResult{ImageRef: ref, Descriptor: desc}, nil
}

func resolvePluginImageRef(manifest agentplugin.Manifest, opts PluginBuildOptions) (string, error) {
	ref := opts.ImageRef
	if ref != "" && strings.TrimSpace(ref) == "" {
		return "", fmt.Errorf("an OCI reference is required")
	}
	if ref == "" {
		tag := opts.Tag
		if tag == "" {
			tag = manifest.Version
		}
		if tag == "" {
			return "", fmt.Errorf("plugin.json has no version; provide a plugin-image-ref or --tag")
		}
		return manifest.Name + ":" + tag, nil
	}
	if opts.Tag != "" {
		return withImageTag(ref, opts.Tag)
	}
	if strings.Contains(ref, "@") || hasImageTag(ref) {
		return ref, nil
	}
	if manifest.Version == "" {
		return "", fmt.Errorf("plugin.json has no version; provide --tag or a tagged plugin-image-ref")
	}
	return withImageTag(ref, manifest.Version)
}

func hasImageTag(ref string) bool {
	return strings.LastIndex(ref, ":") > strings.LastIndex(ref, "/")
}

func withImageTag(ref, tag string) (string, error) {
	if strings.Contains(ref, "@") {
		return "", fmt.Errorf("--tag cannot override a digest reference")
	}
	colon := strings.LastIndex(ref, ":")
	slash := strings.LastIndex(ref, "/")
	if colon > slash {
		return ref[:colon+1] + tag, nil
	}
	return ref + ":" + tag, nil
}

// InspectPlugin retrieves plugin metadata from a local image.
func (c *Client) InspectPlugin(ctx context.Context, ref string) (*PluginInspectResult, error) {
	return inspectPlugin(ctx, c.store, ref)
}

// InspectPluginRemote retrieves plugin metadata from a registry without
// downloading the plugin filesystem layer.
func (c *Client) InspectPluginRemote(ctx context.Context, ref string, opts InspectOptions) (*PluginInspectResult, error) {
	repo, err := newRemoteRepository(ref, opts.SkipTLSVerify)
	if err != nil {
		return nil, fmt.Errorf("creating remote repository: %w", err)
	}
	return inspectPlugin(ctx, repo, ref)
}

// PullPlugin pulls an Agent Plugin image into the local store. If OutputDir is
// set, it also extracts the complete plugin into OutputDir/<plugin-name> and
// returns that path.
func (c *Client) PullPlugin(ctx context.Context, ref string, opts PullOptions) (ocispec.Descriptor, string, error) {
	desc, err := c.Pull(ctx, ref, PullOptions{SkipTLSVerify: opts.SkipTLSVerify})
	if err != nil {
		return ocispec.Descriptor{}, "", err
	}
	if _, _, err := readPluginManifest(ctx, c.store, ref); err != nil {
		return desc, "", err
	}
	if opts.OutputDir == "" {
		return desc, "", nil
	}
	path, err := c.UnpackPlugin(ctx, ref, opts.OutputDir)
	if err != nil {
		return desc, "", fmt.Errorf("unpacking plugin: %w", err)
	}
	return desc, path, nil
}

// UnpackPlugin extracts a locally stored Agent Plugin into outputDir/<name>.
// The destination must not already exist.
func (c *Client) UnpackPlugin(ctx context.Context, ref, outputDir string) (string, error) {
	manifest, _, err := readPluginManifest(ctx, c.store, ref)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", fmt.Errorf("creating output directory: %w", err)
	}
	outputAbs, err := filepath.Abs(outputDir)
	if err != nil {
		return "", fmt.Errorf("resolving output directory: %w", err)
	}
	tempDir, err := os.MkdirTemp(outputAbs, ".skillctl-plugin-")
	if err != nil {
		return "", fmt.Errorf("creating temporary plugin directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	for _, layer := range manifest.Layers {
		if layer.MediaType != ocispec.MediaTypeImageLayerGzip {
			return "", fmt.Errorf("unsupported plugin layer media type %q", layer.MediaType)
		}
		if err := extractPluginLayer(ctx, c, layer, tempDir); err != nil {
			return "", fmt.Errorf("extracting plugin layer %s: %w", layer.Digest, err)
		}
	}
	info, err := agentplugin.ValidateDir(tempDir)
	if err != nil {
		return "", fmt.Errorf("validating pulled plugin: %w", err)
	}
	destination := filepath.Join(outputAbs, info.Manifest.Name)
	if _, err := os.Lstat(destination); err == nil {
		return "", fmt.Errorf("destination already exists: %s", destination)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("checking destination: %w", err)
	}
	if err := os.Rename(tempDir, destination); err != nil {
		return "", fmt.Errorf("installing plugin directory: %w", err)
	}
	return destination, nil
}

func inspectPlugin(ctx context.Context, store inspectableStore, ref string) (*PluginInspectResult, error) {
	manifest, desc, err := readPluginManifest(ctx, store, ref)
	if err != nil {
		return nil, err
	}
	return inspectPluginManifest(manifest, desc)
}

func inspectPluginManifest(manifest ocispec.Manifest, desc ocispec.Descriptor) (*PluginInspectResult, error) {
	ann := manifest.Annotations
	if ann[AnnotationPluginType] != PluginType {
		return nil, fmt.Errorf("manifest is not an Agent Plugin image")
	}
	if ann[AnnotationPluginSchema] != agentplugin.ManifestSchemaV1 {
		return nil, fmt.Errorf("invalid plugin schema annotation %q", ann[AnnotationPluginSchema])
	}
	if len(manifest.Layers) == 0 {
		return nil, fmt.Errorf("plugin image has no layers")
	}
	for _, layer := range manifest.Layers {
		if layer.MediaType != ocispec.MediaTypeImageLayerGzip {
			return nil, fmt.Errorf("unsupported plugin layer media type %q", layer.MediaType)
		}
	}
	result := &PluginInspectResult{
		Name:        ann[ocispec.AnnotationTitle],
		Version:     ann[ocispec.AnnotationVersion],
		Description: ann[ocispec.AnnotationDescription],
		Author:      ann[ocispec.AnnotationAuthors],
		AuthorURL:   ann[AnnotationPluginAuthorURL],
		Homepage:    ann[ocispec.AnnotationURL],
		License:     ann[ocispec.AnnotationLicenses],
		Repository:  ann[ocispec.AnnotationSource],
		SpecVersion: agentplugin.SpecVersionV1,
		Digest:      desc.Digest.String(),
		Created:     ann[ocispec.AnnotationCreated],
		MediaType:   desc.MediaType,
		LayerCount:  len(manifest.Layers),
	}
	if err := json.Unmarshal([]byte(ann[AnnotationPluginSkills]), &result.Skills); err != nil && ann[AnnotationPluginSkills] != "" {
		return nil, fmt.Errorf("parsing plugin skill annotations: %w", err)
	}
	if err := json.Unmarshal([]byte(ann[AnnotationPluginMCP]), &result.MCPServers); err != nil && ann[AnnotationPluginMCP] != "" {
		return nil, fmt.Errorf("parsing plugin MCP annotations: %w", err)
	}
	if err := json.Unmarshal([]byte(ann[AnnotationPluginExtensions]), &result.Extensions); err != nil && ann[AnnotationPluginExtensions] != "" {
		return nil, fmt.Errorf("parsing plugin extension annotations: %w", err)
	}
	if err := json.Unmarshal([]byte(ann[AnnotationPluginKeywords]), &result.Keywords); err != nil && ann[AnnotationPluginKeywords] != "" {
		return nil, fmt.Errorf("parsing plugin keyword annotations: %w", err)
	}
	for _, layer := range manifest.Layers {
		result.Size += layer.Size
		if result.LayerType == "" {
			result.LayerType = layer.MediaType
		}
	}
	return result, nil
}

func readPluginManifest(ctx context.Context, store inspectableStore, ref string) (ocispec.Manifest, ocispec.Descriptor, error) {
	desc, err := store.Resolve(ctx, ref)
	if err != nil {
		return ocispec.Manifest{}, ocispec.Descriptor{}, fmt.Errorf("resolving %s: %w", ref, err)
	}
	rc, err := store.Fetch(ctx, desc)
	if err != nil {
		return ocispec.Manifest{}, ocispec.Descriptor{}, fmt.Errorf("fetching plugin manifest: %w", err)
	}
	defer func() { _ = rc.Close() }()
	data, err := readManifestBytes(rc)
	if err != nil {
		return ocispec.Manifest{}, ocispec.Descriptor{}, fmt.Errorf("reading plugin manifest: %w", err)
	}
	var manifest ocispec.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return ocispec.Manifest{}, ocispec.Descriptor{}, fmt.Errorf("parsing plugin manifest: %w", err)
	}
	if manifest.Annotations[AnnotationPluginType] != PluginType {
		return ocispec.Manifest{}, ocispec.Descriptor{}, fmt.Errorf("ref %s is not an Agent Plugin image", ref)
	}
	if manifest.Annotations[AnnotationPluginSchema] != agentplugin.ManifestSchemaV1 {
		return ocispec.Manifest{}, ocispec.Descriptor{}, fmt.Errorf("ref %s has an unsupported Agent Plugin schema", ref)
	}
	if len(manifest.Layers) == 0 {
		return ocispec.Manifest{}, ocispec.Descriptor{}, fmt.Errorf("plugin image has no layers")
	}
	for _, layer := range manifest.Layers {
		if layer.MediaType != ocispec.MediaTypeImageLayerGzip {
			return ocispec.Manifest{}, ocispec.Descriptor{}, fmt.Errorf("unsupported plugin layer media type %q", layer.MediaType)
		}
	}
	return manifest, desc, nil
}

func pluginAnnotations(info *agentplugin.Info) (map[string]string, error) {
	ann := map[string]string{
		ocispec.AnnotationTitle:    info.Manifest.Name,
		ocispec.AnnotationCreated:  time.Now().UTC().Format(time.RFC3339),
		AnnotationPluginType:       PluginType,
		AnnotationPluginSchema:     info.Manifest.Schema,
		lifecycle.StatusAnnotation: string(lifecycle.Draft),
		AnnotationPluginSkills:     marshalNames(info.Skills),
		AnnotationPluginMCP:        marshalNames(info.MCPServers),
		AnnotationPluginExtensions: marshalNames(info.Extensions),
		AnnotationPluginKeywords:   marshalNames(info.Manifest.Keywords),
	}
	if info.Manifest.Version != "" {
		ann[ocispec.AnnotationVersion] = info.Manifest.Version
	}
	if info.Manifest.Description != "" {
		ann[ocispec.AnnotationDescription] = info.Manifest.Description
	}
	if info.Manifest.License != "" {
		ann[ocispec.AnnotationLicenses] = info.Manifest.License
	}
	if info.Manifest.Repository != "" {
		ann[ocispec.AnnotationSource] = info.Manifest.Repository
	}
	if info.Manifest.Homepage != "" {
		ann[ocispec.AnnotationURL] = info.Manifest.Homepage
	}
	if info.Manifest.Author != nil {
		author := info.Manifest.Author.Name
		if info.Manifest.Author.Email != "" {
			if author != "" {
				author += " "
			}
			author += "<" + info.Manifest.Author.Email + ">"
		}
		if author != "" {
			ann[ocispec.AnnotationAuthors] = author
		}
		if info.Manifest.Author.URL != "" {
			ann[AnnotationPluginAuthorURL] = info.Manifest.Author.URL
		}
	}
	if _, err := url.ParseRequestURI(info.Manifest.Repository); err != nil && info.Manifest.Repository != "" {
		// Agent Plugins validates metadata strings by type only. Keep non-URL
		// repository strings in the package, but omit them from the OCI source
		// annotation, whose value is expected to be a URI.
		delete(ann, ocispec.AnnotationSource)
	}
	return ann, nil
}

func marshalNames(names []string) string {
	if names == nil {
		names = []string{}
	}
	data, _ := json.Marshal(names)
	return string(data)
}

func createPluginLayer(root string) (*bytes.Buffer, godigest.Digest, error) {
	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	err := filepath.WalkDir(root, func(filePath string, entry os.DirEntry, walkErr error) error {
		if filePath != root && agentplugin.IsVCSMetadataEntry(filepath.Base(filePath)) {
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if walkErr != nil {
			return walkErr
		}
		if filePath == root {
			return nil
		}
		rel, err := filepath.Rel(root, filePath)
		if err != nil {
			return err
		}
		info, err := os.Lstat(filePath)
		if err != nil {
			return err
		}
		linkTarget := ""
		if info.Mode()&os.ModeSymlink != 0 {
			linkTarget, err = os.Readlink(filePath)
			if err != nil {
				return fmt.Errorf("reading symlink %s: %w", rel, err)
			}
		}
		header, err := tar.FileInfoHeader(info, filepath.ToSlash(linkTarget))
		if err != nil {
			return fmt.Errorf("creating tar header for %s: %w", rel, err)
		}
		header.Name = filepath.ToSlash(rel)
		header.Mode = int64(info.Mode().Perm() & 0o755)
		if info.IsDir() && header.Mode == 0 {
			header.Mode = 0o755
		}
		if err := tw.WriteHeader(header); err != nil {
			return fmt.Errorf("writing tar header for %s: %w", rel, err)
		}
		if info.Mode().IsRegular() {
			f, err := os.Open(filePath)
			if err != nil {
				return fmt.Errorf("opening %s: %w", rel, err)
			}
			_, copyErr := io.Copy(tw, f)
			closeErr := f.Close()
			if copyErr != nil {
				return fmt.Errorf("copying %s: %w", rel, copyErr)
			}
			if closeErr != nil {
				return fmt.Errorf("closing %s: %w", rel, closeErr)
			}
		}
		return nil
	})
	if err != nil {
		return nil, "", fmt.Errorf("walking plugin directory: %w", err)
	}
	if err := tw.Close(); err != nil {
		return nil, "", fmt.Errorf("closing plugin tar: %w", err)
	}
	diffID := godigest.FromBytes(tarBuf.Bytes())
	var compressed bytes.Buffer
	gw := gzip.NewWriter(&compressed)
	if _, err := io.Copy(gw, &tarBuf); err != nil {
		return nil, "", fmt.Errorf("compressing plugin layer: %w", err)
	}
	if err := gw.Close(); err != nil {
		return nil, "", fmt.Errorf("closing compressed plugin layer: %w", err)
	}
	return &compressed, diffID, nil
}

type pluginSymlink struct {
	path   string
	target string
}

func extractPluginLayer(ctx context.Context, c *Client, layer ocispec.Descriptor, targetDir string) error {
	_ = ctx // Keep the extraction signature aligned with other context-aware OCI operations.
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
	root, err := filepath.EvalSymlinks(targetDir)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	seen := make(map[string]bool)
	var links []pluginSymlink
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar entry: %w", err)
		}
		target, rel, err := pluginTarTarget(root, header.Name)
		if err != nil {
			return err
		}
		if seen[rel] {
			return fmt.Errorf("duplicate tar entry %q", header.Name)
		}
		seen[rel] = true
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, clampDirMode(header.Mode)); err != nil {
				return fmt.Errorf("creating directory %s: %w", rel, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("creating parent directory for %s: %w", rel, err)
			}
			mode := os.FileMode(header.Mode) & 0o755
			if mode == 0 {
				mode = 0o644
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
			if err != nil {
				return fmt.Errorf("creating file %s: %w", rel, err)
			}
			_, copyErr := io.Copy(f, tr)
			closeErr := f.Close()
			if copyErr != nil {
				return fmt.Errorf("writing file %s: %w", rel, copyErr)
			}
			if closeErr != nil {
				return fmt.Errorf("closing file %s: %w", rel, closeErr)
			}
		case tar.TypeSymlink:
			if filepath.IsAbs(filepath.FromSlash(header.Linkname)) || filepath.VolumeName(filepath.FromSlash(header.Linkname)) != "" || strings.Contains(header.Linkname, "\\") {
				return fmt.Errorf("symlink %q has an absolute or invalid target", rel)
			}
			links = append(links, pluginSymlink{path: target, target: header.Linkname})
		default:
			return fmt.Errorf("unsupported tar entry type %d for %q", header.Typeflag, rel)
		}
	}
	for _, link := range links {
		if err := ensureRealDirectory(root, filepath.Dir(link.path)); err != nil {
			return fmt.Errorf("checking symlink parent: %w", err)
		}
		lexicalTarget := filepath.Clean(filepath.Join(filepath.Dir(link.path), filepath.FromSlash(link.target)))
		if !withinFilesystemRoot(root, lexicalTarget) {
			return fmt.Errorf("symlink %s escapes plugin root", link.path)
		}
		if err := os.Symlink(filepath.FromSlash(link.target), link.path); err != nil {
			return fmt.Errorf("creating symlink %s: %w", link.path, err)
		}
	}
	for _, link := range links {
		resolved, err := filepath.EvalSymlinks(link.path)
		if err != nil {
			return fmt.Errorf("resolving symlink %s: %w", link.path, err)
		}
		if !withinFilesystemRoot(root, resolved) {
			return fmt.Errorf("symlink %s resolves outside plugin root", link.path)
		}
	}
	return nil
}

func pluginTarTarget(root, name string) (string, string, error) {
	platformName := filepath.FromSlash(name)
	if name == "" || strings.ContainsAny(name, "\\\x00") || path.IsAbs(name) || filepath.IsAbs(platformName) || filepath.VolumeName(platformName) != "" {
		return "", "", fmt.Errorf("unsafe tar entry path %q", name)
	}
	normalized := name
	for strings.HasPrefix(normalized, "./") {
		normalized = strings.TrimPrefix(normalized, "./")
	}
	for _, part := range strings.Split(normalized, "/") {
		if part == ".." {
			return "", "", fmt.Errorf("unsafe tar entry path %q", name)
		}
	}
	normalized = strings.TrimRight(normalized, "/")
	if normalized == "" {
		normalized = "."
	}
	clean := path.Clean(normalized)
	if clean == ".." || strings.HasPrefix(clean, "../") || path.IsAbs(clean) {
		return "", "", fmt.Errorf("unsafe tar entry path %q", name)
	}
	target := filepath.Join(root, filepath.FromSlash(clean))
	if !withinFilesystemRoot(root, target) {
		return "", "", fmt.Errorf("tar entry %q escapes plugin root", name)
	}
	return target, filepath.ToSlash(clean), nil
}

func ensureRealDirectory(root, dir string) error {
	rel, err := filepath.Rel(root, dir)
	if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("directory escapes plugin root")
	}
	current := root
	if rel == "." {
		return nil
	}
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("path component %s is not a real directory", current)
		}
	}
	return nil
}

func withinFilesystemRoot(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
