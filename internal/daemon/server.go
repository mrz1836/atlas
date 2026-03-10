package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

//nolint:gochecknoglobals // package-level pool is the intended pattern for sync.Pool
var scannerBufferPool = sync.Pool{
	New: func() interface{} {
		buf := make([]byte, 1<<20)
		return &buf // pointer avoids SA6002 allocation on Put
	},
}

// Server listens on a Unix domain socket and dispatches JSON-RPC requests via Router.
type Server struct {
	socketPath string
	listener   net.Listener
	router     *Router
	logger     zerolog.Logger
	conns      sync.WaitGroup
	stopCh     chan struct{}
	stopOnce   sync.Once
	mu         sync.Mutex // guards listener
}

// NewServer creates a Server that will bind to socketPath when Start is called.
func NewServer(socketPath string, router *Router, logger zerolog.Logger) *Server {
	return &Server{
		socketPath: socketPath,
		router:     router,
		logger:     logger,
		stopCh:     make(chan struct{}),
	}
}

// Start removes any stale socket file, binds to the Unix socket, and begins
// accepting connections in a background goroutine.
func (s *Server) Start(ctx context.Context) error {
	// Remove stale socket from a previous (crashed) run.
	if err := os.Remove(s.socketPath); err != nil && !os.IsNotExist(err) {
		return err
	}

	lc := &net.ListenConfig{}
	ln, err := lc.Listen(ctx, "unix", s.socketPath)
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.listener = ln
	s.mu.Unlock()

	s.logger.Info().Str("socket", s.socketPath).Msg("server: listening")

	// Bridge context cancellation to the stop channel so acceptLoop responds
	// to ctx.Done() without polling.
	go func() {
		select {
		case <-ctx.Done():
			s.Stop()
		case <-s.stopCh:
		}
	}()

	go s.acceptLoop(ctx)
	return nil
}

// Stop closes the listener and waits for all active connections to drain.
func (s *Server) Stop() {
	// Signal stop once; concurrent callers are safe via sync.Once.
	s.stopOnce.Do(func() { close(s.stopCh) })

	s.mu.Lock()
	ln := s.listener
	s.mu.Unlock()

	if ln != nil {
		if err := ln.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			s.logger.Warn().Err(err).Msg("server: error closing listener")
		}
	}

	s.conns.Wait()

	// Best-effort cleanup of the socket file.
	if removeErr := os.Remove(s.socketPath); removeErr != nil && !os.IsNotExist(removeErr) {
		s.logger.Warn().Err(removeErr).Str("socket", s.socketPath).Msg("server: failed to remove socket")
	}

	s.logger.Info().Msg("server: stopped")
}

// serveRequest decodes a single JSON-RPC request from data, dispatches it, and writes the response.
func (s *Server) serveRequest(ctx context.Context, encoder *json.Encoder, data []byte) {
	var req Request
	if err := json.Unmarshal(data, &req); err != nil {
		s.logger.Warn().Err(err).Msg("server: parse error")
		errResp := NewErrorResponse(ErrCodeParseError, "parse error", nil)
		if encErr := encoder.Encode(errResp); encErr != nil {
			s.logger.Debug().Err(encErr).Msg("server: failed to encode parse error response")
		}
		return
	}

	resp := s.router.Dispatch(ctx, &req)
	if resp != nil {
		if encErr := encoder.Encode(resp); encErr != nil {
			s.logger.Warn().Err(encErr).Msg("server: encode response error")
		}
	}
}

// acceptLoop accepts new connections until Stop is called.
func (s *Server) acceptLoop(ctx context.Context) {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.stopCh:
				// Normal shutdown path.
				return
			default:
				if !errors.Is(err, net.ErrClosed) {
					s.logger.Error().Err(err).Msg("server: accept error")
				}
				return
			}
		}

		s.conns.Add(1)
		go func() {
			defer s.conns.Done()
			s.handleConn(ctx, conn)
		}()
	}
}

// connReadTimeout is the per-request idle timeout on a client connection.
// A connection that sends no data for this duration is closed.
const connReadTimeout = 30 * time.Second

// handleConn reads newline-delimited JSON-RPC requests from conn and writes responses.
// Each request is dispatched synchronously. A nil response (e.g. events.subscribe stub)
// produces no output on the wire.
func (s *Server) handleConn(ctx context.Context, conn net.Conn) {
	defer func() {
		if err := conn.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			s.logger.Debug().Err(err).Msg("server: error closing connection")
		}
	}()

	// Q3: raise the line scanner limit to 1 MB to handle large task payloads.
	// We rent from a sync.Pool to reduce GC allocations on reconnects.
	scanner := bufio.NewScanner(conn)
	bufp := scannerBufferPool.Get().(*[]byte)
	defer scannerBufferPool.Put(bufp)
	scanner.Buffer(*bufp, 1<<20)
	encoder := json.NewEncoder(conn)

	for {
		// Q2: apply a per-request read deadline so an idle client does not hold the
		// connection open indefinitely. The deadline is cleared before dispatching so
		// slow handlers are not interrupted.
		if err := conn.SetReadDeadline(time.Now().Add(connReadTimeout)); err != nil {
			break
		}
		if !scanner.Scan() {
			break
		}
		// Clear deadline while the handler runs.
		_ = conn.SetDeadline(time.Time{})

		s.serveRequest(ctx, encoder, scanner.Bytes())
	}

	if err := scanner.Err(); err != nil {
		select {
		case <-s.stopCh:
			// Ignore scanner errors on shutdown.
		default:
			s.logger.Debug().Err(err).Msg("server: scanner error")
		}
	}
}
