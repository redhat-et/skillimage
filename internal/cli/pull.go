package cli

import (
	"context"
	"fmt"

	"github.com/redhat-et/oci-skill-registry/pkg/oci"
	"github.com/spf13/cobra"
)

func newPullCmd() *cobra.Command {
	var outputDir string
	cmd := &cobra.Command{
		Use:   "pull <ref>",
		Short: "Pull a skill image from a remote registry",
		Long: `Pull a skill image from an OCI registry to the local store.

Use -o to unpack skill files to a directory. If -o points to an
existing directory, a subdirectory named after the skill is created
automatically.

Examples:
  skillctl pull quay.io/acme/hr-onboarding:1.0.0
  skillctl pull quay.io/acme/hr-onboarding:1.0.0 -o ./skills/`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPull(cmd, args[0], outputDir)
		},
	}
	cmd.Flags().StringVarP(&outputDir, "output", "o", "", "unpack skill files to directory")
	return cmd
}

func runPull(cmd *cobra.Command, ref string, outputDir string) error {
	client, err := defaultClient()
	if err != nil {
		return err
	}

	desc, err := client.Pull(context.Background(), ref, oci.PullOptions{
		OutputDir: outputDir,
	})
	if err != nil {
		return fmt.Errorf("pulling: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Pulled %s\nDigest: %s\n", ref, desc.Digest)
	if outputDir != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Unpacked to %s\n", outputDir)
	}
	return nil
}
