package store

import (
	"errors"
	"testing"
	"time"
)

func TestUpsertOIDCUser(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	first, err := s.UpsertOIDCUser(ctx, "https://idp.example", "sub-1", "old@example.com", "Old")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.UpsertOIDCUser(ctx, "https://idp.example", "sub-1", "new@example.com", "New")
	if err != nil || second.ID != first.ID || second.Email != "new@example.com" || second.Name != "New" {
		t.Fatalf("%+v %v", second, err)
	}
	other, err := s.UpsertOIDCUser(ctx, "https://other.example", "sub-1", "new@example.com", "")
	if err != nil || other.ID == first.ID {
		t.Fatalf("same subject at another issuer must be a new user: %+v %v", other, err)
	}
	users, err := s.ListUsers(ctx)
	if err != nil || len(users) != 2 {
		t.Fatalf("%+v %v", users, err)
	}

	actor := Actor{UserID: other.ID}
	if err := s.SetUserDisabled(ctx, actor, first.ID, true); err != nil {
		t.Fatal(err)
	}
	loginBefore, err := s.GetUser(ctx, first.ID)
	if err != nil || loginBefore.DisabledAt == nil {
		t.Fatalf("%+v %v", loginBefore, err)
	}
	time.Sleep(2 * time.Millisecond)
	disabled, err := s.UpsertOIDCUser(ctx, "https://idp.example", "sub-1", "new@example.com", "New")
	if !errors.Is(err, ErrUserDisabled) || disabled.ID != first.ID {
		t.Fatalf("%+v %v", disabled, err)
	}
	loginAfter, err := s.GetUser(ctx, first.ID)
	if err != nil || !loginAfter.LastLoginAt.Equal(*loginBefore.LastLoginAt) {
		t.Fatalf("last_login_at bumped for disabled user: %+v %v", loginAfter, err)
	}
	if err := s.SetUserDisabled(ctx, actor, first.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertOIDCUser(ctx, "https://idp.example", "sub-1", "new@example.com", "New"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetUserDisabled(ctx, actor, 9999, true); !errors.Is(err, ErrUserNotExist) {
		t.Fatal(err)
	}
	if _, err := s.GetUser(ctx, 9999); !errors.Is(err, ErrUserNotExist) {
		t.Fatal(err)
	}
	entries, _, err := s.ListAudit(ctx, "", 1, 10)
	if err != nil || len(entries) != 2 || entries[0].Action != ActionUserEnable || entries[1].Action != ActionUserDisable ||
		string(entries[1].After) != `{"email":"new@example.com"}` {
		t.Fatalf("%+v %v", entries, err)
	}
}

func TestEnsureDevUserIsIdempotent(t *testing.T) {
	s := newTestStore(t)
	a, err := s.EnsureDevUser(t.Context(), "dev@example.com")
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.EnsureDevUser(t.Context(), "dev@example.com")
	if err != nil || a.ID != b.ID {
		t.Fatalf("%+v %+v %v", a, b, err)
	}
}
