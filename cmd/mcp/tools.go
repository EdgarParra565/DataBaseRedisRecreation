package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Options controls which tools are registered and which keys they may touch.
type Options struct {
	ReadOnly  bool
	KeyPrefix string
}

// KeyInput is the input for tools that take a single key.
type KeyInput struct {
	Key string `json:"key" jsonschema:"the key to operate on"`
}

// SetInput is the input for redis_set.
type SetInput struct {
	Key   string `json:"key" jsonschema:"the key to store the value under"`
	Value string `json:"value" jsonschema:"the string value to store"`
}

// HGetInput is the input for redis_hget.
type HGetInput struct {
	Hash  string `json:"hash" jsonschema:"the hash name"`
	Field string `json:"field" jsonschema:"the field inside the hash"`
}

// HSetInput is the input for redis_hset.
type HSetInput struct {
	Hash  string `json:"hash" jsonschema:"the hash name"`
	Field string `json:"field" jsonschema:"the field inside the hash"`
	Value string `json:"value" jsonschema:"the string value to store"`
}

// IncrByInput is the input for redis_incrby.
type IncrByInput struct {
	Key    string `json:"key" jsonschema:"the key holding an integer"`
	Amount int    `json:"amount" jsonschema:"the amount to add, can be negative"`
}

// ExpireInput is the input for redis_expire.
type ExpireInput struct {
	Key     string `json:"key" jsonschema:"the key to expire"`
	Seconds int    `json:"seconds" jsonschema:"seconds until the key is deleted"`
}

// KeysInput is the input for redis_keys.
type KeysInput struct {
	Pattern string `json:"pattern" jsonschema:"glob pattern such as user:* or * for all keys"`
}

// VectorAddInput is the input for vector_add.
type VectorAddInput struct {
	Key    string    `json:"key" jsonschema:"the key to store the vector under"`
	Vector []float64 `json:"vector" jsonschema:"the embedding values"`
}

// VectorSearchInput is the input for vector_search.
type VectorSearchInput struct {
	Vector []float64 `json:"vector" jsonschema:"the query embedding, same dimension as the stored vectors"`
	K      int       `json:"k" jsonschema:"how many nearest vectors to return"`
	Metric string    `json:"metric,omitempty" jsonschema:"COSINE, L2, or DOT; defaults to COSINE"`
}

// Match is one vector_search result.
type Match struct {
	Key   string  `json:"key"`
	Score float64 `json:"score"`
}

// tools holds what every tool handler needs.
type tools struct {
	client *Client
	opts   Options
}

// newServer builds the MCP server with tools, resources, and prompts for the Redis clone.
func newServer(client *Client, opts Options) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "redis-clone", Version: "v0.1.0"}, nil)
	t := &tools{client: client, opts: opts}

	addTool(server, t, "redis_get", "Get the string value stored at key, or null if missing.", true,
		func(in KeyInput) ([]string, []string) { return []string{"GET", in.Key}, []string{in.Key} })
	addTool(server, t, "redis_exists", "Return 1 if key exists as a string, hash, or vector, otherwise 0.", true,
		func(in KeyInput) ([]string, []string) { return []string{"EXISTS", in.Key}, []string{in.Key} })
	addTool(server, t, "redis_hget", "Get one field from a hash, or null if missing.", true,
		func(in HGetInput) ([]string, []string) { return []string{"HGET", in.Hash, in.Field}, []string{in.Hash} })
	addTool(server, t, "redis_ttl", "Return seconds until key expires, -1 if it has no expiry, -2 if it does not exist.", true,
		func(in KeyInput) ([]string, []string) { return []string{"TTL", in.Key}, []string{in.Key} })
	addTool(server, t, "redis_set", "Store a string value at key, replacing any existing value.", false,
		func(in SetInput) ([]string, []string) { return []string{"SET", in.Key, in.Value}, []string{in.Key} })
	addTool(server, t, "redis_del", "Delete key. Returns 1 if something was deleted, otherwise 0.", false,
		func(in KeyInput) ([]string, []string) { return []string{"DEL", in.Key}, []string{in.Key} })
	addTool(server, t, "redis_hset", "Store a field value inside a hash.", false,
		func(in HSetInput) ([]string, []string) {
			return []string{"HSET", in.Hash, in.Field, in.Value}, []string{in.Hash}
		})
	addTool(server, t, "redis_incr", "Increment the integer at key by 1 and return the new value.", false,
		func(in KeyInput) ([]string, []string) { return []string{"INCR", in.Key}, []string{in.Key} })
	addTool(server, t, "redis_incrby", "Add amount to the integer at key and return the new value.", false,
		func(in IncrByInput) ([]string, []string) {
			return []string{"INCRBY", in.Key, strconv.Itoa(in.Amount)}, []string{in.Key}
		})
	addTool(server, t, "redis_expire", "Delete key automatically after the given number of seconds.", false,
		func(in ExpireInput) ([]string, []string) {
			return []string{"EXPIRE", in.Key, strconv.Itoa(in.Seconds)}, []string{in.Key}
		})
	addTool(server, t, "vector_add", "Store an embedding vector at key so it can be found by vector_search.", false,
		func(in VectorAddInput) ([]string, []string) {
			args := append([]string{"VADD", in.Key, strconv.Itoa(len(in.Vector))}, formatFloats(in.Vector)...)
			return args, []string{in.Key}
		})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "redis_keys",
		Description: "List keys matching a glob pattern, sorted.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in KeysInput) (*mcp.CallToolResult, any, error) {
		keys, err := t.keys(in.Pattern)
		if err != nil {
			return nil, nil, err
		}
		return textResult(keys)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "vector_search",
		Description: "Find the k stored vectors nearest to a query embedding. Returns keys with scores, best first.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in VectorSearchInput) (*mcp.CallToolResult, any, error) {
		matches, err := t.vectorSearch(in)
		if err != nil {
			return nil, nil, err
		}
		return textResult(matches)
	})

	server.AddResource(&mcp.Resource{
		URI:         "redis://keys",
		Name:        "keys",
		Description: "All keys in the store.",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		keys, err := t.keys("*")
		if err != nil {
			return nil, err
		}
		return jsonResource(req.Params.URI, keys)
	})

	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "redis://key/{+name}",
		Name:        "key",
		Description: "The string or vector value stored at a key.",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		key, err := url.PathUnescape(strings.TrimPrefix(req.Params.URI, "redis://key/"))
		if err != nil {
			return nil, err
		}
		value, err := t.value(key)
		if err != nil {
			return nil, err
		}
		if value == nil {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		return jsonResource(req.Params.URI, value)
	})

	server.AddPrompt(&mcp.Prompt{
		Name:        "summarize_store",
		Description: "Describe what is stored in the database based on its keys.",
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		keys, err := t.keys("*")
		if err != nil {
			return nil, err
		}
		text := "These are the keys in my Redis clone:\n" + strings.Join(keys, "\n") +
			"\n\nDescribe what kind of data is stored and how the keys are organized."
		return &mcp.GetPromptResult{
			Messages: []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: text}}},
		}, nil
	})

	return server
}

