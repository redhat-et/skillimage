package installed

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// TagLister queries a remote registry for available tags.
// The repo argument is a full repository reference without a tag
// (e.g., "quay.io/acme/my-skill").
type TagLister func(ctx context.Context, repo string, skipTLSVerify bool) ([]string, error)

// CheckOptions configures the CheckUpgrades operation.
type CheckOptions struct {
	SkipTLSVerify bool
	TagLister     TagLister
}

// UpgradeCandidate describes an installed skill that has a newer
// published version available in its source registry.
type UpgradeCandidate struct {
	Installed     InstalledSkill
	LatestVersion string
	LatestRef     string
}

// CheckUpgrades queries source registries for each installed skill
// and returns candidates that have newer published versions available.
// Skills without provenance, with non-semver versions, or with
// local-only source refs are silently skipped.
func CheckUpgrades(ctx context.Context, skills []InstalledSkill, opts CheckOptions) ([]UpgradeCandidate, error) {
	var candidates []UpgradeCandidate

	for _, skill := range skills {
		if skill.Source == "" {
			continue
		}

		if !looksRemote(skill.Source) {
			continue
		}

		installedVer, err := semver.StrictNewVersion(skill.Version)
		if err != nil {
			continue
		}

		repo := repoFromRef(skill.Source)

		tags, err := opts.TagLister(ctx, repo, opts.SkipTLSVerify)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: cannot check %s: %v\n", skill.Name, err)
			continue
		}

		latest := highestPublished(tags)
		if latest == nil || !latest.GreaterThan(installedVer) {
			continue
		}

		candidates = append(candidates, UpgradeCandidate{
			Installed:     skill,
			LatestVersion: latest.Original(),
			LatestRef:     repo + ":" + latest.Original(),
		})
	}

	return candidates, nil
}

// looksRemote returns true if the ref contains a registry host.
// A ref is remote if its first path segment contains a dot or colon
// (e.g., "quay.io/...", "localhost:5000/...") but does not start
// with "." (which indicates a relative filesystem path).
func looksRemote(ref string) bool {
	if strings.HasPrefix(ref, ".") {
		return false
	}
	first := ref
	if idx := strings.Index(ref, "/"); idx >= 0 {
		first = ref[:idx]
	}
	return strings.ContainsAny(first, ".:")
}

// repoFromRef strips the tag or digest from a ref, returning the
// repository portion (e.g., "quay.io/acme/skill:1.0.0" becomes
// "quay.io/acme/skill").
func repoFromRef(ref string) string {
	if idx := strings.Index(ref, "@"); idx >= 0 {
		return ref[:idx]
	}
	lastSlash := strings.LastIndex(ref, "/")
	if lastSlash < 0 {
		if idx := strings.LastIndex(ref, ":"); idx >= 0 {
			return ref[:idx]
		}
		return ref
	}
	tail := ref[lastSlash+1:]
	if idx := strings.LastIndex(tail, ":"); idx >= 0 {
		return ref[:lastSlash+1+idx]
	}
	return ref
}

// highestPublished finds the highest semver version among tags that
// represent published skills (no -draft or -testing suffix).
func highestPublished(tags []string) *semver.Version {
	var best *semver.Version
	for _, tag := range tags {
		if strings.HasSuffix(tag, "-draft") || strings.HasSuffix(tag, "-testing") {
			continue
		}
		v, err := semver.StrictNewVersion(tag)
		if err != nil {
			continue
		}
		if best == nil || v.GreaterThan(best) {
			best = v
		}
	}
	return best
}
