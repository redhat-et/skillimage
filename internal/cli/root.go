package cli

import (
	"github.com/spf13/cobra"
)

func NewRootCmd(version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "skillctl",
		Short:   "Manage AI agent skills as OCI images",
		Long:    "skillctl packs, pushes, pulls, and manages the lifecycle of AI agent skills stored as OCI images.",
		Version: version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.AddCommand(newValidateCmd())
	cmd.AddCommand(newPackCmd())
	cmd.AddCommand(newPushCmd())
	cmd.AddCommand(newPullCmd())
	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newInspectCmd())
	cmd.AddCommand(newPromoteCmd())
	cmd.AddCommand(newInstallCmd())
	cmd.AddCommand(newPruneCmd())

	return cmd
}
