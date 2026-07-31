// Package cache defines storage-neutral response cache contracts.
package cache

import (
	"context"
	"net/http"
	"time"
)

type Entry struct {
	Status int
	Header http.Header
	Body   []byte
}

// Clone returns an entry whose headers and body may be mutated independently.
func (e Entry) Clone() Entry {
	result := Entry{Status: e.Status, Header: e.Header.Clone()}
	result.Body = append([]byte(nil), e.Body...)
	return result
}

type Store interface {
	Get(context.Context, string) (Entry, bool, error)
	Set(context.Context, string, Entry, time.Duration) error
}
