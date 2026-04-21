package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func newTagCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tag <source> <target>",
		Short: "Create a new reference for a local skill image",
		Long: `Create an additional tag for an existing local skill image.

Works like "docker tag" or "podman tag": the target reference
points to the same image as the source. Use this to set a
remote registry path before pushing.

Example:
  skillctl tag examples/hello-world:1.0.0-draft \
    quay.io/myorg/hello-world:1.0.0-draft
  skillctl push quay.io/myorg/hello-world:1.0.0-draft`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTag(cmd, args[0], args[1])
		},
	}
}

func runTag(cmd *cobra.Command, src, dst string) error {
	client, err := defaultClient()
	if err != nil {
		return err
	}

	if err := client.Tag(context.Background(), src, dst); err != nil {
		return fmt.Errorf("tagging: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Tagged %s as %s\n", src, dst)
	return nil
}
