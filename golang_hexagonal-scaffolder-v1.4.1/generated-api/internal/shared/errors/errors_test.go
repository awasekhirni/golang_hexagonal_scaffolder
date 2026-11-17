package errors

import (
	"testing"
)

// TestConstructors cover each convenience constructor. Since 1.4.0.
func TestConstructors(t *testing.T) {
	cases := []struct {
		got AppErr
		code string
		http int
		cat  string
	}{
		{Validation("x"), CodeValidation, 400, "validation"},
		{NotFound("x"), CodeNotFound, 404, "not-found"},
		{Conflict("x"), CodeConflict, 409, "conflict"},
		{Auth("x"), CodeAuth, 401, "auth"},
		{RateLimit("x"), CodeRateLimit, 429, "rate-limit"},
		{Unavailable("x"), CodeUnavailable, 503, "availability"},
		{Internal("x"), CodeInternal, 500, "internal"},
		{Domain("x"), CodeDomain, 422, "domain"},
	}
	for _, c := range cases {
		if c.got.Code() != c.code || c.got.HTTPStatus() != c.http || c.got.Category() != c.cat {
			t.Fatalf("constructor mismatch: %+v", c)
		}
	}
}

// TestWrap_preservesCause covers Unwrap. Since 1.4.0.
func TestWrap_preservesCause(t *testing.T) {
	cause := Internal("root")
	wrapped := Wrap(CodeInternal, "outer", 500, "internal", cause)
	if wrapped.Unwrap() != cause {
		t.Fatalf("Unwrap: want root cause")
	}
}

// TestIsAppError covers the type assertion. Since 1.4.0.
func TestIsAppError(t *testing.T) {
	ae, ok := IsAppError(NotFound("x"))
	if !ok || ae.Code() != CodeNotFound {
		t.Fatalf("IsAppError: want API-002")
	}
	if _, ok := IsAppError(nil); ok {
		t.Fatalf("IsAppError(nil): want false")
	}
	if _, ok := IsAppError(errString("plain")); ok {
		t.Fatalf("IsAppError(plain): want false")
	}
}

// TestError_format covers the message format. Since 1.4.0.
func TestError_format(t *testing.T) {
	e := New(CodeNotFound, "user gone", 404, "not-found")
	if e.Error() != "API-002: user gone" {
		t.Fatalf("Error: got %q", e.Error())
	}
}

// TestAllCodes_distinct ensures no two codes collide. Since 1.4.0.
func TestAllCodes_distinct(t *testing.T) {
	codes := []string{
		CodeValidation, CodeNotFound, CodeConflict, CodeAuth, CodeRateLimit,
		CodeUnavailable, CodeInternal, CodeDomain, CodeMethodNotAllowed,
		CodeUnsupportedMedia, CodeTimeout, CodeBadGateway,
	}
	seen := map[string]bool{}
	for _, c := range codes {
		if seen[c] {
			t.Fatalf("duplicate code: %s", c)
		}
		seen[c] = true
	}
}

type errString string

func (e errString) Error() string { return string(e) }
