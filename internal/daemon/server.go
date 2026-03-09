package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"sync"

	"github.com/rs/zerolog"
)

// Server listens on a Unix domain socket and dispatches JSON-RPC requests via Router.
type Server struct {
	socketPath string
	listener   net.Listener
	router     *Router
	logger     zerolog.Logger
	conns      sync.WaitGroup
	stopCh     chan struct{}
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

	go s.acceptLoop(ctx)
	return nil
}

// Stop closes the listener and waits for all active connections to drain.
func (s *Server) Stop() {
	// Signal stop once (channel may already be closed on re-entrant calls).
	select {
	case <-s.stopCh:
	default:
		close(s.stopCh)
	}

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

// handleConn reads newline-delimited JSON-RPC requests from conn and writes responses.
// Each request is dispatched synchronously. A nil response (e.g. events.subscribe stub)
// produces no output on the wire.
func (s *Server) handleConn(ctx context.Context, conn net.Conn) {
	defer func() {
		if err := conn.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			s.logger.Debug().Err(err).Msg("server: error closing connection")
		}
	}()

	scanner := bufio.NewScanner(conn)
	encoder := json.NewEncoder(conn)

	for scanner.Scan() {
		var req Request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			s.logger.Warn().Err(err).Msg("server: parse error")
			errResp := NewErrorResponse(ErrCodeParseError, "parse error", nil)
			if encErr := encoder.Encode(errResp); encErr != nil {
				s.logger.Debug().Err(encErr).Msg("server: failed to encode parse error response")
			}
			continue
		}

		resp := s.router.Dispatch(ctx, &req)
		if resp != nil {
			if encErr := encoder.Encode(resp); encErr != nil {
				s.logger.Warn().Err(encErr).Msg("server: encode response error")
			}
		}
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
