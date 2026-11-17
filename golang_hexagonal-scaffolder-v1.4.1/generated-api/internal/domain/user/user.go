// Package user contains the User domain entity and its invariants.
//
// v1.4.0 / P0-3.2: Create enforces invariants by returning a typed
// UserDomainError (API-008), never a bare panic or IllegalArgumentException.
//
// Since 1.4.0.
package user

import (
	"regexp"
	"time"

	"github.com/example/generated-api/internal/shared/applogger"
	hexerr "github.com/example/generated-api/internal/shared/errors"
)

// emailRE is a pragmatic email pattern (not RFC-perfect but good enough for
// domain validation). Since 1.4.0.
var emailRE = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// User is the domain entity. Since 1.4.0.
type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// UserDomainError is returned when a domain invariant is violated (API-008).
// Since 1.4.0.
type UserDomainError struct {
	Field   string
	Message string
}

// Error formats the message. Since 1.4.0.
func (e UserDomainError) Error() string { return e.Field + ": " + e.Message }

// Code returns the stable error code. Since 1.4.0.
func (e UserDomainError) Code() string { return hexerr.CodeDomain }

// HTTPStatus returns 422. Since 1.4.0.
func (e UserDomainError) HTTPStatus() int { return 422 }

// Category returns "domain". Since 1.4.0.
func (e UserDomainError) Category() string { return "domain" }

// Create validates invariants and returns a populated User. P0-3.2: returns
// UserDomainError (never a bare panic). Since 1.4.0.
func Create(log *applogger.Logger, id, email, name string) (User, error) {
	return applogger.Logged1(log, "UserEntity.Create", func() (User, error) {
		if id == "" {
			log.Error("UserEntity.Create: empty id")
			return User{}, UserDomainError{Field: "id", Message: "is required"}
		}
		if email == "" {
			log.Error("UserEntity.Create: empty email")
			return User{}, UserDomainError{Field: "email", Message: "is required"}
		}
		if !emailRE.MatchString(email) {
			log.Error("UserEntity.Create: bad email format", "email", email)
			return User{}, UserDomainError{Field: "email", Message: "must be a valid email address"}
		}
		if name == "" {
			log.Error("UserEntity.Create: empty name")
			return User{}, UserDomainError{Field: "name", Message: "is required"}
		}
		now := time.Now().UTC()
		return User{
			ID:        id,
			Email:     email,
			Name:      name,
			CreatedAt: now,
			UpdatedAt: now,
		}, nil
	})
}

// Update mutates editable fields and stamps UpdatedAt. Since 1.4.0.
func (u *User) Update(log *applogger.Logger, email, name string) error {
	return log.Logged("UserEntity.Update", func() error {
		if email != "" && !emailRE.MatchString(email) {
			log.Error("UserEntity.Update: bad email", "email", email)
			return UserDomainError{Field: "email", Message: "must be a valid email address"}
		}
		if email != "" {
			u.Email = email
		}
		if name != "" {
			u.Name = name
		}
		u.UpdatedAt = time.Now().UTC()
		return nil
	})
}
