package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	domainuser "github.com/example/generated-api/internal/domain/user"
	"github.com/example/generated-api/internal/application/user"
	"github.com/example/generated-api/internal/shared/applogger"
	hexerr "github.com/example/generated-api/internal/shared/errors"
	"github.com/go-chi/chi/v5"
)

func log() *applogger.Logger { return applogger.NewWithEnv(func(string) string { return "" }) }

// fakeRepo is an in-memory repository for controller tests. Since 1.4.0.
type fakeRepo struct {
	users map[string]*domainuser.User
}

func newFakeRepo() *fakeRepo { return &fakeRepo{users: map[string]*domainuser.User{}} }

func (r *fakeRepo) FindByID(ctx context.Context, id string) (*domainuser.User, error) {
	u, ok := r.users[id]
	if !ok {
		return nil, hexerr.NotFound("user not found: id=" + id)
	}
	return u, nil
}
func (r *fakeRepo) List(ctx context.Context) ([]*domainuser.User, error) {
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
		return hexerr.NotFound("user not found")
	}
	r.users[u.ID] = u
	return nil
}
func (r *fakeRepo) Delete(ctx context.Context, id string) error {
	if _, ok := r.users[id]; !ok {
		return hexerr.NotFound("user not found")
	}
	delete(r.users, id)
	return nil
}

func newController() (*UserController, *user.Service) {
	repo := newFakeRepo()
	svc := user.NewService(log(), repo)
	return NewUserController(log(), svc), svc
}

// doJSON sends a request with the given method, path, and body to the handler.
func doJSON(h http.HandlerFunc, method, path string, body string) *httptest.ResponseRecorder {
	var r *strings.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	var req *http.Request
	if r != nil {
		req = httptest.NewRequest(method, path, r)
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

// TestCreate_happy covers 201. Since 1.4.0.
func TestCreate_happy(t *testing.T) {
	c, _ := newController()
	rec := doJSON(c.Create, "POST", "/users", `{"email":"a@b.com","name":"Alice"}`)
	if rec.Code != 201 {
		t.Fatalf("Create: want 201, got %d", rec.Code)
	}
	var u domainuser.User
	_ = json.NewDecoder(rec.Body).Decode(&u)
	if u.Email != "a@b.com" {
		t.Fatalf("email: %q", u.Email)
	}
}

// TestCreate_badJSON covers P0-3.6: API-001 on malformed body. Since 1.4.0.
func TestCreate_badJSON(t *testing.T) {
	c, _ := newController()
	rec := doJSON(c.Create, "POST", "/users", `{not json`)
	if rec.Code != 400 {
		t.Fatalf("bad JSON: want 400, got %d", rec.Code)
	}
	var body map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if body["code"] != "API-001" {
		t.Fatalf("code: want API-001, got %q", body["code"])
	}
}

// TestCreate_missingEmail covers P0-3.6: API-001 on missing field. Since 1.4.0.
func TestCreate_missingEmail(t *testing.T) {
	c, _ := newController()
	rec := doJSON(c.Create, "POST", "/users", `{"name":"Alice"}`)
	if rec.Code != 400 {
		t.Fatalf("missing email: want 400, got %d", rec.Code)
	}
}

// TestCreate_badEmail covers P0-3.6: API-001 on bad email. Since 1.4.0.
func TestCreate_badEmail(t *testing.T) {
	c, _ := newController()
	rec := doJSON(c.Create, "POST", "/users", `{"email":"not-an-email","name":"Alice"}`)
	if rec.Code != 400 {
		t.Fatalf("bad email: want 400, got %d", rec.Code)
	}
}

// TestList_happy covers 200. Since 1.4.0.
func TestList_happy(t *testing.T) {
	c, svc := newController()
	_, _ = svc.Create(context.Background(), "a@b.com", "Alice")
	rec := doJSON(c.List, "GET", "/users", "")
	if rec.Code != 200 {
		t.Fatalf("List: want 200, got %d", rec.Code)
	}
}

// TestGet_notFound covers P0-3.6: API-002 (not raw map). Since 1.4.0.
func TestGet_notFound(t *testing.T) {
	c, _ := newController()
	// Use a chi router so URLParam works.
	r := chi.NewRouter()
	r.Get("/users/{id}", c.Get)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/users/missing-id", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != 404 {
		t.Fatalf("Get not found: want 404, got %d", rec.Code)
	}
	var body map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if body["code"] != "API-002" {
		t.Fatalf("code: want API-002, got %q", body["code"])
	}
	// P0-3.6 regression: the body must NOT be a raw {"error": ...} map.
	if _, hasError := body["error"]; hasError {
		t.Fatalf("P0-3.6 regression: handler returned raw 'error' key")
	}
}

// TestGet_happy covers 200. Since 1.4.0.
func TestGet_happy(t *testing.T) {
	c, svc := newController()
	u, _ := svc.Create(context.Background(), "a@b.com", "Alice")
	r := chi.NewRouter()
	r.Get("/users/{id}", c.Get)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/users/"+u.ID, nil)
	r.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("Get: want 200, got %d", rec.Code)
	}
}

// TestDelete_happy covers 204. Since 1.4.0.
func TestDelete_happy(t *testing.T) {
	c, svc := newController()
	u, _ := svc.Create(context.Background(), "a@b.com", "Alice")
	r := chi.NewRouter()
	r.Delete("/users/{id}", c.Delete)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", "/users/"+u.ID, nil)
	r.ServeHTTP(rec, req)
	if rec.Code != 204 {
		t.Fatalf("Delete: want 204, got %d", rec.Code)
	}
}

// TestUpdate_happy covers 200. Since 1.4.0.
func TestUpdate_happy(t *testing.T) {
	c, svc := newController()
	u, _ := svc.Create(context.Background(), "a@b.com", "Alice")
	r := chi.NewRouter()
	r.Put("/users/{id}", c.Update)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/users/"+u.ID, strings.NewReader(`{"email":"c@d.com","name":"Bob"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("Update: want 200, got %d", rec.Code)
	}
}
