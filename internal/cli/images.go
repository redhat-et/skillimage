package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

func newImagesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "images",
		Short: "List skill images in local store",
		Args:  cobra.NoArgs,
		RunE:  runImages,
	}
}

func runImages(cmd *cobra.Command, args []string) error {
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

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
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
