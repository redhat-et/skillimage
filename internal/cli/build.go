package cli

import (
	"context"
	"fmt"

	"github.com/redhat-et/skillimage/pkg/oci"
	"github.com/spf13/cobra"
)

func newBuildCmd() *cobra.Command {
	var tag string
	var mediaType string
	var bundle bool
	cmd := &cobra.Command{
		Use:   "build <dir>",
		Short: "Build a skill directory into a local OCI image",
		Long: `Build a skill directory into a local OCI image.

Use --bundle to build a directory containing multiple skill
subdirectories into a single OCI image.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if bundle {
				return runBuildBundle(cmd, args[0], tag, mediaType)
			}
			return runBuild(cmd, args[0], tag, mediaType)
		},
	}
	cmd.Flags().StringVar(&tag, "tag", "", "override the image tag (default: <version>-draft)")
	cmd.Flags().StringVar(&mediaType, "media-type", "", `media type profile: "standard" (default) or "redhat" (for oc-mirror)`)
	cmd.Flags().BoolVar(&bundle, "bundle", false, "build multiple skill subdirectories as a single image")
	return cmd
}

func runBuild(cmd *cobra.Command, dir, tag, mediaType string) error {
	profile, err := oci.ParseMediaTypeProfile(mediaType)
	if err != nil {
		return err
	}

	client, err := defaultClient()
	if err != nil {
		return err
	}

	desc, err := client.Build(context.Background(), dir, oci.BuildOptions{
		Tag:       tag,
		MediaType: profile,
	})
	if err != nil {
		return fmt.Errorf("building %s: %w", dir, err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Built %s\nDigest: %s\n", dir, desc.Digest)
	return nil
}

func runBuildBundle(cmd *cobra.Command, dir, tag, mediaType string) error {
	profile, err := oci.ParseMediaTypeProfile(mediaType)
	if err != nil {
		return err
	}

	client, err := defaultClient()
	if err != nil {
		return err
	}

	desc, err := client.BuildBundle(context.Background(), dir, oci.BundleBuildOptions{
		Tag:       tag,
		MediaType: profile,
	})
	if err != nil {
		return fmt.Errorf("building bundle %s: %w", dir, err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Built bundle %s\nDigest: %s\n", dir, desc.Digest)
	return nil
}
