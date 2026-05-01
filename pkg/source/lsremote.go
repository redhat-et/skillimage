package source

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// LsRemote queries a remote Git repository for the commit SHA
// of the given ref without cloning. Returns the full commit SHA.
func LsRemote(ctx context.Context, cloneURL, ref string) (string, error) {
	if err := CheckGit(); err != nil {
		return "", err
	}

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "git", "ls-remote", cloneURL, ref)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git ls-remote %s %s: %s", cloneURL, ref, sanitizeGitOutput(stderr.String()))
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return "", fmt.Errorf("ref %q not found in %s", ref, cloneURL)
	}

	// Output format: "<sha>\t<refname>\n" — may have multiple lines.
	// Take the first line's SHA.
	firstLine := strings.SplitN(output, "\n", 2)[0]
	fields := strings.Fields(firstLine)
	if len(fields) < 1 {
		return "", fmt.Errorf("unexpected ls-remote output for %s %s", cloneURL, ref)
	}

	return fields[0], nil
}