// addTool registers a tool that runs one command built from its input after checking its keys.
func addTool[In any](server *mcp.Server, t *tools, name, description string, readOnly bool, command func(In) (args []string, keys []string)) {
	if t.opts.ReadOnly && !readOnly {
		return
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        name,
		Description: description,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: readOnly},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
		args, keys := command(in)
		if err := t.checkKeys(keys...); err != nil {
			return nil, nil, err
		}
		reply, err := t.client.Do(args...)
		if err != nil {
			return nil, nil, err
		}
		return textResult(reply)
	})
}

// checkKeys rejects keys outside the allowed prefix.
func (t *tools) checkKeys(keys ...string) error {
	for _, key := range keys {
		if !strings.HasPrefix(key, t.opts.KeyPrefix) {
			return fmt.Errorf("key %q is outside the allowed prefix %q", key, t.opts.KeyPrefix)
		}
	}
	return nil
}

// keys runs KEYS and drops anything outside the allowed prefix.
func (t *tools) keys(pattern string) ([]string, error) {
	reply, err := t.client.Do("KEYS", pattern)
	if err != nil {
		return nil, err
	}
	items, _ := reply.([]any)

	keys := []string{}
	for _, item := range items {
		key, _ := item.(string)
		if strings.HasPrefix(key, t.opts.KeyPrefix) {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

// vectorSearch runs VSIMQ and turns the flat [key, score, ...] reply into matches.
func (t *tools) vectorSearch(in VectorSearchInput) ([]Match, error) {
	args := append([]string{"VSIMQ", strconv.Itoa(len(in.Vector))}, formatFloats(in.Vector)...)
	args = append(args, strconv.Itoa(in.K))
	if in.Metric != "" {
		args = append(args, "METRIC", in.Metric)
	}

	reply, err := t.client.Do(args...)
	if err != nil {
		return nil, err
	}
	items, _ := reply.([]any)

	matches := []Match{}
	for i := 0; i+1 < len(items); i += 2 {
		key, _ := items[i].(string)
		raw, _ := items[i+1].(string)
		score, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil, errors.New("invalid score in reply: " + raw)
		}
		// hide vectors outside the allowed prefix
		if strings.HasPrefix(key, t.opts.KeyPrefix) {
			matches = append(matches, Match{Key: key, Score: score})
		}
	}
	return matches, nil
}

// value returns the string at key, falling back to the vector at key, or nil if neither exists.
func (t *tools) value(key string) (any, error) {
	if err := t.checkKeys(key); err != nil {
		return nil, err
	}
	reply, err := t.client.Do("GET", key)
	if err != nil || reply != nil {
		return reply, err
	}
	return t.client.Do("VGET", key)
}

// formatFloats converts JSON numbers into the strings the server parses.
func formatFloats(values []float64) []string {
	strs := make([]string, len(values))
	for i, f := range values {
		strs[i] = strconv.FormatFloat(f, 'f', -1, 64)
	}
	return strs
}

// textResult converts a reply into JSON text content.
func textResult(reply any) (*mcp.CallToolResult, any, error) {
	data, err := json.Marshal(reply)
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(data)}}}, nil, nil
}

// jsonResource wraps a value as JSON resource contents.
func jsonResource(uri string, value any) (*mcp.ReadResourceResult, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{{URI: uri, MIMEType: "application/json", Text: string(data)}},
	}, nil
}
