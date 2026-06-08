package main

import (
	"path"
	"sort"
	"strconv"
	"sync"
	"time"
)

// Handlers maps Redis command names to their command functions.
var Handlers = map[string]func([]Value) Value{
	"PING":   ping,
	"SET":    set,
	"GET":    get,
	"HSET":   hset,
	"HGET":   hget,
	"EXISTS": exists,
	"DEL":    del,
	"INCR":   incr,
	"INCRBY": incrBy,
	"EXPIRE": expire,
	"TTL":    ttl,
	"KEYS":   keys,
	//"HGETALL": hgetall,
}

// ping returns a PONG response for the PING command.
func ping(args []Value) Value {
	return Value{typ: "string", str: "PONG"}
}

// SETs stores string keys and values for SET and GET commands.
var SETs = map[string]string{}

// SETsMu protects concurrent access to SETs.
var SETsMu = sync.RWMutex{}

// EXPIREs stores expiration times for keys.
var EXPIREs = map[string]time.Time{}
var EXPIREsMu = sync.RWMutex{}

// start background goroutine to periodically remove expired keys
func init() {
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		for range ticker.C {
			now := time.Now()
			EXPIREsMu.Lock()
			for k, t := range EXPIREs {
				if now.After(t) {
					// remove from stores
					SETsMu.Lock()
					delete(SETs, k)
					SETsMu.Unlock()
					HSETsMu.Lock()
					delete(HSETs, k)
					HSETsMu.Unlock()
					delete(EXPIREs, k)
				}
			}
			EXPIREsMu.Unlock()
		}
	}()
}

// cleanupIfExpired checks expiration for a key and removes it if expired.
func cleanupIfExpired(key string) bool {
	EXPIREsMu.RLock()
	exp, ok := EXPIREs[key]
	EXPIREsMu.RUnlock()
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		// delete key from stores and expiry map
		SETsMu.Lock()
		delete(SETs, key)
		SETsMu.Unlock()

		HSETsMu.Lock()
		delete(HSETs, key)
		HSETsMu.Unlock()

		EXPIREsMu.Lock()
		delete(EXPIREs, key)
		EXPIREsMu.Unlock()
		return true
	}
	return false
}

// set stores a string value by key.
func set(args []Value) Value {
	if len(args) != 2 {
		return Value{typ: "error", str: "Too many arguments"}
	}

	key := args[0].bulk
	value := args[1].bulk

	SETsMu.Lock()
	SETs[key] = value
	SETsMu.Unlock()
	return Value{typ: "string", str: "OK"}
}

// get returns the string value stored at a key.
func get(args []Value) Value {
	if len(args) != 1 {
		return Value{typ: "error", str: "Invalid number of args for get"}
	}

	key := args[0].bulk

	// remove if expired
	cleanupIfExpired(key)

	SETsMu.RLock()
	value, ok := SETs[key]
	SETsMu.RUnlock()

	if !ok {
		return Value{typ: "null", str: "Key not found"}
	}

	return Value{typ: "bulk", bulk: value}
}

// HSETs stores hash names with their field and value mappings.
var HSETs = map[string]map[string]string{}

// HSETsMu protects concurrent access to HSETs.
var HSETsMu = sync.RWMutex{}

// hset stores a field value inside a hash.
func hset(args []Value) Value {
	if len(args) != 3 {
		return Value{typ: "error", str: "Too many arguments"}
	}
	hash := args[0].bulk
	key := args[1].bulk
	value := args[2].bulk

	HSETsMu.Lock()
	if _, ok := HSETs[hash]; !ok {
		HSETs[hash] = map[string]string{}
	}

	HSETs[hash][key] = value
	HSETsMu.Unlock()

	return Value{typ: "string", str: "OK"}
}

// hget returns a field value from a hash.
func hget(args []Value) Value {
	if len(args) != 2 {
		return Value{typ: "error", str: "Too many arguments"}
	}
	hash := args[0].bulk
	key := args[1].bulk

	// cleanup hash if expired
	cleanupIfExpired(hash)

	HSETsMu.RLock()
	value, ok := HSETs[hash][key]
	HSETsMu.RUnlock()

	if !ok {
		return Value{typ: "null"}
	}

	return Value{typ: "bulk", bulk: value}
}

// exists returns 1 when a string key or hash key exists, otherwise 0.
func exists(args []Value) Value {
	if len(args) != 1 {
		return Value{typ: "error", str: "Too many arguments"}
	}
	key := args[0].bulk

	// cleanup if expired
	cleanupIfExpired(key)

	SETsMu.RLock()
	_, stringExists := SETs[key]
	SETsMu.RUnlock()

	HSETsMu.RLock()
	_, hashExists := HSETs[key]
	HSETsMu.RUnlock()

	if stringExists || hashExists {
		return Value{typ: "integer", num: 1}
	}

	return Value{typ: "integer", num: 0}
}

