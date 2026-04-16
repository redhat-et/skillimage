package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var agentTargets = map[string]string{
	"claude":   ".claude/skills",
	"cursor":   ".cursor/skills",
	"windsurf": ".codeium/windsurf/skills",
	"opencode": ".config/opencode/skills",
	"openclaw": ".openclaw/skills",
}

func newInstallCmd() *cobra.Command {
	var target string
	var outputDir string
	cmd := &cobra.Command{
		Use:   "install <ref>",
		Short: "Install a skill from local store to an agent's skill directory",
		Long: `Install a skill from the local store into a directory where
an agent can find it. Use --target for a known agent or -o for
a custom path.

Supported targets:
  claude    ~/.claude/skills/
  cursor    ~/.cursor/skills/
  windsurf  ~/.codeium/windsurf/skills/
  opencode  ~/.config/opencode/skills/
  openclaw  ~/.openclaw/skills/

Examples:
  skillctl install examples/hello-world:1.0.0-draft --target claude
  skillctl install myorg/my-skill:1.0.0 --target cursor
  skillctl install myorg/my-skill:1.0.0 -o ~/custom/skills/`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInstall(cmd, args[0], target, outputDir)
		},
	}
	cmd.Flags().StringVarP(&target, "target", "t", "", "agent name (claude, cursor, windsurf, opencode, openclaw)")
	cmd.Flags().StringVarP(&outputDir, "output", "o", "", "custom output directory")
	return cmd
}

func runInstall(cmd *cobra.Command, ref string, target string, outputDir string) error {
	if target == "" && outputDir == "" {
		return fmt.Errorf("specify --target <agent> or -o <directory>")
	}
	if target != "" && outputDir != "" {
		return fmt.Errorf("use --target or -o, not both")
	}

	if target != "" {
		relPath, ok := agentTargets[strings.ToLower(target)]
		if !ok {
			var names []string
			for k := range agentTargets {
				names = append(names, k)
			}
			return fmt.Errorf("unknown target %q (supported: %s)", target, strings.Join(names, ", "))
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("finding home directory: %w", err)
		}
		outputDir = filepath.Join(home, relPath)
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("creating directory %s: %w", outputDir, err)
	}

	client, err := defaultClient()
	if err != nil {
		return err
	}

	if err := client.Unpack(context.Background(), ref, outputDir); err != nil {
		return fmt.Errorf("installing %s: %w", ref, err)
	}

	// Extract skill name for the output message
	skillName := ref
	if idx := strings.LastIndex(ref, "/"); idx >= 0 {
		skillName = ref[idx+1:]
	}
	if idx := strings.LastIndex(skillName, ":"); idx >= 0 {
		skillName = skillName[:idx]
	}

	dest := filepath.Join(outputDir, skillName)
	fmt.Fprintf(cmd.OutOrStdout(), "Installed %s to %s\n", ref, dest)
	return nil
}
