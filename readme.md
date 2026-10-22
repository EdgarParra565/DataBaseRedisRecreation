# DataBaseRedisRecreation

A small Redis-like database server written in Go. The project recreates a subset of Redis behavior by parsing RESP messages, dispatching commands, storing values in memory, and writing RESP responses back to the client.

## Features

- TCP server listening on port `6379`
- Concurrent client handling with one goroutine per connection
- RESP parser for arrays and bulk strings
- RESP writer for strings, bulk strings, integers, nulls, errors, and arrays
- In-memory string storage with `SET` and `GET`
- In-memory hash storage with `HSET` and `HGET`
- Key existence checks with `EXISTS`
- Basic `PING` command support
- Vector storage with `VADD` and k-nearest-neighbour search with `VSIM` / `VSIMQ` (cosine, L2, dot)
- Append-only file persistence for successful mutating commands like `SET`, `HSET`, and `VADD`
- MCP server in `cmd/mcp` so AI agents can read and write the database through tools
- Unit tests for handlers, RESP marshaling, AOF replay, vectors, and the MCP server

## Supported Commands

| Command | Example | Description |
| --- | --- | --- |
| `PING` | `PING` | Returns `PONG`. |
| `SET` | `SET name edgar` | Stores a string value by key. |
| `GET` | `GET name` | Returns the value stored for a key. |
| `EXISTS` | `EXISTS name` | Returns `1` if a string key or hash key exists, otherwise `0`. |
| `HSET` | `HSET user:1 name edgar` | Stores a field value inside a hash. |
| `HGET` | `HGET user:1 name` | Returns a field value from a hash. |
| `VADD` | `VADD doc:1 3 0.1 0.5 0.9` | Stores a float32 vector of the given dimension. Returns `1` if new, `0` if replaced. |
| `VGET` | `VGET doc:1` | Returns the vector as an array of floats. |
| `VDIM` | `VDIM doc:1` | Returns the dimension of a stored vector. |
| `VSIM` | `VSIM doc:1 5 METRIC COSINE` | Returns the `k` nearest vectors to the one at key (including itself) as `[key, score, ...]`. |
| `VSIMQ` | `VSIMQ 3 0.1 0.5 0.9 5 METRIC L2` | Returns the `k` nearest vectors to a raw query vector. |
| `VDEL` | `VDEL doc:1` | Removes a vector. `DEL`, `EXISTS`, `KEYS`, `EXPIRE`, and `TTL` also work on vector keys. |

## Project Structure

| File | Purpose |
| --- | --- |
| `go.mod` | Defines the Go module so the project can be run with `go run .`. |
| `main.go` | Starts the TCP server, replays persisted commands, accepts client connections, and calls handlers. |
| `resp.go` | Defines RESP values, parsing logic, marshaling logic, and response writing. |
| `handler.go` | Defines command handlers and the in-memory stores used by commands. |
| `vector.go` | Defines the vector store, vector commands, distance functions, and top-k search. |
| `aof.go` | Defines append-only file write and replay behavior. |
| `cmd/mcp/` | MCP server that connects to the database over TCP and exposes commands as tools. |
| `*_test.go` | Contains unit tests for protocol output, handlers, and AOF replay. |

## Running the Server

Run the server from the project root:

```bash
go run .
```

The server listens on:

```text
localhost:6379
```

## Testing With redis-cli

In another terminal, connect with `redis-cli`:

```bash
redis-cli -p 6379
```

Then run commands:

```text
PING
SET name edgar
GET name
HSET user:1 name edgar
HGET user:1 name
EXISTS name
EXISTS missing
```

## Running Tests

Run the full test suite, including the MCP server:

```bash
go test -v ./...
```

Run one test by name:

```bash
go test -v -run TestValueMarshalInteger .
```

Run the vector search benchmark (10k random 128-dim vectors, top 10):

```bash
go test -run xxx -bench VSIM .
```

On an Apple M4 Max a brute force `VSIMQ` takes about 0.8 ms.

## Vector Search

Vectors are searched by brute force with a bounded heap, so a query is `O(n log k)`. Scores are cosine similarity and dot product (higher is better) or L2 distance (lower is better). Vectors with a different dimension than the query are skipped.

```text
VADD east 2 1 0
VADD north 2 0 1
VADD northeast 2 1 1
VSIMQ 2 1 0.2 2
```

## MCP Server

`cmd/mcp` is a separate binary that speaks MCP over stdio and forwards tool calls to the database over RESP, so agents like Claude or Cursor can use it.

Tools: `redis_get`, `redis_set`, `redis_del`, `redis_exists`, `redis_hget`, `redis_hset`, `redis_incr`, `redis_incrby`, `redis_expire`, `redis_ttl`, `redis_keys`, `vector_add`, `vector_search`.
Resources: `redis://keys` and `redis://key/{name}`. Prompt: `summarize_store`.

Build it with the server running on `localhost:6379`:

```bash
go build -o bin/redis-mcp ./cmd/mcp
```

Add it to `.cursor/mcp.json` or the Claude Desktop config:

```json
{ "mcpServers": { "redis-clone": { "command": "/abs/path/bin/redis-mcp", "args": ["--addr", "localhost:6379"] } } }
```

Flags:

- `--addr` address of the database, defaults to `localhost:6379`
- `--readonly` registers only read tools
- `--allow-keys-prefix agent:` only allows keys starting with the prefix

Try it with the MCP Inspector:

```bash
npx @modelcontextprotocol/inspector go run ./cmd/mcp
```

## Persistence

The server writes successful mutating commands to `database.aof`. On startup, it replays commands from that file to rebuild the in-memory stores.

Example manual persistence check:

```bash
redis-cli -p 6379 SET name edgar
redis-cli -p 6379 GET name
```

Restart the server, then run:

```bash
redis-cli -p 6379 GET name
```

If AOF replay worked, the value should still be available.

## Current Limitations

- Only a small subset of Redis commands is implemented.
- AOF persistence covers `SET`, `HSET`, `DEL`, `INCR`, `INCRBY`, `EXPIRE`, `VADD`, and `VDEL`.
- Vector search is brute force; there is no HNSW or IVF index yet.
- Lists, pub/sub, transactions, snapshots, and replication are not implemented yet.
- RESP parsing currently focuses on the types needed by the supported commands.
