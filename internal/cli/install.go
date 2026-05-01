package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/redhat-et/skillimage/pkg/oci"
	"github.com/redhat-et/skillimage/pkg/skillcard"
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

	ctx := cmd.Context()

	// If the ref looks remote, pull it first if not already in the local store.
	if !looksLocal(ref) {
		if _, err := client.ResolveDigest(ctx, ref); err != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "Pulling %s...\n", ref)
			if _, pullErr := client.Pull(ctx, ref, oci.PullOptions{}); pullErr != nil {
				return fmt.Errorf("pulling %s: %w", ref, pullErr)
			}
		}
	}

	if err := client.Unpack(ctx, ref, outputDir); err != nil {
		return fmt.Errorf("installing %s: %w", ref, err)
	}

	dest := filepath.Join(outputDir, oci.SkillNameFromRef(ref))

	// Write provenance into skill.yaml.
	if err := WriteProvenance(ctx, client, ref, dest); err != nil {
		return fmt.Errorf("writing provenance: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Installed %s to %s\n", ref, dest)
	return nil
}

func WriteProvenance(ctx context.Context, client *oci.Client, ref, skillDir string) error {
	digest, err := client.ResolveDigest(ctx, ref)
	if err != nil {
		return err
	}

	skillPath := filepath.Join(skillDir, "skill.yaml")
	var sc *skillcard.SkillCard

	f, err := os.Open(skillPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("opening skill.yaml: %w", err)
		}
		sc = NewSkillCardFromRef(ref)
	} else {
		sc, err = skillcard.Parse(f)
		_ = f.Close()
		if err != nil {
			return fmt.Errorf("parsing skill.yaml: %w", err)
		}
	}

	if sc.Provenance == nil {
		sc.Provenance = &skillcard.Provenance{}
	}
	sc.Provenance.Source = ref
	sc.Provenance.Commit = digest

	// Update version from the ref tag so it stays in sync with
	// what was actually installed (the tag is authoritative).
	if tag := tagFromRef(ref); tag != "" {
		sc.Metadata.Version = tag
	}

	wf, err := os.Create(skillPath)
	if err != nil {
		return fmt.Errorf("creating skill.yaml: %w", err)
	}
	defer func() { _ = wf.Close() }()
	return skillcard.Serialize(sc, wf)
}

// tagFromRef extracts the tag portion from a ref like
// "quay.io/acme/skill:1.0.0". Returns empty string if no tag.
func tagFromRef(ref string) string {
	if strings.Contains(ref, "@") {
		return ""
	}
	lastSlash := strings.LastIndex(ref, "/")
	if lastSlash < 0 {
		if idx := strings.LastIndex(ref, ":"); idx >= 0 {
			return ref[idx+1:]
		}
		return ""
	}
	tail := ref[lastSlash+1:]
	if idx := strings.LastIndex(tail, ":"); idx >= 0 {
		return tail[idx+1:]
	}
	return ""
}

func NewSkillCardFromRef(ref string) *skillcard.SkillCard {
	name := oci.SkillNameFromRef(ref)

	version := tagFromRef(ref)
	if version == "" {
		version = "unknown"
	}

	namespace := "unknown"
	if idx := strings.Index(ref, "/"); idx >= 0 {
		namespace = ref[:idx]
	}

	return &skillcard.SkillCard{
		APIVersion: "skillimage.io/v1alpha1",
		Kind:       "SkillCard",
		Metadata: skillcard.Metadata{
			Name:        name,
			Namespace:   namespace,
			Version:     version,
			Description: "Installed from " + ref,
		},
	}
}
