// Package bus provides the in-process command/query bus, domain-event bus,
// and service registration used by the generated API's application layer.
//
// All three are Logged-wrapped; all catch-and-rethrow sites log the cause at
// ERROR before re-throwing. Since 1.4.0.
package bus

import (
	"context"
	"sync"
	"time"

	"github.com/example/generated-api/internal/shared/applogger"
	hexerr "github.com/example/generated-api/internal/shared/errors"
)

// ---- EnterpriseBus --------------------------------------------------------

// Command marks write-side messages. Since 1.4.0.
type Command interface{ CommandName() string }

// Query marks read-side messages. Since 1.4.0.
type Query interface{ QueryName() string }

// Handler handles a message of type T. Since 1.4.0.
type Handler[T any] func(ctx context.Context, msg T) (any, error)

// EnterpriseBus is the in-process command/query bus. Since 1.4.0.
type EnterpriseBus struct {
	log     *applogger.Logger
	mu      sync.RWMutex
	cmds    map[string]any
	queries map[string]any
}

// NewEnterpriseBus constructs a bus. Since 1.4.0.
func NewEnterpriseBus(log *applogger.Logger) *EnterpriseBus {
	if log == nil {
		log = applogger.New()
	}
	return &EnterpriseBus{log: log, cmds: map[string]any{}, queries: map[string]any{}}
}

// RegisterCommand registers a command handler. Since 1.4.0.
func (b *EnterpriseBus) RegisterCommand(name string, h Handler[Command]) error {
	return b.log.Logged("EnterpriseBus.RegisterCommand", func() error {
		if name == "" || h == nil {
			b.log.Error("RegisterCommand: bad args", "name", name)
			return hexerr.Validation("command name and handler are required")
		}
		b.mu.Lock()
		defer b.mu.Unlock()
		b.cmds[name] = h
		return nil
	})
}

// RegisterQuery registers a query handler. Since 1.4.0.
func (b *EnterpriseBus) RegisterQuery(name string, h Handler[Query]) error {
	return b.log.Logged("EnterpriseBus.RegisterQuery", func() error {
		if name == "" || h == nil {
			b.log.Error("RegisterQuery: bad args", "name", name)
			return hexerr.Validation("query name and handler are required")
		}
		b.mu.Lock()
		defer b.mu.Unlock()
		b.queries[name] = h
		return nil
	})
}

