package oci

import (
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content/oci"

	"github.com/redhat-et/skillimage/pkg/skillcard"
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

// BuildOptions configures the Build operation.
type BuildOptions struct {
	// Tag overrides the default tag. If empty, defaults to <version>-draft.
	Tag string
	// MediaType selects the media type profile. Empty or "standard" uses
	// standard OCI types; "redhat" uses Red Hat-specific types for oc-mirror.
	MediaType MediaTypeProfile
	// SkillCard is a pre-built SkillCard to use instead of reading skill.yaml
	// from disk. When non-nil, Build skips the file read/parse/validate steps.
	SkillCard *skillcard.SkillCard
}

// PluginBuildOptions configures the BuildPlugin operation.
type PluginBuildOptions struct {
	// ImageRef, if set, overrides the repository name from plugin.json. If it
	// is untagged, is not a digest reference, and Tag is empty, the manifest
	// version is used as the tag.
	ImageRef string
	// Tag overrides the manifest version or the tag in ImageRef. It cannot be
	// used with a digest reference.
	Tag string
	// MediaType selects the media type profile. Agent Plugin images currently
	// support only standard OCI media types; empty defaults to standard.
	MediaType MediaTypeProfile
}

// PluginBuildResult contains the resolved image reference and manifest
// descriptor produced by BuildPlugin.
type PluginBuildResult struct {
	ImageRef   string
	Descriptor ocispec.Descriptor
}

// PushOptions configures the Push operation.
type PushOptions struct {
	// SkipTLSVerify disables TLS certificate verification for the
	// remote registry (equivalent to --tls-verify=false).
	SkipTLSVerify bool
}

// PullOptions configures the Pull operation.
type PullOptions struct {
	// OutputDir, if set, causes the pulled image to be unpacked into this
	// directory after storing it locally.
	OutputDir string
	// SkipTLSVerify disables TLS certificate verification for the
	// remote registry (equivalent to --tls-verify=false).
	SkipTLSVerify bool
}

// LocalImage holds metadata for an image stored in the local OCI layout.
type LocalImage struct {
	Name         string
	Version      string
	Tag          string
	Digest       string
	Status       string
	Created      string
	Type         string
	ArtifactType string
}

// PromoteOptions configures the Promote operation.
type PromoteOptions struct {
	// SkipTLSVerify disables TLS certificate verification for the
	// remote registry (equivalent to --tls-verify=false).
	SkipTLSVerify bool
}

// InspectOptions configures the InspectRemote operation.
type InspectOptions struct {
	// SkipTLSVerify disables TLS certificate verification for the
	// remote registry (equivalent to --tls-verify=false).
	SkipTLSVerify bool
}

// InspectResult holds detailed metadata for a skill or Agent Plugin image.
type InspectResult struct {
	Name            string
	DisplayName     string
	Version         string
	Status          string
	Description     string
	Authors         string
	License         string
	Tags            string
	Compatibility   string
	WordCount       string
	Digest          string
	Created         string
	MediaType       string
	ConfigMediaType string
	LayerMediaType  string
	ArtifactType    string
	Size            int64
	LayerCount      int
	Plugin          *PluginInspectResult
}
