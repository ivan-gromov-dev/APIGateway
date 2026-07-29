// Command gateway starts the API gateway and manages its process lifecycle.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Djunichi/APIGateway/internal/config"
	"github.com/Djunichi/APIGateway/internal/logger"
	"github.com/Djunichi/APIGateway/internal/server"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "configs/gateway.yaml", "path to the YAML configuration")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	log := logger.New(cfg.Log)
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Info("starting gateway",
		"address", cfg.Server.Address,
		"admin_address", cfg.Admin.Address,
		"routes", len(cfg.Routes),
	)

	if err := server.New(cfg, log).Run(ctx); err != nil {
		return fmt.Errorf("run gateway: %w", err)
	}

	log.Info("gateway stopped")
	return nil
}
