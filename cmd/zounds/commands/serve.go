package commands

import (
	"context"
	"fmt"

	"github.com/darkliquid/zounds/web"
	"github.com/spf13/cobra"
)

func newServeCommand(cfg *Config) *cobra.Command {
	var (
		addr string
		port int
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the web UI and API server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(cmd.Context(), cfg, addr, port)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&addr, "addr", "127.0.0.1", "address to bind the web server to")
	flags.IntVar(&port, "port", 8080, "port to bind the web server to")

	return cmd
}

func runServe(ctx context.Context, cfg *Config, addr string, port int) error {
	repo, closer, err := openRepository(ctx, cfg)
	if err != nil {
		return err
	}
	defer closer.Close()

	if port <= 0 {
		return fmt.Errorf("port must be greater than zero")
	}

	return web.ListenAndServe(ctx, fmt.Sprintf("%s:%d", addr, port), repo)
}
