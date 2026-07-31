// Package auth verifies externally issued JWT access tokens.
package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var ErrInvalidToken = errors.New("invalid token")

type Principal struct {
	Subject string
	Scopes  map[string]struct{}
}

func (p Principal) HasScope(scope string) bool {
	_, ok := p.Scopes[scope]
	return ok
}

type Config struct {
	JWKSURL, Issuer, Audience string
	Algorithms                []string
	ClockSkew, HTTPTimeout    time.Duration
}

type Verifier struct {
	cfg             Config
	client          *http.Client
	mu              sync.RWMutex
	keys            map[string]*rsa.PublicKey
	refreshMu       sync.Mutex
	lastMissRefresh time.Time
}

func New(ctx context.Context, cfg Config) (*Verifier, error) {
	v := &Verifier{cfg: cfg, client: &http.Client{Timeout: cfg.HTTPTimeout}}
	if err := v.refresh(ctx); err != nil {
		return nil, fmt.Errorf("load JWKS: %w", err)
	}
	return v, nil
}

type claims struct {
	Scope string `json:"scope"`
	Scp   any    `json:"scp"`
	jwt.RegisteredClaims
}

func (v *Verifier) Verify(ctx context.Context, raw string) (Principal, error) {
	if len(raw) == 0 || len(raw) > 16<<10 {
		return Principal{}, ErrInvalidToken
	}
	parsed, err := jwt.ParseWithClaims(raw, &claims{}, func(token *jwt.Token) (any, error) {
		if !contains(v.cfg.Algorithms, token.Method.Alg()) {
			return nil, ErrInvalidToken
		}
		kid, ok := token.Header["kid"].(string)
		if !ok || kid == "" {
			return nil, ErrInvalidToken
		}
		key := v.key(kid)
		if key == nil {
			if err := v.refreshMissingKey(ctx); err != nil {
				return nil, ErrInvalidToken
			}
			key = v.key(kid)
		}
		if key == nil {
			return nil, ErrInvalidToken
		}
		return key, nil
	}, jwt.WithValidMethods(v.cfg.Algorithms), jwt.WithIssuer(v.cfg.Issuer),
		jwt.WithAudience(v.cfg.Audience), jwt.WithLeeway(v.cfg.ClockSkew),
		jwt.WithExpirationRequired())
	if err != nil || !parsed.Valid {
		return Principal{}, ErrInvalidToken
	}
	value, ok := parsed.Claims.(*claims)
	if !ok || value.Subject == "" {
		return Principal{}, ErrInvalidToken
	}
	scopes := make(map[string]struct{})
	for _, scope := range strings.Fields(value.Scope) {
		scopes[scope] = struct{}{}
	}
	switch scp := value.Scp.(type) {
	case string:
		for _, scope := range strings.Fields(scp) {
			scopes[scope] = struct{}{}
		}
	case []any:
		for _, item := range scp {
			if scope, ok := item.(string); ok {
				scopes[scope] = struct{}{}
			}
		}
	}
	return Principal{Subject: value.Subject, Scopes: scopes}, nil
}

func (v *Verifier) refreshMissingKey(ctx context.Context) error {
	v.refreshMu.Lock()
	defer v.refreshMu.Unlock()
	if time.Since(v.lastMissRefresh) < 30*time.Second {
		return nil
	}
	v.lastMissRefresh = time.Now()
	return v.refresh(ctx)
}

func (v *Verifier) key(kid string) *rsa.PublicKey {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.keys[kid]
}

func (v *Verifier) refresh(ctx context.Context) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, v.cfg.JWKSURL, nil)
	if err != nil {
		return err
	}
	response, err := v.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("JWKS returned %s", response.Status)
	}
	var document struct {
		Keys []struct{ Kty, Kid, Use, Alg, N, E string }
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&document); err != nil {
		return err
	}
	keys := make(map[string]*rsa.PublicKey)
	for _, item := range document.Keys {
		if item.Kty != "RSA" || item.Kid == "" || (item.Use != "" && item.Use != "sig") ||
			(item.Alg != "" && !contains(v.cfg.Algorithms, item.Alg)) {
			continue
		}
		n, err := base64.RawURLEncoding.DecodeString(item.N)
		if err != nil {
			continue
		}
		e, err := base64.RawURLEncoding.DecodeString(item.E)
		if err != nil || len(e) == 0 || len(e) > 4 {
			continue
		}
		exponent := 0
		for _, b := range e {
			exponent = exponent<<8 | int(b)
		}
		if exponent < 3 {
			continue
		}
		keys[item.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: exponent}
	}
	if len(keys) == 0 {
		return errors.New("JWKS contains no usable keys")
	}
	v.mu.Lock()
	v.keys = keys
	v.mu.Unlock()
	return nil
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
