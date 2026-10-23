package cli

import (
	"fmt"
	"strings"

	"github.com/redhat-et/skillimage/pkg/oci"
	"github.com/spf13/cobra"
)

func newPullCmd() *cobra.Command {
	var outputDir string
	var tlsVerify bool
	cmd := &cobra.Command{
		Use:     "pull <ref>",
		Short:   "Pull a skill or Agent Plugin image from a remote registry",
		Long:    "Pull a skill or Agent Plugin image from an OCI registry to the local store. Use -o to unpack it using the package's format under the output directory.",
		Example: "  skillctl pull quay.io/acme/hr-onboarding:1.0.0 -o ./packages/",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPull(cmd, args[0], outputDir, !tlsVerify)
		},
	}
	cmd.Flags().StringVarP(&outputDir, "output", "o", "", "unpack the package under this directory")
	cmd.Flags().BoolVar(&tlsVerify, "tls-verify", true, "require HTTPS and verify certificates")
	return cmd
}

func looksLocal(ref string) bool {
	return !strings.Contains(ref, "/")
}

func runPull(cmd *cobra.Command, ref string, outputDir string, skipTLSVerify bool) error {
	if looksLocal(ref) {
		return fmt.Errorf("%s looks like a local reference, not a remote registry\n\nFor a skill image in the local store, use:\n  skillctl install %s --target <agent>\n  skillctl install %s -o <directory>\n\n`skillctl install` does not install Agent Plugin packages", ref, ref, ref)
	}

	client, err := defaultClient()
	if err != nil {
		return err
	}

	desc, extractedPath, err := client.PullWithUnpack(cmd.Context(), ref, oci.PullOptions{
		OutputDir:     outputDir,
		SkipTLSVerify: skipTLSVerify,
	})
	if err != nil {
		return fmt.Errorf("pulling: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Pulled %s\nDigest: %s\n", ref, desc.Digest)
	if extractedPath != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Unpacked to %s\n", extractedPath)
	}
	return nil
}
