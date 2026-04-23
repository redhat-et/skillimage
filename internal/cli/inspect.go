package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/redhat-et/skillimage/pkg/oci"
)

func newInspectCmd() *cobra.Command {
	var tlsVerify bool
	cmd := &cobra.Command{
		Use:   "inspect <ref>",
		Short: "Show SkillCard metadata and OCI image details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInspect(cmd, args[0], !tlsVerify)
		},
	}
	cmd.Flags().BoolVar(&tlsVerify, "tls-verify", true, "require HTTPS and verify certificates")
	return cmd
}

func runInspect(cmd *cobra.Command, ref string, skipTLSVerify bool) error {
	client, err := defaultClient()
	if err != nil {
		return err
	}

	ctx := context.Background()

	// Try local first, fall back to remote.
	result, localErr := client.Inspect(ctx, ref)
	if localErr != nil {
		var remoteErr error
		result, remoteErr = client.InspectRemote(ctx, ref, oci.InspectOptions{SkipTLSVerify: skipTLSVerify})
		if remoteErr != nil {
			return fmt.Errorf("inspecting %s: %w", ref, errors.Join(localErr, remoteErr))
		}
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Skill:        %s\n", result.Name)
	if result.DisplayName != "" {
		fmt.Fprintf(out, "Display Name: %s\n", result.DisplayName)
	}
	fmt.Fprintf(out, "Version:      %s\n", result.Version)
	fmt.Fprintf(out, "Status:       %s\n", result.Status)
	if result.Description != "" {
		fmt.Fprintf(out, "Description:  %s\n", result.Description)
	}
	if result.Authors != "" {
		fmt.Fprintf(out, "Authors:      %s\n", result.Authors)
	}
	if result.License != "" {
		fmt.Fprintf(out, "License:      %s\n", result.License)
	}
	if result.Tags != "" {
		fmt.Fprintf(out, "Tags:         %s\n", result.Tags)
	}
	if result.Compatibility != "" {
		fmt.Fprintf(out, "Compat:       %s\n", result.Compatibility)
	}
	if result.WordCount != "" {
		fmt.Fprintf(out, "Word Count:   %s\n", result.WordCount)
	}
	fmt.Fprintln(out)
	fmt.Fprintf(out, "OCI Image:\n")
	fmt.Fprintf(out, "  Digest:     %s\n", result.Digest)
	fmt.Fprintf(out, "  Created:    %s\n", result.Created)
	fmt.Fprintf(out, "  Size:       %d bytes\n", result.Size)
	fmt.Fprintf(out, "  Layers:     %d\n", result.LayerCount)

	return nil
}
