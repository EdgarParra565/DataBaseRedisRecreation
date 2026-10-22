package main

import (
	"path/filepath"
	"reflect"
	"strings"
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

func TestAofReplayRestoresVectors(t *testing.T) {
	resetStores()

	path := filepath.Join(t.TempDir(), "database.aof")
	aof, err := NewAof(path)
	if err != nil {
		t.Fatal(err)
	}
	defer aof.Close()

	commands := [][]string{
		{"VADD", "a", "2", "1", "0"},
		{"VADD", "b", "2", "0", "1"},
		{"VADD", "c", "2", "1", "1"},
		{"VDEL", "b"},
	}
	for _, args := range commands {
		if err := aof.Write(Value{typ: "array", array: bulkArgs(args...)}); err != nil {
			t.Fatal(err)
		}
	}

	// wipe memory so only the replay can restore vectors
	resetStores()

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

	got := vsimq(bulkArgs("2", "1", "0", "10"))
	if got.typ != "array" || len(got.array) != 4 {
		t.Fatalf("expected 2 vectors after replay, got %v", got)
	}
	if got.array[0].bulk != "a" || got.array[2].bulk != "c" {
		t.Fatalf("expected a then c after replay, got %v", got.array)
	}
}
