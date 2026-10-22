package main

import (
	"container/heap"
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Vector stores float32 values with a cached norm for cosine similarity.
type Vector struct {
	values []float32
	norm   float64
}

// VECTORs stores vector keys for VADD and VSIM commands.
var VECTORs = map[string]Vector{}

// VECTORsMu protects concurrent access to VECTORs.
var VECTORsMu = sync.RWMutex{}

// newVector builds a Vector and precomputes its norm.
func newVector(values []float32) Vector {
	return Vector{values: values, norm: math.Sqrt(dot(values, values))}
}

// vadd stores a vector by key, returning 1 if new and 0 if replaced.
func vadd(args []Value) Value {
	if len(args) < 3 {
		return Value{typ: "error", str: "Invalid number of args for vadd"}
	}
	key := args[0].bulk

	dim, err := parseDim(args[1])
	if err != nil {
		return Value{typ: "error", str: err.Error()}
	}
	if len(args)-2 != dim {
		return Value{typ: "error", str: "Invalid, dimension mismatch"}
	}
	values, err := parseFloats(args[2:])
	if err != nil {
		return Value{typ: "error", str: err.Error()}
	}

	// cleanup if expired
	cleanupIfExpired(key)

	VECTORsMu.Lock()
	_, replaced := VECTORs[key]
	VECTORs[key] = newVector(values)
	VECTORsMu.Unlock()

	if replaced {
		return Value{typ: "integer", num: 0}
	}

	return Value{typ: "integer", num: 1}
}

// vget returns a vector as an array of bulk string floats.
func vget(args []Value) Value {
	if len(args) != 1 {
		return Value{typ: "error", str: "Too many arguments"}
	}
	key := args[0].bulk

	// remove if expired
	cleanupIfExpired(key)

	VECTORsMu.RLock()
	vector, ok := VECTORs[key]
	VECTORsMu.RUnlock()

	if !ok {
		return Value{typ: "null"}
	}

	vals := make([]Value, len(vector.values))
	for i, f := range vector.values {
		vals[i] = Value{typ: "bulk", bulk: strconv.FormatFloat(float64(f), 'f', -1, 32)}
	}

	return Value{typ: "array", array: vals}
}

// vdim returns the dimension of a stored vector.
func vdim(args []Value) Value {
	if len(args) != 1 {
		return Value{typ: "error", str: "Too many arguments"}
	}
	key := args[0].bulk

	// remove if expired
	cleanupIfExpired(key)

	VECTORsMu.RLock()
	vector, ok := VECTORs[key]
	VECTORsMu.RUnlock()

	if !ok {
		return Value{typ: "null"}
	}

	return Value{typ: "integer", num: len(vector.values)}
}

// vdel removes a vector key.
func vdel(args []Value) Value {
	if len(args) != 1 {
		return Value{typ: "error", str: "Too many arguments"}
	}
	key := args[0].bulk
	deleted := false

	VECTORsMu.Lock()
	if _, ok := VECTORs[key]; ok {
		delete(VECTORs, key)
		deleted = true
	}
	VECTORsMu.Unlock()

	EXPIREsMu.Lock()
	delete(EXPIREs, key)
	EXPIREsMu.Unlock()

	if deleted {
		return Value{typ: "integer", num: 1}
	}

	return Value{typ: "integer", num: 0}
}

// vsim returns the k nearest neighbours to the vector stored at key, including key itself.
func vsim(args []Value) Value {
	if len(args) != 2 && len(args) != 4 {
		return Value{typ: "error", str: "Invalid number of args for vsim"}
	}
	key := args[0].bulk

	k, err := parseK(args[1])
	if err != nil {
		return Value{typ: "error", str: err.Error()}
	}
	metric, err := parseMetric(args[2:])
	if err != nil {
		return Value{typ: "error", str: err.Error()}
	}

	// remove if expired
	cleanupIfExpired(key)

	VECTORsMu.RLock()
	defer VECTORsMu.RUnlock()

	query, ok := VECTORs[key]
	if !ok {
		return Value{typ: "null"}
	}

	return search(query, k, metric)
}

// vsimq returns the k nearest neighbours to a raw query vector.
func vsimq(args []Value) Value {
	if len(args) < 3 {
		return Value{typ: "error", str: "Invalid number of args for vsimq"}
	}

	dim, err := parseDim(args[0])
	if err != nil {
		return Value{typ: "error", str: err.Error()}
	}
	rest := len(args) - 1 - dim
	if rest != 1 && rest != 3 {
		return Value{typ: "error", str: "Invalid, dimension mismatch"}
	}
	values, err := parseFloats(args[1 : 1+dim])
	if err != nil {
		return Value{typ: "error", str: err.Error()}
	}
	k, err := parseK(args[1+dim])
	if err != nil {
		return Value{typ: "error", str: err.Error()}
	}
	metric, err := parseMetric(args[2+dim:])
	if err != nil {
		return Value{typ: "error", str: err.Error()}
	}

	VECTORsMu.RLock()
	defer VECTORsMu.RUnlock()

	return search(newVector(values), k, metric)
}

// parseDim reads a positive vector dimension.
func parseDim(arg Value) (int, error) {
	dim, err := strconv.Atoi(arg.bulk)
	if err != nil {
		return 0, errors.New("Invalid, must be integer")
	}
	if dim <= 0 {
		return 0, errors.New("Invalid, dim must be positive")
	}
	return dim, nil
}

// parseK reads a positive result count.
func parseK(arg Value) (int, error) {
	k, err := strconv.Atoi(arg.bulk)
	if err != nil {
		return 0, errors.New("Invalid, must be integer")
	}
	if k <= 0 {
		return 0, errors.New("Invalid, k must be positive")
	}
	return k, nil
}

// parseFloats converts bulk strings into float32 values.
func parseFloats(args []Value) ([]float32, error) {
	values := make([]float32, len(args))
	for i, arg := range args {
		f, err := strconv.ParseFloat(arg.bulk, 32)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, errors.New("Invalid, must be float")
		}
		values[i] = float32(f)
	}
	return values, nil
}

