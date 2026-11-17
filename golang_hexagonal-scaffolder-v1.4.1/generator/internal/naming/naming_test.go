package naming

import (
	"strings"
	"testing"

	"github.com/example/hexapi/generator/internal/applogger"
)

func newNaming() *Naming { return New(applogger.NewWithEnv(func(string) string { return "" })) }

// TestNaming_Pascal_convertsSnake covers snake_case → PascalCase. Since 1.4.0.
func TestNaming_Pascal_convertsSnake(t *testing.T) {
	n := newNaming()
	got, err := n.Pascal("user_account_id")
	if err != nil {
		t.Fatalf("Pascal: %v", err)
	}
	if got != "UserAccountID" {
		t.Fatalf("Pascal: want UserAccountID, got %q", got)
	}
}

// TestNaming_Pascal_handlesKebab covers kebab-case input. Since 1.4.0.
func TestNaming_Pascal_handlesKebab(t *testing.T) {
	n := newNaming()
	got, _ := n.Pascal("order-line-item")
	if got != "OrderLineItem" {
		t.Fatalf("Pascal kebab: want OrderLineItem, got %q", got)
	}
}

// TestNaming_Camel lowercasesFirstWord. Since 1.4.0.
func TestNaming_Camel_lowercasesFirstWord(t *testing.T) {
	n := newNaming()
	got, _ := n.Camel("user_account_id")
	if got != "userAccountID" {
		t.Fatalf("Camel: want userAccountID, got %q", got)
	}
}

// TestNaming_Snake_lowercasesAndJoins. Since 1.4.0.
func TestNaming_Snake_lowercasesAndJoins(t *testing.T) {
	n := newNaming()
	got, _ := n.Snake("UserAccountID")
	if got != "user_account_id" {
		t.Fatalf("Snake: want user_account_id, got %q", got)
	}
}

// TestNaming_RouteName_pluralisesAndKebabs. Since 1.4.0.
func TestNaming_RouteName_pluralisesAndKebabs(t *testing.T) {
	n := newNaming()
	got, _ := n.RouteName("user_account")
	if got != "user-accounts" {
		t.Fatalf("RouteName: want user-accounts, got %q", got)
	}
}

// TestNaming_RouteName_keepsAlreadyPlural. Since 1.4.0.
func TestNaming_RouteName_keepsAlreadyPlural(t *testing.T) {
	n := newNaming()
	got, _ := n.RouteName("users")
	if got != "users" {
		t.Fatalf("RouteName plural: want users, got %q", got)
	}
}

// TestNaming_EntityName_singularisesAndPascals. Since 1.4.0.
func TestNaming_EntityName_singularisesAndPascals(t *testing.T) {
	n := newNaming()
	got, _ := n.EntityName("user_accounts")
	if got != "UserAccount" {
		t.Fatalf("EntityName: want UserAccount, got %q", got)
	}
}

// TestNaming_EntityName_handlesIes. Since 1.4.0.
func TestNaming_EntityName_handlesIes(t *testing.T) {
	n := newNaming()
	got, _ := n.EntityName("categories")
	if got != "Category" {
		t.Fatalf("EntityName ies: want Category, got %q", got)
	}
}

// TestNaming_Sanitise_rejectsEmpty covers the GEN-005 path. Since 1.4.0.
func TestNaming_Sanitise_rejectsEmpty(t *testing.T) {
	n := newNaming()
	_, err := n.Sanitise("!!!")
	if err == nil || !strings.Contains(err.Error(), "GEN-005") {
		t.Fatalf("Sanitise empty: want GEN-005, got %v", err)
	}
}

// TestNaming_Sanitise_prefixesLeadingDigit. Since 1.4.0.
func TestNaming_Sanitise_prefixesLeadingDigit(t *testing.T) {
	n := newNaming()
	got, _ := n.Sanitise("1st_thing")
	if got != "_1st_thing" {
		t.Fatalf("Sanitise digit: want _1st_thing, got %q", got)
	}
}
