package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startTestServer starts a simple JSON-RPC test server on a temporary Unix socket.
// It returns the socket path and a cleanup function.
func startTestServer(t *testing.T) (socketPath string, cleanup func()) {
	t.Helper()
	// Use os.MkdirTemp with a short prefix to stay within macOS's 104-char socket path limit.
	dir, err := os.MkdirTemp("", "atlsd")
	require.NoError(t, err, "create temp dir")
	socketPath = filepath.Join(dir, "d.sock")

	lc := &net.ListenConfig{}
	ln, listenErr := lc.Listen(context.Background(), "unix", socketPath)
	require.NoError(t, listenErr, "listen on unix socket")

	go func() {
		for {
			conn, acceptErr := ln.Accept()
			if acceptErr != nil {
				return // listener closed
			}
			go handleTestConn(conn)
		}
	}()

	cleanup = func() {
		_ = ln.Close()
		_ = os.RemoveAll(dir)
	}
	return socketPath, cleanup
}

// handleTestConn serves a single client connection for the test server.
func handleTestConn(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	scanner := bufio.NewScanner(conn)
	enc := json.NewEncoder(conn)
	for scanner.Scan() {
		var req Request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			if encErr := enc.Encode(NewErrorResponse(ErrCodeParseError, "parse error", nil)); encErr != nil {
				return
			}
			continue
		}
		var encErr error
		switch req.Method {
		case MethodDaemonPing:
			encErr = enc.Encode(NewResponse(DaemonPingResponse{Alive: true, Version: "test"}, req.ID))
		case MethodDaemonStatus:
			encErr = enc.Encode(NewResponse(DaemonStatusResponse{
				PID:        1234,
				Uptime:     "1m0s",
				RedisAlive: true,
				Workers:    2,
			}, req.ID))
		case MethodTaskSubmit:
			encErr = enc.Encode(NewResponse(TaskSubmitResponse{
				TaskID: "test-task-id",
				Status: "queued",
			}, req.ID))
		case MethodDaemonShutdown:
			encErr = enc.Encode(NewResponse(map[string]interface{}{"ok": true}, req.ID))
			if encErr == nil {
				return // close connection after shutdown
			}
		default:
			encErr = enc.Encode(NewErrorResponse(ErrCodeMethodNotFound, "method not found: "+req.Method, req.ID))
		}
		if encErr != nil {
			return
		}
	}
}

func TestDial_Success(t *testing.T) {
	socketPath, cleanup := startTestServer(t)
	defer cleanup()

	c, err := Dial(socketPath)
	require.NoError(t, err)
	require.NoError(t, c.Close())
}

func TestDial_Failure(t *testing.T) {
	_, err := Dial("/nonexistent/path/to/daemon.sock")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "daemon not running")
}

func TestClient_Ping(t *testing.T) {
	socketPath, cleanup := startTestServer(t)
	defer cleanup()

	c, err := Dial(socketPath)
	require.NoError(t, err)
	defer func() { _ = c.Close() }()

	assert.True(t, c.Ping())
}

func TestClient_Call_Status(t *testing.T) {
	socketPath, cleanup := startTestServer(t)
	defer cleanup()

	c, err := Dial(socketPath)
	require.NoError(t, err)
	defer func() { _ = c.Close() }()

	var resp DaemonStatusResponse
	err = c.Call(MethodDaemonStatus, nil, &resp)
	require.NoError(t, err)
	assert.Equal(t, 1234, resp.PID)
	assert.True(t, resp.RedisAlive)
}

func TestClient_Call_TaskSubmit(t *testing.T) {
	socketPath, cleanup := startTestServer(t)
	defer cleanup()

	c, err := Dial(socketPath)
	require.NoError(t, err)
	defer func() { _ = c.Close() }()

	req := TaskSubmitRequest{
		Description: "test task",
		Template:    "bug",
	}
	var resp TaskSubmitResponse
	err = c.Call(MethodTaskSubmit, req, &resp)
	require.NoError(t, err)
	assert.Equal(t, "test-task-id", resp.TaskID)
	assert.Equal(t, "queued", resp.Status)
}

func TestClient_Call_UnknownMethod(t *testing.T) {
	socketPath, cleanup := startTestServer(t)
	defer cleanup()

	c, err := Dial(socketPath)
	require.NoError(t, err)
	defer func() { _ = c.Close() }()

	err = c.Call("unknown.method", nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rpc error")
}

func TestClient_MultipleSequentialCalls(t *testing.T) {
	socketPath, cleanup := startTestServer(t)
	defer cleanup()

	c, err := Dial(socketPath)
	require.NoError(t, err)
	defer func() { _ = c.Close() }()

	// Multiple sequential calls should work correctly
	assert.True(t, c.Ping())

	var status DaemonStatusResponse
	require.NoError(t, c.Call(MethodDaemonStatus, nil, &status))
	assert.Equal(t, 1234, status.PID)

	assert.True(t, c.Ping())
}

func TestPingSocket_True(t *testing.T) {
	socketPath, cleanup := startTestServer(t)
	defer cleanup()

	assert.True(t, PingSocket(socketPath))
}

func TestPingSocket_False(t *testing.T) {
	assert.False(t, PingSocket("/nonexistent/path/to/daemon.sock"))
}

func TestDialFromConfig_AbsolutePath(t *testing.T) {
	socketPath, cleanup := startTestServer(t)
	defer cleanup()

	// DialFromConfig should work with an absolute path (no ~ expansion needed)
	c, err := DialFromConfig(socketPath)
	require.NoError(t, err)
	defer func() { _ = c.Close() }()
	assert.True(t, c.Ping())
}

func TestExpandSocketPath(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "tilde expansion",
			input:    "~/.atlas/daemon.sock",
			expected: filepath.Join(home, ".atlas/daemon.sock"),
		},
		{
			name:     "absolute path unchanged",
			input:    "/var/run/atlas.sock",
			expected: "/var/run/atlas.sock",
		},
		{
			name:     "relative path unchanged",
			input:    "relative/path.sock",
			expected: "relative/path.sock",
		},
		{
			name:     "tilde without slash unchanged",
			input:    "~nosep",
			expected: "~nosep",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, expErr := ExpandSocketPath(tt.input)
			require.NoError(t, expErr)
			assert.Equal(t, tt.expected, result)
		})
	}
}
