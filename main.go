package main

import (
	"fmt"
	"net"
	"strings"
)

// main starts the TCP server, replays persisted commands, and accepts clients.
func main() {
	fmt.Println("Listening on port:6379")

	// Creates new server
	l, err := net.Listen("tcp", ":6379")
	if err != nil {
		fmt.Println(err)
		return
	}

	aof, err := NewAof("database.aof")
	if err != nil {
		fmt.Println(err)
		return
	}

	defer aof.Close()

	err = aof.Read(func(value Value) {
		if value.typ != "array" || len(value.array) == 0 {
			fmt.Println("Invalid AOF command")
			return
		}

		command := strings.ToUpper(value.array[0].bulk)
		args := value.array[1:]

		handler, ok := Handlers[command]
		if !ok {
			fmt.Println("Invalid command:", command)
			return
		}

		handler(args)
	})

	if err != nil {
		fmt.Println("AOF replay error:", err)
		return
	}

	for {
		conn, err := l.Accept()
		if err != nil {
			fmt.Println(err)
			continue
		}

		go handleConnection(conn, aof)
	}
}

// handleConnection owns the RESP reader and writer for one client connection.
func handleConnection(conn net.Conn, aof *Aof) {
	defer conn.Close()

	resp := NewResp(conn)
	writer := NewWriter(conn)

	for {
		value, err := resp.Read()
		if err != nil {
			fmt.Println(err)
			return
		}
		if value.typ != "array" {
			fmt.Println("Invalid type:", value.typ)
			continue
		}
		if len(value.array) == 0 {
			fmt.Println("Invalid array:", value.typ)
			continue
		}
		command := strings.ToUpper(value.array[0].bulk)
		args := value.array[1:]

		handler, ok := Handlers[command]
		if !ok {
			fmt.Println("Invalid command:", command)
			writer.Write(Value{typ: "error", str: "unknown command '" + command + "'"})
			continue
		}

		result := handler(args)
		if shouldPersist(command, result) {
			if err := aof.Write(value); err != nil {
				writer.Write(Value{typ: "error", str: "failed to persist"})
				continue
			}
		}
		writer.Write(result)
	}
}

// shouldPersist allows only successful mutating commands into the AOF.
func shouldPersist(command string, result Value) bool {
	if result.typ == "error" {
		return false
	}

	switch command {
	case "SET", "HSET":
		return true
	default:
		return false
	}
}
