package oci

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

type manifestReadTestStore struct {
	body []byte
}

func (s manifestReadTestStore) Resolve(context.Context, string) (ocispec.Descriptor, error) {
	return ocispec.Descriptor{}, nil
}

func (s manifestReadTestStore) Fetch(context.Context, ocispec.Descriptor) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(s.body)), nil
}

func TestReadManifestBytesLimitsInput(t *testing.T) {
	body := bytes.Repeat([]byte("x"), maxManifestSize+128)
	reader := bytes.NewReader(body)
	_, err := readManifestBytes(reader)
	if err == nil || !strings.Contains(err.Error(), "manifest exceeds") {
		t.Fatalf("readManifestBytes() error = %v, want oversized manifest error", err)
	}
	if got, want := reader.Len(), 127; got != want {
		t.Errorf("bytes left unread = %d, want %d (read cap plus one byte)", got, want)
	}
}

func TestReadPluginManifestRejectsOversizedInput(t *testing.T) {
	store := manifestReadTestStore{body: bytes.Repeat([]byte("x"), maxManifestSize+1)}
	_, _, err := readPluginManifest(context.Background(), store, "plugin:latest")
	if err == nil || !strings.Contains(err.Error(), "manifest exceeds") {
		t.Fatalf("readPluginManifest() error = %v, want oversized manifest error", err)
	}
}

func TestInspectRejectsOversizedInput(t *testing.T) {
	store := manifestReadTestStore{body: bytes.Repeat([]byte("x"), maxManifestSize+1)}
	_, err := inspect(context.Background(), store, "plugin:latest")
	if err == nil || !strings.Contains(err.Error(), "manifest exceeds") {
		t.Fatalf("inspect() error = %v, want oversized manifest error", err)
	}
}
