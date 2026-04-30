package source

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var commitSHAPattern = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

type CloneOptions struct {
	RefOverride string
}

type CloneResult struct {
	Dir       string
	CommitSHA string
	Cleanup   func()
}

func CheckGit() error {
	_, err := exec.LookPath("git")
	if err != nil {
		return fmt.Errorf("git is required for remote sources — install it from https://git-scm.com")
	}
	return nil
}

func Clone(ctx context.Context, src GitSource, opts CloneOptions) (*CloneResult, error) {
	if err := CheckGit(); err != nil {
		return nil, err
	}

	tmpDir, err := os.MkdirTemp("", "skillctl-clone-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp directory: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(tmpDir) }

	ref := opts.RefOverride
	if ref == "" {
		ref = src.Ref
	}

	if err := cloneRepo(ctx, src.CloneURL, ref, src.SubPath, tmpDir); err != nil {
		cleanup()
		return nil, err
	}

	resolvedDir := tmpDir
	if src.SubPath != "" {
		resolvedDir = filepath.Join(tmpDir, src.SubPath)
		if _, err := os.Stat(resolvedDir); err != nil {
			cleanup()
			return nil, fmt.Errorf("path %q not found in repository %s", src.SubPath, src.CloneURL)
		}
	}

	sha, err := commitSHA(ctx, tmpDir)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("reading commit SHA: %w", err)
	}

	return &CloneResult{Dir: resolvedDir, CommitSHA: sha, Cleanup: cleanup}, nil
}

func isCommitSHA(ref string) bool {
	return commitSHAPattern.MatchString(ref)
}

func cloneRepo(ctx context.Context, cloneURL, ref, subPath, destDir string) error {
	if isCommitSHA(ref) {
		return cloneAtCommit(ctx, cloneURL, ref, destDir)
	}

	if subPath != "" {
		if err := sparseClone(ctx, cloneURL, ref, subPath, destDir); err == nil {
			return nil
		}
		// Sparse clone may have partially populated destDir; clean it
		// before falling back to a full shallow clone.
		entries, _ := os.ReadDir(destDir)
		for _, e := range entries {
			_ = os.RemoveAll(filepath.Join(destDir, e.Name()))
		}
	}

	return shallowClone(ctx, cloneURL, ref, destDir)
}

func cloneAtCommit(ctx context.Context, cloneURL, sha, destDir string) error {
	if err := runGit(ctx, "", "clone", "--no-checkout", cloneURL, destDir); err != nil {
		return err
	}
	return runGit(ctx, destDir, "checkout", sha)
}

func sparseClone(ctx context.Context, cloneURL, ref, subPath, destDir string) error {
	args := []string{"clone", "--depth", "1", "--filter=blob:none", "--sparse"}
	if ref != "" {
		args = append(args, "--branch", ref)
	}
	args = append(args, cloneURL, destDir)

	if err := runGit(ctx, "", args...); err != nil {
		return err
	}

	return runGit(ctx, destDir, "sparse-checkout", "set", subPath)
}

func shallowClone(ctx context.Context, cloneURL, ref, destDir string) error {
	args := []string{"clone", "--depth", "1"}
	if ref != "" {
		args = append(args, "--branch", ref)
	}
	args = append(args, cloneURL, destDir)

	return runGit(ctx, "", args...)
}

func commitSHA(ctx context.Context, repoDir string) (string, error) {
	var stdout bytes.Buffer
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = repoDir
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(stdout.String()), nil
}

func runGit(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s failed: %s", args[0], sanitizeGitOutput(stderr.String()))
	}
	return nil
}

func sanitizeGitOutput(s string) string {
	s = strings.TrimSpace(s)
	// Strip lines that may contain credentials in URLs.
	var clean []string
	for line := range strings.SplitSeq(s, "\n") {
		if strings.Contains(line, "@") && strings.Contains(line, "://") {
			continue
		}
		clean = append(clean, line)
	}
	if len(clean) == 0 {
		return "(git output redacted — may contain credentials)"
	}
	return strings.Join(clean, "\n")
}
