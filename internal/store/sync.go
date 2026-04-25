package store

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"time"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/redhat-et/skillimage/pkg/oci"
)

// SyncConfig controls how the sync engine connects to the registry.
type SyncConfig struct {
	RegistryURL   string
	Namespace     string
	Repositories  []string
	SkipTLSVerify bool
}

// Sync performs a full sync from the OCI registry into the store.
// It lists all repositories, fetches manifests, filters for skill
// images, and upserts into the database. Stale entries (no longer
// present in the registry) are cleaned up.
func (s *Store) Sync(ctx context.Context, cfg SyncConfig) error {
	syncStart := time.Now()

	repos, err := discoverRepositories(ctx, cfg)
	if err != nil {
		return err
	}
	slog.Info("discovered repositories", "count", len(repos))

	if len(repos) == 0 {
		slog.Warn("no repositories found — if using a public registry (quay.io, ghcr.io), set --repositories explicitly since most public registries do not support the /v2/_catalog API")
		return nil
	}

	var indexed int
	for _, repo := range repos {
		tags, err := oci.ListRemoteTags(ctx, cfg.RegistryURL, repo, cfg.SkipTLSVerify)
		if err != nil {
			slog.Warn("listing tags failed, skipping repo", "repo", repo, "error", err)
			continue
		}
		slog.Info("found tags", "repo", repo, "count", len(tags))

		for _, tag := range tags {
			sm, err := oci.FetchManifestAnnotations(ctx, cfg.RegistryURL, repo, tag, cfg.SkipTLSVerify)
			if err != nil {
				slog.Warn("fetching manifest failed, skipping", "repo", repo, "tag", tag, "error", err)
				continue
			}
			if sm == nil {
				continue
			}

			sk := manifestToSkill(sm)
			if err := s.UpsertSkill(sk); err != nil {
				slog.Warn("upserting skill failed", "repo", repo, "tag", tag, "error", err)
			} else {
				indexed++
			}
		}
	}

	slog.Info("sync indexed skills", "count", indexed)

	deleted, err := s.DeleteStale(syncStart)
	if err != nil {
		slog.Warn("stale cleanup failed", "error", err)
	} else if deleted > 0 {
		slog.Info("cleaned up stale entries", "count", deleted)
	}

	return nil
}

func discoverRepositories(ctx context.Context, cfg SyncConfig) ([]string, error) {
	if len(cfg.Repositories) > 0 {
		slog.Info("using explicitly configured repositories", "repos", cfg.Repositories)
		return cfg.Repositories, nil
	}

	repos, err := oci.ListRemoteRepositories(ctx, cfg.RegistryURL, cfg.Namespace, cfg.SkipTLSVerify)
	if err != nil {
		return nil, err
	}
	return repos, nil
}

func manifestToSkill(sm *oci.SkillManifest) Skill {
	ann := sm.Annotations
	var wc int
	if wcStr := ann[oci.AnnotationWordCount]; wcStr != "" {
		var err error
		wc, err = strconv.Atoi(wcStr)
		if err != nil {
			slog.Debug("invalid word count annotation, defaulting to 0", "value", wcStr, "error", err)
		}
	}

	sk := Skill{
		Repository:    sm.Repository,
		Tag:           sm.Tag,
		Digest:        sm.Digest,
		Name:          parseName(sm.Repository),
		Namespace:     ann[ocispec.AnnotationVendor],
		Version:       ann[ocispec.AnnotationVersion],
		Status:        ann[oci.AnnotationStatus],
		DisplayName:   ann[ocispec.AnnotationTitle],
		Description:   ann[ocispec.AnnotationDescription],
		Authors:       ann[ocispec.AnnotationAuthors],
		License:       ann[ocispec.AnnotationLicenses],
		TagsJSON:      ann[oci.AnnotationTags],
		Compatibility: ann[oci.AnnotationCompatibility],
		WordCount:     wc,
		Created:       ann[ocispec.AnnotationCreated],
	}

	if ann[oci.AnnotationBundle] == "true" {
		sk.Bundle = true
		sk.BundleSkills = ann[oci.AnnotationBundleSkills]
	}

	return sk
}

func parseName(repo string) string {
	if idx := strings.LastIndex(repo, "/"); idx >= 0 {
		return repo[idx+1:]
	}
	return repo
}
