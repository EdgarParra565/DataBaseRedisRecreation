package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAofReplayRestoresExpire(t *testing.T) {
	resetStores()
	EXPIREsMu.Lock()
	EXPIREs = map[string]time.Time{}
	EXPIREsMu.Unlock()

	path := filepath.Join(t.TempDir(), "database.aof")
	aof, err := NewAof(path)
	if err != nil {
		t.Fatal(err)
	}
	defer aof.Close()

	setCmd := Value{
		typ: "array",
		array: []Value{
			{typ: "bulk", bulk: "SET"},
			{typ: "bulk", bulk: "k"},
			{typ: "bulk", bulk: "v"},
		},
	}
	expCmd := Value{
		typ: "array",
		array: []Value{
			{typ: "bulk", bulk: "EXPIRE"},
			{typ: "bulk", bulk: "k"},
			{typ: "bulk", bulk: "2"},
		},
	}

	if err := aof.Write(setCmd); err != nil {
		t.Fatal(err)
	}
	if err := aof.Write(expCmd); err != nil {
		t.Fatal(err)
	}

	// replay by invoking handlers (like main.go does)
	err = aof.Read(func(value Value) {
		if value.typ != "array" || len(value.array) == 0 {
			return
		}
		command := strings.ToUpper(value.array[0].bulk)
		args := value.array[1:]
		handler, ok := Handlers[command]
		if !ok {
			return
		}
		handler(args)
	})
	if err != nil {
		t.Fatal(err)
	}

	// ttl should be > 0 after replay
	ttlRes := ttl([]Value{{typ: "bulk", bulk: "k"}})
	if ttlRes.typ != "integer" {
		t.Fatalf("expected integer ttl, got %v", ttlRes)
	}
	if ttlRes.num <= 0 {
		t.Fatalf("expected ttl > 0 after replay, got %d", ttlRes.num)
	}

	// also verify key exists
	getRes := get([]Value{{typ: "bulk", bulk: "k"}})
	if getRes.typ != "bulk" || getRes.bulk != "v" {
		t.Fatalf("expected stored value v after replay, got %v", getRes)
	}

	// ensure expiry actually removes key after waiting
	time.Sleep(3 * time.Second)
	getAfter := get([]Value{{typ: "bulk", bulk: "k"}})
	if getAfter.typ != "null" {
		t.Fatalf("expected key to be expired after wait, got %v", getAfter)
	}
}
