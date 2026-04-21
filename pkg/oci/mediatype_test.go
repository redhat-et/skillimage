package oci

import (
	"strings"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

func TestParseMediaTypeProfile(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    MediaTypeProfile
		wantErr bool
	}{
		{name: "empty", input: "", want: MediaTypeStandard},
		{name: "standard", input: "standard", want: MediaTypeStandard},
		{name: "redhat", input: "redhat", want: MediaTypeRedHat},
		{name: "uppercase", input: "RedHat", want: MediaTypeRedHat},
		{name: "trailing space", input: "redhat ", want: MediaTypeRedHat},
		{name: "leading space", input: " standard", want: MediaTypeStandard},
		{name: "invalid", input: "bogus", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseMediaTypeProfile(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if !strings.Contains(err.Error(), "unknown media type profile") {
					t.Errorf("error = %q, want it to mention unknown profile", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveMediaTypes(t *testing.T) {
	tests := []struct {
		name       string
		profile    MediaTypeProfile
		wantLayer  string
		wantConfig string
	}{
		{
			name:       "default",
			profile:    "",
			wantLayer:  ocispec.MediaTypeImageLayerGzip,
			wantConfig: ocispec.MediaTypeImageConfig,
		},
		{
			name:       "standard",
			profile:    MediaTypeStandard,
			wantLayer:  ocispec.MediaTypeImageLayerGzip,
			wantConfig: ocispec.MediaTypeImageConfig,
		},
		{
			name:       "redhat",
			profile:    MediaTypeRedHat,
			wantLayer:  RedHatMediaTypeSkillLayer,
			wantConfig: RedHatMediaTypeSkillConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layer, config := resolveMediaTypes(tt.profile)
			if layer != tt.wantLayer {
				t.Errorf("layer = %q, want %q", layer, tt.wantLayer)
			}
			if config != tt.wantConfig {
				t.Errorf("config = %q, want %q", config, tt.wantConfig)
			}
		})
	}
}
