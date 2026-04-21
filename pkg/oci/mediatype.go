package oci

import "fmt"

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
	RedHatMediaTypeSkillLayer  = "application/vnd.redhat.agentskill.layer.v1+tar"
	RedHatMediaTypeSkillConfig = "application/vnd.redhat.agentskill.config.v1+json"
)

// resolveMediaTypes returns the layer and config media types for the given
// profile. An empty profile defaults to standard OCI types.
func resolveMediaTypes(profile MediaTypeProfile) (layer, config string, err error) {
	switch profile {
	case "", MediaTypeStandard:
		return "application/vnd.oci.image.layer.v1.tar+gzip",
			"application/vnd.oci.image.config.v1+json",
			nil
	case MediaTypeRedHat:
		return RedHatMediaTypeSkillLayer,
			RedHatMediaTypeSkillConfig,
			nil
	default:
		return "", "", fmt.Errorf("unknown media type profile: %q (valid: standard, redhat)", profile)
	}
}
