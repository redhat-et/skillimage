package cli

import (
	"github.com/spf13/cobra"
)

var (
	flagFormat  string
	flagVerbose bool
)

func NewRootCmd(version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skillctl",
		Short: "Manage AI agent skills as OCI images",
		Long:  "skillctl packs, pushes, pulls, and manages the lifecycle of AI agent skills stored as OCI images.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.PersistentFlags().StringVar(&flagFormat, "format", "text", "output format (text, json)")
	cmd.PersistentFlags().BoolVarP(&flagVerbose, "verbose", "v", false, "verbose output")

	cmd.AddCommand(newValidateCmd())
	cmd.AddCommand(newPackCmd())
	cmd.AddCommand(newImagesCmd())

	return cmd
}
