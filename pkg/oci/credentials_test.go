package oci

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPodmanAuthPaths(t *testing.T) {
	t.Run("returns XDG_RUNTIME_DIR path first on Linux-like env", func(t *testing.T) {
		t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
		t.Setenv("XDG_CONFIG_HOME", "")

		paths := podmanAuthPaths()

		if len(paths) < 1 {
			t.Fatal("expected at least 1 path")
		}
		want := filepath.Join("/run/user/1000", "containers", "auth.json")
		if paths[0] != want {
			t.Errorf("paths[0] = %q, want %q", paths[0], want)
		}
	})

	t.Run("includes XDG_CONFIG_HOME path as fallback", func(t *testing.T) {
		t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
		t.Setenv("XDG_CONFIG_HOME", "/home/testuser/.config")

		paths := podmanAuthPaths()

		if len(paths) < 2 {
			t.Fatalf("expected at least 2 paths, got %d", len(paths))
		}
		want := filepath.Join("/home/testuser/.config", "containers", "auth.json")
		if paths[1] != want {
			t.Errorf("paths[1] = %q, want %q", paths[1], want)
		}
	})

	t.Run("deduplicates when XDG_RUNTIME_DIR and XDG_CONFIG_HOME match", func(t *testing.T) {
		t.Setenv("XDG_RUNTIME_DIR", "/tmp/shared-config")
		t.Setenv("XDG_CONFIG_HOME", "/tmp/shared-config")

		paths := podmanAuthPaths()

		if len(paths) != 1 {
			t.Fatalf("expected 1 path, got %d: %v", len(paths), paths)
		}
	})

	t.Run("falls back to home config when no XDG vars set", func(t *testing.T) {
		t.Setenv("XDG_RUNTIME_DIR", "")
		t.Setenv("XDG_CONFIG_HOME", "")

		paths := podmanAuthPaths()

		if len(paths) == 0 {
			t.Fatal("expected at least 1 path")
		}
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skip("cannot determine home dir")
		}
		want := filepath.Join(home, ".config", "containers", "auth.json")
		if paths[0] != want {
			t.Errorf("paths[0] = %q, want %q", paths[0], want)
		}
	})
}
