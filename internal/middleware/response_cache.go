package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"hash/fnv"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/Djunichi/APIGateway/internal/cache"
	"github.com/Djunichi/APIGateway/internal/config"
)

func ResponseCache(store cache.Store, global config.Cache, route config.RouteCache, routePrefix string) Middleware {
	vary := append([]string(nil), route.VaryHeaders...)
	var flights [64]sync.Mutex
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !cacheableRequest(r) {
				next.ServeHTTP(w, r)
				return
			}
			key := cacheKey(r, routePrefix, vary)
			ctx, cancel := context.WithTimeout(r.Context(), global.OperationTimeout)
			entry, found, err := store.Get(ctx, key)
			cancel()
			if err != nil && global.OnBackendError == "deny" {
				writeCacheError(w)
				return
			}
			if found {
				writeCacheHit(w, r, entry)
				return
			}
			flight := &flights[cacheShard(key)]
			flight.Lock()
			defer flight.Unlock()
			ctx, cancel = context.WithTimeout(r.Context(), global.OperationTimeout)
			entry, found, err = store.Get(ctx, key)
			cancel()
			if err != nil && global.OnBackendError == "deny" {
				writeCacheError(w)
				return
			}
			if found {
				writeCacheHit(w, r, entry)
				return
			}
			recorder := &cacheWriter{ResponseWriter: w, status: http.StatusOK, limit: global.MaxBodyBytes}
			w.Header().Set("X-Cache", "MISS")
			next.ServeHTTP(recorder, r)
			if !recorder.overflow && cacheableResponse(recorder.status, w.Header(), vary) {
				entry := cache.Entry{Status: recorder.status, Header: cacheHeaders(w.Header()), Body: append([]byte(nil), recorder.body.Bytes()...)}
				ctx, cancel := context.WithTimeout(r.Context(), global.OperationTimeout)
				err := store.Set(ctx, key, entry, route.TTL)
				cancel()
				if err != nil && global.OnBackendError == "deny" {
					// The upstream response is already committed; failures are observable only on later requests.
					return
				}
			}
		})
	}
}

func cacheableRequest(r *http.Request) bool {
	return (r.Method == http.MethodGet || r.Method == http.MethodHead) &&
		r.Header.Get("Authorization") == "" && !strings.Contains(strings.ToLower(r.Header.Get("Cache-Control")), "no-store")
}

func cacheableResponse(status int, header http.Header, configuredVary []string) bool {
	control := strings.ToLower(header.Get("Cache-Control"))
	return status == http.StatusOK && header.Get("Set-Cookie") == "" && varyAllowed(header.Values("Vary"), configuredVary) &&
		!strings.Contains(control, "no-store") && !strings.Contains(control, "private") &&
		header.Get("Vary") != "*"
}

func varyAllowed(values, configured []string) bool {
	allowed := make(map[string]struct{}, len(configured))
	for _, name := range configured {
		allowed[http.CanonicalHeaderKey(name)] = struct{}{}
	}
	for _, value := range values {
		for _, name := range strings.Split(value, ",") {
			name = http.CanonicalHeaderKey(strings.TrimSpace(name))
			if name == "" {
				continue
			}
			if _, ok := allowed[name]; !ok {
				return false
			}
		}
	}
	return true
}

func cacheKey(r *http.Request, route string, vary []string) string {
	query := r.URL.Query().Encode()
	var b strings.Builder
	b.WriteString(route)
	b.WriteByte('|')
	b.WriteString(r.Method)
	b.WriteByte('|')
	b.WriteString(r.URL.EscapedPath())
	b.WriteByte('?')
	b.WriteString(query)
	headers := append([]string(nil), vary...)
	sort.Strings(headers)
	for _, name := range headers {
		b.WriteByte('|')
		b.WriteString(http.CanonicalHeaderKey(name))
		b.WriteByte('=')
		b.WriteString(r.Header.Get(name))
	}
	return b.String()
}

type cacheWriter struct {
	http.ResponseWriter
	status      int
	body        bytes.Buffer
	limit       int64
	overflow    bool
	wroteHeader bool
}

func (w *cacheWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *cacheWriter) Write(data []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if int64(w.body.Len()+len(data)) <= w.limit {
		_, _ = w.body.Write(data)
	} else {
		w.overflow = true
	}
	return w.ResponseWriter.Write(data)
}
func (w *cacheWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func cacheHeaders(source http.Header) http.Header {
	result := make(http.Header)
	for name, values := range source {
		if hopByHop(name) || strings.EqualFold(name, "Set-Cookie") || strings.EqualFold(name, "X-Cache") {
			continue
		}
		result[name] = append([]string(nil), values...)
	}
	return result
}

func hopByHop(name string) bool {
	switch strings.ToLower(name) {
	case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade":
		return true
	default:
		return false
	}
}

func copyHeader(destination, source http.Header) {
	for name := range destination {
		destination.Del(name)
	}
	for name, values := range source {
		destination[name] = append([]string(nil), values...)
	}
}

func writeCacheError(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": "cache unavailable"})
}

func writeCacheHit(w http.ResponseWriter, r *http.Request, entry cache.Entry) {
	entry = entry.Clone()
	copyHeader(w.Header(), entry.Header)
	w.Header().Set("X-Cache", "HIT")
	w.WriteHeader(entry.Status)
	if r.Method != http.MethodHead {
		_, _ = w.Write(entry.Body)
	}
}

func cacheShard(key string) uint32 {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(key))
	return hash.Sum32() % 64
}
