package main

import (
	"context"
	"flag"
	"log"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// main runs the MCP server over stdio and forwards tool calls to the Redis clone.
func main() {
	addr := flag.String("addr", "localhost:6379", "address of the Redis clone")
	readOnly := flag.Bool("readonly", false, "register only read tools")
	keyPrefix := flag.String("allow-keys-prefix", "", "only allow keys starting with this prefix")
	flag.Parse()

	client := NewClient(*addr)
	defer client.Close()

	server := newServer(client, Options{ReadOnly: *readOnly, KeyPrefix: *keyPrefix})

	// stdout carries the protocol, so errors go to stderr through log
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}
