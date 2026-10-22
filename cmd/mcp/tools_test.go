package main

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// connect starts the MCP server against a fake backend and returns a client session over an in-memory transport.
func connect(t *testing.T, opts Options, reply func(args []string) string) (*mcp.ClientSession, *fakeServer) {
	fs := startFakeServer(t, reply)
	client := NewClient(fs.addr)
	t.Cleanup(func() { client.Close() })

	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := newServer(client, opts).Connect(ctx, serverTransport, nil); err != nil {
		t.Fatal(err)
	}

	session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0.0.1"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })

	return session, fs
}

// callTool calls a tool and returns its text output and whether it was a tool error.
func callTool(t *testing.T, session *mcp.ClientSession, name string, args map[string]any) (string, bool) {
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Content) == 0 {
		t.Fatalf("expected content from %s, got none", name)
	}
	return res.Content[0].(*mcp.TextContent).Text, res.IsError
}

// toolNames lists the tools the server registered.
func toolNames(t *testing.T, session *mcp.ClientSession) []string {
	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(res.Tools))
	for i, tool := range res.Tools {
		names[i] = tool.Name
	}
	return names
}

func TestToolsList(t *testing.T) {
	session, _ := connect(t, Options{}, func(args []string) string { return "+OK\r\n" })

	names := strings.Join(toolNames(t, session), ",")

	for _, want := range []string{"redis_get", "redis_set", "redis_keys", "vector_add", "vector_search"} {
		if !strings.Contains(names, want) {
			t.Fatalf("expected %s in tools list, got %s", want, names)
		}
	}
}

func TestToolsListReadOnly(t *testing.T) {
	session, _ := connect(t, Options{ReadOnly: true}, func(args []string) string { return "+OK\r\n" })

	names := strings.Join(toolNames(t, session), ",")

	if strings.Contains(names, "redis_set") || strings.Contains(names, "vector_add") {
		t.Fatalf("expected no write tools in readonly mode, got %s", names)
	}
	if !strings.Contains(names, "redis_get") {
		t.Fatalf("expected read tools in readonly mode, got %s", names)
	}
}

func TestToolCallGet(t *testing.T) {
	session, fs := connect(t, Options{}, func(args []string) string { return "$5\r\nedgar\r\n" })

	got, isError := callTool(t, session, "redis_get", map[string]any{"key": "name"})

	if isError || got != `"edgar"` {
		t.Fatalf("expected \"edgar\", got %s (isError=%v)", got, isError)
	}
	if !reflect.DeepEqual(fs.lastCommand(), []string{"GET", "name"}) {
		t.Fatalf("expected GET name, got %v", fs.lastCommand())
	}
}

func TestToolCallIncrBy(t *testing.T) {
	session, fs := connect(t, Options{}, func(args []string) string { return ":15\r\n" })

	got, isError := callTool(t, session, "redis_incrby", map[string]any{"key": "count", "amount": 5})

	if isError || got != "15" {
		t.Fatalf("expected 15, got %s (isError=%v)", got, isError)
	}
	if !reflect.DeepEqual(fs.lastCommand(), []string{"INCRBY", "count", "5"}) {
		t.Fatalf("expected INCRBY count 5, got %v", fs.lastCommand())
	}
}

func TestToolServerErrorIsToolError(t *testing.T) {
	session, _ := connect(t, Options{}, func(args []string) string { return "-ERR Invalid, not an integer\r\n" })

	got, isError := callTool(t, session, "redis_incr", map[string]any{"key": "name"})

	if !isError || !strings.Contains(got, "not an integer") {
		t.Fatalf("expected tool error with server message, got %s (isError=%v)", got, isError)
	}
}

func TestToolArgumentValidation(t *testing.T) {
	session, fs := connect(t, Options{}, func(args []string) string { return "+OK\r\n" })

	tests := []struct {
		name string
		tool string
		args map[string]any
	}{
		{name: "missing key", tool: "redis_get", args: map[string]any{}},
		{name: "missing value", tool: "redis_set", args: map[string]any{"key": "name"}},
		{name: "non-integer amount", tool: "redis_incrby", args: map[string]any{"key": "count", "amount": "five"}},
		{name: "non-integer seconds", tool: "redis_expire", args: map[string]any{"key": "count", "seconds": 1.5}},
		{name: "vector not array", tool: "vector_add", args: map[string]any{"key": "doc", "vector": "1,2"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, isError := callTool(t, session, tt.tool, tt.args)
			if !isError {
				t.Fatalf("expected tool error for %v", tt.args)
			}
		})
	}

	if fs.lastCommand() != nil {
		t.Fatalf("expected invalid input to never reach the server, got %v", fs.lastCommand())
	}
}

