package cli

import (
	"fmt"
	"os"

	"github.com/redhat-et/skillimage/pkg/oci"
)

func defaultClient() (*oci.Client, error) {
	storeDir, err := defaultStoreDir()
	if err != nil {
		return nil, err
	}
	return oci.NewClient(storeDir)
}

func defaultStoreDir() (string, error) {
	dataDir := os.Getenv("XDG_DATA_HOME")
	if dataDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("finding home directory: %w", err)
		}
		dataDir = home + "/.local/share"
	}
	dir := dataDir + "/skillctl/store"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating store directory: %w", err)
	}
	return dir, nil
}
