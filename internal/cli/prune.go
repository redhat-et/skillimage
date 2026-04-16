package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func newPruneCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "prune",
		Short: "Remove local images that have been superseded by promotion",
		Long: `Remove local skill images whose lifecycle state has been
superseded. For each skill version, keeps only the image with the
highest lifecycle state and removes the rest.

For example, if a skill has draft, testing, and published images,
prune removes the draft and testing images.`,
		Args: cobra.NoArgs,
		RunE: runPrune,
	}
}

func runPrune(cmd *cobra.Command, args []string) error {
	client, err := defaultClient()
	if err != nil {
		return err
	}

	result, err := client.Prune(context.Background())
	if err != nil {
		return fmt.Errorf("pruning: %w", err)
	}

	if len(result.Removed) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "Nothing to prune.")
		return nil
	}

	for _, img := range result.Removed {
		fmt.Fprintf(cmd.OutOrStdout(), "Removed %s:%s (%s)\n", img.Name, img.Tag, img.Status)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "\nPruned %d image(s).\n", len(result.Removed))
	return nil
}