func TestKeyPrefixRejected(t *testing.T) {
	session, fs := connect(t, Options{KeyPrefix: "agent:"}, func(args []string) string { return "+OK\r\n" })

	got, isError := callTool(t, session, "redis_set", map[string]any{"key": "user:1", "value": "x"})

	if !isError || !strings.Contains(got, "outside the allowed prefix") {
		t.Fatalf("expected prefix error, got %s (isError=%v)", got, isError)
	}
	if fs.lastCommand() != nil {
		t.Fatalf("expected rejected key to never reach the server, got %v", fs.lastCommand())
	}

	_, isError = callTool(t, session, "redis_set", map[string]any{"key": "agent:1", "value": "x"})
	if isError {
		t.Fatal("expected key inside the prefix to be allowed")
	}
}

func TestKeysFilteredByPrefix(t *testing.T) {
	session, _ := connect(t, Options{KeyPrefix: "agent:"}, func(args []string) string {
		return "*3\r\n$7\r\nagent:1\r\n$7\r\nagent:2\r\n$6\r\nuser:1\r\n"
	})

	got, isError := callTool(t, session, "redis_keys", map[string]any{"pattern": "*"})

	if isError || got != `["agent:1","agent:2"]` {
		t.Fatalf("expected only agent keys, got %s (isError=%v)", got, isError)
	}
}

func TestVectorAdd(t *testing.T) {
	session, fs := connect(t, Options{}, func(args []string) string { return ":1\r\n" })

	got, isError := callTool(t, session, "vector_add", map[string]any{"key": "doc:1", "vector": []float64{0.5, -1, 2}})

	if isError || got != "1" {
		t.Fatalf("expected 1, got %s (isError=%v)", got, isError)
	}
	if !reflect.DeepEqual(fs.lastCommand(), []string{"VADD", "doc:1", "3", "0.5", "-1", "2"}) {
		t.Fatalf("expected VADD doc:1 3 0.5 -1 2, got %v", fs.lastCommand())
	}
}

func TestVectorSearch(t *testing.T) {
	session, fs := connect(t, Options{}, func(args []string) string {
		return "*4\r\n$5\r\ndoc:1\r\n$3\r\n0.9\r\n$5\r\ndoc:2\r\n$3\r\n0.5\r\n"
	})

	got, isError := callTool(t, session, "vector_search", map[string]any{"vector": []float64{1, 0}, "k": 2, "metric": "L2"})

	want := `[{"key":"doc:1","score":0.9},{"key":"doc:2","score":0.5}]`
	if isError || got != want {
		t.Fatalf("expected %s, got %s (isError=%v)", want, got, isError)
	}
	if !reflect.DeepEqual(fs.lastCommand(), []string{"VSIMQ", "2", "1", "0", "2", "METRIC", "L2"}) {
		t.Fatalf("expected VSIMQ 2 1 0 2 METRIC L2, got %v", fs.lastCommand())
	}
}

func TestKeysResource(t *testing.T) {
	session, _ := connect(t, Options{}, func(args []string) string { return "*1\r\n$4\r\nname\r\n" })

	res, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "redis://keys"})
	if err != nil {
		t.Fatal(err)
	}

	if len(res.Contents) != 1 || res.Contents[0].Text != `["name"]` {
		t.Fatalf("expected [\"name\"], got %v", res.Contents)
	}
}

func TestKeyResourceFallsBackToVector(t *testing.T) {
	session, _ := connect(t, Options{}, func(args []string) string {
		if args[0] == "GET" {
			return "$-1\r\n"
		}
		return "*2\r\n$1\r\n1\r\n$1\r\n0\r\n"
	})

	res, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "redis://key/doc:1"})
	if err != nil {
		t.Fatal(err)
	}

	if len(res.Contents) != 1 || res.Contents[0].Text != `["1","0"]` {
		t.Fatalf("expected vector values, got %v", res.Contents)
	}
}

func TestSummarizeStorePrompt(t *testing.T) {
	session, _ := connect(t, Options{}, func(args []string) string { return "*2\r\n$4\r\nname\r\n$6\r\nuser:1\r\n" })

	res, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{Name: "summarize_store"})
	if err != nil {
		t.Fatal(err)
	}

	text := res.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(text, "name\nuser:1") {
		t.Fatalf("expected keys in prompt, got %q", text)
	}
}
