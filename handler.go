package main

import "sync"

// Handlers maps Redis command names to their command functions.
var Handlers = map[string]func([]Value) Value{
	"PING":   ping,
	"SET":    set,
	"GET":    get,
	"HSET":   hset,
	"HGET":   hget,
	"EXISTS": exists,
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
