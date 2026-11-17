// Package repository contains the UserRepository port-out interface and its
// Postgres adapter.
//
// v1.4.0 / P0-3.1: Save maps a duplicate-key driver error to API-003. The
// mapping is reachable because DatabaseFactory.Tx has an AppError
// pass-through (the raw driver error is returned by the tx body, then Tx
// wraps non-AppErrors as API-007; Save catches the duplicate pattern on the
// returned error and converts to API-003). FindByID returns API-002 when the
// row is missing.
//
// Since 1.4.0.
package repository

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/example/generated-api/internal/domain/user"
	"github.com/example/generated-api/internal/infra/database"
	"github.com/example/generated-api/internal/shared/applogger"
	hexerr "github.com/example/generated-api/internal/shared/errors"
)

// UserRepository is the port-out. Since 1.4.0.
type UserRepository interface {
	FindByID(ctx context.Context, id string) (*user.User, error)
	List(ctx context.Context) ([]*user.User, error)
	Save(ctx context.Context, u *user.User) error
	Update(ctx context.Context, u *user.User) error
	Delete(ctx context.Context, id string) error
}

// UserPostgres is the Postgres adapter. Since 1.4.0.
type UserPostgres struct {
	log *applogger.Logger
	db  *database.Factory
}

// NewUserPostgres constructs the adapter. Since 1.4.0.
func NewUserPostgres(log *applogger.Logger, db *database.Factory) *UserPostgres {
	if log == nil {
		log = applogger.New()
	}
	return &UserPostgres{log: log, db: db}
}

// FindByID loads a user by id. API-002 on not-found. Since 1.4.0.
func (r *UserPostgres) FindByID(ctx context.Context, id string) (*user.User, error) {
	return applogger.Logged1(r.log, "UserRepository.FindByID", func() (*user.User, error) {
		if id == "" {
			r.log.Error("UserRepository.FindByID: empty id")
			return nil, hexerr.Validation("id is required")
		}
		var u user.User
		err := r.db.Tx(ctx, func(tx *sql.Tx) error {
			return tx.QueryRowContext(ctx,
				`SELECT id, email, name, created_at, updated_at FROM users WHERE id = $1`, id).
				Scan(&u.ID, &u.Email, &u.Name, &u.CreatedAt, &u.UpdatedAt)
		})
		if err != nil {
			if isNotFound(err) {
				r.log.Error("UserRepository.FindByID: not found", "id", id)
				return nil, hexerr.NotFound("user not found: id=" + id)
			}
			// AppError pass-through from Tx.
			if _, ok := hexerr.IsAppError(err); ok {
				return nil, err
			}
			r.log.Error("UserRepository.FindByID: query failed", "err", err)
			return nil, hexerr.Wrap(hexerr.CodeInternal, "find failed", 500, "internal", err)
		}
		return &u, nil
	})
}

// List returns all users. Since 1.4.0.
func (r *UserPostgres) List(ctx context.Context) ([]*user.User, error) {
	return applogger.Logged1(r.log, "UserRepository.List", func() ([]*user.User, error) {
		rows, err := r.db.DB().QueryContext(ctx,
			`SELECT id, email, name, created_at, updated_at FROM users ORDER BY created_at DESC`)
		if err != nil {
			r.log.Error("UserRepository.List: query failed", "err", err)
			return nil, hexerr.Wrap(hexerr.CodeInternal, "list failed", 500, "internal", err)
		}
		defer rows.Close()
		var out []*user.User
		for rows.Next() {
			var u user.User
			if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.CreatedAt, &u.UpdatedAt); err != nil {
				r.log.Error("UserRepository.List: scan failed", "err", err)
				return nil, hexerr.Wrap(hexerr.CodeInternal, "scan failed", 500, "internal", err)
			}
			out = append(out, &u)
		}
		return out, rows.Err()
	})
}

