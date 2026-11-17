package enterprisebus

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/example/hexapi/generator/internal/applogger"
)

func newBus() *Bus { return New(applogger.NewWithEnv(func(string) string { return "" })) }

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
		if ec, ok := c.(echoCmd); !ok || ec.V != "hi" {
			t.Fatalf("unexpected command %+v", c)
		}
		return nil, nil
	})
	if err := b.DispatchResolved(context.Background(), echoCmd{V: "hi"}); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if !called {
		t.Fatalf("handler not called")
	}
}

// TestDispatch_noHandler covers the GEN-001 path. Since 1.4.0.
func TestDispatch_noHandler(t *testing.T) {
	b := newBus()
	err := b.DispatchResolved(context.Background(), echoCmd{})
	if err == nil || !strings.Contains(err.Error(), "GEN-001") {
		t.Fatalf("no handler: want GEN-001, got %v", err)
	}
}

// TestAsk_happy covers the query path returning a value. Since 1.4.0.
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

// TestAsk_timeout covers the API-011 path. Since 1.4.0.
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

// TestAsk_handlerError covers the error pass-through. Since 1.4.0.
func TestAsk_handlerError(t *testing.T) {
	b := newBus()
	_ = b.RegisterQuery("echo-q", func(ctx context.Context, q Query) (any, error) {
		return nil, errors.New("boom")
	})
	_, err := b.AskResolved(context.Background(), echoQuery{})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("Ask error: want boom, got %v", err)
	}
}
