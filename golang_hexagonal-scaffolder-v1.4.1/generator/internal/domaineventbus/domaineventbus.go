// Package domaineventbus provides a minimal in-process domain-event publisher
// that the generated API's services use to signal domain events. The
// generator itself ships a bus so the pipeline can emit events for
// "schema-introspected", "files-written", etc., keeping the two
// implementations structurally identical.
//
// v1.4.0: Publish logs the cause at ERROR before re-throwing, and is wrapped
// by AppLogger.Logged.
//
// Since 1.4.0.
package domaineventbus

import (
	"context"
	"sync"

	"github.com/example/hexapi/generator/internal/applogger"
	hexerrors "github.com/example/hexapi/generator/internal/errors"
)

// Event is a domain event. Since 1.4.0.
type Event struct {
	Name    string
	Payload any
}

// Subscriber is called for each published event whose Name matches.
// Returning an error stops further subscribers for that event. Since 1.4.0.
type Subscriber func(ctx context.Context, e Event) error

// Bus is the in-process event bus. Since 1.4.0.
type Bus struct {
	log  *applogger.Logger
	mu   sync.RWMutex
	subs map[string][]Subscriber
}

// New constructs a Bus. Since 1.4.0.
func New(log *applogger.Logger) *Bus {
	if log == nil {
		log = applogger.New()
	}
	return &Bus{log: log, subs: map[string][]Subscriber{}}
}

// Subscribe registers a subscriber for an event name. Since 1.4.0.
func (b *Bus) Subscribe(name string, s Subscriber) error {
	return b.log.Logged("DomainEventBus.Subscribe", func() error {
		if name == "" {
			b.log.Error("Subscribe: empty name")
			return hexerrors.New(hexerrors.CodeConfig, "event name is required")
		}
		if s == nil {
			b.log.Error("Subscribe: nil subscriber", "name", name)
			return hexerrors.New(hexerrors.CodeConfig, "subscriber is required: "+name)
		}
		b.mu.Lock()
		defer b.mu.Unlock()
		b.subs[name] = append(b.subs[name], s)
		return nil
	})
}

// Publish dispatches e to all matching subscribers. API-012 is returned if a
// subscriber fails; earlier subscribers still run. Since 1.4.0.
func (b *Bus) Publish(ctx context.Context, e Event) error {
	return b.log.Logged("DomainEventBus.Publish", func() error {
		if e.Name == "" {
			b.log.Error("Publish: empty event name")
			return hexerrors.New(hexerrors.CodeConfig, "event name is required")
		}
		b.mu.RLock()
		subs := append([]Subscriber(nil), b.subs[e.Name]...)
		b.mu.RUnlock()
		for _, s := range subs {
			if err := s(ctx, e); err != nil {
				b.log.Error("Publish: subscriber failed", "event", e.Name, "err", err)
				return hexerrors.Wrap("API-012", "subscriber failed for "+e.Name, err)
			}
		}
		return nil
	})
}

// SubscriberCount returns the number of subscribers for a name (test helper).
// Since 1.4.0.
func (b *Bus) SubscriberCount(name string) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subs[name])
}
