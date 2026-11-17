package user

import (
	"strings"
	"testing"

	"github.com/example/generated-api/internal/shared/applogger"
)

func log() *applogger.Logger { return applogger.NewWithEnv(func(string) string { return "" }) }

// TestCreate_happy covers the success path. Since 1.4.0.
func TestCreate_happy(t *testing.T) {
	u, err := Create(log(), "id1", "a@b.com", "Alice")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if u.Email != "a@b.com" || u.Name != "Alice" {
		t.Fatalf("Create: %+v", u)
	}
}

// TestCreate_emptyID covers P0-3.2: typed UserDomainError, not a panic.
// Since 1.4.0.
func TestCreate_emptyID(t *testing.T) {
	_, err := Create(log(), "", "a@b.com", "Alice")
	if err == nil {
		t.Fatalf("Create: want error")
	}
	if !strings.Contains(err.Error(), "id") {
		t.Fatalf("Create empty id: %v", err)
	}
}

// TestCreate_emptyEmail covers P0-3.2. Since 1.4.0.
func TestCreate_emptyEmail(t *testing.T) {
	_, err := Create(log(), "id1", "", "Alice")
	if err == nil || !strings.Contains(err.Error(), "email") {
		t.Fatalf("Create empty email: %v", err)
	}
}

// TestCreate_badEmail covers P0-3.2: invalid email format. Since 1.4.0.
func TestCreate_badEmail(t *testing.T) {
	_, err := Create(log(), "id1", "not-an-email", "Alice")
	if err == nil || !strings.Contains(err.Error(), "email") {
		t.Fatalf("Create bad email: %v", err)
	}
}

// TestCreate_emptyName covers P0-3.2. Since 1.4.0.
func TestCreate_emptyName(t *testing.T) {
	_, err := Create(log(), "id1", "a@b.com", "")
	if err == nil || !strings.Contains(err.Error(), "name") {
		t.Fatalf("Create empty name: %v", err)
	}
}

// TestUserDomainError_code covers the typed error contract. Since 1.4.0.
func TestUserDomainError_code(t *testing.T) {
	e := UserDomainError{Field: "email", Message: "bad"}
	if e.Code() != "API-008" {
		t.Fatalf("Code: want API-008, got %s", e.Code())
	}
	if e.HTTPStatus() != 422 {
		t.Fatalf("HTTPStatus: want 422, got %d", e.HTTPStatus())
	}
	if e.Category() != "domain" {
		t.Fatalf("Category: want domain, got %s", e.Category())
	}
}

// TestUpdate_happy covers mutating fields. Since 1.4.0.
func TestUpdate_happy(t *testing.T) {
	u, _ := Create(log(), "id1", "a@b.com", "Alice")
	if err := u.Update(log(), "c@d.com", "Bob"); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if u.Email != "c@d.com" || u.Name != "Bob" {
		t.Fatalf("Update: %+v", u)
	}
}

// TestUpdate_badEmail covers the validation path. Since 1.4.0.
func TestUpdate_badEmail(t *testing.T) {
	u, _ := Create(log(), "id1", "a@b.com", "Alice")
	if err := u.Update(log(), "bad", ""); err == nil {
		t.Fatalf("Update bad email: want error")
	}
}