// del removes a string key or hash key.
func del(args []Value) Value {
	if len(args) != 1 {
		return Value{typ: "error", str: "Too many arguments"}
	}
	key := args[0].bulk
	deleted := false

	// cleanup first if expired
	cleanupIfExpired(key)

	SETsMu.Lock()
	if _, ok := SETs[key]; ok {
		delete(SETs, key)
		deleted = true
	}
	SETsMu.Unlock()

	HSETsMu.Lock()
	if _, ok := HSETs[key]; ok {
		delete(HSETs, key)
		deleted = true
	}
	HSETsMu.Unlock()

	EXPIREsMu.Lock()
	if _, ok := EXPIREs[key]; ok {
		delete(EXPIREs, key)
	}
	EXPIREsMu.Unlock()

	if deleted {
		return Value{typ: "integer", num: 1}
	}

	return Value{typ: "integer", num: 0}
}

// incr increments a string value as an integer by 1.
func incr(args []Value) Value {
	if len(args) != 1 {
		return Value{typ: "error", str: "Too many arguments"}
	}
	return incrementBy(args[0].bulk, 1)
}

// incrBy increments a string value as an integer by the provided amount.
func incrBy(args []Value) Value {
	if len(args) != 2 {
		return Value{typ: "error", str: "Too many arguments"}
	}
	amount, err := strconv.Atoi(args[1].bulk)
	if err != nil {
		return Value{typ: "error", str: "Invalid, must be integer"}
	}
	return incrementBy(args[0].bulk, amount)
}

// incrementBy helper that increments key values by amount
func incrementBy(key string, amount int) Value {
	// cleanup if expired
	cleanupIfExpired(key)

	SETsMu.Lock()
	defer SETsMu.Unlock()

	current := 0

	if value, ok := SETs[key]; ok {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return Value{typ: "error", str: "Invalid, not an integer"}
		}
		current = parsed
	}

	next := current + amount
	SETs[key] = strconv.Itoa(next)

	return Value{typ: "integer", num: next}
}

// expire sets a TTL in seconds on a key.
func expire(args []Value) Value {
	if len(args) != 2 {
		return Value{typ: "error", str: "Too many arguments"}
	}
	key := args[0].bulk
	seconds, err := strconv.Atoi(args[1].bulk)
	if err != nil {
		return Value{typ: "error", str: "Invalid, must be integer"}
	}

	// only set expiry if key exists
	SETsMu.RLock()
	_, stringExists := SETs[key]
	SETsMu.RUnlock()

	HSETsMu.RLock()
	_, hashExists := HSETs[key]
	HSETsMu.RUnlock()

	if !stringExists && !hashExists {
		return Value{typ: "integer", num: 0}
	}

	expireAt := time.Now().Add(time.Duration(seconds) * time.Second)
	EXPIREsMu.Lock()
	EXPIREs[key] = expireAt
	EXPIREsMu.Unlock()

	return Value{typ: "integer", num: 1}
}

// ttl returns remaining TTL in seconds, or -2 if key does not exist, -1 if no ttl.
func ttl(args []Value) Value {
	if len(args) != 1 {
		return Value{typ: "error", str: "Too many arguments"}
	}
	key := args[0].bulk

	// if key missing
	SETsMu.RLock()
	_, stringExists := SETs[key]
	SETsMu.RUnlock()

	HSETsMu.RLock()
	_, hashExists := HSETs[key]
	HSETsMu.RUnlock()

	if !stringExists && !hashExists {
		return Value{typ: "integer", num: -2}
	}

	EXPIREsMu.RLock()
	exp, ok := EXPIREs[key]
	EXPIREsMu.RUnlock()

	if !ok {
		return Value{typ: "integer", num: -1}
	}

	secs := int(time.Until(exp).Seconds())
	if secs < 0 {
		// expired
		cleanupIfExpired(key)
		return Value{typ: "integer", num: -2}
	}
	return Value{typ: "integer", num: secs}
}

// keys returns all keys matching a glob pattern.
func keys(args []Value) Value {
	if len(args) != 1 {
		return Value{typ: "error", str: "Too many arguments"}
	}
	pattern := args[0].bulk

	// collect keys from both stores
	set := map[string]struct{}{}

	SETsMu.RLock()
	for k := range SETs {
		set[k] = struct{}{}
	}
	SETsMu.RUnlock()

	HSETsMu.RLock()
	for k := range HSETs {
		set[k] = struct{}{}
	}
	HSETsMu.RUnlock()

	// filter and sort for deterministic order
	var matches []string
	for k := range set {
		m, _ := path.Match(pattern, k)
		if m {
			// skip expired
			if cleanupIfExpired(k) {
				continue
			}
			matches = append(matches, k)
		}
	}
	sort.Strings(matches)

	vals := make([]Value, len(matches))
	for i, k := range matches {
		vals[i] = Value{typ: "bulk", bulk: k}
	}

	return Value{typ: "array", array: vals}
}
