package oci

import (
	"fmt"
	"io"
)

const maxManifestSize = 4 << 20 // 4 MiB

// readManifestBytes caps manifest reads because manifests can come from remote
// registries.
func readManifestBytes(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxManifestSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxManifestSize {
		return nil, fmt.Errorf("manifest exceeds %d bytes", maxManifestSize)
	}
	return data, nil
}
