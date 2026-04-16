package cli

import (
	"context"
	"fmt"

	"github.com/redhat-et/skillimage/pkg/oci"
	"github.com/spf13/cobra"
)

func newPushCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "push <ref>",
		Short: "Push a skill image from local store to a remote registry",
		Long: `Push a skill image to a remote OCI registry.

The ref should be a full OCI reference: registry/namespace/name:tag
Example: quay.io/acme/hr-onboarding:1.0.0-draft`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPush(cmd, args[0])
		},
	}
}

func runPush(cmd *cobra.Command, ref string) error {
	client, err := defaultClient()
	if err != nil {
		return err
	}

	if err := client.Push(context.Background(), ref, oci.PushOptions{}); err != nil {
		return fmt.Errorf("pushing: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Pushed %s\n", ref)
	return nil
}
