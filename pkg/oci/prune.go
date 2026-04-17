package oci

import (
	"context"
	"fmt"

	"github.com/redhat-et/skillimage/pkg/lifecycle"
)

var stateRank = map[lifecycle.State]int{
	lifecycle.Draft:      0,
	lifecycle.Testing:    1,
	lifecycle.Published:  2,
	lifecycle.Deprecated: 3,
	lifecycle.Archived:   4,
}

// PruneResult describes what was removed by a prune operation.
type PruneResult struct {
	Removed []LocalImage
}

// Prune removes local images that have been superseded by a
// promotion. For each name+version group, it keeps only the image
// with the highest lifecycle state and removes the rest.
func (c *Client) Prune(ctx context.Context) (*PruneResult, error) {
	images, err := c.listLocalWithTags(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing images: %w", err)
	}

	type groupKey struct{ name, version string }
	groups := make(map[groupKey][]LocalImage)
	for _, img := range images {
		k := groupKey{img.Name, img.Version}
		groups[k] = append(groups[k], img)
	}

	var removed []LocalImage
	for _, imgs := range groups {
		if len(imgs) < 2 {
			continue
		}

		maxRank := -1
		for _, img := range imgs {
			state, err := lifecycle.ParseState(img.Status)
			if err != nil {
				continue
			}
			if r := stateRank[state]; r > maxRank {
				maxRank = r
			}
		}

		for _, img := range imgs {
			state, err := lifecycle.ParseState(img.Status)
			if err != nil {
				continue
			}
			if stateRank[state] < maxRank {
				fullRef := img.Name + ":" + img.Tag
				if err := c.store.Untag(ctx, fullRef); err != nil {
					return nil, fmt.Errorf("untagging %s: %w", fullRef, err)
				}
				removed = append(removed, img)
			}
		}
	}

	return &PruneResult{Removed: removed}, nil
}

// listLocalWithTags is like ListLocal but does NOT deduplicate by
// digest, so we can see all tags including "latest".
func (c *Client) listLocalWithTags(ctx context.Context) ([]LocalImage, error) {
	var images []LocalImage

	err := c.store.Tags(ctx, "", func(tags []string) error {
		for _, tag := range tags {
			desc, err := c.store.Resolve(ctx, tag)
			if err != nil {
				continue
			}

			img, err := c.imageFromManifest(ctx, tag, desc)
			if err != nil {
				continue
			}
			images = append(images, *img)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return images, nil
}
