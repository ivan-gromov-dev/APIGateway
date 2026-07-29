package logger

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Djunichi/APIGateway/internal/config"
)

func TestNewWithWriter(t *testing.T) {
	var output bytes.Buffer
	log := NewWithWriter(config.Log{Level: "debug", Format: "text"}, &output)

	log.Debug("configured", "component", "test")

	if text := output.String(); !strings.Contains(text, "level=DEBUG") || !strings.Contains(text, "component=test") {
		t.Fatalf("unexpected log output: %q", text)
	}
}

func TestNewWithWriterFallsBackToInfoLevel(t *testing.T) {
	var output bytes.Buffer
	log := NewWithWriter(config.Log{Level: "invalid", Format: "json"}, &output)

	log.Debug("hidden")
	log.Info("visible")

	if text := output.String(); strings.Contains(text, "hidden") || !strings.Contains(text, `"msg":"visible"`) {
		t.Fatalf("unexpected log output: %q", text)
	}
}
