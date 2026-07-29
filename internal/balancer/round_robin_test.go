package balancer

import (
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
)

func TestNewRoundRobinRequiresUpstream(t *testing.T) {
	if _, err := NewRoundRobin(nil); err == nil {
		t.Fatal("expected an error for an empty upstream list")
	}
}

func TestRoundRobinCyclesInOrder(t *testing.T) {
	first := &url.URL{Host: "first"}
	second := &url.URL{Host: "second"}
	third := &url.URL{Host: "third"}
	balancer, err := NewRoundRobin([]*url.URL{first, second, third})
	if err != nil {
		t.Fatal(err)
	}

	want := []*url.URL{first, second, third, first, second, third}
	for i, expected := range want {
		if selected := balancer.Next(); selected != expected {
			t.Fatalf("selection %d = %s, want %s", i, selected, expected)
		}
	}
}

func TestRoundRobinIsSafeForConcurrentUse(t *testing.T) {
	upstreams := []*url.URL{{Host: "first"}, {Host: "second"}}
	balancer, err := NewRoundRobin(upstreams)
	if err != nil {
		t.Fatal(err)
	}

	var counts [2]atomic.Int64
	var waitGroup sync.WaitGroup
	const requests = 100
	waitGroup.Add(requests)
	for range requests {
		go func() {
			defer waitGroup.Done()
			selected := balancer.Next()
			if selected == upstreams[0] {
				counts[0].Add(1)
			} else {
				counts[1].Add(1)
			}
		}()
	}
	waitGroup.Wait()

	if counts[0].Load() != requests/2 || counts[1].Load() != requests/2 {
		t.Fatalf("distribution = [%d, %d]", counts[0].Load(), counts[1].Load())
	}
}
