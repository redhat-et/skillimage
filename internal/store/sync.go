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
	SkipTLSVerify bool
}

// Sync performs a full sync from the OCI registry into the store.
// It lists all repositories, fetches manifests, filters for skill
// images, and upserts into the database. Stale entries (no longer
// present in the registry) are cleaned up.
func (s *Store) Sync(ctx context.Context, cfg SyncConfig) error {
	syncStart := time.Now()

	repos, err := oci.ListRemoteRepositories(ctx, cfg.RegistryURL, cfg.Namespace, cfg.SkipTLSVerify)
	if err != nil {
		return err
	}

	for _, repo := range repos {
		tags, err := oci.ListRemoteTags(ctx, cfg.RegistryURL, repo, cfg.SkipTLSVerify)
		if err != nil {
			slog.Warn("listing tags failed, skipping repo", "repo", repo, "error", err)
			continue
		}

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
			}
		}
	}

	deleted, err := s.DeleteStale(syncStart)
	if err != nil {
		slog.Warn("stale cleanup failed", "error", err)
	} else if deleted > 0 {
		slog.Info("cleaned up stale entries", "count", deleted)
	}

	return nil
}

func manifestToSkill(sm *oci.SkillManifest) Skill {
	ann := sm.Annotations
	wc, _ := strconv.Atoi(ann[oci.AnnotationWordCount])

	sk := Skill{
		Repository:    sm.Repository,
		Tag:           sm.Tag,
		Digest:        sm.Digest,
		Name:          parseName(ann, sm.Repository),
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

func parseName(ann map[string]string, repo string) string {
	if title := ann[ocispec.AnnotationTitle]; title != "" {
		return title
	}
	if idx := strings.LastIndex(repo, "/"); idx >= 0 {
		return repo[idx+1:]
	}
	return repo
}
