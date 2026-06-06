package main

import "testing"

func TestValueMarshalNull(t *testing.T) {
	value := Value{typ: "null"}

	got := string(value.Marshal())
	want := "$-1\r\n"

	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestValueMarshalError(t *testing.T) {
	value := Value{typ: "error", str: "unknown command"}

	got := string(value.Marshal())
	want := "-ERR unknown command\r\n"

	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestValueMarshalBulk(t *testing.T) {
	value := Value{typ: "bulk", bulk: "hello"}

	got := string(value.Marshal())
	want := "$5\r\nhello\r\n"

	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestValueMarshalInteger(t *testing.T) {
	value := Value{typ: "integer", num: 1}

	got := string(value.Marshal())
	want := ":1\r\n"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
