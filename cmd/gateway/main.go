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

	"github.com/ivan-gromov-dev/APIGateway/internal/config"
	"github.com/ivan-gromov-dev/APIGateway/internal/configwatcher"
	"github.com/ivan-gromov-dev/APIGateway/internal/logger"
	"github.com/ivan-gromov-dev/APIGateway/internal/server"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "configs/gateway.yaml", "path to the YAML configuration")
	reloadInterval := flag.Duration("reload-interval", 0, "configuration polling interval override; 0 uses config")
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

	gateway := server.New(cfg, log)
	interval := cfg.Reload.Interval
	if *reloadInterval > 0 {
		interval = *reloadInterval
	}
	if interval > 0 {
		go func() {
			if err := configwatcher.Watch(ctx, *configPath, interval, gateway.Reload); err != nil {
				log.Error("configuration watcher stopped", "error", err)
			}
		}()
	}
	if err := gateway.Run(ctx); err != nil {
		return fmt.Errorf("run gateway: %w", err)
	}

	log.Info("gateway stopped")
	return nil
}
