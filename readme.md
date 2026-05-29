# DataBaseRedisRecreation

A small Redis-like database server written in Go. The project recreates a subset of Redis behavior by parsing RESP messages, dispatching commands, storing values in memory, and writing RESP responses back to the client.

## Features

- TCP server listening on port `6379`
- RESP parser for arrays and bulk strings
- RESP writer for strings, bulk strings, nulls, errors, and arrays
- In-memory string storage with `SET` and `GET`
- In-memory hash storage with `HSET` and `HGET`
- Basic `PING` command support
- Append-only file helper for command persistence work

## Supported Commands

| Command | Example | Description |
| --- | --- | --- |
| `PING` | `PING` | Returns `PONG`. |
| `SET` | `SET name edgar` | Stores a string value by key. |
| `GET` | `GET name` | Returns the value stored for a key. |
| `HSET` | `HSET user:1 name edgar` | Stores a field value inside a hash. |
| `HGET` | `HGET user:1 name` | Returns a field value from a hash. |

## Project Structure

| File | Purpose |
| --- | --- |
| `main.go` | Starts the TCP server, accepts a client connection, reads RESP commands, and calls handlers. |
| `resp.go` | Defines RESP values, parsing logic, marshaling logic, and response writing. |
| `handler.go` | Defines command handlers and the in-memory stores used by commands. |
| `aof.go` | Defines the append-only file helper used for persistence-related work. |

## Running the Server

This project does not currently include a `go.mod` file, so run it by passing the Go files directly:

```bash
go run main.go resp.go handler.go aof.go
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
```

## Notes

- The server currently accepts one client connection at a time.
- Data is stored in memory while the server is running.
- Append-only file support exists in `aof.go`, but persistence behavior is still a work in progress.
- Only a small subset of Redis commands and RESP types is implemented.
