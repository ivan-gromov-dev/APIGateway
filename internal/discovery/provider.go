package discovery

import (
	"context"
	"fmt"

	"github.com/Djunichi/APIGateway/internal/config"
)

// Provider resolves one configured discovery target into HTTP upstream URLs.
type Provider interface {
	Resolve(context.Context, config.Discovery) ([]string, error)
}

// Registry selects providers by the configured provider name.
type Registry map[string]Provider

func (r Registry) Resolve(ctx context.Context, cfg config.Discovery) ([]string, error) {
	provider := r[cfg.Provider]
	if provider == nil {
		return nil, fmt.Errorf("unsupported discovery provider %q", cfg.Provider)
	}
	return provider.Resolve(ctx, cfg)
}

// DefaultRegistry returns providers included in the gateway binary.
func DefaultRegistry() Registry { return Registry{"dns": DNS{}} }
