package cli

import (
	"context"
	"fmt"
	"os"

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

func defaultClient() (*oci.Client, error) {
	storeDir, err := defaultStoreDir()
	if err != nil {
		return nil, err
	}
	return oci.NewClient(storeDir)
}

func defaultStoreDir() (string, error) {
	dataDir := os.Getenv("XDG_DATA_HOME")
	if dataDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("finding home directory: %w", err)
		}
		dataDir = home + "/.local/share"
	}
	dir := dataDir + "/skillctl/store"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating store directory: %w", err)
	}
	return dir, nil
}
