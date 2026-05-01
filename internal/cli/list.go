package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	humanize "github.com/dustin/go-humanize"
	"github.com/spf13/cobra"

	"github.com/redhat-et/skillimage/pkg/installed"
	"github.com/redhat-et/skillimage/pkg/oci"
)

func newListCmd() *cobra.Command {
	var showInstalled bool
	var target string
	var outputDir string
	var upgradable bool

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
			if upgradable && !showInstalled {
				return fmt.Errorf("--upgradable requires --installed")
			}
			if showInstalled {
				return runListInstalled(cmd, target, outputDir, upgradable)
			}
			return runList(cmd)
		},
	}

	cmd.Flags().BoolVarP(&showInstalled, "installed", "i", false, "list installed skills")
	cmd.Flags().StringVarP(&target, "target", "t", "", "filter to a specific agent target")
	cmd.Flags().StringVarP(&outputDir, "output", "o", "", "scan a custom directory")
	cmd.Flags().BoolVarP(&upgradable, "upgradable", "u", false, "show only upgradable skills (requires --installed)")

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

func runListInstalled(cmd *cobra.Command, target, outputDir string, upgradable bool) error {
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

	if !upgradable {
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

	candidates, err := installed.CheckUpgrades(cmd.Context(), skills,
		installed.CheckOptions{
			TagLister: oci.ListTagsForRepo,
		})
	if err != nil {
		return fmt.Errorf("checking upgrades: %w", err)
	}

	if len(candidates) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "All installed skills are up to date.")
		return nil
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tVERSION\tLATEST\tSOURCE\tTARGET")
	for _, c := range candidates {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			c.Installed.Name, c.Installed.Version, c.LatestVersion,
			c.LatestRef, c.Installed.Target)
	}
	return w.Flush()
}

func resolveListTargets(target, outputDir string) (map[string]string, error) {
	return resolveTargetDirs(target, outputDir, true)
}

func formatDigest(digest string, noTrunc bool) string {
	if digest == "" {
		return ""
	}
	if noTrunc {
		return digest
	}
	if idx := strings.IndexByte(digest, ':'); idx >= 0 {
		digest = digest[idx+1:]
	}
	if len(digest) > 12 {
		digest = digest[:12]
	}
	return digest
}

func formatCreated(created string, noTrunc bool) string {
	if created == "" {
		return ""
	}
	if noTrunc {
		return created
	}
	t, err := time.Parse(time.RFC3339, created)
	if err != nil {
		return created
	}
	return humanize.Time(t)
}
