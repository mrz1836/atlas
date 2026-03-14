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
	"sync"
	"time"
)

// Sentinel errors for the client.
var (
	errConnectionClosed = errors.New("connection closed")
	errRPCError         = errors.New("rpc error")
)

// Client is a JSON-RPC client that communicates with the Atlas daemon via Unix socket.
type Client struct {
	conn         net.Conn
	encoder      *json.Encoder
	mu           sync.Mutex
	writeMu      sync.Mutex
	nextID       int
	pendingCalls map[int]chan *Response
	stopCh       chan struct{}
	stopOnce     sync.Once
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
	c := &Client{
		conn:         conn,
		encoder:      json.NewEncoder(conn),
		pendingCalls: make(map[int]chan *Response),
		stopCh:       make(chan struct{}),
	}
	go c.readLoop()
	return c, nil
}

// Call sends a JSON-RPC request and decodes the response into result.
// Safe for concurrent use from multiple goroutines.
// If ctx carries a deadline, it is applied to the underlying connection.
func (c *Client) Call(ctx context.Context, method string, params, result interface{}) error {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	respCh := make(chan *Response, 1)
	c.pendingCalls[id] = respCh
	c.mu.Unlock()

	req := &Request{
		JSONRPC: "2.0",
		Method:  method,
		ID:      id,
	}
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			c.removePending(id)
			return fmt.Errorf("marshal params: %w", err)
		}
		req.Params = json.RawMessage(b)
	}

	c.writeMu.Lock()
	if dl, ok := ctx.Deadline(); ok {
		_ = c.conn.SetWriteDeadline(dl)
	} else {
		_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	}
	err := c.encoder.Encode(req)
	c.writeMu.Unlock()

	if err != nil {
		c.removePending(id)
		return fmt.Errorf("send request: %w", err)
	}

	select {
	case <-ctx.Done():
		c.removePending(id)
		return ctx.Err()
	case <-c.stopCh:
		return errConnectionClosed
	case resp, ok := <-respCh:
		if !ok {
			return errConnectionClosed
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
}

// Ping checks if the daemon is alive. Returns true if alive.
func (c *Client) Ping() bool {
	return c.PingContext(context.Background())
}

// PingContext checks if the daemon is alive, propagating the provided context.
func (c *Client) PingContext(ctx context.Context) bool {
	var resp DaemonPingResponse
	return c.Call(ctx, MethodDaemonPing, nil, &resp) == nil && resp.Alive
}

// Close disconnects from the daemon.
func (c *Client) Close() error {
	c.stopOnce.Do(func() { close(c.stopCh) })
	return c.conn.Close()
}

func (c *Client) readLoop() {
	scanner := bufio.NewScanner(c.conn)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)

	for scanner.Scan() {
		var resp Response
		if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
			continue // ignore unparseable
		}

		var respID int
		if idF, ok := resp.ID.(float64); ok {
			respID = int(idF)
		} else if idI, ok := resp.ID.(int); ok {
			respID = idI
		} else {
			continue // ignoring notification or invalid ID
		}

		c.mu.Lock()
		ch, ok := c.pendingCalls[respID]
		if ok {
			delete(c.pendingCalls, respID)
		}
		c.mu.Unlock()
		if ok {
			ch <- &resp
		}
	}
	c.stopOnce.Do(func() { close(c.stopCh) })

	c.mu.Lock()
	for id, ch := range c.pendingCalls {
		close(ch)
		delete(c.pendingCalls, id)
	}
	c.mu.Unlock()
}

func (c *Client) removePending(id int) {
	c.mu.Lock()
	if ch, ok := c.pendingCalls[id]; ok {
		close(ch)
		delete(c.pendingCalls, id)
	}
	c.mu.Unlock()
}

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
