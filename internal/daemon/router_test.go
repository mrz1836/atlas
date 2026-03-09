package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errTestHandler is a static sentinel error for router tests.
var errTestHandler = errors.New("handler error")

func TestRouterDispatch(t *testing.T) {
	logger := zerolog.Nop()
	router := NewRouter(logger)

	// Register a simple echo handler.
	router.Register("test.echo", func(_ context.Context, params json.RawMessage) (interface{}, error) {
		return string(params), nil
	})

	// Register a handler that returns an error.
	router.Register("test.fail", func(_ context.Context, _ json.RawMessage) (interface{}, error) {
		return nil, errTestHandler
	})

	// Register a handler that returns a non-nil result (suppresses nil-nil).
	router.Register("test.accepted", func(_ context.Context, _ json.RawMessage) (interface{}, error) {
		return map[string]interface{}{"accepted": true}, nil
	})

	ctx := context.Background()

	t.Run("known method returns result", func(t *testing.T) {
		req := &Request{JSONRPC: "2.0", Method: "test.echo", Params: json.RawMessage(`"hello"`), ID: 1}
		resp := router.Dispatch(ctx, req)
		require.NotNil(t, resp)
		assert.Nil(t, resp.Error)
		assert.Equal(t, `"hello"`, resp.Result)
		assert.Equal(t, 1, resp.ID)
	})

	t.Run("unknown method returns method-not-found error", func(t *testing.T) {
		req := &Request{JSONRPC: "2.0", Method: "no.such.method", ID: 2}
		resp := router.Dispatch(ctx, req)
		require.NotNil(t, resp)
		require.NotNil(t, resp.Error)
		assert.Equal(t, ErrCodeMethodNotFound, resp.Error.Code)
		assert.Equal(t, 2, resp.ID)
	})

	t.Run("handler error returns internal error", func(t *testing.T) {
		req := &Request{JSONRPC: "2.0", Method: "test.fail", ID: 3}
		resp := router.Dispatch(ctx, req)
		require.NotNil(t, resp)
		require.NotNil(t, resp.Error)
		assert.Equal(t, ErrCodeInternal, resp.Error.Code)
		assert.Contains(t, resp.Error.Message, "handler error")
	})

	t.Run("accepted result returns response", func(t *testing.T) {
		req := &Request{JSONRPC: "2.0", Method: "test.accepted", ID: 4}
		resp := router.Dispatch(ctx, req)
		require.NotNil(t, resp)
		assert.Nil(t, resp.Error)
		assert.NotNil(t, resp.Result)
	})
}
