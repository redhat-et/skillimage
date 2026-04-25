package cli

import (
	"context"
	"fmt"

	"github.com/redhat-et/skillimage/pkg/oci"
	"github.com/spf13/cobra"
)

func newPackCmd() *cobra.Command {
	var tag string
	var mediaType string
	var bundle bool
	cmd := &cobra.Command{
		Use:   "pack <dir>",
		Short: "Pack a skill directory into a local OCI image",
		Long: `Pack a skill directory into a local OCI image.

Use --bundle to pack a directory containing multiple skill
subdirectories into a single OCI image.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if bundle {
				return runPackBundle(cmd, args[0], tag, mediaType)
			}
			return runPack(cmd, args[0], tag, mediaType)
		},
	}
	cmd.Flags().StringVar(&tag, "tag", "", "override the image tag (default: <version>-draft)")
	cmd.Flags().StringVar(&mediaType, "media-type", "", `media type profile: "standard" (default) or "redhat" (for oc-mirror)`)
	cmd.Flags().BoolVar(&bundle, "bundle", false, "pack multiple skill subdirectories as a single image")
	return cmd
}

func runPack(cmd *cobra.Command, dir, tag, mediaType string) error {
	profile, err := oci.ParseMediaTypeProfile(mediaType)
	if err != nil {
		return err
	}

	client, err := defaultClient()
	if err != nil {
		return err
	}

	desc, err := client.Pack(context.Background(), dir, oci.PackOptions{
		Tag:       tag,
		MediaType: profile,
	})
	if err != nil {
		return fmt.Errorf("packing %s: %w", dir, err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Packed %s\nDigest: %s\n", dir, desc.Digest)
	return nil
}

func runPackBundle(cmd *cobra.Command, dir, tag, mediaType string) error {
	profile, err := oci.ParseMediaTypeProfile(mediaType)
	if err != nil {
		return err
	}

	client, err := defaultClient()
	if err != nil {
		return err
	}

	desc, err := client.PackBundle(context.Background(), dir, oci.BundlePackOptions{
		Tag:       tag,
		MediaType: profile,
	})
	if err != nil {
		return fmt.Errorf("packing bundle %s: %w", dir, err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Packed bundle %s\nDigest: %s\n", dir, desc.Digest)
	return nil
}
