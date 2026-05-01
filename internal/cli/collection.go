package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/redhat-et/skillimage/pkg/collection"
	"github.com/redhat-et/skillimage/pkg/oci"
	"github.com/redhat-et/skillimage/pkg/skillcard"
	"github.com/redhat-et/skillimage/pkg/source"
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
	cmd.AddCommand(newCollectionGenerateCmd())
	cmd.AddCommand(newCollectionInstallCmd())
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

	col, err := client.PullCollection(context.Background(), ref, oci.PullOptions{
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
	return nil, fmt.Errorf("registry references not supported; use -f <file>")
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

func newCollectionGenerateCmd() *cobra.Command {
	var file string
	var mountRoot string
	cmd := &cobra.Command{
		Use:   "generate [-f <file> | <ref>]",
		Short: "Generate Kubernetes volume YAML from a collection",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			col, err := resolveCollection(file, args)
			if err != nil {
				return err
			}
			collection.GenerateKubeYAML(cmd.OutOrStdout(), col, mountRoot)
			return nil
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "path to collection YAML file")
	cmd.Flags().StringVar(&mountRoot, "mount-root", "/skills", "root mount path for volumes")
	return cmd
}

func newCollectionInstallCmd() *cobra.Command {
	var file string
	var target string
	var outputDir string
	var force bool
	var ref string
	cmd := &cobra.Command{
		Use:   "install [-f <file> | <git-url>]",
		Short: "Install skills from a collection into an agent's skill directory",
		Long: `Install skills defined in a collection YAML into a directory
where an agent can find them. Skills can be referenced by OCI
image (image:) or Git source URL (source:).

Source entries clone the repo, build locally, and install.
Image entries pull from the registry and install.

Skills that haven't changed since last install are skipped
unless --force is set.

Examples:
  skillctl collection install -f ./collection.yaml --target claude
  skillctl collection install https://github.com/myorg/skills/tree/main/collection.yaml -t claude
  skillctl collection install -f ./collection.yaml -o ~/my-skills --force`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCollectionInstall(cmd, file, args, target, outputDir, force, ref)
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "path to local collection YAML file")
	cmd.Flags().StringVarP(&target, "target", "t", "", "agent name (claude, cursor, windsurf, opencode, openclaw)")
	cmd.Flags().StringVarP(&outputDir, "output", "o", "", "custom output directory")
	cmd.Flags().BoolVar(&force, "force", false, "reinstall even if skills are up to date")
	cmd.Flags().StringVar(&ref, "ref", "", "Git ref override (for collection YAML URL)")
	return cmd
}

func runCollectionInstall(cmd *cobra.Command, file string, args []string, target, outputDir string, force bool, ref string) error {
	col, cleanup, err := resolveCollectionInput(cmd.Context(), file, args, ref)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return err
	}

	if errs := collection.Validate(col); len(errs) > 0 {
		return fmt.Errorf("invalid collection:\n  %s", strings.Join(errs, "\n  "))
	}

	dirs, err := resolveTargetDirs(target, outputDir, false)
	if err != nil {
		return err
	}
	var destDir string
	for _, d := range dirs {
		destDir = d
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("creating directory %s: %w", destDir, err)
	}

	client, err := defaultClient()
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	w := cmd.OutOrStdout()
	errW := cmd.ErrOrStderr()

	fmt.Fprintf(w, "Installing collection %q (%d skills)\n", col.Metadata.Name, len(col.Skills))

	var installed, skipped, failed int
	for _, s := range col.Skills {
		switch {
		case s.Source != "":
			result, installErr := installFromSource(ctx, client, s, destDir, force, w)
			switch {
			case installErr != nil:
				fmt.Fprintf(errW, "  %s (source)  error: %v\n", skillLabel(s), installErr)
				failed++
			case result == "skipped":
				skipped++
			default:
				installed++
			}
		case s.Image != "":
			result, installErr := installFromImage(ctx, client, s, destDir, force, w)
			switch {
			case installErr != nil:
				fmt.Fprintf(errW, "  %s (image)  error: %v\n", s.Name, installErr)
				failed++
			case result == "skipped":
				skipped++
			default:
				installed++
			}
		}
	}

	fmt.Fprintf(w, "Installed %d skills", installed)
	if skipped > 0 {
		fmt.Fprintf(w, ", %d up to date", skipped)
	}
	fmt.Fprintln(w)

	if failed > 0 {
		return fmt.Errorf("%d skill(s) failed to install", failed)
	}
	return nil
}

func resolveCollectionInput(ctx context.Context, file string, args []string, ref string) (*collection.SkillCollection, func(), error) {
	if file != "" && len(args) > 0 {
		return nil, nil, fmt.Errorf("specify -f <file> or a Git URL, not both")
	}
	if file != "" {
		col, err := collection.ParseFile(file)
		return col, nil, err
	}
	if len(args) == 0 {
		return nil, nil, fmt.Errorf("specify -f <file> or a Git URL")
	}

	rawURL := args[0]
	if !source.IsRemote(rawURL) {
		return nil, nil, fmt.Errorf("not a valid URL: %s\n\nUse -f for local files", rawURL)
	}

	src, err := source.ParseGitURL(rawURL)
	if err != nil {
		return nil, nil, err
	}

	cloneResult, err := source.Clone(ctx, src, source.CloneOptions{RefOverride: ref})
	if err != nil {
		return nil, nil, fmt.Errorf("cloning collection: %w", err)
	}

	yamlPath := cloneResult.Dir
	info, statErr := os.Stat(yamlPath)
	if statErr != nil {
		cloneResult.Cleanup()
		return nil, nil, fmt.Errorf("collection file not found at %s in repository", src.SubPath)
	}
	if info.IsDir() {
		cloneResult.Cleanup()
		return nil, nil, fmt.Errorf("URL must point to a collection YAML file, not a directory: %s", src.SubPath)
	}

	col, err := collection.ParseFile(yamlPath)
	if err != nil {
		cloneResult.Cleanup()
		return nil, nil, err
	}

	return col, cloneResult.Cleanup, nil
}

func installFromSource(ctx context.Context, client *oci.Client, s collection.SkillRef, destDir string, force bool, w io.Writer) (string, error) {
	label := skillLabel(s)

	src, err := source.ParseGitURL(s.Source)
	if err != nil {
		return "", err
	}

	if !force {
		lookupName := label
		if lookupName == "" {
			lookupName = filepath.Base(src.SubPath)
		}
		if lookupName != "" && lookupName != "." {
			installedSHA := readInstalledCommit(destDir, lookupName)
			if installedSHA != "" {
				refToCheck := src.Ref
				if refToCheck == "" {
					refToCheck = "HEAD"
				}
				remoteSHA, lsErr := source.LsRemote(ctx, src.CloneURL, refToCheck)
				if lsErr == nil && remoteSHA == installedSHA {
					fmt.Fprintf(w, "  %s (source)  up to date\n", lookupName)
					return "skipped", nil
				}
			}
		}
	}

	fmt.Fprintf(w, "  %s (source)  cloning...", label)

	result, err := source.Resolve(ctx, s.Source, "", "")
	if err != nil {
		fmt.Fprintln(w)
		return "", err
	}
	defer result.Cleanup()

	if len(result.Skills) == 0 {
		fmt.Fprintln(w)
		return "", fmt.Errorf("no skills found at %s", s.Source)
	}

	skill := result.Skills[0]

	fmt.Fprintf(w, "  building...")

	if _, err := client.Build(ctx, skill.Dir, oci.BuildOptions{SkillCard: skill.SkillCard}); err != nil {
		fmt.Fprintln(w)
		return "", fmt.Errorf("building: %w", err)
	}

	buildRef := fmt.Sprintf("%s/%s:%s", skill.SkillCard.Metadata.Namespace, skill.SkillCard.Metadata.Name, skill.SkillCard.Metadata.Version)
	if err := client.Unpack(ctx, buildRef, destDir); err != nil {
		fmt.Fprintln(w)
		return "", fmt.Errorf("unpacking: %w", err)
	}

	skillDir := filepath.Join(destDir, skill.Name)
	writeSourceProvenance(skillDir, skill.SkillCard)

	fmt.Fprintf(w, "  installed\n")
	return "installed", nil
}

func installFromImage(ctx context.Context, client *oci.Client, s collection.SkillRef, destDir string, force bool, w io.Writer) (string, error) {
	if !looksLocal(s.Image) {
		fmt.Fprintf(w, "  %s (image)  pulling...", s.Name)
		desc, err := client.Pull(ctx, s.Image, oci.PullOptions{})
		if err != nil {
			fmt.Fprintln(w)
			return "", fmt.Errorf("pulling %s: %w", s.Image, err)
		}

		if !force {
			installedDigest := readInstalledCommit(destDir, s.Name)
			if installedDigest == desc.Digest.String() {
				fmt.Fprintf(w, "  up to date\n")
				return "skipped", nil
			}
		}
	} else {
		fmt.Fprintf(w, "  %s (image)  installing...", s.Name)
	}

	if err := client.Unpack(ctx, s.Image, destDir); err != nil {
		fmt.Fprintln(w)
		return "", fmt.Errorf("unpacking %s: %w", s.Image, err)
	}

	skillDir := filepath.Join(destDir, oci.SkillNameFromRef(s.Image))
	if err := WriteProvenance(ctx, client, s.Image, skillDir); err != nil {
		fmt.Fprintf(w, "  warning: provenance write failed: %v\n", err)
	}

	fmt.Fprintf(w, "  installed\n")
	return "installed", nil
}

func skillLabel(s collection.SkillRef) string {
	if s.Name != "" {
		return s.Name
	}
	return ""
}

func readInstalledCommit(destDir, skillName string) string {
	skillPath := filepath.Join(destDir, skillName, "skill.yaml")
	f, err := os.Open(skillPath)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()

	sc, err := skillcard.Parse(f)
	if err != nil {
		return ""
	}
	if sc.Provenance == nil {
		return ""
	}
	return sc.Provenance.Commit
}

func writeSourceProvenance(skillDir string, sc *skillcard.SkillCard) {
	if sc.Provenance == nil {
		sc.Provenance = &skillcard.Provenance{}
	}

	skillPath := filepath.Join(skillDir, "skill.yaml")
	wf, err := os.Create(skillPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  warning: could not write provenance: %v\n", err)
		return
	}
	defer func() { _ = wf.Close() }()
	if err := skillcard.Serialize(sc, wf); err != nil {
		fmt.Fprintf(os.Stderr, "  warning: could not serialize provenance: %v\n", err)
	}
}
