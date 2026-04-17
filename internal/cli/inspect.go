package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func newInspectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "inspect <ref>",
		Short: "Show SkillCard metadata and OCI image details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInspect(cmd, args[0])
		},
	}
}

func runInspect(cmd *cobra.Command, ref string) error {
	client, err := defaultClient()
	if err != nil {
		return err
	}

	result, err := client.Inspect(context.Background(), ref)
	if err != nil {
		return fmt.Errorf("inspecting %s: %w", ref, err)
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
	fmt.Fprintln(out)
	fmt.Fprintf(out, "OCI Image:\n")
	fmt.Fprintf(out, "  Digest:     %s\n", result.Digest)
	fmt.Fprintf(out, "  Created:    %s\n", result.Created)
	fmt.Fprintf(out, "  Size:       %d bytes\n", result.Size)
	fmt.Fprintf(out, "  Layers:     %d\n", result.LayerCount)

	return nil
}
