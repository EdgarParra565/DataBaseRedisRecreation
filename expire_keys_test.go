package main

import (
	"testing"
	"time"
)

func TestExpireAndTTL(t *testing.T) {
	resetStores()
	EXPIREsMu.Lock()
	EXPIREs = map[string]time.Time{}
	EXPIREsMu.Unlock()

	// set key
	setRes := set([]Value{{typ: "bulk", bulk: "temp"}, {typ: "bulk", bulk: "v"}})
	if setRes.typ != "string" {
		t.Fatalf("expected OK from set, got %v", setRes)
	}

	// expire for 2 seconds
	expRes := expire([]Value{{typ: "bulk", bulk: "temp"}, {typ: "bulk", bulk: "2"}})
	if expRes.typ != "integer" || expRes.num != 1 {
		t.Fatalf("expected integer 1 from expire, got %v", expRes)
	}

	// ttl should be <=2 and >=0
	ttlRes := ttl([]Value{{typ: "bulk", bulk: "temp"}})
	if ttlRes.typ != "integer" {
		t.Fatalf("expected integer ttl, got %v", ttlRes)
	}
	if ttlRes.num < 0 || ttlRes.num > 2 {
		t.Fatalf("unexpected ttl value: %d", ttlRes.num)
	}

	// wait for expiry
	time.Sleep(3 * time.Second)

	// key should be gone
	getRes := get([]Value{{typ: "bulk", bulk: "temp"}})
	if getRes.typ != "null" {
		t.Fatalf("expected null after expiry, got %v", getRes)
	}

	existsRes := exists([]Value{{typ: "bulk", bulk: "temp"}})
	if existsRes.typ != "integer" || existsRes.num != 0 {
		t.Fatalf("expected exists 0 after expiry, got %v", existsRes)
	}

	ttlAfter := ttl([]Value{{typ: "bulk", bulk: "temp"}})
	if ttlAfter.typ != "integer" || ttlAfter.num != -2 {
		t.Fatalf("expected ttl -2 for missing key, got %v", ttlAfter)
	}
}

func TestExpireNonExistent(t *testing.T) {
	resetStores()
	EXPIREsMu.Lock()
	EXPIREs = map[string]time.Time{}
	EXPIREsMu.Unlock()

	expRes := expire([]Value{{typ: "bulk", bulk: "nope"}, {typ: "bulk", bulk: "10"}})
	if expRes.typ != "integer" || expRes.num != 0 {
		t.Fatalf("expected 0 when expiring non-existent key, got %v", expRes)
	}

	ttlRes := ttl([]Value{{typ: "bulk", bulk: "nope"}})
	if ttlRes.typ != "integer" || ttlRes.num != -2 {
		t.Fatalf("expected ttl -2 for missing key, got %v", ttlRes)
	}
}

func TestKeysPatternMatching(t *testing.T) {
	resetStores()
	EXPIREsMu.Lock()
	EXPIREs = map[string]time.Time{}
	EXPIREsMu.Unlock()

	// add keys
	set([]Value{{typ: "bulk", bulk: "a"}, {typ: "bulk", bulk: "1"}})
	set([]Value{{typ: "bulk", bulk: "aa"}, {typ: "bulk", bulk: "2"}})
	set([]Value{{typ: "bulk", bulk: "ab"}, {typ: "bulk", bulk: "3"}})
	set([]Value{{typ: "bulk", bulk: "user:1"}, {typ: "bulk", bulk: "u1"}})
	set([]Value{{typ: "bulk", bulk: "user:2"}, {typ: "bulk", bulk: "u2"}})
	hset([]Value{{typ: "bulk", bulk: "user:3"}, {typ: "bulk", bulk: "name"}, {typ: "bulk", bulk: "edgar"}})

	// pattern user:*
	res := keys([]Value{{typ: "bulk", bulk: "user:*"}})
	if res.typ != "array" {
		t.Fatalf("expected array from keys, got %v", res)
	}
	if len(res.array) != 3 {
		t.Fatalf("expected 3 user keys, got %d", len(res.array))
	}
	expected := []string{"user:1", "user:2", "user:3"}
	for i, v := range res.array {
		if v.bulk != expected[i] {
			t.Fatalf("expected %s at index %d, got %s", expected[i], i, v.bulk)
		}
	}

	// expired keys should be skipped
	set([]Value{{typ: "bulk", bulk: "tmp"}, {typ: "bulk", bulk: "x"}})
	expire([]Value{{typ: "bulk", bulk: "tmp"}, {typ: "bulk", bulk: "1"}})
	time.Sleep(2 * time.Second)
	all := keys([]Value{{typ: "bulk", bulk: "*"}})
	for _, v := range all.array {
		if v.bulk == "tmp" {
			t.Fatalf("expected tmp to be expired and not returned in keys")
		}
	}
}
