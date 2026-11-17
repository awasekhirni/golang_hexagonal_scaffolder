package user

import (
	"context"
	"strings"
	"testing"

	domainuser "github.com/example/generated-api/internal/domain/user"
	"github.com/example/generated-api/internal/shared/applogger"
	hexerr "github.com/example/generated-api/internal/shared/errors"
)

func log() *applogger.Logger { return applogger.NewWithEnv(func(string) string { return "" }) }

// fakeRepo is an in-memory UserRepository for service/controller tests.
// Since 1.4.0.
type fakeRepo struct {
	users map[string]*domainuser.User
	fail  bool
}

func newFakeRepo() *fakeRepo { return &fakeRepo{users: map[string]*domainuser.User{}} }

func (r *fakeRepo) FindByID(ctx context.Context, id string) (*domainuser.User, error) {
	if r.fail {
		return nil, hexerr.Internal("forced failure")
	}
	u, ok := r.users[id]
	if !ok {
		return nil, hexerr.NotFound("user not found: id=" + id)
	}
	return u, nil
}

func (r *fakeRepo) List(ctx context.Context) ([]*domainuser.User, error) {
	if r.fail {
		return nil, hexerr.Internal("forced failure")
	}
	var out []*domainuser.User
	for _, u := range r.users {
		out = append(out, u)
	}
	return out, nil
}

func (r *fakeRepo) Save(ctx context.Context, u *domainuser.User) error {
	r.users[u.ID] = u
	return nil
}

func (r *fakeRepo) Update(ctx context.Context, u *domainuser.User) error {
	if _, ok := r.users[u.ID]; !ok {
		return hexerr.NotFound("user not found: id=" + u.ID)
	}
	r.users[u.ID] = u
	return nil
}

func (r *fakeRepo) Delete(ctx context.Context, id string) error {
	if _, ok := r.users[id]; !ok {
		return hexerr.NotFound("user not found: id=" + id)
	}
	delete(r.users, id)
	return nil
}

// TestService_Create_happy covers the create use case. Since 1.4.0.
func TestService_Create_happy(t *testing.T) {
	s := NewService(log(), newFakeRepo())
	u, err := s.Create(context.Background(), "a@b.com", "Alice")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if u.Email != "a@b.com" {
		t.Fatalf("email: %q", u.Email)
	}
}

// TestService_Create_badEmail covers the domain error pass-through. Since 1.4.0.
func TestService_Create_badEmail(t *testing.T) {
	s := NewService(log(), newFakeRepo())
	_, err := s.Create(context.Background(), "not-an-email", "Alice")
	if err == nil {
		t.Fatalf("Create bad email: want error")
	}
	ae, ok := hexerr.IsAppError(err)
	if !ok || ae.Code() != "API-008" {
		t.Fatalf("Create bad email: want API-008, got %v", err)
	}
}

// TestService_Get_notFound covers API-002. Since 1.4.0.
func TestService_Get_notFound(t *testing.T) {
	s := NewService(log(), newFakeRepo())
	_, err := s.Get(context.Background(), "missing")
	if err == nil || !strings.Contains(err.Error(), "API-002") {
		t.Fatalf("Get not found: want API-002, got %v", err)
	}
}

// TestService_List_empty covers the empty list default. Since 1.4.0.
func TestService_List_empty(t *testing.T) {
	s := NewService(log(), newFakeRepo())
	users, err := s.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(users) != 0 {
		t.Fatalf("empty list: want 0, got %d", len(users))
	}
}

// TestService_Update_happy covers update. Since 1.4.0.
func TestService_Update_happy(t *testing.T) {
	repo := newFakeRepo()
	s := NewService(log(), repo)
	u, _ := s.Create(context.Background(), "a@b.com", "Alice")
	updated, err := s.Update(context.Background(), u.ID, "c@d.com", "Bob")
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Email != "c@d.com" || updated.Name != "Bob" {
		t.Fatalf("Update: %+v", updated)
	}
}

// TestService_Delete_happy covers delete. Since 1.4.0.
func TestService_Delete_happy(t *testing.T) {
	repo := newFakeRepo()
	s := NewService(log(), repo)
	u, _ := s.Create(context.Background(), "a@b.com", "Alice")
	if err := s.Delete(context.Background(), u.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := s.Get(context.Background(), u.ID)
	if err == nil || !strings.Contains(err.Error(), "API-002") {
		t.Fatalf("Get after delete: want API-002, got %v", err)
	}
}
