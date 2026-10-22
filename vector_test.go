package main

import (
	"math"
	"math/rand"
	"strconv"
	"testing"
	"time"
)

// bulkArgs builds handler args from strings since vector commands take many of them.
func bulkArgs(args ...string) []Value {
	vals := make([]Value, len(args))
	for i, arg := range args {
		vals[i] = Value{typ: "bulk", bulk: arg}
	}
	return vals
}

func TestVAddAndVGet(t *testing.T) {
	resetStores()

	addResult := vadd(bulkArgs("doc:1", "3", "1", "0.5", "-2"))
	if addResult.typ != "integer" || addResult.num != 1 {
		t.Fatalf("expected integer 1, got typ=%q num=%d", addResult.typ, addResult.num)
	}

	getResult := vget(bulkArgs("doc:1"))
	if getResult.typ != "array" || len(getResult.array) != 3 {
		t.Fatalf("expected array of 3, got typ=%q len=%d", getResult.typ, len(getResult.array))
	}
	expected := []string{"1", "0.5", "-2"}
	for i, v := range getResult.array {
		if v.bulk != expected[i] {
			t.Fatalf("expected %s at index %d, got %s", expected[i], i, v.bulk)
		}
	}
}

func TestVAddReplace(t *testing.T) {
	resetStores()

	vadd(bulkArgs("doc:1", "2", "1", "2"))
	got := vadd(bulkArgs("doc:1", "2", "3", "4"))

	if got.typ != "integer" || got.num != 0 {
		t.Fatalf("expected integer 0, got typ=%q num=%d", got.typ, got.num)
	}

	stored := vget(bulkArgs("doc:1"))
	if stored.array[0].bulk != "3" {
		t.Fatal("expected replaced vector, got", stored.array[0].bulk)
	}
}

func TestVAddParseErrors(t *testing.T) {
	resetStores()

	tests := []struct {
		name string
		got  Value
	}{
		{name: "missing values", got: vadd(bulkArgs("doc:1", "2"))},
		{name: "dim not integer", got: vadd(bulkArgs("doc:1", "x", "1"))},
		{name: "dim zero", got: vadd(bulkArgs("doc:1", "0", "1"))},
		{name: "dimension mismatch", got: vadd(bulkArgs("doc:1", "3", "1", "2"))},
		{name: "value not float", got: vadd(bulkArgs("doc:1", "2", "1", "abc"))},
		{name: "value NaN", got: vadd(bulkArgs("doc:1", "2", "1", "NaN"))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got.typ != "error" {
				t.Fatalf("expected error, got typ=%q", tt.got.typ)
			}
		})
	}
}

func TestVDim(t *testing.T) {
	resetStores()

	vadd(bulkArgs("doc:1", "4", "1", "2", "3", "4"))

	got := vdim(bulkArgs("doc:1"))
	if got.typ != "integer" || got.num != 4 {
		t.Fatalf("expected integer 4, got typ=%q num=%d", got.typ, got.num)
	}

	missing := vdim(bulkArgs("missing"))
	if missing.typ != "null" {
		t.Fatalf("expected null for missing key, got typ=%q", missing.typ)
	}
}

func TestVDel(t *testing.T) {
	resetStores()

	vadd(bulkArgs("doc:1", "2", "1", "2"))

	got := vdel(bulkArgs("doc:1"))
	if got.typ != "integer" || got.num != 1 {
		t.Fatalf("expected integer 1, got typ=%q num=%d", got.typ, got.num)
	}

	if vget(bulkArgs("doc:1")).typ != "null" {
		t.Fatal("expected vector to be deleted")
	}
}

func TestVectorKeyCommands(t *testing.T) {
	resetStores()
	EXPIREsMu.Lock()
	EXPIREs = map[string]time.Time{}
	EXPIREsMu.Unlock()

	vadd(bulkArgs("doc:1", "2", "1", "2"))
	vadd(bulkArgs("doc:2", "2", "3", "4"))

	if exists(bulkArgs("doc:1")).num != 1 {
		t.Fatal("expected exists 1 for vector key")
	}

	all := keys(bulkArgs("doc:*"))
	if len(all.array) != 2 {
		t.Fatalf("expected 2 vector keys, got %d", len(all.array))
	}

	if del(bulkArgs("doc:1")).num != 1 {
		t.Fatal("expected del to remove vector key")
	}

	expRes := expire(bulkArgs("doc:2", "10"))
	if expRes.typ != "integer" || expRes.num != 1 {
		t.Fatalf("expected integer 1 from expire, got %v", expRes)
	}

	// force expiry instead of sleeping
	EXPIREsMu.Lock()
	EXPIREs["doc:2"] = time.Now().Add(-time.Second)
	EXPIREsMu.Unlock()

	if vget(bulkArgs("doc:2")).typ != "null" {
		t.Fatal("expected expired vector to be removed")
	}
}

