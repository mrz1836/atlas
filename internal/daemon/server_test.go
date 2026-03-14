package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tempSocketPath returns a temporary Unix socket path for tests.
func tempSocketPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "test.sock")
}

// dialUnix dials a Unix socket using DialContext (noctx-safe).
func dialUnix(t *testing.T, sockPath string) net.Conn {
	t.Helper()
	d := &net.Dialer{}
	conn, err := d.DialContext(context.Background(), "unix", sockPath)
	require.NoError(t, err)
	return conn
}

func TestServerStartStop(t *testing.T) {
	t.Parallel()
	logger := zerolog.Nop()
	router := NewRouter(logger)
	sockPath := tempSocketPath(t)

	srv := NewServer(sockPath, router, logger)
	ctx := context.Background()

	// Start should create the socket file.
	require.NoError(t, srv.Start(ctx))
	defer srv.Stop()

	// Socket file should exist after Start.
	_, err := os.Stat(sockPath)
	require.NoError(t, err, "socket file should exist after Start")

	// Stop should close the listener and remove the socket file.
	srv.Stop()

	_, err = os.Stat(sockPath)
	assert.True(t, os.IsNotExist(err), "socket file should be removed after Stop")
}

func TestServerStartStopStaleSock(t *testing.T) {
	t.Parallel()
	logger := zerolog.Nop()
	router := NewRouter(logger)
	sockPath := tempSocketPath(t)

	// Create a stale socket file to simulate a previous crash.
	require.NoError(t, os.WriteFile(sockPath, []byte("stale"), 0o600))

	srv := NewServer(sockPath, router, logger)
	ctx := context.Background()

	// Start should remove the stale socket and bind successfully.
	require.NoError(t, srv.Start(ctx))
	srv.Stop()
}

func TestServerHandleConn(t *testing.T) {
	t.Parallel()
	logger := zerolog.Nop()
	router := NewRouter(logger)

	// Register a ping handler.
	router.Register(MethodDaemonPing, func(_ context.Context, _ json.RawMessage) (interface{}, error) {
		return DaemonPingResponse{Alive: true, Version: "test"}, nil
	})

	sockPath := tempSocketPath(t)
	srv := NewServer(sockPath, router, logger)
	ctx := context.Background()
	require.NoError(t, srv.Start(ctx))
	defer srv.Stop()

	// Connect to the socket.
	conn := dialUnix(t, sockPath)
	defer func() { _ = conn.Close() }()

	// Send a ping request.
	req := Request{JSONRPC: "2.0", Method: MethodDaemonPing, ID: 1}
	reqBytes, err := json.Marshal(req)
	require.NoError(t, err)

	_, err = conn.Write(append(reqBytes, '\n'))
	require.NoError(t, err)

	// Read the response with a deadline.
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))
	scanner := bufio.NewScanner(conn)
	require.True(t, scanner.Scan(), "expected a response line")

	var resp Response
	require.NoError(t, json.Unmarshal(scanner.Bytes(), &resp))
	assert.Nil(t, resp.Error)
	assert.NotNil(t, resp.Result)
}

func TestServerHandleConnParseError(t *testing.T) {
	t.Parallel()
	logger := zerolog.Nop()
	router := NewRouter(logger)
	sockPath := tempSocketPath(t)

	srv := NewServer(sockPath, router, logger)
	ctx := context.Background()
	require.NoError(t, srv.Start(ctx))
	defer srv.Stop()

	conn := dialUnix(t, sockPath)
	defer func() { _ = conn.Close() }()

	// Send invalid JSON.
	_, err := conn.Write([]byte("this is not json\n"))
	require.NoError(t, err)

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))
	scanner := bufio.NewScanner(conn)
	require.True(t, scanner.Scan())

	var resp Response
	require.NoError(t, json.Unmarshal(scanner.Bytes(), &resp))
	require.NotNil(t, resp.Error)
	assert.Equal(t, ErrCodeParseError, resp.Error.Code)
}
