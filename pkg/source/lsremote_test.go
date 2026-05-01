package source

import (
	"context"
	"testing"
	"time"
)

func TestLsRemoteReturnsCommitSHA(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network test in short mode")
	}
	if err := CheckGit(); err != nil {
		t.Skip("git not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sha, err := LsRemote(ctx, "https://github.com/octocat/Hello-World.git", "master")
	if err != nil {
		t.Fatalf("LsRemote: %v", err)
	}
	if len(sha) < 7 {
		t.Errorf("expected commit SHA, got %q", sha)
	}
	if !commitSHAPattern.MatchString(sha) {
		t.Errorf("SHA %q does not match commit pattern", sha)
	}
}

func TestLsRemoteBadRef(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network test in short mode")
	}
	if err := CheckGit(); err != nil {
		t.Skip("git not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := LsRemote(ctx, "https://github.com/octocat/Hello-World.git", "nonexistent-branch-xyz")
	if err == nil {
		t.Fatal("expected error for nonexistent ref")
	}
}
