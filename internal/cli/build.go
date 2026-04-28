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
	cmd := &cobra.Command{
		Use:   "build <dir>",
		Short: "Build a skill directory into a local OCI image",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBuild(cmd, args[0], tag, mediaType)
		},
	}
	cmd.Flags().StringVar(&tag, "tag", "", "override the image tag (default: <version>-draft)")
	cmd.Flags().StringVar(&mediaType, "media-type", "", `media type profile: "standard" (default) or "redhat" (for oc-mirror)`)
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
