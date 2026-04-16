package oci

import (
	"oras.land/oras-go/v2/content/oci"
)

// Client provides OCI operations against a local OCI layout store.
type Client struct {
	store     *oci.Store
	storePath string
}

// NewClient creates a new OCI client backed by a local OCI layout store
// at the given path. The directory is created if it does not exist.
func NewClient(storePath string) (*Client, error) {
	store, err := oci.New(storePath)
	if err != nil {
		return nil, err
	}
	return &Client{
		store:     store,
		storePath: storePath,
	}, nil
}

// PackOptions configures the Pack operation.
type PackOptions struct {
	// Tag overrides the default tag. If empty, defaults to <version>-draft.
	Tag string
}

// LocalImage holds metadata for an image stored in the local OCI layout.
type LocalImage struct {
	Name    string
	Version string
	Tag     string
	Digest  string
	Status  string
	Created string
}