func TestCosineIdentical(t *testing.T) {
	a := newVector([]float32{1, 2, 3})

	got := cosine(a, a)

	if math.Abs(got-1) > 1e-9 {
		t.Fatalf("expected cosine 1, got %v", got)
	}
}

func TestL2Identical(t *testing.T) {
	a := []float32{1, 2, 3}

	got := l2Squared(a, a)

	if got != 0 {
		t.Fatalf("expected l2 0, got %v", got)
	}
}

func TestVSimSortedOrder(t *testing.T) {
	resetStores()

	vadd(bulkArgs("east", "2", "1", "0"))
	vadd(bulkArgs("north", "2", "0", "1"))
	vadd(bulkArgs("northeast", "2", "1", "1"))
	vadd(bulkArgs("west", "2", "-1", "0"))

	got := vsim(bulkArgs("east", "3"))
	if got.typ != "array" || len(got.array) != 6 {
		t.Fatalf("expected array of 3 key score pairs, got typ=%q len=%d", got.typ, len(got.array))
	}

	expected := []string{"east", "northeast", "north"}
	for i, key := range expected {
		if got.array[i*2].bulk != key {
			t.Fatalf("expected %s at rank %d, got %s", key, i, got.array[i*2].bulk)
		}
	}

	score, err := strconv.ParseFloat(got.array[1].bulk, 64)
	if err != nil || math.Abs(score-1) > 1e-9 {
		t.Fatalf("expected score 1 for identical vector, got %s", got.array[1].bulk)
	}
}

func TestVSimKGreaterThanN(t *testing.T) {
	resetStores()

	vadd(bulkArgs("a", "2", "1", "0"))
	vadd(bulkArgs("b", "2", "0", "1"))

	got := vsimq(bulkArgs("2", "1", "0", "10"))

	if got.typ != "array" || len(got.array) != 4 {
		t.Fatalf("expected array of 2 key score pairs, got typ=%q len=%d", got.typ, len(got.array))
	}
}

func TestVSimQMetrics(t *testing.T) {
	resetStores()

	vadd(bulkArgs("near", "2", "1", "1"))
	vadd(bulkArgs("far", "2", "10", "10"))

	l2Res := vsimq(bulkArgs("2", "1", "1", "1", "METRIC", "l2"))
	if l2Res.array[0].bulk != "near" || l2Res.array[1].bulk != "0" {
		t.Fatalf("expected near with distance 0 for l2, got %v", l2Res.array)
	}

	dotRes := vsimq(bulkArgs("2", "1", "1", "1", "METRIC", "DOT"))
	if dotRes.array[0].bulk != "far" || dotRes.array[1].bulk != "20" {
		t.Fatalf("expected far with score 20 for dot, got %v", dotRes.array)
	}
}

func TestVSimSkipsOtherDimensions(t *testing.T) {
	resetStores()

	vadd(bulkArgs("two", "2", "1", "0"))
	vadd(bulkArgs("three", "3", "1", "0", "0"))

	got := vsimq(bulkArgs("2", "1", "0", "5"))

	if len(got.array) != 2 || got.array[0].bulk != "two" {
		t.Fatalf("expected only the 2-dim vector, got %v", got.array)
	}
}

func TestVSimErrors(t *testing.T) {
	resetStores()

	vadd(bulkArgs("a", "2", "1", "0"))

	tests := []struct {
		name string
		got  Value
	}{
		{name: "vsim k zero", got: vsim(bulkArgs("a", "0"))},
		{name: "vsim k not integer", got: vsim(bulkArgs("a", "x"))},
		{name: "vsim unknown metric", got: vsim(bulkArgs("a", "1", "METRIC", "HAMMING"))},
		{name: "vsim missing metric name", got: vsim(bulkArgs("a", "1", "METRIC"))},
		{name: "vsimq dimension mismatch", got: vsimq(bulkArgs("3", "1", "0", "1"))},
		{name: "vsimq value not float", got: vsimq(bulkArgs("2", "1", "x", "1"))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got.typ != "error" {
				t.Fatalf("expected error, got typ=%q", tt.got.typ)
			}
		})
	}
}

func TestVSimMissingKey(t *testing.T) {
	resetStores()

	got := vsim(bulkArgs("missing", "1"))

	if got.typ != "null" {
		t.Fatalf("expected null for missing key, got typ=%q", got.typ)
	}
}

func BenchmarkVSIM(b *testing.B) {
	resetStores()

	const n = 10000
	const dim = 128

	rng := rand.New(rand.NewSource(1))
	for i := 0; i < n; i++ {
		values := make([]float32, dim)
		for j := range values {
			values[j] = rng.Float32()
		}
		VECTORs["vec:"+strconv.Itoa(i)] = newVector(values)
	}

	query := make([]string, 0, dim+2)
	query = append(query, strconv.Itoa(dim))
	for j := 0; j < dim; j++ {
		query = append(query, strconv.FormatFloat(rng.Float64(), 'f', -1, 32))
	}
	query = append(query, "10")
	args := bulkArgs(query...)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vsimq(args)
	}
}
