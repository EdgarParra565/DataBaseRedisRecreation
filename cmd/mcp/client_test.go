package main

import (
	"bufio"
	"io"
	"net"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// fakeServer answers RESP commands with raw replies from reply and records the commands it saw.
type fakeServer struct {
	addr     string
	mu       sync.Mutex
	commands [][]string
}

// startFakeServer listens on a random port so tests do not need the real server running.
func startFakeServer(t *testing.T, reply func(args []string) string) *fakeServer {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })

	fs := &fakeServer{addr: l.Addr().String()}
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go fs.serve(conn, reply)
		}
	}()
	return fs
}

// serve reads one command at a time and writes back its canned reply.
func (fs *fakeServer) serve(conn net.Conn, reply func(args []string) string) {
	defer conn.Close()
	rd := bufio.NewReader(conn)

	for {
		args, err := readCommand(rd)
		if err != nil {
			return
		}
		fs.mu.Lock()
		fs.commands = append(fs.commands, args)
		fs.mu.Unlock()

		conn.Write([]byte(reply(args)))
	}
}

// lastCommand returns the most recent command the fake server received.
func (fs *fakeServer) lastCommand() []string {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if len(fs.commands) == 0 {
		return nil
	}
	return fs.commands[len(fs.commands)-1]
}

// readCommand parses a RESP array of bulk strings.
func readCommand(rd *bufio.Reader) ([]string, error) {
	line, err := rd.ReadString('\n')
	if err != nil {
		return nil, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(line[1:]))
	if err != nil {
		return nil, err
	}

	args := make([]string, n)
	for i := range args {
		line, err := rd.ReadString('\n')
		if err != nil {
			return nil, err
		}
		size, err := strconv.Atoi(strings.TrimSpace(line[1:]))
		if err != nil {
			return nil, err
		}
		bulk := make([]byte, size+2)
		if _, err := io.ReadFull(rd, bulk); err != nil {
			return nil, err
		}
		args[i] = string(bulk[:size])
	}
	return args, nil
}

func TestMarshalCommand(t *testing.T) {
	got := string(marshalCommand([]string{"SET", "name", "edgar"}))
	want := "*3\r\n$3\r\nSET\r\n$4\r\nname\r\n$5\r\nedgar\r\n"

	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestClientDoReplies(t *testing.T) {
	tests := []struct {
		name  string
		reply string
		want  any
	}{
		{name: "simple string", reply: "+OK\r\n", want: "OK"},
		{name: "integer", reply: ":42\r\n", want: 42},
		{name: "bulk", reply: "$5\r\nhello\r\n", want: "hello"},
		{name: "null", reply: "$-1\r\n", want: nil},
		{name: "array", reply: "*2\r\n$1\r\na\r\n$1\r\nb\r\n", want: []any{"a", "b"}},
		{name: "empty array", reply: "*0\r\n", want: []any{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := startFakeServer(t, func(args []string) string { return tt.reply })
			client := NewClient(fs.addr)
			defer client.Close()

			got, err := client.Do("GET", "name")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("expected %#v, got %#v", tt.want, got)
			}
			if !reflect.DeepEqual(fs.lastCommand(), []string{"GET", "name"}) {
				t.Fatalf("expected GET name to be sent, got %v", fs.lastCommand())
			}
		})
	}
}

func TestClientDoErrorReply(t *testing.T) {
	fs := startFakeServer(t, func(args []string) string { return "-ERR Invalid, not an integer\r\n" })
	client := NewClient(fs.addr)
	defer client.Close()

	_, err := client.Do("INCR", "name")

	if _, ok := err.(RedisError); !ok {
		t.Fatalf("expected RedisError, got %v", err)
	}
	if err.Error() != "ERR Invalid, not an integer" {
		t.Fatalf("expected server message, got %q", err.Error())
	}
}

func TestClientDoReusesConnection(t *testing.T) {
	fs := startFakeServer(t, func(args []string) string { return "+PONG\r\n" })
	client := NewClient(fs.addr)
	defer client.Close()

	for i := 0; i < 3; i++ {
		if _, err := client.Do("PING"); err != nil {
			t.Fatal(err)
		}
	}

	fs.mu.Lock()
	defer fs.mu.Unlock()
	if len(fs.commands) != 3 {
		t.Fatalf("expected 3 commands, got %d", len(fs.commands))
	}
}

func TestClientDoServerDown(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()

	client := NewClient(addr)

	if _, err := client.Do("PING"); err == nil {
		t.Fatal("expected dial error when server is down")
	}
}
