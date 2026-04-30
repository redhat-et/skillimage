package source_test

import (
	"testing"

	"github.com/redhat-et/skillimage/pkg/source"
)

func TestParseGitURL(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    source.GitSource
		wantErr bool
	}{
		{
			name: "github repo root",
			raw:  "https://github.com/anthropics/skills",
			want: source.GitSource{
				CloneURL: "https://github.com/anthropics/skills.git",
				Ref:      "",
				SubPath:  "",
			},
		},
		{
			name: "github with ref and subpath",
			raw:  "https://github.com/anthropics/skills/tree/main/skills",
			want: source.GitSource{
				CloneURL: "https://github.com/anthropics/skills.git",
				Ref:      "main",
				SubPath:  "skills",
			},
		},
		{
			name: "github with tag ref and deep subpath",
			raw:  "https://github.com/anthropics/skills/tree/v1.0/skills/internal-comms",
			want: source.GitSource{
				CloneURL: "https://github.com/anthropics/skills.git",
				Ref:      "v1.0",
				SubPath:  "skills/internal-comms",
			},
		},
		{
			name: "gitlab with tree path",
			raw:  "https://gitlab.com/org/repo/-/tree/main/path/to/skill",
			want: source.GitSource{
				CloneURL: "https://gitlab.com/org/repo.git",
				Ref:      "main",
				SubPath:  "path/to/skill",
			},
		},
		{
			name: "unknown host plain URL",
			raw:  "https://unknown-host.com/org/repo",
			want: source.GitSource{
				CloneURL: "https://unknown-host.com/org/repo",
				Ref:      "",
				SubPath:  "",
			},
		},
		{
			name: "github repo with .git suffix",
			raw:  "https://github.com/anthropics/skills.git",
			want: source.GitSource{
				CloneURL: "https://github.com/anthropics/skills.git",
				Ref:      "",
				SubPath:  "",
			},
		},
		{
			name: "gitlab nested group",
			raw:  "https://gitlab.com/org/subgroup/repo/-/tree/main/skills",
			want: source.GitSource{
				CloneURL: "https://gitlab.com/org/subgroup/repo.git",
				Ref:      "main",
				SubPath:  "skills",
			},
		},
		{
			name:    "not a URL",
			raw:     "/some/local/path",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := source.ParseGitURL(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseGitURL() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got != tt.want {
				t.Errorf("ParseGitURL() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
