package cli

import (
	"testing"
)

func TestNewSkillCardFromRef(t *testing.T) {
	tests := []struct {
		ref       string
		wantName  string
		wantVer   string
		wantNS    string
	}{
		{
			ref:      "test/hello-world:1.0.0-draft",
			wantName: "hello-world",
			wantVer:  "1.0.0-draft",
			wantNS:   "test",
		},
		{
			ref:      "quay.io/skills/summarize:2.1.0",
			wantName: "summarize",
			wantVer:  "2.1.0",
			wantNS:   "quay.io",
		},
		{
			ref:      "skill:latest",
			wantName: "skill",
			wantVer:  "latest",
			wantNS:   "unknown",
		},
		{
			ref:      "ns/skill@sha256:abc123",
			wantName: "skill",
			wantVer:  "unknown",
			wantNS:   "ns",
		},
	}

	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			sc := NewSkillCardFromRef(tt.ref)
			if sc.Metadata.Name != tt.wantName {
				t.Errorf("name = %q, want %q", sc.Metadata.Name, tt.wantName)
			}
			if sc.Metadata.Version != tt.wantVer {
				t.Errorf("version = %q, want %q", sc.Metadata.Version, tt.wantVer)
			}
			if sc.Metadata.Namespace != tt.wantNS {
				t.Errorf("namespace = %q, want %q", sc.Metadata.Namespace, tt.wantNS)
			}
			if sc.APIVersion != "skillimage.io/v1alpha1" {
				t.Errorf("apiVersion = %q, want %q", sc.APIVersion, "skillimage.io/v1alpha1")
			}
		})
	}
}
