package discovery

import (
	"context"
	"errors"
	"testing"

	"github.com/ivan-gromov-dev/APIGateway/internal/config"
)

func TestDNSResolvesAddresses(t *testing.T) {
	dns := DNS{LookupHost: func(context.Context, string) ([]string, error) {
		return []string{"10.0.0.2", "10.0.0.1", "10.0.0.1"}, nil
	}}
	got, err := dns.Resolve(context.Background(), config.Discovery{Name: "users", Scheme: "http", Port: 8081})
	if err != nil || len(got) != 2 || got[0] != "http://10.0.0.1:8081" || got[1] != "http://10.0.0.2:8081" {
		t.Fatalf("got=%v err=%v", got, err)
	}
}

func TestDNSReturnsLookupErrorAndEmptyResult(t *testing.T) {
	want := errors.New("lookup failed")
	dns := DNS{LookupHost: func(context.Context, string) ([]string, error) { return nil, want }}
	if _, err := dns.Resolve(context.Background(), config.Discovery{Name: "users"}); !errors.Is(err, want) {
		t.Fatalf("err=%v", err)
	}
	dns.LookupHost = func(context.Context, string) ([]string, error) { return nil, nil }
	if _, err := dns.Resolve(context.Background(), config.Discovery{Name: "users"}); err == nil {
		t.Fatal("expected empty result error")
	}
}

func TestDNSBuildsBracketedIPv6Target(t *testing.T) {
	dns := DNS{LookupHost: func(context.Context, string) ([]string, error) { return []string{"2001:db8::10"}, nil }}
	got, err := dns.Resolve(context.Background(), config.Discovery{Name: "users", Scheme: "https", Port: 8443})
	if err != nil || len(got) != 1 || got[0] != "https://[2001:db8::10]:8443" {
		t.Fatalf("got=%v err=%v", got, err)
	}
}
