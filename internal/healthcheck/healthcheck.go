package healthcheck

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/Djunichi/APIGateway/internal/config"
	"github.com/Djunichi/APIGateway/internal/upstream"
)

type Checker struct {
	targets []*upstream.Target
	cfg     config.ActiveHealthCheck
	client  *http.Client
	mu      sync.Mutex
	streak  map[*upstream.Target][2]int
}

func New(targets []*upstream.Target, cfg config.ActiveHealthCheck, client *http.Client) *Checker {
	if client == nil {
		client = &http.Client{}
	}
	return &Checker{targets: append([]*upstream.Target(nil), targets...), cfg: cfg, client: client, streak: make(map[*upstream.Target][2]int)}
}

func (c *Checker) Run(ctx context.Context) {
	if !c.cfg.Enabled || len(c.targets) == 0 {
		return
	}
	c.checkAll(ctx)
	t := time.NewTicker(c.cfg.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.checkAll(ctx)
		}
	}
}

func (c *Checker) checkAll(ctx context.Context) {
	for _, target := range c.targets {
		probeCtx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
		u := target.URL()
		u.Path = c.cfg.Path
		u.RawQuery = ""
		req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, u.String(), nil)
		if err == nil {
			resp, callErr := c.client.Do(req)
			if callErr == nil {
				resp.Body.Close()
				err = statusErr(resp.StatusCode)
			}
		}
		cancel()
		c.record(target, err == nil)
	}
}

func statusErr(status int) error {
	if status >= 200 && status < 400 {
		return nil
	}
	return fmt.Errorf("health check returned status %d", status)
}

func (c *Checker) record(target *upstream.Target, healthy bool) {
	c.mu.Lock()
	s := c.streak[target]
	if healthy {
		s[0]++
		s[1] = 0
	} else {
		s[1]++
		s[0] = 0
	}
	c.streak[target] = s
	c.mu.Unlock()
	if healthy && s[0] >= c.cfg.HealthyThreshold {
		target.SetHealth(true, time.Now())
	}
	if !healthy && s[1] >= c.cfg.UnhealthyThreshold {
		target.SetHealth(false, time.Now())
	}
}
