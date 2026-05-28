package main

import (
	"fmt"
	"net"
	"strings"
)

// main starts the TCP server and responds to client messages with OK.
func main() {
	fmt.Println("Listening on port:6379")

	// Creates new server
	l, err := net.Listen("tcp", ":6379")
	if err != nil {
		fmt.Println(err)
		return
	}

	conn, err := l.Accept()
	if err != nil {
		fmt.Println(err)
		return
	}

	defer conn.Close()

	for {
		resp := NewResp(conn)
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

		writer := NewWriter(conn)

		handler, ok := Handlers[command]

		if !ok {
			fmt.Println("Invalid command:", command)
			writer.Write(Value{typ: "string", str: "OK"})
			continue
		}

		result := handler(args)
		writer.Write(result)

	}
}
