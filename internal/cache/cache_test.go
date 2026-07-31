package cache

import (
	"net/http"
	"testing"
)

func TestEntryCloneIsIndependent(t *testing.T) {
	original := Entry{Status: 200, Header: http.Header{"X-Test": {"one"}}, Body: []byte("body")}
	cloned := original.Clone()
	cloned.Header.Set("X-Test", "two")
	cloned.Body[0] = 'B'
	if original.Header.Get("X-Test") != "one" || string(original.Body) != "body" {
		t.Fatalf("clone mutated original: %+v", original)
	}
}