// DispatchResolved dispatches a command. Since 1.4.0.
func (b *EnterpriseBus) DispatchResolved(ctx context.Context, cmd Command) error {
	return b.log.Logged("EnterpriseBus.DispatchResolved", func() error {
		if cmd == nil {
			return hexerr.Validation("command is nil")
		}
		name := cmd.CommandName()
		b.mu.RLock()
		h, ok := b.cmds[name]
		b.mu.RUnlock()
		if !ok {
			b.log.Error("DispatchResolved: no handler", "name", name)
			return hexerr.Validation("no handler registered for command: " + name)
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

// AskResolved dispatches a query. API-011 on timeout. Since 1.4.0.
func (b *EnterpriseBus) AskResolved(ctx context.Context, q Query) (any, error) {
	return applogger.Logged1(b.log, "EnterpriseBus.AskResolved", func() (any, error) {
		if q == nil {
			return nil, hexerr.Validation("query is nil")
		}
		name := q.QueryName()
		b.mu.RLock()
		h, ok := b.queries[name]
		b.mu.RUnlock()
		if !ok {
			b.log.Error("AskResolved: no handler", "name", name)
			return nil, hexerr.Validation("no handler registered for query: " + name)
		}
		handler := h.(Handler[Query])
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
			return nil, hexerr.Wrap(hexerr.CodeTimeout, "query timed out", 504, "gateway-timeout", ctx.Err())
		}
	})
}

// ---- DomainEventBus -------------------------------------------------------

// Event is a domain event. Since 1.4.0.
type Event struct {
	Name    string
	Payload any
}

// Subscriber is called for each matching event. Since 1.4.0.
type Subscriber func(ctx context.Context, e Event) error

// DomainEventBus is the in-process event bus. Since 1.4.0.
type DomainEventBus struct {
	log  *applogger.Logger
	mu   sync.RWMutex
	subs map[string][]Subscriber
}

// NewDomainEventBus constructs a bus. Since 1.4.0.
func NewDomainEventBus(log *applogger.Logger) *DomainEventBus {
	if log == nil {
		log = applogger.New()
	}
	return &DomainEventBus{log: log, subs: map[string][]Subscriber{}}
}

// Subscribe registers a subscriber. Since 1.4.0.
func (b *DomainEventBus) Subscribe(name string, s Subscriber) error {
	return b.log.Logged("DomainEventBus.Subscribe", func() error {
		if name == "" || s == nil {
			b.log.Error("Subscribe: bad args", "name", name)
			return hexerr.Validation("event name and subscriber are required")
		}
		b.mu.Lock()
		defer b.mu.Unlock()
		b.subs[name] = append(b.subs[name], s)
		return nil
	})
}

// Publish dispatches e to all matching subscribers. Since 1.4.0.
func (b *DomainEventBus) Publish(ctx context.Context, e Event) error {
	return b.log.Logged("DomainEventBus.Publish", func() error {
		if e.Name == "" {
			return hexerr.Validation("event name is required")
		}
		b.mu.RLock()
		subs := append([]Subscriber(nil), b.subs[e.Name]...)
		b.mu.RUnlock()
		for _, s := range subs {
			if err := s(ctx, e); err != nil {
				b.log.Error("Publish: subscriber failed", "event", e.Name, "err", err)
				return hexerr.Wrap(hexerr.CodeBadGateway, "subscriber failed for "+e.Name, 502, "bad-gateway", err)
			}
		}
		return nil
	})
}

// SubscriberCount returns the number of subscribers for a name. Since 1.4.0.
func (b *DomainEventBus) SubscriberCount(name string) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subs[name])
}

// ---- ServiceRegistration --------------------------------------------------

// ServiceRegistration tracks the services wired into the application so they
// can be started/stopped cleanly. Since 1.4.0.
type ServiceRegistration struct {
	log     *applogger.Logger
	mu      sync.Mutex
	services []Registered
}

// Registered is one wired service. Since 1.4.0.
type Registered struct {
	Name    string
	Start   func(ctx context.Context) error
	Stop    func(ctx context.Context) error
}

// NewServiceRegistration constructs a registry. Since 1.4.0.
func NewServiceRegistration(log *applogger.Logger) *ServiceRegistration {
	if log == nil {
		log = applogger.New()
	}
	return &ServiceRegistration{log: log}
}

// Register adds a service. Since 1.4.0.
func (r *ServiceRegistration) Register(name string, start, stop func(ctx context.Context) error) error {
	return r.log.Logged("ServiceRegistration.Register", func() error {
		if name == "" {
			r.log.Error("Register: empty name")
			return hexerr.Validation("service name is required")
		}
		if start == nil {
			r.log.Error("Register: nil start", "name", name)
			return hexerr.Validation("start function is required: " + name)
		}
		r.mu.Lock()
		defer r.mu.Unlock()
		r.services = append(r.services, Registered{Name: name, Start: start, Stop: stop})
		return nil
	})
}

// StartAll starts every registered service in order. Since 1.4.0.
func (r *ServiceRegistration) StartAll(ctx context.Context) error {
	return r.log.Logged("ServiceRegistration.StartAll", func() error {
		r.mu.Lock()
		svcs := append([]Registered(nil), r.services...)
		r.mu.Unlock()
		for _, s := range svcs {
			if err := s.Start(ctx); err != nil {
				r.log.Error("StartAll: service failed to start", "name", s.Name, "err", err)
				return hexerr.Wrap(hexerr.CodeInternal, "service start failed: "+s.Name, 500, "internal", err)
			}
		}
		return nil
	})
}

// StopAll stops every registered service in reverse order. Since 1.4.0.
func (r *ServiceRegistration) StopAll(ctx context.Context) error {
	return r.log.Logged("ServiceRegistration.StopAll", func() error {
		r.mu.Lock()
		svcs := append([]Registered(nil), r.services...)
		r.mu.Unlock()
		for i := len(svcs) - 1; i >= 0; i-- {
			s := svcs[i]
			if s.Stop == nil {
				continue
			}
			if err := s.Stop(ctx); err != nil {
				r.log.Error("StopAll: service failed to stop", "name", s.Name, "err", err)
				return hexerr.Wrap(hexerr.CodeInternal, "service stop failed: "+s.Name, 500, "internal", err)
			}
		}
		return nil
	})
}

// DrainTimeout forces a minimum timeout for AskResolved tests. Since 1.4.0.
func DrainTimeout() time.Duration { return 100 * time.Millisecond }
