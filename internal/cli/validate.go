package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/redhat-et/skillimage/pkg/agentplugin"
	"github.com/redhat-et/skillimage/pkg/skillcard"
	"github.com/spf13/cobra"
)

func newValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate <dir|file>",
		Short: "Validate a SkillCard or Agent Plugin package",
		Args:  cobra.ExactArgs(1),
		RunE:  runValidate,
	}
}

func runValidate(cmd *cobra.Command, args []string) error {
	path := args[0]
	if filepath.Base(path) == "plugin.json" {
		return validatePlugin(cmd, filepath.Dir(path))
	}

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("accessing %s: %w", path, err)
	}
	if info.IsDir() {
		isPlugin, err := hasPluginManifest(path)
		if err != nil {
			return err
		}
		if isPlugin {
			return validatePlugin(cmd, path)
		}
		path = filepath.Join(path, "skill.yaml")
	}

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("opening %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	sc, err := skillcard.Parse(f)
	if err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}

	errs, err := skillcard.Validate(sc)
	if err != nil {
		return fmt.Errorf("validating %s: %w", path, err)
	}

	if len(errs) > 0 {
		fmt.Fprintf(cmd.ErrOrStderr(), "✗ %s has %d error(s):\n", path, len(errs))
		for _, e := range errs {
			fmt.Fprintf(cmd.ErrOrStderr(), "  %s: %s\n", e.Field, e.Message)
		}
		return fmt.Errorf("validation failed with %d error(s)", len(errs))
	}

	fmt.Fprintf(cmd.OutOrStdout(), "✓ %s is valid\n", path)
	return nil
}

func validatePlugin(cmd *cobra.Command, dir string) error {
	info, err := agentplugin.ValidateDir(dir)
	if err != nil {
		return fmt.Errorf("validating Agent Plugin: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "✓ %s is a valid Agent Plugin (%d skills, %d MCP servers, %d extensions)\n",
		dir, len(info.Skills), len(info.MCPServers), len(info.Extensions))
	return nil
}
