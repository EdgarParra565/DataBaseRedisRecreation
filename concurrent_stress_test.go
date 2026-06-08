package main

import (
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"
)

// TestConcurrentStress exercises the handlers concurrently to validate correctness under contention.
func TestConcurrentStress(t *testing.T) {
	resetStores()
	EXPIREsMu.Lock()
	EXPIREs = map[string]time.Time{}
	EXPIREsMu.Unlock()

	const goroutines = 50
	const opsPerG = 200

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for gid := 0; gid < goroutines; gid++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < opsPerG; j++ {
				// increment shared counter
				res := incr([]Value{{typ: "bulk", bulk: "counter"}})
				if res.typ == "error" {
					t.Errorf("incr failed: %v", res)
					return
				}

				// occasionally create a separate key
				if j%50 == 0 {
					key := fmt.Sprintf("k-%d-%d", id, j)
					set([]Value{{typ: "bulk", bulk: key}, {typ: "bulk", bulk: "v"}})
				}

				// write into a per-goroutine hash
				hash := fmt.Sprintf("h-%d", id)
				field := fmt.Sprintf("f-%d", j)
				hset([]Value{{typ: "bulk", bulk: hash}, {typ: "bulk", bulk: field}, {typ: "bulk", bulk: "v"}})
			}
		}(gid)
	}

	wg.Wait()

	// verify counter
	got := get([]Value{{typ: "bulk", bulk: "counter"}})
	if got.typ != "bulk" {
		t.Fatalf("expected bulk counter, got %v", got)
	}

	val, err := strconv.Atoi(got.bulk)
	if err != nil {
		t.Fatalf("failed to parse counter: %v", err)
	}

	expected := goroutines * opsPerG
	if val != expected {
		t.Fatalf("expected counter %d, got %d", expected, val)
	}
}
