// Package controller contains the UserController HTTP handlers.
//
// v1.4.0 / P0-3.6: every handler renders typed AppErrors (API-001/002/...)
// for client errors via middleware.WriteError — never a raw map["error"].
// Every handler is Logged-wrapped.
//
// Since 1.4.0.
package controller

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"

	"github.com/example/generated-api/internal/app/middleware"
	"github.com/example/generated-api/internal/application/user"
	"github.com/example/generated-api/internal/shared/applogger"
	hexerr "github.com/example/generated-api/internal/shared/errors"
	"github.com/go-chi/chi/v5"
)

// UserController is the HTTP driving adapter for users. Since 1.4.0.
type UserController struct {
	log *applogger.Logger
	svc *user.Service
}

// NewUserController constructs the controller. Since 1.4.0.
func NewUserController(log *applogger.Logger, svc *user.Service) *UserController {
	if log == nil {
		log = applogger.New()
	}
	return &UserController{log: log, svc: svc}
}

// userInput is the create/update body. Since 1.4.0.
type userInput struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

// List handles GET /users. Since 1.4.0.
func (c *UserController) List(w http.ResponseWriter, r *http.Request) {
	err := c.log.Logged("UserController.List", func() error {
		users, err := c.svc.List(r.Context())
		if err != nil {
			return err
		}
		writeJSON(w, http.StatusOK, users)
		return nil
	})
	if err != nil {
		middleware.WriteError(w, r, err, c.log)
	}
}

// Create handles POST /users. Since 1.4.0.
func (c *UserController) Create(w http.ResponseWriter, r *http.Request) {
	err := c.log.Logged("UserController.Create", func() error {
		in, err := decodeUserInput(r)
		if err != nil {
			return err
		}
		u, err := c.svc.Create(r.Context(), in.Email, in.Name)
		if err != nil {
			return err
		}
		writeJSON(w, http.StatusCreated, u)
		return nil
	})
	if err != nil {
		middleware.WriteError(w, r, err, c.log)
	}
}

// Get handles GET /users/{id}. Since 1.4.0.
func (c *UserController) Get(w http.ResponseWriter, r *http.Request) {
	err := c.log.Logged("UserController.Get", func() error {
		id := chi.URLParam(r, "id")
		if id == "" {
			c.log.Error("UserController.Get: missing id")
			return hexerr.Validation("id is required")
		}
		u, err := c.svc.Get(r.Context(), id)
		if err != nil {
			return err
		}
		writeJSON(w, http.StatusOK, u)
		return nil
	})
	if err != nil {
		middleware.WriteError(w, r, err, c.log)
	}
}

// Update handles PUT /users/{id}. Since 1.4.0.
func (c *UserController) Update(w http.ResponseWriter, r *http.Request) {
	err := c.log.Logged("UserController.Update", func() error {
		id := chi.URLParam(r, "id")
		if id == "" {
			return hexerr.Validation("id is required")
		}
		in, err := decodeUserInput(r)
		if err != nil {
			return err
		}
		u, err := c.svc.Update(r.Context(), id, in.Email, in.Name)
		if err != nil {
			return err
		}
		writeJSON(w, http.StatusOK, u)
		return nil
	})
	if err != nil {
		middleware.WriteError(w, r, err, c.log)
	}
}

// Delete handles DELETE /users/{id}. Since 1.4.0.
func (c *UserController) Delete(w http.ResponseWriter, r *http.Request) {
	err := c.log.Logged("UserController.Delete", func() error {
		id := chi.URLParam(r, "id")
		if id == "" {
			return hexerr.Validation("id is required")
		}
		if err := c.svc.Delete(r.Context(), id); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	})
	if err != nil {
		middleware.WriteError(w, r, err, c.log)
	}
}

// decodeUserInput parses and validates the JSON body. P0-3.6: returns typed
// API-001 on a malformed body. Since 1.4.0.
func decodeUserInput(r *http.Request) (userInput, error) {
	if r.Body == nil {
		return userInput{}, hexerr.Validation("request body is required")
	}
	if ct := r.Header.Get("Content-Type"); ct != "" && !containsJSON(ct) {
		return userInput{}, hexerr.New(hexerr.CodeUnsupportedMedia, "Content-Type must be application/json", 415, "unsupported-media-type")
	}
	var in userInput
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		if err == io.EOF {
			return userInput{}, hexerr.Validation("request body is empty")
		}
		return userInput{}, hexerr.Validation("invalid JSON: " + err.Error())
	}
	if in.Email == "" {
		return userInput{}, hexerr.Validation("email is required")
	}
	if !emailRE.MatchString(in.Email) {
		return userInput{}, hexerr.Validation("email must be a valid email address")
	}
	if in.Name == "" {
		return userInput{}, hexerr.Validation("name is required")
	}
	return in, nil
}

// writeJSON renders a JSON response. Since 1.4.0.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// emailRE validates the input email. Since 1.4.0.
var emailRE = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// containsJSON reports whether a Content-Type header indicates JSON.
// Since 1.4.0.
func containsJSON(ct string) bool {
	return ct == "application/json" || (len(ct) >= 16 && ct[:16] == "application/json")
}
