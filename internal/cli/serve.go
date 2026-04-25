package cli

import (
	"context"
	"fmt"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/redhat-et/skillimage/internal/server"
)

func newServeCmd() *cobra.Command {
	var (
		port         int
		dbPath       string
		registryURL  string
		namespace    string
		repositories string
		syncInterval string
		tlsVerify    bool
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the skill catalog server",
		Long: `Start an HTTP server that indexes skills from an OCI registry
and serves them via a REST API.

The server syncs skill metadata from the configured registry into
a local SQLite database and serves it for fast listing, filtering,
and search.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			interval, err := time.ParseDuration(syncInterval)
			if err != nil {
				return fmt.Errorf("invalid sync interval: %w", err)
			}

			if registryURL == "" {
				return fmt.Errorf("--registry is required")
			}

			ctx, cancel := signal.NotifyContext(
				context.Background(), syscall.SIGINT, syscall.SIGTERM,
			)
			defer cancel()

			var repos []string
			if repositories != "" {
				repos = strings.Split(repositories, ",")
			}

			return server.Run(ctx, server.Config{
				Port:          port,
				DBPath:        dbPath,
				RegistryURL:   registryURL,
				Namespace:     namespace,
				Repositories:  repos,
				SkipTLSVerify: !tlsVerify,
				SyncInterval:  interval,
			})
		},
	}

	cmd.Flags().IntVar(&port, "port", 8080, "HTTP listen port")
	cmd.Flags().StringVar(&dbPath, "db", "skillctl.db", "SQLite database path")
	cmd.Flags().StringVar(&registryURL, "registry", "", "OCI registry URL (required)")
	cmd.Flags().StringVar(&namespace, "namespace", "", "limit sync to a namespace prefix (requires /v2/_catalog support)")
	cmd.Flags().StringVar(&repositories, "repositories", "", "comma-separated list of repository names to sync (bypasses /v2/_catalog discovery)")
	cmd.Flags().StringVar(&syncInterval, "sync-interval", "60s", "background sync interval")
	cmd.Flags().BoolVar(&tlsVerify, "tls-verify", true, "require HTTPS and verify certificates")

	return cmd
}
