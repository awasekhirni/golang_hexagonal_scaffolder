// Package enterprisebus provides the command/query bus the generated API uses
// to dispatch use cases without the HTTP layer knowing about the concrete
// services. The generator itself ships a bus too (the pipeline uses it for
// the introspect/render/report commands) so the two implementations stay
// structurally identical.
//
// v1.4.0 (P0-3.1 mirror): DispatchResolved and AskResolved log the cause at
// ERROR before re-throwing, and both are wrapped by AppLogger.Logged.
//
// Since 1.4.0.
package enterprisebus

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/example/hexapi/generator/internal/applogger"
	hexerrors "github.com/example/hexapi/generator/internal/errors"
)

// Command is a marker for command messages (write-side). Since 1.4.0.
type Command interface{ CommandName() string }

// Query is a marker for query messages (read-side). Since 1.4.0.
type Query interface{ QueryName() string }

// Handler handles a command or query of type T. Since 1.4.0.
type Handler[T any] func(ctx context.Context, msg T) (any, error)

// Bus is the in-process command/query bus. Since 1.4.0.
type Bus struct {
	log     *applogger.Logger
	mu      sync.RWMutex
	cmds    map[string]any // name → Handler[Command]
	queries map[string]any // name → Handler[Query]
}

// New constructs a Bus. Since 1.4.0.
func New(log *applogger.Logger) *Bus {
	if log == nil {
		log = applogger.New()
	}
	return &Bus{
		log:     log,
		cmds:    map[string]any{},
		queries: map[string]any{},
	}
}

// RegisterCommand registers a handler for a command name. Since 1.4.0.
func (b *Bus) RegisterCommand(name string, h Handler[Command]) error {
	return b.log.Logged("EnterpriseBus.RegisterCommand", func() error {
		if name == "" {
			b.log.Error("RegisterCommand: empty name")
			return hexerrors.New(hexerrors.CodeConfig, "command name is required")
		}
		if h == nil {
			b.log.Error("RegisterCommand: nil handler", "name", name)
			return hexerrors.New(hexerrors.CodeConfig, "handler is required: "+name)
		}
		b.mu.Lock()
		defer b.mu.Unlock()
		b.cmds[name] = h
		return nil
	})
}

// RegisterQuery registers a handler for a query name. Since 1.4.0.
func (b *Bus) RegisterQuery(name string, h Handler[Query]) error {
	return b.log.Logged("EnterpriseBus.RegisterQuery", func() error {
		if name == "" {
			b.log.Error("RegisterQuery: empty name")
			return hexerrors.New(hexerrors.CodeConfig, "query name is required")
		}
		if h == nil {
			b.log.Error("RegisterQuery: nil handler", "name", name)
			return hexerrors.New(hexerrors.CodeConfig, "handler is required: "+name)
		}
		b.mu.Lock()
		defer b.mu.Unlock()
		b.queries[name] = h
		return nil
	})
}

// DispatchResolved dispatches a command and discards the result, returning
// only the error. "Resolved" means the handler is guaranteed to have been
// looked up successfully (otherwise GEN-001). Since 1.4.0.
func (b *Bus) DispatchResolved(ctx context.Context, cmd Command) error {
	return b.log.Logged("EnterpriseBus.DispatchResolved", func() error {
		if cmd == nil {
			b.log.Error("DispatchResolved: nil command")
			return hexerrors.New(hexerrors.CodeConfig, "command is nil")
		}
		name := cmd.CommandName()
		b.mu.RLock()
		h, ok := b.cmds[name]
		b.mu.RUnlock()
		if !ok {
			b.log.Error("DispatchResolved: no handler", "name", name)
			return hexerrors.New(hexerrors.CodeConfig, "no handler registered for command: "+name)
		}
		handler := h.(Handler[Command])
		_, err := handler(ctx, cmd)
		if err != nil {
			b.log.Error("DispatchResolved: handler failed", "name", name, "err", err)
			return err
		}
		return nil
	})
}

// AskResolved dispatches a query and returns the result. Returns API-011
// (timeout) if the context deadline is exceeded. Since 1.4.0.
func (b *Bus) AskResolved(ctx context.Context, q Query) (any, error) {
	return applogger.Logged1(b.log, "EnterpriseBus.AskResolved", func() (any, error) {
		if q == nil {
			b.log.Error("AskResolved: nil query")
			return nil, hexerrors.New(hexerrors.CodeConfig, "query is nil")
		}
		name := q.QueryName()
		b.mu.RLock()
		h, ok := b.queries[name]
		b.mu.RUnlock()
		if !ok {
			b.log.Error("AskResolved: no handler", "name", name)
			return nil, hexerrors.New(hexerrors.CodeConfig, "no handler registered for query: "+name)
		}
		handler := h.(Handler[Query])
		// Honour the context deadline.
		done := make(chan struct{})
		var result any
		var err error
		go func() {
			defer close(done)
			result, err = handler(ctx, q)
		}()
		select {
		case <-done:
			if err != nil {
				b.log.Error("AskResolved: handler failed", "name", name, "err", err)
				return nil, err
			}
			return result, nil
		case <-ctx.Done():
			b.log.Error("AskResolved: timeout", "name", name, "err", ctx.Err())
			return nil, hexerrors.Wrap("API-011", "query timed out", ctx.Err())
		}
	})
}

// DrainTimeout forces a minimum timeout for AskResolved tests. Since 1.4.0.
func DrainTimeout() time.Duration { return 100 * time.Millisecond }

// ErrNoHandler is returned when a command/query has no registered handler.
// Since 1.4.0.
var ErrNoHandler = errors.New("no handler registered")
