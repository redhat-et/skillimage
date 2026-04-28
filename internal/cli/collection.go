package cli

import (
	"context"
	"fmt"

	"github.com/redhat-et/skillimage/pkg/collection"
	"github.com/redhat-et/skillimage/pkg/oci"
	"github.com/spf13/cobra"
)

func newCollectionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "collection",
		Short: "Manage skill collections",
	}
	cmd.AddCommand(newCollectionPushCmd())
	cmd.AddCommand(newCollectionPullCmd())
	cmd.AddCommand(newCollectionVolumeCmd())
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

func newCollectionPullCmd() *cobra.Command {
	var outputDir string
	var tlsVerify bool
	cmd := &cobra.Command{
		Use:   "pull <ref>",
		Short: "Pull a collection and all its skills from a registry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCollectionPull(cmd, args[0], outputDir, !tlsVerify)
		},
	}
	cmd.Flags().StringVarP(&outputDir, "output", "o", "", "directory to extract skills into")
	cmd.Flags().BoolVar(&tlsVerify, "tls-verify", true, "require HTTPS and verify certificates")
	return cmd
}

func runCollectionPull(cmd *cobra.Command, ref, outputDir string, skipTLSVerify bool) error {
	client, err := defaultClient()
	if err != nil {
		return err
	}

	col, err := client.PullCollection(context.Background(), ref, outputDir, oci.PullOptions{
		OutputDir:     outputDir,
		SkipTLSVerify: skipTLSVerify,
	})
	if err != nil {
		return fmt.Errorf("pulling collection: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Pulled collection %s (%d skills)\n", col.Metadata.Name, len(col.Skills))
	for _, s := range col.Skills {
		fmt.Fprintf(cmd.OutOrStdout(), "  - %s (%s)\n", s.Name, s.Image)
	}
	return nil
}

func resolveCollection(file string, args []string) (*collection.SkillCollection, error) {
	if file != "" {
		return collection.ParseFile(file)
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("specify -f <file> or a registry reference")
	}
	return nil, fmt.Errorf("pulling collections from registry not yet supported in this command; use -f <file>")
}

func newCollectionVolumeCmd() *cobra.Command {
	var file string
	var mountRoot string
	var execute bool
	cmd := &cobra.Command{
		Use:   "volume [-f <file> | <ref>]",
		Short: "Generate Podman volume commands from a collection",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			col, err := resolveCollection(file, args)
			if err != nil {
				return err
			}
			if execute {
				fmt.Fprintf(cmd.OutOrStdout(), "# --execute is not yet implemented; printing commands instead:\n\n")
			}
			collection.GeneratePodmanVolumes(cmd.OutOrStdout(), col, mountRoot)
			return nil
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "path to collection YAML file")
	cmd.Flags().StringVar(&mountRoot, "mount-root", "/skills", "root mount path for volumes")
	cmd.Flags().BoolVar(&execute, "execute", false, "run the commands instead of printing them")
	return cmd
}
