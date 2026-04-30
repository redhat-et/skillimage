package source_test

import (
	"testing"

	"github.com/redhat-et/skillimage/pkg/source"
)

func TestIsRemote(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"https://github.com/org/repo", true},
		{"http://gitlab.com/org/repo", true},
		{"/local/path/to/skills", false},
		{"./relative/path", false},
		{"skills/", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := source.IsRemote(tt.input); got != tt.want {
				t.Errorf("IsRemote(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestOrgFromCloneURL(t *testing.T) {
	tests := []struct {
		cloneURL string
		want     string
	}{
		{"https://github.com/anthropics/skills.git", "anthropics"},
		{"https://gitlab.com/org/repo.git", "org"},
		{"https://example.com/repo", ""},
	}

	for _, tt := range tests {
		t.Run(tt.cloneURL, func(t *testing.T) {
			if got := source.OrgFromCloneURL(tt.cloneURL); got != tt.want {
				t.Errorf("OrgFromCloneURL(%q) = %q, want %q", tt.cloneURL, got, tt.want)
			}
		})
	}
}
