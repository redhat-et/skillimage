package cli

import (
	"context"
	"fmt"

	"github.com/redhat-et/skillimage/pkg/oci"
	"github.com/spf13/cobra"
)

func newCollectionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "collection",
		Short: "Manage skill collections",
	}
	cmd.AddCommand(newCollectionPushCmd())
	return cmd
}

func newCollectionPushCmd() *cobra.Command {
	var file string
	var tlsVerify bool
	cmd := &cobra.Command{
		Use:   "push <ref>",
		Short: "Push a collection YAML to a registry as an OCI artifact",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCollectionPush(cmd, args[0], file, !tlsVerify)
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "path to collection YAML file (required)")
	_ = cmd.MarkFlagRequired("file")
	cmd.Flags().BoolVar(&tlsVerify, "tls-verify", true, "require HTTPS and verify certificates")
	return cmd
}

func runCollectionPush(cmd *cobra.Command, ref, file string, skipTLSVerify bool) error {
	client, err := defaultClient()
	if err != nil {
		return err
	}

	ctx := context.Background()
	desc, err := client.BuildCollectionArtifact(ctx, file, ref)
	if err != nil {
		return fmt.Errorf("building collection artifact: %w", err)
	}

	if err := client.PushCollection(ctx, ref, oci.PushOptions{SkipTLSVerify: skipTLSVerify}); err != nil {
		return fmt.Errorf("pushing collection: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Pushed collection %s\nDigest: %s\n", ref, desc.Digest)
	return nil
}
