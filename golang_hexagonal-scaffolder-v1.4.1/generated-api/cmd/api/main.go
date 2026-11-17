// Package main is the entry point for the generated-api HTTP server.
//
// It constructs the AppLogger, calls server.Boot (which loads settings, opens
// the DB, applies migrations, wires deps, and runs the server with graceful
// shutdown), and installs the SIGINT/SIGTERM hook that triggers shutdown.
//
// Since 1.4.0.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/example/generated-api/internal/app/server"
	"github.com/example/generated-api/internal/shared/applogger"
)

func main() {
	log := applogger.New()

	// Install the signal hook early so the server's Shutdown can drain.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		log.Info("main: signal received, shutting down")
		stop()
	}()

	if err := server.Boot(log); err != nil {
		// Distinguish AppError-typed messages (already user-friendly) from
		// bare errors.
		var msg string
		if err == nil {
			msg = "unknown"
		} else {
			msg = err.Error()
		}
		log.Error("main: boot failed", "err", err)
		fmt.Fprintln(os.Stderr, "api:", msg)
		// http.ErrServerClosed is expected on graceful shutdown.
		if errors.Is(err, context.Canceled) {
			return
		}
		os.Exit(1)
	}
}