// Save inserts a new user. P0-3.1: duplicate-key detection happens INSIDE
// the Tx body so the typed Conflict (API-003) is returned as an AppError,
// which Tx passes through unchanged (instead of being swallowed by Tx's
// generic catch). Since 1.4.0.
func (r *UserPostgres) Save(ctx context.Context, u *user.User) error {
	return r.log.Logged("UserRepository.Save", func() error {
		if u == nil {
			r.log.Error("UserRepository.Save: nil user")
			return hexerr.Validation("user is required")
		}
		err := r.db.Tx(ctx, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx,
				`INSERT INTO users (id, email, name, created_at, updated_at) VALUES ($1, $2, $3, $4, $5)`,
				u.ID, u.Email, u.Name, u.CreatedAt, u.UpdatedAt)
			if err != nil {
				// P0-3.1: detect the duplicate INSIDE the tx body, before Tx
				// wraps it as a generic API-007. Return a typed AppError so
				// Tx's AppError pass-through preserves it.
				if isDuplicateKey(err) {
					r.log.Error("UserRepository.Save: duplicate key", "email", u.Email, "err", err)
					return hexerr.Conflict("a user with this email already exists")
				}
				return err
			}
			return nil
		})
		if err != nil {
			// AppError pass-through (the Conflict we returned above).
			if ae, ok := hexerr.IsAppError(err); ok {
				return ae
			}
			r.log.Error("UserRepository.Save: insert failed", "err", err)
			return hexerr.Wrap(hexerr.CodeInternal, "save failed", 500, "internal", err)
		}
		return nil
	})
}

// Update mutates an existing user. P0-3.1: not-found and duplicate detection
// happen inside the Tx body. Since 1.4.0.
func (r *UserPostgres) Update(ctx context.Context, u *user.User) error {
	return r.log.Logged("UserRepository.Update", func() error {
		if u == nil {
			return hexerr.Validation("user is required")
		}
		err := r.db.Tx(ctx, func(tx *sql.Tx) error {
			res, err := tx.ExecContext(ctx,
				`UPDATE users SET email=$1, name=$2, updated_at=$3 WHERE id=$4`,
				u.Email, u.Name, time.Now().UTC(), u.ID)
			if err != nil {
				if isDuplicateKey(err) {
					return hexerr.Conflict("a user with this email already exists")
				}
				return err
			}
			if n, _ := res.RowsAffected(); n == 0 {
				return hexerr.NotFound("user not found: id=" + u.ID)
			}
			return nil
		})
		if err != nil {
			if ae, ok := hexerr.IsAppError(err); ok {
				return ae
			}
			r.log.Error("UserRepository.Update: failed", "err", err)
			return hexerr.Wrap(hexerr.CodeInternal, "update failed", 500, "internal", err)
		}
		return nil
	})
}

// Delete removes a user by id. P0-3.1: not-found detection inside the Tx.
// Since 1.4.0.
func (r *UserPostgres) Delete(ctx context.Context, id string) error {
	return r.log.Logged("UserRepository.Delete", func() error {
		if id == "" {
			return hexerr.Validation("id is required")
		}
		err := r.db.Tx(ctx, func(tx *sql.Tx) error {
			res, err := tx.ExecContext(ctx, `DELETE FROM users WHERE id=$1`, id)
			if err != nil {
				return err
			}
			if n, _ := res.RowsAffected(); n == 0 {
				return hexerr.NotFound("user not found: id=" + id)
			}
			return nil
		})
		if err != nil {
			if ae, ok := hexerr.IsAppError(err); ok {
				return ae
			}
			r.log.Error("UserRepository.Delete: failed", "err", err)
			return hexerr.Wrap(hexerr.CodeInternal, "delete failed", 500, "internal", err)
		}
		return nil
	})
}

// isNotFound reports whether err is sql.ErrNoRows. Since 1.4.0.
func isNotFound(err error) bool {
	return err == sql.ErrNoRows || strings.Contains(err.Error(), "no rows")
}

// isDuplicateKey detects a Postgres unique-violation. Since 1.4.0.
func isDuplicateKey(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "unique constraint") ||
		strings.Contains(msg, "duplicate key") ||
		strings.Contains(msg, "23505") // SQLSTATE for unique_violation
}
