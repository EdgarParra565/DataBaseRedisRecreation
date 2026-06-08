package main

import "testing"

// resetStores keeps tests isolated because handlers use package-level maps.
func resetStores() {
	SETsMu.Lock()
	SETs = map[string]string{}
	SETsMu.Unlock()

	HSETsMu.Lock()
	HSETs = map[string]map[string]string{}
	HSETsMu.Unlock()
}

func TestPing(t *testing.T) {
	got := ping(nil)

	if got.typ != "string" || got.str != "PONG" {
		t.Fatalf("expected string PONG, got typ=%q str=%q", got.typ, got.str)
	}
}

func TestSetAndGet(t *testing.T) {
	resetStores()

	setResult := set([]Value{
		{typ: "bulk", bulk: "name"},
		{typ: "bulk", bulk: "edgar"},
	})
	if setResult.typ != "string" || setResult.str != "OK" {
		t.Fatalf("expected string OK, got typ=%q str=%q", setResult.typ, setResult.str)
	}

	getResult := get([]Value{
		{typ: "bulk", bulk: "name"},
	})
	if getResult.typ != "bulk" || getResult.bulk != "edgar" {
		t.Fatalf("expected bulk edgar, got typ=%q bulk=%q", getResult.typ, getResult.bulk)
	}
}

func TestGetMissingKey(t *testing.T) {
	resetStores()

	got := get([]Value{
		{typ: "bulk", bulk: "missing"},
	})

	if got.typ != "null" {
		t.Fatalf("expected null for missing key, got typ=%q", got.typ)
	}
}

func TestHSetAndHGet(t *testing.T) {
	resetStores()

	hsetResult := hset([]Value{
		{typ: "bulk", bulk: "user:1"},
		{typ: "bulk", bulk: "name"},
		{typ: "bulk", bulk: "edgar"},
	})
	if hsetResult.typ != "string" || hsetResult.str != "OK" {
		t.Fatalf("expected string OK, got typ=%q str=%q", hsetResult.typ, hsetResult.str)
	}

	hgetResult := hget([]Value{
		{typ: "bulk", bulk: "user:1"},
		{typ: "bulk", bulk: "name"},
	})
	if hgetResult.typ != "bulk" || hgetResult.bulk != "edgar" {
		t.Fatalf("expected bulk edgar, got typ=%q bulk=%q", hgetResult.typ, hgetResult.bulk)
	}
}

func TestWrongArgumentCounts(t *testing.T) {
	tests := []struct {
		name string
		got  Value
	}{
		{name: "set missing value", got: set([]Value{{typ: "bulk", bulk: "name"}})},
		{name: "get too many args", got: get([]Value{{typ: "bulk", bulk: "name"}, {typ: "bulk", bulk: "extra"}})},
		{name: "hset missing value", got: hset([]Value{{typ: "bulk", bulk: "user:1"}, {typ: "bulk", bulk: "name"}})},
		{name: "hget missing field", got: hget([]Value{{typ: "bulk", bulk: "user:1"}})},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got.typ != "error" {
				t.Fatalf("expected error, got typ=%q", tt.got.typ)
			}
		})
	}
}

func TestExists(t *testing.T) {
	resetStores()

	set([]Value{
		{typ: "bulk", bulk: "name"},
		{typ: "bulk", bulk: "edgar"},
	})

	got := exists([]Value{
		{typ: "bulk", bulk: "name"},
	})

	if got.typ != "integer" || got.num != 1 {
		t.Fatalf("expected integer 1, got typ=%q num=%d", got.typ, got.num)
	}
}

func TestExistsMissingKey(t *testing.T) {
	resetStores()

	got := exists([]Value{{typ: "bulk", bulk: "missing"}})

	if got.typ != "integer" || got.num != 0 {
		t.Fatalf("expected integer 0, got typ=%q num=%d", got.typ, got.num)
	}
}

func TestDelStringKey(t *testing.T) {
	resetStores()

	set([]Value{
		{typ: "bulk", bulk: "name"},
		{typ: "bulk", bulk: "edgar"},
	})

	got := del([]Value{{typ: "bulk", bulk: "name"}})

	if got.typ != "integer" || got.num != 1 {
		t.Fatalf("expected integer 1, got typ=%q num=%d", got.typ, got.num)
	}

	if exists([]Value{{typ: "bulk", bulk: "name"}}).num != 0 {
		t.Fatal("expected key to be deleted")
	}
}

func TestIncrMissingKey(t *testing.T) {
	resetStores()

	got := incr([]Value{{typ: "bulk", bulk: "count"}})
	if got.typ != "integer" || got.num != 1 {
		t.Fatalf("expected integer 1, got typ=%q num=%d", got.typ, got.num)
	}

	stored := get([]Value{{typ: "bulk", bulk: "count"}})

	if stored.bulk != "1" {
		t.Fatal("expected stored value to be 1, got", stored.bulk)
	}
}

func TestIncrExistingKey(t *testing.T) {
	resetStores()

	set([]Value{
		{typ: "bulk", bulk: "count"},
		{typ: "bulk", bulk: "10"},
	})

	got := incr([]Value{{typ: "bulk", bulk: "count"}})

	if got.typ != "integer" || got.num != 11 {
		t.Fatalf("expected integer 11, got typ=%q num=%d", got.typ, got.num)
	}
}

func TestIncrBy(t *testing.T) {
	resetStores()
	set([]Value{
		{typ: "bulk", bulk: "count"},
		{typ: "bulk", bulk: "10"},
	})

	got := incrBy([]Value{
		{typ: "bulk", bulk: "count"},
		{typ: "bulk", bulk: "10"},
	})

	if got.typ != "integer" || got.num != 20 {
		t.Fatalf("expected integer 20, got typ=%q num=%d", got.typ, got.num)
	}
}

func TestIncrNonIntValue(t *testing.T) {
	resetStores()

	set([]Value{
		{typ: "bulk", bulk: "name"},
		{typ: "bulk", bulk: "edgar"},
	})

	got := incr([]Value{
		{typ: "bulk", bulk: "name"},
	})

	if got.typ != "error" {
		t.Fatalf("expected error, got typ=%q", got.typ)
	}

}
