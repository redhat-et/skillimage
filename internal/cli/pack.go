package cli

import (
	"context"
	"fmt"

	"github.com/redhat-et/skillimage/pkg/oci"
	"github.com/spf13/cobra"
)

func newPackCmd() *cobra.Command {
	var tag string
	cmd := &cobra.Command{
		Use:   "pack <dir>",
		Short: "Pack a skill directory into a local OCI image",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPack(cmd, args[0], tag)
		},
	}
	cmd.Flags().StringVar(&tag, "tag", "", "override the image tag (default: <version>-draft)")
	return cmd
}

func runPack(cmd *cobra.Command, dir string, tag string) error {
	client, err := defaultClient()
	if err != nil {
		return err
	}

	desc, err := client.Pack(context.Background(), dir, oci.PackOptions{Tag: tag})
	if err != nil {
		return fmt.Errorf("packing %s: %w", dir, err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Packed %s\nDigest: %s\n", dir, desc.Digest)
	return nil
}
