// Package balancer distributes requests across upstream service instances.
package balancer

import "net/url"

// Balancer selects an upstream for the next request.
type Balancer interface {
	Next() *url.URL
}

// Factory creates an independent balancer for a route.
type Factory func(upstreams []*url.URL) (Balancer, error)
