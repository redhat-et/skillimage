package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/redhat-et/skillimage/pkg/lifecycle"
	"github.com/redhat-et/skillimage/pkg/oci"
)

func newPromoteCmd() *cobra.Command {
	var toState string
	var local bool
	cmd := &cobra.Command{
		Use:   "promote <ref>",
		Short: "Promote a skill to the next lifecycle state",
		Long: `Promote a skill image to a new lifecycle state.

State transitions: draft -> testing -> published -> deprecated -> archived

By default, operates on a remote registry. Use --local to promote
images in the local store.

Examples:
  skillctl promote quay.io/acme/hr-onboarding:1.0.0-draft --to testing
  skillctl promote test/test-skill:1.0.0-draft --to testing --local`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPromote(cmd, args[0], toState, local)
		},
	}
	cmd.Flags().StringVar(&toState, "to", "", "target lifecycle state (required)")
	_ = cmd.MarkFlagRequired("to")
	cmd.Flags().BoolVar(&local, "local", false, "promote in local store instead of remote registry")
	return cmd
}

func runPromote(cmd *cobra.Command, ref, toState string, local bool) error {
	to, err := lifecycle.ParseState(toState)
	if err != nil {
		return fmt.Errorf("invalid target state: %w", err)
	}

	client, err := defaultClient()
	if err != nil {
		return err
	}

	ctx := context.Background()

	if local {
		if err := client.PromoteLocal(ctx, ref, to); err != nil {
			return fmt.Errorf("promoting %s: %w", ref, err)
		}
	} else {
		if err := client.Promote(ctx, ref, to, oci.PromoteOptions{}); err != nil {
			return fmt.Errorf("promoting %s: %w", ref, err)
		}
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Promoted %s to %s\n", ref, to)
	return nil
}
