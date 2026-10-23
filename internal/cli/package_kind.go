package cli

import (
	"context"
	"fmt"

	"github.com/redhat-et/skillimage/pkg/oci"
)

func requireSkillImage(ctx context.Context, client *oci.Client, ref, command string) error {
	result, err := client.Inspect(ctx, ref)
	if err != nil {
		return fmt.Errorf("inspecting %s: %w", ref, err)
	}
	if result.Plugin != nil {
		return fmt.Errorf("%s does not support Agent Plugin image %s; use `skillctl pull -o <directory>` to unpack it", command, ref)
	}
	if result.ArtifactType != "" {
		return fmt.Errorf("%s requires a skill image; %s is OCI artifact %q", command, ref, result.ArtifactType)
	}
	return nil
}
