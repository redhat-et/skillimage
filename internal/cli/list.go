package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/redhat-et/skillimage/pkg/installed"
)

func newListCmd() *cobra.Command {
	var showInstalled bool
	var target string
	var outputDir string

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List skill images in local store",
		Long: `List skill images stored locally, or show installed skills
across agent directories.

Without --installed, lists images in the local OCI store.
With --installed, scans agent skill directories for installed skills.

Supported targets for --installed:
  claude    ~/.claude/skills/
  cursor    ~/.cursor/skills/
  windsurf  ~/.codeium/windsurf/skills/
  opencode  ~/.config/opencode/skills/
  openclaw  ~/.openclaw/skills/`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if showInstalled {
				return runListInstalled(cmd, target, outputDir)
			}
			return runList(cmd)
		},
	}

	cmd.Flags().BoolVarP(&showInstalled, "installed", "i", false, "list installed skills")
	cmd.Flags().StringVarP(&target, "target", "t", "", "filter to a specific agent target")
	cmd.Flags().StringVarP(&outputDir, "output", "o", "", "scan a custom directory")

	return cmd
}

func runList(cmd *cobra.Command) error {
	client, err := defaultClient()
	if err != nil {
		return err
	}

	images, err := client.ListLocal()
	if err != nil {
		return fmt.Errorf("listing images: %w", err)
	}

	if len(images) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No images found in local store.")
		return nil
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tTAG\tSTATUS\tDIGEST\tCREATED")
	for _, img := range images {
		shortDigest := img.Digest
		if len(shortDigest) > 19 {
			shortDigest = shortDigest[:19]
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			img.Name, img.Tag, img.Status, shortDigest, img.Created)
	}
	return w.Flush()
}

func runListInstalled(cmd *cobra.Command, target, outputDir string) error {
	if target != "" && outputDir != "" {
		return fmt.Errorf("use --target or -o, not both")
	}

	targets, err := resolveListTargets(target, outputDir)
	if err != nil {
		return err
	}

	skills, err := installed.Scan(targets)
	if err != nil {
		return fmt.Errorf("scanning installed skills: %w", err)
	}

	if len(skills) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No installed skills found.")
		return nil
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tVERSION\tSOURCE\tTARGET")
	for _, s := range skills {
		source := s.Source
		if source == "" {
			source = "(local)"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", s.Name, s.Version, source, s.Target)
	}
	return w.Flush()
}

func resolveListTargets(target, outputDir string) (map[string]string, error) {
	if outputDir != "" {
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

	if target != "" {
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

	targets := make(map[string]string, len(agentTargets))
	for name, relPath := range agentTargets {
		targets[name] = filepath.Join(home, relPath)
	}
	return targets, nil
}
