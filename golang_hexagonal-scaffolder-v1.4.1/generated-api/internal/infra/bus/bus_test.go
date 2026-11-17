package bus

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/example/generated-api/internal/shared/applogger"
)

func newBus() *EnterpriseBus { return NewEnterpriseBus(applogger.NewWithEnv(func(string) string { return "" })) }
func newEventBus() *DomainEventBus {
	return NewDomainEventBus(applogger.NewWithEnv(func(string) string { return "" }))
}

type echoCmd struct{ V string }

func (echoCmd) CommandName() string { return "echo" }

type echoQuery struct{ V string }

func (echoQuery) QueryName() string { return "echo-q" }

// TestDispatch_happy covers command dispatch. Since 1.4.0.
func TestDispatch_happy(t *testing.T) {
	b := newBus()
	called := false
	_ = b.RegisterCommand("echo", func(ctx context.Context, c Command) (any, error) {
		called = true
		return nil, nil
	})
	if err := b.DispatchResolved(context.Background(), echoCmd{V: "hi"}); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if !called {
		t.Fatalf("handler not called")
	}
}

// TestDispatch_noHandler covers the validation path. Since 1.4.0.
func TestDispatch_noHandler(t *testing.T) {
	b := newBus()
	err := b.DispatchResolved(context.Background(), echoCmd{})
	if err == nil || !strings.Contains(err.Error(), "API-001") {
		t.Fatalf("no handler: want API-001, got %v", err)
	}
}

// TestAsk_happy covers the query path. Since 1.4.0.
func TestAsk_happy(t *testing.T) {
	b := newBus()
	_ = b.RegisterQuery("echo-q", func(ctx context.Context, q Query) (any, error) {
		return q.(echoQuery).V + "!", nil
	})
	got, err := b.AskResolved(context.Background(), echoQuery{V: "hi"})
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if got != "hi!" {
		t.Fatalf("Ask result: want hi!, got %v", got)
	}
}

// TestAsk_timeout covers API-011. Since 1.4.0.
func TestAsk_timeout(t *testing.T) {
	b := newBus()
	_ = b.RegisterQuery("echo-q", func(ctx context.Context, q Query) (any, error) {
		time.Sleep(50 * time.Millisecond)
		return "late", nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	_, err := b.AskResolved(ctx, echoQuery{})
	if err == nil || !strings.Contains(err.Error(), "API-011") {
		t.Fatalf("Ask timeout: want API-011, got %v", err)
	}
}

// TestPublish_happy covers the event bus. Since 1.4.0.
func TestPublish_happy(t *testing.T) {
	b := newEventBus()
	got := ""
	_ = b.Subscribe("user.created", func(ctx context.Context, e Event) error {
		got = e.Name
		return nil
	})
	if err := b.Publish(context.Background(), Event{Name: "user.created"}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if got != "user.created" {
		t.Fatalf("subscriber got %q", got)
	}
}

// TestPublish_subscriberFails covers API-012. Since 1.4.0.
func TestPublish_subscriberFails(t *testing.T) {
	b := newEventBus()
	_ = b.Subscribe("x", func(ctx context.Context, e Event) error { return errors.New("boom") })
	err := b.Publish(context.Background(), Event{Name: "x"})
	if err == nil || !strings.Contains(err.Error(), "API-012") {
		t.Fatalf("Publish fail: want API-012, got %v", err)
	}
}

// TestServiceRegistration_startStop covers the registry. Since 1.4.0.
func TestServiceRegistration_startStop(t *testing.T) {
	r := NewServiceRegistration(applogger.NewWithEnv(func(string) string { return "" }))
	started := false
	stopped := false
	_ = r.Register("svc",
		func(ctx context.Context) error { started = true; return nil },
		func(ctx context.Context) error { stopped = true; return nil },
	)
	if err := r.StartAll(context.Background()); err != nil {
		t.Fatalf("StartAll: %v", err)
	}
	if err := r.StopAll(context.Background()); err != nil {
		t.Fatalf("StopAll: %v", err)
	}
	if !started || !stopped {
		t.Fatalf("started=%v stopped=%v", started, stopped)
	}
}
