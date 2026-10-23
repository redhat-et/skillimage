package oci

import (
	"context"
	"strings"
	"testing"

	"github.com/redhat-et/skillimage/pkg/agentplugin"
)

func TestResolvePluginImageRef(t *testing.T) {
	manifest := agentplugin.Manifest{Name: "example-plugin", Version: "1.2.3"}
	manifestWithoutVersion := agentplugin.Manifest{Name: "example-plugin"}

	tests := []struct {
		name     string
		manifest agentplugin.Manifest
		opts     PluginBuildOptions
		want     string
		wantErr  string
	}{
		{
			name:     "defaults to manifest name and version",
			manifest: manifest,
			want:     "example-plugin:1.2.3",
		},
		{
			name:     "tag overrides manifest version",
			manifest: manifest,
			opts:     PluginBuildOptions{Tag: "2.0.0"},
			want:     "example-plugin:2.0.0",
		},
		{
			name:     "untagged reference uses manifest version",
			manifest: manifest,
			opts:     PluginBuildOptions{ImageRef: "quay.io/example/plugin"},
			want:     "quay.io/example/plugin:1.2.3",
		},
		{
			name:     "existing reference tag is kept",
			manifest: manifest,
			opts:     PluginBuildOptions{ImageRef: "quay.io/example/plugin:stable"},
			want:     "quay.io/example/plugin:stable",
		},
		{
			name:     "tag overrides reference tag",
			manifest: manifest,
			opts:     PluginBuildOptions{ImageRef: "quay.io/example/plugin:stable", Tag: "2.0.0"},
			want:     "quay.io/example/plugin:2.0.0",
		},
		{
			name:     "registry port is not treated as an image tag",
			manifest: manifest,
			opts:     PluginBuildOptions{ImageRef: "registry.example:8443/example/plugin"},
			want:     "registry.example:8443/example/plugin:1.2.3",
		},
		{
			name:     "digest reference is kept",
			manifest: manifestWithoutVersion,
			opts:     PluginBuildOptions{ImageRef: "quay.io/example/plugin@sha256:deadbeef"},
			want:     "quay.io/example/plugin@sha256:deadbeef",
		},
		{
			name:     "missing version when defaulting",
			manifest: manifestWithoutVersion,
			wantErr:  "plugin.json has no version; provide a plugin-image-ref or --tag",
		},
		{
			name:     "missing version for untagged reference",
			manifest: manifestWithoutVersion,
			opts:     PluginBuildOptions{ImageRef: "quay.io/example/plugin"},
			wantErr:  "plugin.json has no version; provide --tag or a tagged plugin-image-ref",
		},
		{
			name:     "tag cannot override digest reference",
			manifest: manifest,
			opts:     PluginBuildOptions{ImageRef: "quay.io/example/plugin@sha256:deadbeef", Tag: "2.0.0"},
			wantErr:  "--tag cannot override a digest reference",
		},
		{
			name:     "whitespace-only reference is rejected",
			manifest: manifest,
			opts:     PluginBuildOptions{ImageRef: "  "},
			wantErr:  "an OCI reference is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolvePluginImageRef(tt.manifest, tt.opts)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("resolvePluginImageRef() error = %v, want error containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolvePluginImageRef() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("resolvePluginImageRef() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildPluginValidatesMediaType(t *testing.T) {
	client := &Client{}
	_, err := client.BuildPlugin(context.Background(), "", PluginBuildOptions{MediaType: MediaTypeRedHat})
	if err == nil || !strings.Contains(err.Error(), "agent plugin packages require standard OCI media types") {
		t.Fatalf("BuildPlugin() error = %v, want standard media type restriction", err)
	}
}

func TestBuildPluginRejectsUnknownMediaType(t *testing.T) {
	client := &Client{}
	_, err := client.BuildPlugin(context.Background(), "", PluginBuildOptions{MediaType: MediaTypeProfile("unknown")})
	if err == nil || !strings.Contains(err.Error(), "unknown media type profile") {
		t.Fatalf("BuildPlugin() error = %v, want unknown media type error", err)
	}
}
