package oci_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/redhat-et/skillimage/pkg/lifecycle"
	"github.com/redhat-et/skillimage/pkg/oci"
	"github.com/redhat-et/skillimage/pkg/skillcard"
)

func writeTestSkill(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillYAML := []byte(`apiVersion: skillimage.io/v1alpha1
kind: SkillCard
metadata:
  name: test-skill
  namespace: test
  version: 1.0.0
  description: A test skill.
spec:
  prompt: SKILL.md
`)
	if err := os.WriteFile(filepath.Join(dir, "skill.yaml"), skillYAML, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("Test prompt content."), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeTestPlugin(t *testing.T, dir string) {
	t.Helper()
	files := map[string]string{
		"plugin.json": `{
  "$schema": "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",
  "name": "test-plugin",
  "version": "1.2.3",
  "description": "A test Agent Plugin.",
  "author": {"name": "Test Author", "url": "https://example.com/authors/test"},
  "license": "Apache-2.0",
  "keywords": ["test", "plugin"],
  "extensions": {"com.example.skillctl": {"enabled": true}}
}`,
		"skills/test-skill/SKILL.md": `---
name: test-skill
description: A test skill included in the plugin.
---
# Test skill
`,
		".claude-plugin/plugin.json": `{"name":"test-plugin"}`,
	}
	for name, contents := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBuildAndListLocal(t *testing.T) {
	skillDir := t.TempDir()
	writeTestSkill(t, skillDir)

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	desc, err := client.Build(ctx, skillDir, oci.BuildOptions{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if desc.Digest.String() == "" {
		t.Error("expected non-empty digest")
	}

	pluginDir := t.TempDir()
	writeTestPlugin(t, pluginDir)
	if _, err := client.BuildPlugin(ctx, pluginDir, oci.PluginBuildOptions{ImageRef: "test/test-plugin:1.2.3"}); err != nil {
		t.Fatalf("BuildPlugin: %v", err)
	}

	images, err := client.ListLocal()
	if err != nil {
		t.Fatalf("ListLocal: %v", err)
	}
	if len(images) != 2 {
		t.Fatalf("expected 2 images, got %d", len(images))
	}
	var skillImage, pluginImage *oci.LocalImage
	for i := range images {
		if images[i].Type == oci.PluginType {
			pluginImage = &images[i]
		} else {
			skillImage = &images[i]
		}
	}
	if skillImage == nil {
		t.Fatal("expected a skill image in the local list")
	}
	if skillImage.Name != "test/test-skill" {
		t.Errorf("skill image name = %q, want %q", skillImage.Name, "test/test-skill")
	}
	if skillImage.Version != "1.0.0" {
		t.Errorf("skill image version = %q, want %q", skillImage.Version, "1.0.0")
	}
	if skillImage.Tag != "1.0.0-draft" {
		t.Errorf("skill image tag = %q, want %q", skillImage.Tag, "1.0.0-draft")
	}
	if pluginImage == nil {
		t.Fatal("expected a plugin image in the local list")
	}
	if pluginImage.Name != "test/test-plugin" {
		t.Errorf("plugin image name = %q, want %q", pluginImage.Name, "test/test-plugin")
	}
	if pluginImage.Version != "1.2.3" || pluginImage.Tag != "1.2.3" {
		t.Errorf("plugin image version/tag = %q/%q, want 1.2.3/1.2.3", pluginImage.Version, pluginImage.Tag)
	}
}

func TestBuildPluginDefaultsReferenceFromManifest(t *testing.T) {
	pluginDir := t.TempDir()
	writeTestPlugin(t, pluginDir)

	client, err := oci.NewClient(t.TempDir())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	result, err := client.BuildPlugin(context.Background(), pluginDir, oci.PluginBuildOptions{})
	if err != nil {
		t.Fatalf("BuildPlugin: %v", err)
	}
	if result.ImageRef != "test-plugin:1.2.3" {
		t.Errorf("BuildPlugin image ref = %q, want %q", result.ImageRef, "test-plugin:1.2.3")
	}
	if result.Descriptor.Digest.String() == "" {
		t.Fatal("BuildPlugin returned an empty descriptor digest")
	}
	if _, err := client.Inspect(context.Background(), result.ImageRef); err != nil {
		t.Fatalf("Inspect built plugin at resolved ref %q: %v", result.ImageRef, err)
	}
}

func TestBuildValidatesSkillCard(t *testing.T) {
	skillDir := t.TempDir()
	badYAML := []byte(`apiVersion: wrong/v1
kind: SkillCard
metadata:
  name: BAD
  namespace: test
  version: 1.0.0
  description: test
`)
	if err := os.WriteFile(filepath.Join(skillDir, "skill.yaml"), badYAML, 0o644); err != nil {
		t.Fatal(err)
	}

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = client.Build(context.Background(), skillDir, oci.BuildOptions{})
	if err == nil {
		t.Fatal("expected error for invalid SkillCard")
	}
}

func TestBuildMissingSkillYAML(t *testing.T) {
	emptyDir := t.TempDir()

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = client.Build(context.Background(), emptyDir, oci.BuildOptions{})
	if err == nil {
		t.Fatal("expected error for missing skill.yaml")
	}
}

func TestCopyToAndBack(t *testing.T) {
	// Build into source store
	skillDir := t.TempDir()
	writeTestSkill(t, skillDir)

	srcStoreDir := t.TempDir()
	srcClient, err := oci.NewClient(srcStoreDir)
	if err != nil {
		t.Fatalf("NewClient (src): %v", err)
	}

	ctx := context.Background()
	_, err = srcClient.Build(ctx, skillDir, oci.BuildOptions{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// Copy to destination store
	dstStoreDir := t.TempDir()
	dstClient, err := oci.NewClient(dstStoreDir)
	if err != nil {
		t.Fatalf("NewClient (dst): %v", err)
	}

	ref := "test/test-skill:1.0.0-draft"
	err = srcClient.CopyTo(ctx, ref, dstClient)
	if err != nil {
		t.Fatalf("CopyTo: %v", err)
	}

	// Verify the image exists in destination
	images, err := dstClient.ListLocal()
	if err != nil {
		t.Fatalf("ListLocal: %v", err)
	}
	if len(images) != 1 {
		t.Fatalf("expected 1 image after copy, got %d", len(images))
	}
}

func TestUnpack(t *testing.T) {
	skillDir := t.TempDir()
	writeTestSkill(t, skillDir)

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	_, err = client.Build(ctx, skillDir, oci.BuildOptions{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// Unpack to output directory
	outputDir := t.TempDir()
	err = client.Unpack(ctx, "test/test-skill:1.0.0-draft", outputDir)
	if err != nil {
		t.Fatalf("Unpack: %v", err)
	}

	// Verify: auto-creates subdirectory named after skill
	expectedFile := filepath.Join(outputDir, "test-skill", "skill.yaml")
	if _, err := os.Stat(expectedFile); err != nil {
		t.Errorf("expected %s to exist: %v", expectedFile, err)
	}

	promptFile := filepath.Join(outputDir, "test-skill", "SKILL.md")
	if _, err := os.Stat(promptFile); err != nil {
		t.Errorf("expected %s to exist: %v", promptFile, err)
	}

	pluginDir := t.TempDir()
	writeTestPlugin(t, pluginDir)
	if _, err := client.BuildPlugin(ctx, pluginDir, oci.PluginBuildOptions{ImageRef: "test/test-plugin:1.2.3"}); err != nil {
		t.Fatalf("BuildPlugin: %v", err)
	}
	if err := client.Unpack(ctx, "test/test-plugin:1.2.3", outputDir); err != nil {
		t.Fatalf("Unpack plugin: %v", err)
	}
	for _, relPath := range []string{
		"test-plugin/plugin.json",
		"test-plugin/skills/test-skill/SKILL.md",
		"test-plugin/.claude-plugin/plugin.json",
	} {
		if _, err := os.Stat(filepath.Join(outputDir, relPath)); err != nil {
			t.Errorf("expected unpacked plugin file %s to exist: %v", relPath, err)
		}
	}
}

func TestPullWithUnpackPlugin(t *testing.T) {
	pluginDir := t.TempDir()
	writeTestPlugin(t, pluginDir)

	sourceStoreDir := t.TempDir()
	sourceClient, err := oci.NewClient(sourceStoreDir)
	if err != nil {
		t.Fatalf("NewClient (source): %v", err)
	}
	ctx := context.Background()
	manifestDesc, err := sourceClient.BuildPlugin(ctx, pluginDir, oci.PluginBuildOptions{ImageRef: "test/test-plugin:1.2.3"})
	if err != nil {
		t.Fatalf("BuildPlugin: %v", err)
	}

	registry := newReadOnlyTestRegistry(t, sourceStoreDir, "test/test-plugin", "1.2.3", manifestDesc.Descriptor)
	remoteRef := strings.TrimPrefix(registry.URL, "https://") + "/test/test-plugin:1.2.3"
	remoteInspect, err := sourceClient.InspectRemote(ctx, remoteRef, oci.InspectOptions{SkipTLSVerify: true})
	if err != nil {
		t.Fatalf("InspectRemote plugin: %v", err)
	}
	if remoteInspect.Plugin == nil || remoteInspect.Plugin.Name != "test-plugin" {
		t.Errorf("remote inspection plugin = %#v, want test-plugin metadata", remoteInspect.Plugin)
	}

	destinationClient, err := oci.NewClient(t.TempDir())
	if err != nil {
		t.Fatalf("NewClient (destination): %v", err)
	}
	outputDir := t.TempDir()
	_, unpackedPath, err := destinationClient.PullWithUnpack(ctx, remoteRef, oci.PullOptions{
		OutputDir:     outputDir,
		SkipTLSVerify: true,
	})
	if err != nil {
		t.Fatalf("PullWithUnpack: %v", err)
	}
	if want := filepath.Join(outputDir, "test-plugin"); unpackedPath != want {
		t.Errorf("unpacked path = %q, want %q", unpackedPath, want)
	}
	for _, relPath := range []string{
		"plugin.json",
		"skills/test-skill/SKILL.md",
		".claude-plugin/plugin.json",
	} {
		if _, err := os.Stat(filepath.Join(unpackedPath, relPath)); err != nil {
			t.Errorf("expected pulled plugin file %s to exist: %v", relPath, err)
		}
	}
}

func TestBuildPluginExcludesVCSMetadata(t *testing.T) {
	pluginDir := t.TempDir()
	writeTestPlugin(t, pluginDir)
	metadata := map[string]string{
		".git":                                "gitdir: ../.git/worktrees/test-plugin\n",
		"skills/test-skill/.git/objects/pack": "git object history",
		".hg/store/metadata":                  "mercurial metadata",
		".svn/entries":                        "subversion metadata",
	}
	for name, contents := range metadata {
		path := filepath.Join(pluginDir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create parent for %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	client, err := oci.NewClient(t.TempDir())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := client.BuildPlugin(context.Background(), pluginDir, oci.PluginBuildOptions{ImageRef: "test/test-plugin:1.2.3"}); err != nil {
		t.Fatalf("BuildPlugin: %v", err)
	}

	outputDir := t.TempDir()
	if err := client.Unpack(context.Background(), "test/test-plugin:1.2.3", outputDir); err != nil {
		t.Fatalf("Unpack: %v", err)
	}
	unpackedDir := filepath.Join(outputDir, "test-plugin")
	for _, relPath := range []string{
		".git",
		"skills/test-skill/.git",
		".hg",
		".svn",
	} {
		if _, err := os.Lstat(filepath.Join(unpackedDir, relPath)); !os.IsNotExist(err) {
			t.Errorf("VCS metadata %s was included in the plugin layer (stat error: %v)", relPath, err)
		}
	}
	if _, err := os.Stat(filepath.Join(unpackedDir, ".claude-plugin", "plugin.json")); err != nil {
		t.Errorf("expected non-VCS dotfiles to remain in the plugin layer: %v", err)
	}
}

func newReadOnlyTestRegistry(t *testing.T, storeDir, repository, tag string, manifestDesc ocispec.Descriptor) *httptest.Server {
	t.Helper()
	type blob struct {
		data      []byte
		mediaType string
	}
	readBlob := func(digest string) []byte {
		algorithm, encoded, ok := strings.Cut(digest, ":")
		if !ok {
			t.Fatalf("invalid test OCI digest %q", digest)
		}
		data, err := os.ReadFile(filepath.Join(storeDir, "blobs", algorithm, encoded))
		if err != nil {
			t.Fatalf("read OCI blob %s: %v", digest, err)
		}
		return data
	}

	manifestBytes := readBlob(manifestDesc.Digest.String())
	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatalf("parse built manifest: %v", err)
	}
	blobs := map[string]blob{
		manifest.Config.Digest.String(): {data: readBlob(manifest.Config.Digest.String()), mediaType: manifest.Config.MediaType},
	}
	for _, layer := range manifest.Layers {
		blobs[layer.Digest.String()] = blob{data: readBlob(layer.Digest.String()), mediaType: layer.MediaType}
	}

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/" {
			w.Header().Set("Docker-Distribution-API-Version", "registry/2.0")
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "read-only test registry", http.StatusMethodNotAllowed)
			return
		}

		prefix := "/v2/" + repository + "/"
		if !strings.HasPrefix(r.URL.Path, prefix) {
			http.NotFound(w, r)
			return
		}
		pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, prefix), "/")
		var data []byte
		var mediaType, digest string
		switch {
		case len(pathParts) == 2 && pathParts[0] == "manifests" && (pathParts[1] == tag || pathParts[1] == manifestDesc.Digest.String()):
			data = manifestBytes
			mediaType = manifestDesc.MediaType
			digest = manifestDesc.Digest.String()
		case len(pathParts) == 2 && pathParts[0] == "blobs":
			stored, ok := blobs[pathParts[1]]
			if !ok {
				http.NotFound(w, r)
				return
			}
			data = stored.data
			mediaType = stored.mediaType
			digest = pathParts[1]
		default:
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", mediaType)
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.Header().Set("Docker-Content-Digest", digest)
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodGet {
			_, _ = w.Write(data)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestInspect(t *testing.T) {
	skillDir := t.TempDir()
	writeTestSkill(t, skillDir)

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	_, err = client.Build(ctx, skillDir, oci.BuildOptions{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	result, err := client.Inspect(ctx, "test/test-skill:1.0.0-draft")
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if result.Version != "1.0.0" {
		t.Errorf("version = %q, want %q", result.Version, "1.0.0")
	}
	if result.Status != "draft" {
		t.Errorf("status = %q, want %q", result.Status, "draft")
	}
	if result.Name != "test/test-skill" {
		t.Errorf("name = %q, want %q", result.Name, "test/test-skill")
	}
	if result.LayerCount != 1 {
		t.Errorf("layer count = %d, want 1", result.LayerCount)
	}

	pluginDir := t.TempDir()
	writeTestPlugin(t, pluginDir)
	if _, err := client.BuildPlugin(ctx, pluginDir, oci.PluginBuildOptions{ImageRef: "test/test-plugin:1.2.3"}); err != nil {
		t.Fatalf("BuildPlugin: %v", err)
	}
	pluginResult, err := client.Inspect(ctx, "test/test-plugin:1.2.3")
	if err != nil {
		t.Fatalf("Inspect plugin: %v", err)
	}
	if pluginResult.Plugin == nil {
		t.Fatal("Inspect plugin returned no plugin metadata")
	}
	if pluginResult.Name != "test-plugin" || pluginResult.Plugin.Name != "test-plugin" {
		t.Errorf("plugin name = %q/%q, want test-plugin", pluginResult.Name, pluginResult.Plugin.Name)
	}
	if pluginResult.Version != "1.2.3" || pluginResult.Plugin.Version != "1.2.3" {
		t.Errorf("plugin version = %q/%q, want 1.2.3", pluginResult.Version, pluginResult.Plugin.Version)
	}
	if pluginResult.Plugin.Author != "Test Author" || pluginResult.Plugin.AuthorURL != "https://example.com/authors/test" {
		t.Errorf("plugin author = %q (%q), want Test Author (author URL)", pluginResult.Plugin.Author, pluginResult.Plugin.AuthorURL)
	}
	if len(pluginResult.Plugin.Skills) != 1 || pluginResult.Plugin.Skills[0] != "test-skill" {
		t.Errorf("plugin skills = %v, want [test-skill]", pluginResult.Plugin.Skills)
	}
	if len(pluginResult.Plugin.Keywords) != 2 || pluginResult.Plugin.Keywords[0] != "test" || pluginResult.Plugin.Keywords[1] != "plugin" {
		t.Errorf("plugin keywords = %v, want [test plugin]", pluginResult.Plugin.Keywords)
	}
	if len(pluginResult.Plugin.Extensions) != 1 || pluginResult.Plugin.Extensions[0] != "com.example.skillctl" {
		t.Errorf("plugin extensions = %v, want [com.example.skillctl]", pluginResult.Plugin.Extensions)
	}
}

func TestPromoteLocal(t *testing.T) {
	skillDir := t.TempDir()
	writeTestSkill(t, skillDir)

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	_, err = client.Build(ctx, skillDir, oci.BuildOptions{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// Promote draft -> testing
	err = client.PromoteLocal(ctx, "test/test-skill:1.0.0-draft", lifecycle.Testing)
	if err != nil {
		t.Fatalf("Promote to testing: %v", err)
	}

	// Verify new tag exists with correct status
	result, err := client.Inspect(ctx, "test/test-skill:1.0.0-testing")
	if err != nil {
		t.Fatalf("Inspect after promote to testing: %v", err)
	}
	if result.Status != "testing" {
		t.Errorf("status = %q, want %q", result.Status, "testing")
	}

	// Promote testing -> published
	err = client.PromoteLocal(ctx, "test/test-skill:1.0.0-testing", lifecycle.Published)
	if err != nil {
		t.Fatalf("Promote to published: %v", err)
	}

	// Verify published tag
	result, err = client.Inspect(ctx, "test/test-skill:1.0.0")
	if err != nil {
		t.Fatalf("Inspect after publish: %v", err)
	}
	if result.Status != "published" {
		t.Errorf("status = %q, want %q", result.Status, "published")
	}

	// Verify latest tag also exists
	result, err = client.Inspect(ctx, "test/test-skill:latest")
	if err != nil {
		t.Fatalf("Inspect latest after publish: %v", err)
	}
	if result.Status != "published" {
		t.Errorf("latest status = %q, want %q", result.Status, "published")
	}
}

func TestAnnotationsIncludeTags(t *testing.T) {
	skillDir := t.TempDir()
	skillYAML := []byte(`apiVersion: skillimage.io/v1alpha1
kind: SkillCard
metadata:
  name: annotated-skill
  namespace: test
  version: 2.0.0
  description: Skill with tags and compat.
  tags:
    - kubernetes
    - debugging
  compatibility: claude-3.5-sonnet
spec:
  prompt: SKILL.md
`)
	if err := os.WriteFile(filepath.Join(skillDir, "skill.yaml"), skillYAML, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("one two three four five"), 0o644); err != nil {
		t.Fatal(err)
	}

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	_, err = client.Build(ctx, skillDir, oci.BuildOptions{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	result, err := client.Inspect(ctx, "test/annotated-skill:2.0.0-draft")
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}

	if result.Tags != `["kubernetes","debugging"]` {
		t.Errorf("tags = %q, want %q", result.Tags, `["kubernetes","debugging"]`)
	}
	if result.Compatibility != "claude-3.5-sonnet" {
		t.Errorf("compatibility = %q, want %q", result.Compatibility, "claude-3.5-sonnet")
	}
	if result.WordCount != "5" {
		t.Errorf("wordcount = %q, want %q", result.WordCount, "5")
	}
}

func TestWordCountExcludesFrontmatter(t *testing.T) {
	skillDir := t.TempDir()
	skillYAML := []byte(`apiVersion: skillimage.io/v1alpha1
kind: SkillCard
metadata:
  name: frontmatter-skill
  namespace: test
  version: 1.0.0
  description: Skill with YAML frontmatter in SKILL.md.
spec:
  prompt: SKILL.md
`)
	skillMD := []byte(`---
name: frontmatter-skill
description: This has frontmatter that should not be counted.
license: Apache-2.0
---

These five words are counted.
`)
	if err := os.WriteFile(filepath.Join(skillDir, "skill.yaml"), skillYAML, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), skillMD, 0o644); err != nil {
		t.Fatal(err)
	}

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	_, err = client.Build(ctx, skillDir, oci.BuildOptions{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	result, err := client.Inspect(ctx, "test/frontmatter-skill:1.0.0-draft")
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}

	if result.WordCount != "5" {
		t.Errorf("wordcount = %q, want %q (frontmatter should be excluded)", result.WordCount, "5")
	}
}

func TestPromoteInvalidTransition(t *testing.T) {
	skillDir := t.TempDir()
	writeTestSkill(t, skillDir)

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	_, err = client.Build(ctx, skillDir, oci.BuildOptions{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// Try invalid transition: draft -> published
	err = client.PromoteLocal(ctx, "test/test-skill:1.0.0-draft", lifecycle.Published)
	if err == nil {
		t.Fatal("expected error for invalid transition draft -> published")
	}
}

func TestAnnotationsOmittedWhenEmpty(t *testing.T) {
	skillDir := t.TempDir()
	skillYAML := []byte(`apiVersion: skillimage.io/v1alpha1
kind: SkillCard
metadata:
  name: minimal-skill
  namespace: test
  version: 1.0.0
  description: Minimal skill with no optional fields.
spec:
  prompt: SKILL.md
`)
	if err := os.WriteFile(filepath.Join(skillDir, "skill.yaml"), skillYAML, 0o644); err != nil {
		t.Fatal(err)
	}

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	_, err = client.Build(ctx, skillDir, oci.BuildOptions{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	result, err := client.Inspect(ctx, "test/minimal-skill:1.0.0-draft")
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}

	if result.Tags != "" {
		t.Errorf("tags should be empty, got %q", result.Tags)
	}
	if result.Compatibility != "" {
		t.Errorf("compatibility should be empty, got %q", result.Compatibility)
	}
	if result.WordCount != "" {
		t.Errorf("wordcount should be empty, got %q", result.WordCount)
	}
}

func TestTag(t *testing.T) {
	skillDir := t.TempDir()
	writeTestSkill(t, skillDir)

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	_, err = client.Build(ctx, skillDir, oci.BuildOptions{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	err = client.Tag(ctx, "test/test-skill:1.0.0-draft", "quay.io/myorg/test-skill:1.0.0-draft")
	if err != nil {
		t.Fatalf("Tag: %v", err)
	}

	// Inspect via the new tag should return the same image.
	result, err := client.Inspect(ctx, "quay.io/myorg/test-skill:1.0.0-draft")
	if err != nil {
		t.Fatalf("Inspect new tag: %v", err)
	}
	if result.Version != "1.0.0" {
		t.Errorf("version = %q, want %q", result.Version, "1.0.0")
	}
	if result.Status != "draft" {
		t.Errorf("status = %q, want %q", result.Status, "draft")
	}
}

func TestBuildRedHatMediaType(t *testing.T) {
	skillDir := t.TempDir()
	writeTestSkill(t, skillDir)

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	desc, err := client.Build(ctx, skillDir, oci.BuildOptions{
		MediaType: oci.MediaTypeRedHat,
	})
	if err != nil {
		t.Fatalf("Build with redhat media type: %v", err)
	}
	if desc.Digest.String() == "" {
		t.Error("expected non-empty digest")
	}

	result, err := client.Inspect(ctx, "test/test-skill:1.0.0-draft")
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if result.MediaType != "application/vnd.oci.image.manifest.v1+json" {
		t.Errorf("manifest media type = %q, want standard OCI manifest type", result.MediaType)
	}
	if result.ConfigMediaType != oci.RedHatMediaTypeSkillConfig {
		t.Errorf("config media type = %q, want %q", result.ConfigMediaType, oci.RedHatMediaTypeSkillConfig)
	}
	if result.LayerMediaType != oci.RedHatMediaTypeSkillLayer {
		t.Errorf("layer media type = %q, want %q", result.LayerMediaType, oci.RedHatMediaTypeSkillLayer)
	}
}

func TestParseMediaTypeProfileRejectsInvalid(t *testing.T) {
	_, err := oci.ParseMediaTypeProfile("bogus")
	if err == nil {
		t.Fatal("expected error for invalid media type profile")
	}
}

func TestAnnotationsEmptySKILLmd(t *testing.T) {
	skillDir := t.TempDir()
	skillYAML := []byte(`apiVersion: skillimage.io/v1alpha1
kind: SkillCard
metadata:
  name: empty-md-skill
  namespace: test
  version: 1.0.0
  description: Skill with empty SKILL.md.
spec:
  prompt: SKILL.md
`)
	if err := os.WriteFile(filepath.Join(skillDir, "skill.yaml"), skillYAML, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	_, err = client.Build(ctx, skillDir, oci.BuildOptions{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	result, err := client.Inspect(ctx, "test/empty-md-skill:1.0.0-draft")
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}

	if result.WordCount != "" {
		t.Errorf("wordcount should be empty for empty SKILL.md, got %q", result.WordCount)
	}
}

func TestBuildWithPrebuiltSkillCard(t *testing.T) {
	skillDir := t.TempDir()
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(skillDir, "SKILL.md"),
		[]byte("You are a test skill."),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	sc := &skillcard.SkillCard{
		APIVersion: "skillimage.io/v1alpha1",
		Kind:       "SkillCard",
		Metadata: skillcard.Metadata{
			Name:        "prebuilt-test",
			Namespace:   "test",
			Version:     "1.0.0",
			Description: "A prebuilt test skill.",
		},
		Spec: &skillcard.Spec{Prompt: "SKILL.md"},
	}

	ctx := context.Background()
	desc, err := client.Build(ctx, skillDir, oci.BuildOptions{SkillCard: sc})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if desc.Digest.String() == "" {
		t.Error("expected non-empty digest")
	}

	images, err := client.ListLocal()
	if err != nil {
		t.Fatalf("ListLocal: %v", err)
	}
	if len(images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(images))
	}
	if images[0].Name != "test/prebuilt-test" {
		t.Errorf("image name = %q, want %q", images[0].Name, "test/prebuilt-test")
	}
}
