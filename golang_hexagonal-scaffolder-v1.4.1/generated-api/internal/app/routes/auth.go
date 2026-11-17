// Package routes — auth token issuer (POST /auth/token).
//
// This is a dev-friendly token issuer: any non-empty email/password yields a
// JWT. Production should replace it with a real credential check. Since 1.4.0.

package routes

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/example/generated-api/internal/app/middleware"
	"github.com/example/generated-api/internal/shared/applogger"
	hexerr "github.com/example/generated-api/internal/shared/errors"
)

// AuthRoute issues JWTs. Since 1.4.0.
type AuthRoute struct {
	log      *applogger.Logger
	secret   string
	issuer   string
	ttlMin   int
}

// NewAuthRoute constructs the route. Since 1.4.0.
func NewAuthRoute(log *applogger.Logger, secret, issuer string, ttlMin int) *AuthRoute {
	if log == nil {
		log = applogger.New()
	}
	if ttlMin <= 0 {
		ttlMin = 60
	}
	return &AuthRoute{log: log, secret: secret, issuer: issuer, ttlMin: ttlMin}
}

// authInput is the token request body. Since 1.4.0.
type authInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Issue handles POST /auth/token. Since 1.4.0.
func (a *AuthRoute) Issue(w http.ResponseWriter, r *http.Request) {
	_ = a.log.Logged("AuthRoute.Issue", func() error {
		if a.secret == "" {
			a.log.Error("AuthRoute.Issue: JWT_SECRET not configured")
			return hexerr.New(hexerr.CodeInternal, "JWT_SECRET not configured", 500, "internal")
		}
		var in authInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			return hexerr.Validation("invalid JSON: " + err.Error())
		}
		if in.Email == "" || in.Password == "" {
			return hexerr.Validation("email and password are required")
		}
		// Build a minimal JWT payload.
		payload := `{"iss":"` + a.issuer + `","sub":"` + in.Email + `","exp":` + expStr(a.ttlMin) + `}`
		token := middleware.IssueToken(a.secret, payload)
		writeJSON(w, http.StatusOK, map[string]any{
			"token":      token,
			"expires_in": a.ttlMin * 60,
		})
		return nil
	})
}

// expStr returns the unix expiry timestamp as a string. Since 1.4.0.
func expStr(ttlMin int) string {
	return time.Now().UTC().Add(time.Duration(ttlMin) * time.Minute).Format("1700000000")
}

// silence unused import
var _ = strings.TrimSpace
