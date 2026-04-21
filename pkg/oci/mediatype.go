package oci

import (
	"fmt"
	"strings"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// MediaTypeProfile selects which set of OCI media types to use when packing
// skill images. The default (empty string or "standard") uses standard OCI
// image media types. The "redhat" profile uses Red Hat-specific types that
// enable oc-mirror to identify skill artifacts for disconnected mirroring.
type MediaTypeProfile string

const (
	MediaTypeStandard MediaTypeProfile = "standard"
	MediaTypeRedHat   MediaTypeProfile = "redhat"
)

const (
	RedHatMediaTypeSkillLayer  = "application/vnd.redhat.agentskill.layer.v1.tar+gzip"
	RedHatMediaTypeSkillConfig = "application/vnd.redhat.agentskill.config.v1+json"
)

// ParseMediaTypeProfile validates and normalizes a media type profile string.
func ParseMediaTypeProfile(s string) (MediaTypeProfile, error) {
	p := MediaTypeProfile(strings.TrimSpace(strings.ToLower(s)))
	switch p {
	case "", MediaTypeStandard:
		return MediaTypeStandard, nil
	case MediaTypeRedHat:
		return MediaTypeRedHat, nil
	default:
		return "", fmt.Errorf("unknown media type profile: %q (valid: standard, redhat)", s)
	}
}

// resolveMediaTypes returns the layer and config media types for the given
// profile. An empty profile defaults to standard OCI types.
func resolveMediaTypes(profile MediaTypeProfile) (layer, config string) {
	switch profile {
	case MediaTypeRedHat:
		return RedHatMediaTypeSkillLayer, RedHatMediaTypeSkillConfig
	default:
		return ocispec.MediaTypeImageLayerGzip, ocispec.MediaTypeImageConfig
	}
}
