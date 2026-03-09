package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"
)

// Sentinel errors for the client.
var (
	errConnectionClosed = errors.New("connection closed")
	errRPCError         = errors.New("rpc error")
)

// Client is a JSON-RPC client that communicates with the Atlas daemon via Unix socket.
type Client struct {
	conn    net.Conn
	encoder *json.Encoder
	scanner *bufio.Scanner
	nextID  int
}

// Dial connects to the daemon Unix socket with a 5-second timeout.
func Dial(socketPath string) (*Client, error) {
	return DialContext(context.Background(), socketPath)
}

// DialContext connects to the daemon Unix socket using the provided context with a 5-second timeout.
func DialContext(ctx context.Context, socketPath string) (*Client, error) {
	d := &net.Dialer{Timeout: 5 * time.Second}
	conn, err := d.DialContext(ctx, "unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("daemon not running at %s: %w", socketPath, err)
	}
	return &Client{
		conn:    conn,
		encoder: json.NewEncoder(conn),
		scanner: bufio.NewScanner(conn),
	}, nil
}

// Call sends a JSON-RPC request and decodes the response into result.
func (c *Client) Call(method string, params, result interface{}) error {
	c.nextID++
	req := &Request{
		JSONRPC: "2.0",
		Method:  method,
		ID:      c.nextID,
	}
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("marshal params: %w", err)
		}
		req.Params = json.RawMessage(b)
	}
	if err := c.encoder.Encode(req); err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	if !c.scanner.Scan() {
		return errConnectionClosed
	}
	var resp Response
	if err := json.Unmarshal(c.scanner.Bytes(), &resp); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	if resp.Error != nil {
		return fmt.Errorf("%w %d: %s", errRPCError, resp.Error.Code, resp.Error.Message)
	}
	if result != nil && resp.Result != nil {
		b, err := json.Marshal(resp.Result)
		if err != nil {
			return err
		}
		return json.Unmarshal(b, result)
	}
	return nil
}

// Ping checks if the daemon is alive. Returns true if alive.
func (c *Client) Ping() bool {
	var resp DaemonPingResponse
	return c.Call(MethodDaemonPing, nil, &resp) == nil && resp.Alive
}

// Close disconnects from the daemon.
func (c *Client) Close() error { return c.conn.Close() }

// PingSocket is a convenience function that tries to connect to the daemon socket and ping.
// Returns true if the daemon is reachable and responding.
func PingSocket(socketPath string) bool {
	c, err := Dial(socketPath)
	if err != nil {
		return false
	}
	defer func() { _ = c.Close() }()
	return c.Ping()
}

// DialFromConfig connects using the configured socket path.
// Expands "~/" to the home directory automatically.
func DialFromConfig(socketPath string) (*Client, error) {
	return DialFromConfigContext(context.Background(), socketPath)
}

// DialFromConfigContext connects using the configured socket path and the given context.
// Expands "~/" to the home directory automatically.
func DialFromConfigContext(ctx context.Context, socketPath string) (*Client, error) {
	expanded, err := ExpandSocketPath(socketPath)
	if err != nil {
		return nil, err
	}
	return DialContext(ctx, expanded)
}

// ExpandSocketPath expands ~ in socket path strings to the user's home directory.
func ExpandSocketPath(p string) (string, error) {
	if len(p) >= 2 && p[:2] == "~/" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("expand socket path: %w", err)
		}
		return filepath.Join(home, p[2:]), nil
	}
	return p, nil
}
