package main

import (
	"bufio"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Client sends RESP commands to the Redis clone over one TCP connection.
type Client struct {
	addr string
	conn net.Conn
	rd   *bufio.Reader
	mu   sync.Mutex
}

// RedisError is an error reply sent back by the server.
type RedisError string

// Error returns the server's error message.
func (e RedisError) Error() string {
	return string(e)
}

// NewClient creates a client that dials addr on first use.
func NewClient(addr string) *Client {
	return &Client{addr: addr}
}

// Close closes the connection if one is open.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

// Do sends one command as an array of bulk strings and reads one reply.
func (c *Client) Do(args ...string) (any, error) {
	// one connection, so calls take turns
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		conn, err := net.Dial("tcp", c.addr)
		if err != nil {
			return nil, err
		}
		c.conn = conn
		c.rd = bufio.NewReader(conn)
	}

	reply, err := c.roundTrip(args)
	if err != nil {
		// drop the connection so the next call redials
		c.conn.Close()
		c.conn = nil
		return nil, err
	}
	if e, ok := reply.(RedisError); ok {
		return nil, e
	}

	return reply, nil
}

// roundTrip writes a command and reads its reply with a deadline so a silent server cannot hang a tool call.
func (c *Client) roundTrip(args []string) (any, error) {
	if err := c.conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return nil, err
	}
	if _, err := c.conn.Write(marshalCommand(args)); err != nil {
		return nil, err
	}
	return readReply(c.rd)
}

// marshalCommand converts args into a RESP array of bulk strings.
func marshalCommand(args []string) []byte {
	var bytes []byte
	bytes = append(bytes, '*')
	bytes = append(bytes, strconv.Itoa(len(args))...)
	bytes = append(bytes, '\r', '\n')

	for _, arg := range args {
		bytes = append(bytes, '$')
		bytes = append(bytes, strconv.Itoa(len(arg))...)
		bytes = append(bytes, '\r', '\n')
		bytes = append(bytes, arg...)
		bytes = append(bytes, '\r', '\n')
	}

	return bytes
}

// readReply parses one RESP reply into plain Go values that marshal cleanly to JSON.
func readReply(rd *bufio.Reader) (any, error) {
	line, err := rd.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimSuffix(line, "\r\n")
	if line == "" {
		return nil, errors.New("empty reply")
	}

	switch line[0] {
	case '+':
		return line[1:], nil
	case '-':
		return RedisError(line[1:]), nil
	case ':':
		return strconv.Atoi(line[1:])
	case '$':
		n, err := strconv.Atoi(line[1:])
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, nil
		}
		bulk := make([]byte, n+2)
		if _, err := io.ReadFull(rd, bulk); err != nil {
			return nil, err
		}
		return string(bulk[:n]), nil
	case '*':
		n, err := strconv.Atoi(line[1:])
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, nil
		}
		items := make([]any, n)
		for i := range items {
			item, err := readReply(rd)
			if err != nil {
				return nil, err
			}
			items[i] = item
		}
		return items, nil
	default:
		return nil, errors.New("unknown reply type: " + string(line[0]))
	}
}
