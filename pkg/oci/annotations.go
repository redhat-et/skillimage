package oci

import (
	"fmt"
	"strings"
	"time"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/redhat-et/oci-skill-registry/pkg/lifecycle"
	"github.com/redhat-et/oci-skill-registry/pkg/skillcard"
)

// buildAnnotations maps SkillCard fields to standard OCI annotation keys
// and the custom skillregistry status annotation.
func buildAnnotations(sc *skillcard.SkillCard) map[string]string {
	ann := make(map[string]string)

	// Title: use display-name if set, otherwise name.
	title := sc.Metadata.DisplayName
	if title == "" {
		title = sc.Metadata.Name
	}
	ann[ocispec.AnnotationTitle] = title

	// Description: first 256 characters.
	desc := sc.Metadata.Description
	if len(desc) > 256 {
		desc = desc[:256]
	}
	ann[ocispec.AnnotationDescription] = desc

	// Version.
	ann[ocispec.AnnotationVersion] = sc.Metadata.Version

	// Authors: comma-separated "name <email>".
	if len(sc.Metadata.Authors) > 0 {
		var parts []string
		for _, a := range sc.Metadata.Authors {
			if a.Email != "" {
				parts = append(parts, fmt.Sprintf("%s <%s>", a.Name, a.Email))
			} else {
				parts = append(parts, a.Name)
			}
		}
		ann[ocispec.AnnotationAuthors] = strings.Join(parts, ", ")
	}

	// License.
	if sc.Metadata.License != "" {
		ann[ocispec.AnnotationLicenses] = sc.Metadata.License
	}

	// Vendor: namespace.
	ann[ocispec.AnnotationVendor] = sc.Metadata.Namespace

	// Created: RFC 3339 timestamp.
	ann[ocispec.AnnotationCreated] = time.Now().UTC().Format(time.RFC3339)

	// Provenance fields.
	if sc.Provenance != nil {
		if sc.Provenance.Source != "" {
			ann[ocispec.AnnotationSource] = sc.Provenance.Source
		}
		if sc.Provenance.Commit != "" {
			ann[ocispec.AnnotationRevision] = sc.Provenance.Commit
		}
	}

	// Lifecycle status: initial state is always draft.
	ann[lifecycle.StatusAnnotation] = string(lifecycle.Draft)

	return ann
}