// parseMetric reads an optional METRIC COSINE|L2|DOT pair, defaulting to COSINE.
func parseMetric(args []Value) (string, error) {
	if len(args) == 0 {
		return "COSINE", nil
	}
	if len(args) == 2 && strings.ToUpper(args[0].bulk) == "METRIC" {
		metric := strings.ToUpper(args[1].bulk)
		switch metric {
		case "COSINE", "L2", "DOT":
			return metric, nil
		}
	}
	return "", errors.New("Invalid, expected METRIC COSINE|L2|DOT")
}

// match is one search result.
type match struct {
	key   string
	score float64
}

// better reports whether a ranks ahead of b, breaking ties by key so results are stable.
func better(metric string, a, b match) bool {
	if a.score != b.score {
		if metric == "L2" {
			return a.score < b.score
		}
		return a.score > b.score
	}
	return a.key < b.key
}

// matchHeap keeps the worst of the current top k at the root so it can be replaced.
type matchHeap struct {
	matches []match
	metric  string
}

// Len, Less, Swap, Push, and Pop implement heap.Interface.
func (h *matchHeap) Len() int           { return len(h.matches) }
func (h *matchHeap) Less(i, j int) bool { return better(h.metric, h.matches[j], h.matches[i]) }
func (h *matchHeap) Swap(i, j int)      { h.matches[i], h.matches[j] = h.matches[j], h.matches[i] }
func (h *matchHeap) Push(x any)         { h.matches = append(h.matches, x.(match)) }

func (h *matchHeap) Pop() any {
	n := len(h.matches)
	m := h.matches[n-1]
	h.matches = h.matches[:n-1]
	return m
}

// search brute forces the k nearest vectors to query as [key, score, ...]; caller holds VECTORsMu.
func search(query Vector, k int, metric string) Value {
	h := &matchHeap{metric: metric}

	for key, vector := range VECTORs {
		// skip vectors with a different dimension
		if len(vector.values) != len(query.values) {
			continue
		}
		m := match{key: key, score: score(metric, query, vector)}
		if h.Len() < k {
			heap.Push(h, m)
		} else if better(metric, m, h.matches[0]) {
			h.matches[0] = m
			heap.Fix(h, 0)
		}
	}

	// heap keeps worst first, so sort best first for the reply
	sort.Slice(h.matches, func(i, j int) bool {
		return better(metric, h.matches[i], h.matches[j])
	})

	vals := make([]Value, 0, 2*len(h.matches))
	for _, m := range h.matches {
		s := m.score
		if metric == "L2" {
			s = math.Sqrt(s)
		}
		vals = append(vals,
			Value{typ: "bulk", bulk: m.key},
			Value{typ: "bulk", bulk: strconv.FormatFloat(s, 'f', -1, 64)},
		)
	}

	return Value{typ: "array", array: vals}
}

// score compares two vectors with the given metric; L2 skips the sqrt since ranking does not need it.
func score(metric string, a, b Vector) float64 {
	switch metric {
	case "L2":
		return l2Squared(a.values, b.values)
	case "DOT":
		return dot(a.values, b.values)
	default:
		return cosine(a, b)
	}
}

// cosine returns dot(a, b) / (norm(a) * norm(b)) using the cached norms.
func cosine(a, b Vector) float64 {
	if a.norm == 0 || b.norm == 0 {
		return 0
	}
	return dot(a.values, b.values) / (a.norm * b.norm)
}

// dot returns sum(a[i] * b[i]).
func dot(a, b []float32) float64 {
	var sum float64
	for i := range a {
		sum += float64(a[i]) * float64(b[i])
	}
	return sum
}

// l2Squared returns sum((a[i] - b[i])^2).
func l2Squared(a, b []float32) float64 {
	var sum float64
	for i := range a {
		d := float64(a[i]) - float64(b[i])
		sum += d * d
	}
	return sum
}
