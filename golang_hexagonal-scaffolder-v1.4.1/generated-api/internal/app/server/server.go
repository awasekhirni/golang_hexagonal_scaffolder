// Package server wires the chi router, middleware stack, and routes, and
// runs the HTTP server with graceful shutdown.
//
// Since 1.4.0.
package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/example/generated-api/internal/app/middleware"
	"github.com/example/generated-api/internal/app/routes"
	"github.com/example/generated-api/internal/application/user"
	userctrl "github.com/example/generated-api/internal/application/user/controller"
	"github.com/example/generated-api/internal/config/settings"
	"github.com/example/generated-api/internal/domain/user/repository"
	"github.com/example/generated-api/internal/infra/bus"
	"github.com/example/generated-api/internal/infra/database"
	"github.com/example/generated-api/internal/shared/applogger"
	hexerr "github.com/example/generated-api/internal/shared/errors"
	"github.com/go-chi/chi/v5"
)

// Deps bundles the wired dependencies the server needs. Since 1.4.0.
type Deps struct {
	Log      *applogger.Logger
	Settings settings.Settings
	DB       *database.Factory
	UserRepo repository.UserRepository
	EventBus *bus.DomainEventBus
	CmdBus   *bus.EnterpriseBus
	Services *bus.ServiceRegistration
}

// Build constructs the chi router with the full middleware stack and routes.
// Since 1.4.0.
func Build(d Deps) *chi.Mux {
	r := chi.NewRouter()

	// Global middleware (order matters: outer → inner).
	r.Use(middleware.RequestID(d.Log))
	r.Use(middleware.Recover(d.Log))
	r.Use(middleware.Logging(d.Log))
	r.Use(middleware.SecurityHeaders)
	r.Use(middleware.CORS(d.Settings.CORSOrigins))
	rl := middleware.NewRateLimiter(d.Settings.RateLimitRPM)
	r.Use(rl.RateLimit)

	// Health (no auth).
	health := routes.NewHealthRoute(d.Log, d.DB)
	r.Get("/health", health.Combined)
	r.Get("/health/live", health.Live)
	r.Get("/health/ready", health.Ready)

	// Metrics (no auth).
	r.Get("/metrics", routes.Metrics)

	// Auth (no auth — issues tokens).
	auth := routes.NewAuthRoute(d.Log, d.Settings.JWTSecret, d.Settings.JWTIssuer, d.Settings.JWTTTLMin)
	r.Post("/auth/token", auth.Issue)

	// User routes (auth-protected).
	userSvc := user.NewService(d.Log, d.UserRepo)
	userController := userctrl.NewUserController(d.Log, userSvc)
	r.Group(func(r chi.Router) {
		r.Use(middleware.Auth(d.Settings.JWTSecret))
		r.Get("/users", userController.List)
		r.Post("/users", userController.Create)
		r.Get("/users/{id}", userController.Get)
		r.Put("/users/{id}", userController.Update)
		r.Delete("/users/{id}", userController.Delete)
	})

	return r
}

// Run starts the HTTP server with graceful shutdown. Since 1.4.0.
func Run(d Deps) error {
	return d.Log.Logged("server.Run", func() error {
		srv := &http.Server{
			Addr:              ":" + itoa(d.Settings.HTTPPort),
			Handler:           Build(d),
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       120 * time.Second,
		}

		errCh := make(chan error, 1)
		go func() {
			if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- err
			}
		}()

		// Block until the process is signalled (handled in main) or the
		// server errors.
		select {
		case err := <-errCh:
			d.Log.Error("server.Run: listen failed", "err", err)
			return hexerr.Wrap(hexerr.CodeInternal, "listen failed", 500, "internal", err)
		case <-context.Background().Done():
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(d.Settings.GracefulSec)*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			d.Log.Error("server.Run: graceful shutdown failed", "err", err)
			return hexerr.Wrap(hexerr.CodeInternal, "shutdown failed", 500, "internal", err)
		}
		return nil
	})
}

// Boot is the entry point used by main: it loads settings, opens the DB,
// applies migrations, wires deps, and runs the server. Since 1.4.0.
func Boot(log *applogger.Logger) error {
	return log.Logged("server.Boot", func() error {
		s, err := settings.ToSettings(log, envGet)
		if err != nil {
			return err
		}
		db := database.New(log)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := db.Init(ctx, database.DriverName, s.DSN(), s.PoolSize, s.MaxOverflow, s.ConnTimeout); err != nil {
			return err
		}
		// Apply migrations from the embedded resources path.
		if err := db.Migrate(ctx, migrations()); err != nil {
			return err
		}
		repo := repository.NewUserPostgres(log, db)
		deps := Deps{
			Log:      log,
			Settings: s,
			DB:       db,
			UserRepo: repo,
			EventBus: bus.NewDomainEventBus(log),
			CmdBus:   bus.NewEnterpriseBus(log),
			Services: bus.NewServiceRegistration(log),
		}
		defer db.Close()
		return Run(deps)
	})
}

// itoa is a dependency-free int → string. Since 1.4.0.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		return "-" + string(b)
	}
	return string(b)
}

// envGet is os.Getenv, swappable for tests. Since 1.4.0.
var envGet = func(k string) string { return _osGetenv(k) }
