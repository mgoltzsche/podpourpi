package main

import (
	"context"

	"github.com/mgoltzsche/podpourpi/internal/server"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newServeCommand(ctx context.Context, logger *logrus.Entry) *cobra.Command {
	opts := server.Options{
		Address:    "127.0.0.1:8080",
		DockerHost: "unix:///var/run/docker.sock",
		UIDir:      "./ui",
		Logger:     logger,
	}
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "run the API and web UI server",
		Args:  cobra.ExactArgs(0),
		RunE: func(_ *cobra.Command, _ []string) error {
			return server.RunServer(ctx, opts)
		},
	}
	f := cmd.Flags()
	f.StringVar(&opts.Address, "address", opts.Address, "server listen address")
	f.StringVar(&opts.DockerHost, "docker-address", opts.DockerHost, "docker client address")
	f.StringVar(&opts.UIDir, "ui", opts.UIDir, "directory containing the web UI")
	return cmd
}
