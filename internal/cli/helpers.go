package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/redhat-et/skillimage/pkg/oci"
)

// resolveTargetDirs resolves agent target and/or output directory
// flags into a map of target names to absolute paths. If both are
// empty and allTargets is true, returns all known agent targets.
func resolveTargetDirs(target, outputDir string, allTargets bool) (map[string]string, error) {
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

	if !allTargets {
		return nil, fmt.Errorf("specify --target <agent> or -o <directory>")
	}

	targets := make(map[string]string, len(agentTargets))
	for name, relPath := range agentTargets {
		targets[name] = filepath.Join(home, relPath)
	}
	return targets, nil
}

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
