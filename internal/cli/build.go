package cli

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"

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
		Use:   "build <dir-or-url> [plugin-image-ref]",
		Short: "Build a skill or Agent Plugin directory or Git repo into a local OCI image",
		Long:  "Build a skill or Agent Plugin package into a local OCI image. The package type is detected from the input. For plugin packages, an optional full OCI reference can be supplied as the second argument.",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			imageRef := ""
			if len(args) == 2 {
				imageRef = args[1]
			}
			if source.IsRemote(args[0]) {
				return runBuildRemote(cmd, args[0], imageRef, tag, mediaType, ref, filter)
			}
			return runBuild(cmd, args[0], imageRef, tag, mediaType)
		},
	}
	cmd.Flags().StringVar(&tag, "tag", "", "override the image tag (default: skill version-draft or plugin version)")
	cmd.Flags().StringVar(&mediaType, "media-type", "", "media type profile: \"standard\" (default) or \"redhat\" (for oc-mirror)")
	cmd.Flags().StringVar(&ref, "ref", "", "Git ref to checkout (branch, tag, or commit SHA)")
	cmd.Flags().StringVar(&filter, "filter", "", "glob pattern to filter skills by name")
	return cmd
}

func runBuild(cmd *cobra.Command, dir, imageRef, tag, mediaType string) error {
	isPlugin, err := hasPluginManifest(dir)
	if err != nil {
		return err
	}
	if isPlugin {
		return runBuildPlugin(cmd, dir, imageRef, tag, mediaType)
	}
	if imageRef != "" {
		return fmt.Errorf("the positional OCI reference is only supported when building an Agent Plugin package")
	}

	profile, err := oci.ParseMediaTypeProfile(mediaType)
	if err != nil {
		return err
	}

	client, err := defaultClient()
	if err != nil {
		return err
	}

	desc, err := client.Build(cmd.Context(), dir, oci.BuildOptions{
		Tag:       tag,
		MediaType: profile,
	})
	if err != nil {
		return fmt.Errorf("building %s: %w", dir, err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Built %s\nDigest: %s\n", dir, desc.Digest)
	return nil
}

func runBuildPlugin(cmd *cobra.Command, dir, imageRef, tag, mediaType string) error {
	profile, err := oci.ParseMediaTypeProfile(mediaType)
	if err != nil {
		return err
	}

	client, err := defaultClient()
	if err != nil {
		return err
	}
	result, err := client.BuildPlugin(cmd.Context(), dir, oci.PluginBuildOptions{
		ImageRef:  imageRef,
		Tag:       tag,
		MediaType: profile,
	})
	if err != nil {
		return fmt.Errorf("building Agent Plugin: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Built plugin %s\nDigest: %s\n", result.ImageRef, result.Descriptor.Digest)
	return nil
}

func hasPluginManifest(dir string) (bool, error) {
	_, err := os.Lstat(filepath.Join(dir, "plugin.json"))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("checking for plugin.json: %w", err)
}

func runBuildRemote(cmd *cobra.Command, rawURL, imageRef, tag, mediaType, ref, filter string) error {
	profile, err := oci.ParseMediaTypeProfile(mediaType)
	if err != nil {
		return err
	}

	ctx := cmd.Context()

	displayURL := sanitizeURL(rawURL)
	fmt.Fprintf(cmd.OutOrStdout(), "Cloning %s", displayURL)
	if ref != "" {
		fmt.Fprintf(cmd.OutOrStdout(), " (ref: %s)", ref)
	}
	fmt.Fprintln(cmd.OutOrStdout(), "...")

	result, err := source.Resolve(ctx, rawURL, ref, filter)
	if err != nil {
		return err
	}
	defer result.Cleanup()

	if result.PluginDir != "" {
		if filter != "" {
			return fmt.Errorf("--filter applies only to repositories containing skills")
		}
		return runBuildPlugin(cmd, result.PluginDir, imageRef, tag, mediaType)
	}
	if imageRef != "" {
		return fmt.Errorf("the positional OCI reference is only supported when building an Agent Plugin package")
	}

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

	fmt.Fprintf(cmd.OutOrStdout(), "\nBuilt %d skills from %s\n", built, displayURL)
	if failed > 0 {
		return fmt.Errorf("%d skill(s) failed to build", failed)
	}
	return nil
}

func sanitizeURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	u.User = nil
	return u.String()
}
