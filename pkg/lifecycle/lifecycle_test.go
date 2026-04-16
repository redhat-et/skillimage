package lifecycle_test

import (
	"testing"

	"github.com/redhat-et/oci-skill-registry/pkg/lifecycle"
)

func TestValidTransition(t *testing.T) {
	tests := []struct {
		from lifecycle.State
		to   lifecycle.State
		want bool
	}{
		{lifecycle.Draft, lifecycle.Testing, true},
		{lifecycle.Testing, lifecycle.Published, true},
		{lifecycle.Published, lifecycle.Deprecated, true},
		{lifecycle.Deprecated, lifecycle.Archived, true},
		// Invalid transitions
		{lifecycle.Draft, lifecycle.Published, false},
		{lifecycle.Draft, lifecycle.Deprecated, false},
		{lifecycle.Testing, lifecycle.Draft, false},
		{lifecycle.Published, lifecycle.Testing, false},
		{lifecycle.Published, lifecycle.Archived, false},
		{lifecycle.Archived, lifecycle.Draft, false},
		{lifecycle.Archived, lifecycle.Published, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.from)+"->"+string(tt.to), func(t *testing.T) {
			got := lifecycle.ValidTransition(tt.from, tt.to)
			if got != tt.want {
				t.Errorf("ValidTransition(%s, %s) = %v, want %v",
					tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestTagForState(t *testing.T) {
	tests := []struct {
		version string
		state   lifecycle.State
		want    string
	}{
		{"1.2.0", lifecycle.Draft, "1.2.0-draft"},
		{"1.2.0", lifecycle.Testing, "1.2.0-rc"},
		{"1.2.0", lifecycle.Published, "1.2.0"},
		{"1.2.0", lifecycle.Deprecated, "1.2.0"},
		{"1.2.0", lifecycle.Archived, ""},
	}
	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			got := lifecycle.TagForState(tt.version, tt.state)
			if got != tt.want {
				t.Errorf("TagForState(%q, %s) = %q, want %q",
					tt.version, tt.state, got, tt.want)
			}
		})
	}
}

func TestParseState(t *testing.T) {
	tests := []struct {
		input   string
		want    lifecycle.State
		wantErr bool
	}{
		{"draft", lifecycle.Draft, false},
		{"testing", lifecycle.Testing, false},
		{"published", lifecycle.Published, false},
		{"deprecated", lifecycle.Deprecated, false},
		{"archived", lifecycle.Archived, false},
		{"invalid", "", true},
		{"", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := lifecycle.ParseState(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseState(%q) error = %v, wantErr %v",
					tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ParseState(%q) = %q, want %q",
					tt.input, got, tt.want)
			}
		})
	}
}
