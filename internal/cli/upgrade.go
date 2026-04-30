package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/redhat-et/skillimage/pkg/installed"
	"github.com/redhat-et/skillimage/pkg/oci"
)

func newUpgradeCmd() *cobra.Command {
	var target string
	var outputDir string
	var all bool
	var tlsVerify bool

	cmd := &cobra.Command{
		Use:   "upgrade [skill-name]",
		Short: "Upgrade installed skills to latest published version",
		Long: `Upgrade one or all installed skills to their latest published
version from the source registry.

Requires --target or -o to locate installed skills.

Examples:
  skillctl upgrade red-hat-quick-deck --target opencode
  skillctl upgrade --all --target claude
  skillctl upgrade my-skill -o ~/custom/skills/`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			skillName := ""
			if len(args) == 1 {
				skillName = args[0]
			}
			return runUpgrade(cmd, skillName, target, outputDir, all, !tlsVerify)
		},
	}

	cmd.Flags().StringVarP(&target, "target", "t", "", "agent name (claude, cursor, windsurf, opencode, openclaw)")
	cmd.Flags().StringVarP(&outputDir, "output", "o", "", "custom skill directory")
	cmd.Flags().BoolVarP(&all, "all", "a", false, "upgrade all installed skills")
	cmd.Flags().BoolVar(&tlsVerify, "tls-verify", true, "require HTTPS and verify certificates")

	return cmd
}

func runUpgrade(cmd *cobra.Command, skillName, target, outputDir string, all, skipTLSVerify bool) error {
	if skillName != "" && all {
		return fmt.Errorf("cannot specify skill name with --all")
	}
	if skillName == "" && !all {
		return fmt.Errorf("specify a skill name or use --all")
	}
	if target == "" && outputDir == "" {
		return fmt.Errorf("specify --target <agent> or -o <directory>")
	}
	if target != "" && outputDir != "" {
		return fmt.Errorf("use --target or -o, not both")
	}

	targets, err := resolveUpgradeTarget(target, outputDir)
	if err != nil {
		return err
	}

	skills, err := installed.Scan(targets)
	if err != nil {
		return fmt.Errorf("scanning installed skills: %w", err)
	}

	if skillName != "" {
		found := false
		for i, s := range skills {
			if s.Name == skillName {
				skills = skills[i : i+1]
				found = true
				break
			}
		}
		if !found {
			targetLabel := target
			if targetLabel == "" {
				targetLabel = outputDir
			}
			return fmt.Errorf("skill not found: %s in target %s", skillName, targetLabel)
		}

		if skills[0].Source == "" {
			return fmt.Errorf("no source registry for %s (installed locally)", skillName)
		}
	}

	candidates, err := installed.CheckUpgrades(cmd.Context(), skills,
		installed.CheckOptions{
			SkipTLSVerify: skipTLSVerify,
			TagLister:     oci.ListTagsForRepo,
		})
	if err != nil {
		return fmt.Errorf("checking upgrades: %w", err)
	}

	if len(candidates) == 0 {
		if all {
			fmt.Fprintln(cmd.OutOrStdout(), "All skills are up to date.")
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "%s is already at the latest version.\n", skillName)
		}
		return nil
	}

	client, err := defaultClient()
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	var upgraded int
	for _, c := range candidates {
		if err := upgradeSkill(ctx, client, c, skipTLSVerify); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error upgrading %s: %v\n", c.Installed.Name, err)
			continue
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Upgraded %s %s → %s (%s)\n",
			c.Installed.Name, c.Installed.Version, c.LatestVersion, c.Installed.Target)
		upgraded++
	}

	if all && upgraded > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "\nUpgraded %d skill(s).\n", upgraded)
	}

	return nil
}

func upgradeSkill(ctx context.Context, client *oci.Client, c installed.UpgradeCandidate, skipTLSVerify bool) error {
	_, err := client.Pull(ctx, c.LatestRef, oci.PullOptions{
		SkipTLSVerify: skipTLSVerify,
	})
	if err != nil {
		return fmt.Errorf("pulling %s: %w", c.LatestRef, err)
	}

	parentDir := filepath.Dir(c.Installed.Path)
	if err := client.Unpack(ctx, c.LatestRef, parentDir); err != nil {
		return fmt.Errorf("unpacking %s: %w", c.LatestRef, err)
	}

	if err := writeProvenance(ctx, client, c.LatestRef, c.Installed.Path); err != nil {
		return fmt.Errorf("writing provenance: %w", err)
	}

	return nil
}

func resolveUpgradeTarget(target, outputDir string) (map[string]string, error) {
	if outputDir != "" {
		if strings.HasPrefix(outputDir, "~/") || outputDir == "~" {
			h, err := os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("finding home directory: %w", err)
			}
			outputDir = filepath.Join(h, outputDir[1:])
		}
		abs, err := filepath.Abs(outputDir)
		if err != nil {
			return nil, fmt.Errorf("resolving path: %w", err)
		}
		return map[string]string{outputDir: abs}, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("finding home directory: %w", err)
	}

	relPath, ok := agentTargets[strings.ToLower(target)]
	if !ok {
		var names []string
		for k := range agentTargets {
			names = append(names, k)
		}
		return nil, fmt.Errorf("unknown target %q (supported: %s)", target, strings.Join(names, ", "))
	}
	return map[string]string{target: filepath.Join(home, relPath)}, nil
}
