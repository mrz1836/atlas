package daemon

import (
	"context"
	"encoding/json"

	"github.com/rs/zerolog"
)

// HandlerFunc is a JSON-RPC method handler.
type HandlerFunc func(ctx context.Context, params json.RawMessage) (interface{}, error)

// Router dispatches JSON-RPC methods to registered handlers.
type Router struct {
	handlers map[string]HandlerFunc
	logger   zerolog.Logger
}

// NewRouter creates a new Router.
func NewRouter(logger zerolog.Logger) *Router {
	return &Router{
		handlers: make(map[string]HandlerFunc),
		logger:   logger,
	}
}

// Register adds a method handler.
func (r *Router) Register(method string, handler HandlerFunc) {
	r.handlers[method] = handler
}

// Dispatch routes a request to the appropriate handler and returns a Response.
func (r *Router) Dispatch(ctx context.Context, req *Request) *Response {
	handler, ok := r.handlers[req.Method]
	if !ok {
		r.logger.Debug().Str("method", req.Method).Msg("router: unknown method")
		return NewErrorResponse(ErrCodeMethodNotFound, "method not found", req.ID)
	}

	result, err := handler(ctx, req.Params)
	if err != nil {
		r.logger.Error().Err(err).Str("method", req.Method).Msg("router: handler error")
		return NewErrorResponse(ErrCodeInternal, err.Error(), req.ID)
	}

	// Handlers may return nil to suppress a response (e.g. streaming subscriptions).
	if result == nil {
		return nil
	}

	return NewResponse(result, req.ID)
}
