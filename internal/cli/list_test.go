package cli

import (
	"testing"
	"time"
)

func TestFormatDigest(t *testing.T) {
	tests := []struct {
		name    string
		digest  string
		noTrunc bool
		want    string
	}{
		{
			name:   "strips sha256 prefix and truncates",
			digest: "sha256:a593244d38f0e1b2c3d4e5f6a7b8c9d0e1f2a3b4",
			want:   "a593244d38f0",
		},
		{
			name:   "strips other algo prefix",
			digest: "sha512:abcdef123456789012345678",
			want:   "abcdef123456",
		},
		{
			name:   "handles digest shorter than 12 chars",
			digest: "sha256:abcd",
			want:   "abcd",
		},
		{
			name:   "handles digest with no prefix",
			digest: "a593244d38f0e1b2c3d4",
			want:   "a593244d38f0",
		},
		{
			name:    "no-trunc preserves full digest",
			digest:  "sha256:a593244d38f0e1b2c3d4e5f6a7b8c9d0e1f2a3b4",
			noTrunc: true,
			want:    "sha256:a593244d38f0e1b2c3d4e5f6a7b8c9d0e1f2a3b4",
		},
		{
			name:   "digest exactly 12 chars no prefix",
			digest: "a593244d38f0",
			want:   "a593244d38f0",
		},
		{
			name:   "empty digest",
			digest: "",
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDigest(tt.digest, tt.noTrunc)
			if got != tt.want {
				t.Errorf("formatDigest(%q, %v) = %q, want %q",
					tt.digest, tt.noTrunc, got, tt.want)
			}
		})
	}
}

func TestFormatCreated(t *testing.T) {
	tests := []struct {
		name    string
		created string
		noTrunc bool
		want    string
	}{
		{
			name:    "no-trunc returns raw timestamp",
			created: "2026-04-29T22:07:29Z",
			noTrunc: true,
			want:    "2026-04-29T22:07:29Z",
		},
		{
			name:    "empty string returns empty",
			created: "",
			want:    "",
		},
		{
			name:    "unparseable falls back to raw string",
			created: "not-a-timestamp",
			want:    "not-a-timestamp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatCreated(tt.created, tt.noTrunc)
			if got != tt.want {
				t.Errorf("formatCreated(%q, %v) = %q, want %q",
					tt.created, tt.noTrunc, got, tt.want)
			}
		})
	}

	t.Run("recent timestamp shows relative time", func(t *testing.T) {
		recent := time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)
		got := formatCreated(recent, false)
		if got == recent {
			t.Errorf("expected humanized time, got raw timestamp %q", got)
		}
		if got == "" {
			t.Error("expected non-empty result")
		}
	})
}
