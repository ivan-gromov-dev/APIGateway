// Package logger creates structured application loggers from runtime configuration.
package logger

import (
	"io"
	"log/slog"
	"os"

	"github.com/Djunichi/APIGateway/internal/config"
)

// New creates a logger that writes to standard output.
func New(cfg config.Log) *slog.Logger {
	return NewWithWriter(cfg, os.Stdout)
}

// NewWithWriter creates a logger that writes to the provided destination.
// It is useful for embedding the gateway and for deterministic tests.
func NewWithWriter(cfg config.Log, writer io.Writer) *slog.Logger {
	var level slog.Level
	if err := level.UnmarshalText([]byte(cfg.Level)); err != nil {
		level = slog.LevelInfo
	}

	options := &slog.HandlerOptions{Level: level}
	if cfg.Format == "text" {
		return slog.New(slog.NewTextHandler(writer, options))
	}
	return slog.New(slog.NewJSONHandler(writer, options))
}
