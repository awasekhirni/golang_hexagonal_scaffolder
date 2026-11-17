// Package user contains the UserService use-case layer.
//
// UserService orchestrates the domain entity and the repository port-out.
// Every method is Logged-wrapped; every catch-and-rethrow logs the cause.
//
// Since 1.4.0.
package user

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	domainuser "github.com/example/generated-api/internal/domain/user"
	"github.com/example/generated-api/internal/domain/user/repository"
	"github.com/example/generated-api/internal/shared/applogger"
	hexerr "github.com/example/generated-api/internal/shared/errors"
)

// Service is the application service for users. Since 1.4.0.
type Service struct {
	log *applogger.Logger
	repo repository.UserRepository
}

// NewService constructs a Service. Since 1.4.0.
func NewService(log *applogger.Logger, repo repository.UserRepository) *Service {
	if log == nil {
		log = applogger.New()
	}
	return &Service{log: log, repo: repo}
}

// Create builds a domain user and persists it. Since 1.4.0.
func (s *Service) Create(ctx context.Context, email, name string) (*domainuser.User, error) {
	return applogger.Logged1(s.log, "UserService.Create", func() (*domainuser.User, error) {
		id := newID()
		u, err := domainuser.Create(s.log, id, email, name)
		if err != nil {
			return nil, err
		}
		if err := s.repo.Save(ctx, &u); err != nil {
			return nil, err
		}
		return &u, nil
	})
}

// Get loads a user by id. Since 1.4.0.
func (s *Service) Get(ctx context.Context, id string) (*domainuser.User, error) {
	return applogger.Logged1(s.log, "UserService.Get", func() (*domainuser.User, error) {
		u, err := s.repo.FindByID(ctx, id)
		if err != nil {
			return nil, err
		}
		return u, nil
	})
}

// List returns all users. Since 1.4.0.
func (s *Service) List(ctx context.Context) ([]*domainuser.User, error) {
	return applogger.Logged1(s.log, "UserService.List", func() ([]*domainuser.User, error) {
		users, err := s.repo.List(ctx)
		if err != nil {
			return nil, err
		}
		if users == nil {
			users = []*domainuser.User{}
		}
		return users, nil
	})
}

// Update mutates a user. Since 1.4.0.
func (s *Service) Update(ctx context.Context, id, email, name string) (*domainuser.User, error) {
	return applogger.Logged1(s.log, "UserService.Update", func() (*domainuser.User, error) {
		u, err := s.repo.FindByID(ctx, id)
		if err != nil {
			return nil, err
		}
		if err := u.Update(s.log, email, name); err != nil {
			return nil, err
		}
		if err := s.repo.Update(ctx, u); err != nil {
			return nil, err
		}
		return u, nil
	})
}

// Delete removes a user. Since 1.4.0.
func (s *Service) Delete(ctx context.Context, id string) error {
	return s.log.Logged("UserService.Delete", func() error {
		if id == "" {
			s.log.Error("UserService.Delete: empty id")
			return hexerr.Validation("id is required")
		}
		return s.repo.Delete(ctx, id)
	})
}

// newID generates a random hex id (not a real UUID, but unique enough for the
// reference implementation; the schema's DEFAULT gen_random_uuid() is the
// real source of truth in production). Since 1.4.0.
func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
