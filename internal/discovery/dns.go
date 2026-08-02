// Package discovery contains concrete upstream discovery providers.
package discovery

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"

	"github.com/Djunichi/APIGateway/internal/config"
)

// DNS resolves a service name into HTTP targets.
type DNS struct {
	LookupHost func(context.Context, string) ([]string, error)
}

func (d DNS) Resolve(ctx context.Context, cfg config.Discovery) ([]string, error) {
	lookup := d.LookupHost
	if lookup == nil {
		lookup = net.DefaultResolver.LookupHost
	}
	hosts, err := lookup(ctx, cfg.Name)
	if err != nil {
		return nil, fmt.Errorf("resolve %q: %w", cfg.Name, err)
	}
	if len(hosts) == 0 {
		return nil, fmt.Errorf("resolve %q returned no addresses", cfg.Name)
	}
	result := make([]string, 0, len(hosts))
	for _, host := range hosts {
		u := url.URL{Scheme: cfg.Scheme, Host: net.JoinHostPort(host, strconv.Itoa(cfg.Port))}
		result = append(result, u.String())
	}
	return result, nil
}
