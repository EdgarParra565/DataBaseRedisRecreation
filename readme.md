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
- Append-only file persistence for successful mutating commands like `SET` and `HSET`
- Unit tests for handlers, RESP marshaling, and AOF replay

## Supported Commands

| Command | Example | Description |
| --- | --- | --- |
| `PING` | `PING` | Returns `PONG`. |
| `SET` | `SET name edgar` | Stores a string value by key. |
| `GET` | `GET name` | Returns the value stored for a key. |
| `EXISTS` | `EXISTS name` | Returns `1` if a string key or hash key exists, otherwise `0`. |
| `HSET` | `HSET user:1 name edgar` | Stores a field value inside a hash. |
| `HGET` | `HGET user:1 name` | Returns a field value from a hash. |

## Project Structure

| File | Purpose |
| --- | --- |
| `go.mod` | Defines the Go module so the project can be run with `go run .`. |
| `main.go` | Starts the TCP server, replays persisted commands, accepts client connections, and calls handlers. |
| `resp.go` | Defines RESP values, parsing logic, marshaling logic, and response writing. |
| `handler.go` | Defines command handlers and the in-memory stores used by commands. |
| `aof.go` | Defines append-only file write and replay behavior. |
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

Run the full test suite:

```bash
go test -v .
```

Run one test by name:

```bash
go test -v -run TestValueMarshalInteger .
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
- AOF persistence currently covers `SET` and `HSET`.
- Expiration, deletion, lists, pub/sub, transactions, snapshots, and replication are not implemented yet.
- RESP parsing currently focuses on the types needed by the supported commands.
