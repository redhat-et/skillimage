package oci

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/registry/remote"

	"github.com/redhat-et/skillimage/pkg/lifecycle"
)

// SkillManifest holds metadata extracted from a manifest's annotations.
type SkillManifest struct {
	Repository  string
	Tag         string
	Digest      string
	Annotations map[string]string
}

// ListRemoteRepositories lists repository names from a remote registry.
// If prefix is non-empty, only repos starting with prefix are returned.
func ListRemoteRepositories(ctx context.Context, registryURL, prefix string, skipTLSVerify bool) ([]string, error) {
	reg, err := remote.NewRegistry(registryURL)
	if err != nil {
		return nil, fmt.Errorf("creating registry client: %w", err)
	}

	creds, err := credentialStore()
	if err != nil {
		slog.Warn("loading credentials failed, registry calls may fail with 401/403", "error", err)
		if skipTLSVerify {
			reg.Client = insecureHTTPClient()
		}
	} else {
		reg.Client = newAuthClient(creds, skipTLSVerify)
	}

	var repos []string
	err = reg.Repositories(ctx, "", func(repoNames []string) error {
		for _, name := range repoNames {
			if prefix == "" || strings.HasPrefix(name, prefix) {
				repos = append(repos, name)
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing repositories: %w", err)
	}
	return repos, nil
}

// ListRemoteTags lists all tags for a repository on a remote registry.
func ListRemoteTags(ctx context.Context, registryURL, repoName string, skipTLSVerify bool) ([]string, error) {
	ref := registryURL + "/" + repoName
	repo, err := newRemoteRepository(ref, skipTLSVerify)
	if err != nil {
		return nil, err
	}

	var tags []string
	err = repo.Tags(ctx, "", func(tagList []string) error {
		tags = append(tags, tagList...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing tags for %s: %w", repoName, err)
	}
	return tags, nil
}

// FetchManifestAnnotations fetches a manifest from a remote registry
// and returns its annotations and digest. Returns nil if the manifest
// has no io.skillimage.status annotation (not a skill image).
func FetchManifestAnnotations(ctx context.Context, registryURL, repoName, tag string, skipTLSVerify bool) (*SkillManifest, error) {
	ref := fmt.Sprintf("%s/%s:%s", registryURL, repoName, tag)
	repo, err := newRemoteRepository(ref, skipTLSVerify)
	if err != nil {
		return nil, err
	}

	desc, err := repo.Resolve(ctx, tag)
	if err != nil {
		return nil, fmt.Errorf("resolving %s:%s: %w", repoName, tag, err)
	}

	rc, err := repo.Fetch(ctx, desc)
	if err != nil {
		return nil, fmt.Errorf("fetching manifest: %w", err)
	}
	defer func() { _ = rc.Close() }()

	const maxManifestSize = 4 << 20 // 4 MiB
	manifestBytes, err := io.ReadAll(io.LimitReader(rc, maxManifestSize))
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}
	if int64(len(manifestBytes)) >= maxManifestSize {
		return nil, fmt.Errorf("manifest exceeds %d bytes", maxManifestSize)
	}

	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}

	if manifest.Annotations == nil {
		return nil, nil
	}
	if _, ok := manifest.Annotations[lifecycle.StatusAnnotation]; !ok {
		return nil, nil
	}

	return &SkillManifest{
		Repository:  repoName,
		Tag:         tag,
		Digest:      desc.Digest.String(),
		Annotations: manifest.Annotations,
	}, nil
}
