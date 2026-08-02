// Package configwatcher periodically detects configuration file changes.
package configwatcher

import (
	"context"
	"fmt"
	"os"
	"time"
)

// Watch polls path and invokes reload after a metadata change. The initial
// file metadata is recorded without invoking reload.
func Watch(ctx context.Context, path string, interval time.Duration, reload func(context.Context, string) error) error {
	if interval <= 0 {
		return fmt.Errorf("watch interval must be positive")
	}
	if reload == nil {
		return fmt.Errorf("reload callback is required")
	}
	previous, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat configuration: %w", err)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			current, err := os.Stat(path)
			if err != nil {
				continue
			}
			if current.ModTime().Equal(previous.ModTime()) && current.Size() == previous.Size() {
				continue
			}
			previous = current
			if err := reload(ctx, path); err != nil {
				continue
			}
		}
	}
}
