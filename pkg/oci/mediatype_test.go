package oci

import "testing"

func TestResolveMediaTypes(t *testing.T) {
	tests := []struct {
		profile    MediaTypeProfile
		wantLayer  string
		wantConfig string
		wantErr    bool
	}{
		{
			profile:    "",
			wantLayer:  "application/vnd.oci.image.layer.v1.tar+gzip",
			wantConfig: "application/vnd.oci.image.config.v1+json",
		},
		{
			profile:    MediaTypeStandard,
			wantLayer:  "application/vnd.oci.image.layer.v1.tar+gzip",
			wantConfig: "application/vnd.oci.image.config.v1+json",
		},
		{
			profile:    MediaTypeRedHat,
			wantLayer:  RedHatMediaTypeSkillLayer,
			wantConfig: RedHatMediaTypeSkillConfig,
		},
		{
			profile: "invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(string(tt.profile), func(t *testing.T) {
			layer, config, err := resolveMediaTypes(tt.profile)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if layer != tt.wantLayer {
				t.Errorf("layer = %q, want %q", layer, tt.wantLayer)
			}
			if config != tt.wantConfig {
				t.Errorf("config = %q, want %q", config, tt.wantConfig)
			}
		})
	}
}
