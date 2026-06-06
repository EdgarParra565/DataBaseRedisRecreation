package main

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestAofReadReplaysWrittenCommands(t *testing.T) {
	// t.TempDir gives this test its own disposable AOF file.
	path := filepath.Join(t.TempDir(), "database.aof")

	aof, err := NewAof(path)
	if err != nil {
		t.Fatal(err)
	}
	defer aof.Close()

	command := Value{
		typ: "array",
		array: []Value{
			{typ: "bulk", bulk: "SET"},
			{typ: "bulk", bulk: "name"},
			{typ: "bulk", bulk: "edgar"},
		},
	}
	if err := aof.Write(command); err != nil {
		t.Fatal(err)
	}

	var replayed []Value
	err = aof.Read(func(value Value) {
		replayed = append(replayed, value)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(replayed) != 1 {
		t.Fatal("expected 1 replayed command, got ", len(replayed))
	}
	if !reflect.DeepEqual(replayed[0], command) {
		t.Fatal("expected command, got ", command, replayed[0])
	}
}
