package oci

import "testing"

func TestSplitRefTag(t *testing.T) {
	tests := []struct {
		ref      string
		wantRepo string
		wantTag  string
	}{
		{"ns/name:1.0.0", "ns/name", "1.0.0"},
		{"ns/name@sha256:abc123", "ns/name", "sha256:abc123"},
		{"localhost:5000/ns/name:tag", "localhost:5000/ns/name", "tag"},
		{"localhost:5000/ns/name@sha256:abc", "localhost:5000/ns/name", "sha256:abc"},
		{"registry.svc:5000/team1/skill@sha256:ffa608d3", "registry.svc:5000/team1/skill", "sha256:ffa608d3"},
		{"name:tag", "name", "tag"},
		{"name@sha256:abc", "name", "sha256:abc"},
		{"name", "name", ""},
	}
	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			repo, tag := splitRefTag(tt.ref)
			if repo != tt.wantRepo || tag != tt.wantTag {
				t.Errorf("splitRefTag(%q) = (%q, %q), want (%q, %q)",
					tt.ref, repo, tag, tt.wantRepo, tt.wantTag)
			}
		})
	}
}

func TestParseNameFromTag(t *testing.T) {
	tests := []struct {
		ref  string
		want string
	}{
		{"ns/name:1.0.0", "ns/name"},
		{"ns/name@sha256:abc123", "ns/name"},
		{"registry:5000/ns/name:tag", "registry:5000/ns/name"},
		{"registry:5000/ns/name@sha256:abc", "registry:5000/ns/name"},
		{"name", "name"},
	}
	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			got := parseNameFromTag(tt.ref)
			if got != tt.want {
				t.Errorf("parseNameFromTag(%q) = %q, want %q", tt.ref, got, tt.want)
			}
		})
	}
}

func TestSkillNameFromRef(t *testing.T) {
	tests := []struct {
		ref  string
		want string
	}{
		{"ns/name:1.0.0", "name"},
		{"ns/name@sha256:abc123", "name"},
		{"registry:5000/ns/skill@sha256:ffa608d3", "skill"},
		{"registry:5000/ns/skill:1.0.0-draft", "skill"},
		{"name:tag", "name"},
		{"name", "name"},
	}
	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			got := skillNameFromRef(tt.ref)
			if got != tt.want {
				t.Errorf("skillNameFromRef(%q) = %q, want %q", tt.ref, got, tt.want)
			}
		})
	}
}
