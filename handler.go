package main

import "sync"

var Handlers = map[string]func([]Value) Value{
	"PING": ping,
	"SET":  set,
	"GET":  get,
}

func ping(args []Value) Value {
	return Value{typ: "string", str: "PONG"}
}

var SETs = map[string]string{}
var SETsMu = sync.RWMutex{}

// set is a Hash Map method that stores a value and sets a key equal to it
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

// get is a Hash Map method that takes a key and returns the value
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
