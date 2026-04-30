package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func newRmCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:     "rm <ref> [<ref>...]",
		Aliases: []string{"delete"},
		Short:   "Remove skill images from local store",
		Long: `Remove one or more skill images from the local store by
tag reference (e.g., test/hello-world:1.0.0-draft).

Does not clean up unreferenced blobs — run 'skillctl prune'
after removal to reclaim disk space.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRm(cmd, args, force)
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "skip confirmation prompt")
	return cmd
}

func runRm(cmd *cobra.Command, refs []string, force bool) error {
	client, err := defaultClient()
	if err != nil {
		return err
	}

	ctx := context.Background()

	// Resolve all refs first to report errors before confirming.
	images, err := client.ListLocal()
	if err != nil {
		return fmt.Errorf("listing images: %w", err)
	}
	knownRefs := make(map[string]bool, len(images))
	for _, img := range images {
		knownRefs[img.Name+":"+img.Tag] = true
	}

	var valid []string
	var hadErrors bool
	for _, ref := range refs {
		if !knownRefs[ref] {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: image not found: %s\n", ref)
			hadErrors = true
			continue
		}
		valid = append(valid, ref)
	}

	if len(valid) == 0 {
		if hadErrors {
			return fmt.Errorf("no valid images to remove")
		}
		return nil
	}

	// Confirm unless --force.
	if !force {
		if len(valid) == 1 {
			fmt.Fprintf(cmd.OutOrStdout(), "Remove %s? [y/N] ", valid[0])
		} else {
			for _, ref := range valid {
				fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", ref)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Remove %d image(s)? [y/N] ", len(valid))
		}

		scanner := bufio.NewScanner(os.Stdin)
		if !scanner.Scan() {
			return nil
		}
		answer := strings.TrimSpace(scanner.Text())
		if answer != "y" && answer != "Y" {
			fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
			return nil
		}
	}

	// Remove each valid ref.
	for _, ref := range valid {
		if err := client.Remove(ctx, ref); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)
			hadErrors = true
			continue
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Removed %s\n", ref)
	}

	if hadErrors {
		return fmt.Errorf("some images could not be removed")
	}
	return nil
}
