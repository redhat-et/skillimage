package cli

import (
	"context"
	"fmt"

	"github.com/redhat-et/skillimage/pkg/oci"
	"github.com/redhat-et/skillimage/pkg/source"
	"github.com/spf13/cobra"
)

func newBuildCmd() *cobra.Command {
	var tag string
	var mediaType string
	var ref string
	var filter string
	cmd := &cobra.Command{
		Use:   "build <dir-or-url>",
		Short: "Build a skill directory or Git repo into local OCI images",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if source.IsRemote(args[0]) {
				return runBuildRemote(cmd, args[0], tag, mediaType, ref, filter)
			}
			return runBuild(cmd, args[0], tag, mediaType)
		},
	}
	cmd.Flags().StringVar(&tag, "tag", "", "override the image tag (default: <version>-draft)")
	cmd.Flags().StringVar(&mediaType, "media-type", "", `media type profile: "standard" (default) or "redhat" (for oc-mirror)`)
	cmd.Flags().StringVar(&ref, "ref", "", "Git ref to checkout (branch, tag, or commit SHA)")
	cmd.Flags().StringVar(&filter, "filter", "", "glob pattern to filter skills by name")
	return cmd
}

func runBuild(cmd *cobra.Command, dir, tag, mediaType string) error {
	profile, err := oci.ParseMediaTypeProfile(mediaType)
	if err != nil {
		return err
	}

	client, err := defaultClient()
	if err != nil {
		return err
	}

	desc, err := client.Build(context.Background(), dir, oci.BuildOptions{
		Tag:       tag,
		MediaType: profile,
	})
	if err != nil {
		return fmt.Errorf("building %s: %w", dir, err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Built %s\nDigest: %s\n", dir, desc.Digest)
	return nil
}

func runBuildRemote(cmd *cobra.Command, rawURL, tag, mediaType, ref, filter string) error {
	profile, err := oci.ParseMediaTypeProfile(mediaType)
	if err != nil {
		return err
	}

	ctx := context.Background()

	fmt.Fprintf(cmd.OutOrStdout(), "Cloning %s", rawURL)
	if ref != "" {
		fmt.Fprintf(cmd.OutOrStdout(), " (ref: %s)", ref)
	}
	fmt.Fprintln(cmd.OutOrStdout(), "...")

	result, err := source.Resolve(ctx, rawURL, ref, filter)
	if err != nil {
		return err
	}
	defer result.Cleanup()

	if tag != "" && len(result.Skills) > 1 {
		return fmt.Errorf("--tag cannot be used when building multiple skills")
	}

	client, err := defaultClient()
	if err != nil {
		return err
	}

	var built, failed int
	for i, skill := range result.Skills {
		fmt.Fprintf(cmd.OutOrStdout(), "Building %s (%d/%d)...\n", skill.Name, i+1, len(result.Skills))

		desc, err := client.Build(ctx, skill.Dir, oci.BuildOptions{
			Tag:       tag,
			MediaType: profile,
			SkillCard: skill.SkillCard,
		})
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "  Error: %v\n", err)
			failed++
			continue
		}
		fmt.Fprintf(cmd.OutOrStdout(), "  Digest: %s\n", desc.Digest)
		built++
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\nBuilt %d skills from %s\n", built, rawURL)
	if failed > 0 {
		return fmt.Errorf("%d skill(s) failed to build", failed)
	}
	return nil
}
