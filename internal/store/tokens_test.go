package store

import (
	"errors"
	"strings"
	"testing"
)

func TestAPITokenLifecycle(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	alice := newActor(t, s)
	bob := Actor{UserID: newUser(t, s, "bob@example.com").ID}

	plaintext, token, err := s.CreateAPIToken(ctx, alice, "ci")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(plaintext, "lil_") || token.TokenPrefix != plaintext[:12] || token.Name != "ci" {
		t.Fatalf("%q %+v", plaintext, token)
	}
	actor, user, err := s.AuthenticateAPIToken(ctx, plaintext)
	if err != nil || actor.UserID != alice.UserID || actor.TokenID == nil || *actor.TokenID != token.ID || user.Email != "alice@example.com" {
		t.Fatalf("%+v %+v %v", actor, user, err)
	}
	if _, _, err := s.AuthenticateAPIToken(ctx, plaintext+"x"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal(err)
	}

	if err := s.RevokeAPIToken(ctx, bob, token.ID); !errors.Is(err, ErrTokenNotExist) {
		t.Fatalf("other user revoked token: %v", err)
	}
	if _, _, err := s.AuthenticateAPIToken(ctx, plaintext); err != nil {
		t.Fatal(err)
	}
	tokens, err := s.ListAPITokens(ctx, alice.UserID)
	if err != nil || len(tokens) != 1 {
		t.Fatalf("%+v %v", tokens, err)
	}

	if err := s.RevokeAPIToken(ctx, alice, token.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.AuthenticateAPIToken(ctx, plaintext); !errors.Is(err, ErrUnauthorized) {
		t.Fatal(err)
	}
	if err := s.RevokeAPIToken(ctx, alice, token.ID); !errors.Is(err, ErrTokenNotExist) {
		t.Fatalf("double revoke: %v", err)
	}
	if tokens, err := s.ListAPITokens(ctx, alice.UserID); err != nil || len(tokens) != 0 {
		t.Fatalf("%+v %v", tokens, err)
	}

	entries, _, err := s.ListAudit(ctx, "", 1, 10)
	if err != nil || len(entries) != 2 || entries[0].Action != ActionTokenRevoke || entries[1].Action != ActionTokenCreate {
		t.Fatalf("%+v %v", entries, err)
	}
	if strings.Contains(string(entries[1].After), plaintext) {
		t.Fatal("audit snapshot leaks token plaintext")
	}
}

func TestDisabledUserTokenFails(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	alice := newActor(t, s)
	bob := Actor{UserID: newUser(t, s, "bob@example.com").ID}
	plaintext, _, err := s.CreateAPIToken(ctx, bob, "laptop")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetUserDisabled(ctx, alice, bob.UserID, true); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.AuthenticateAPIToken(ctx, plaintext); !errors.Is(err, ErrUnauthorized) {
		t.Fatal(err)
	}
	if err := s.SetUserDisabled(ctx, alice, bob.UserID, false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.AuthenticateAPIToken(ctx, plaintext); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("token restored by enable: %v", err)
	}
}

func TestDisableRevokesTokensAndBumpsEpoch(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	alice := newActor(t, s)
	bob := Actor{UserID: newUser(t, s, "bob@example.com").ID}
	bobToken, _, err := s.CreateAPIToken(ctx, bob, "one")
	if err != nil {
		t.Fatal(err)
	}
	aliceToken, _, err := s.CreateAPIToken(ctx, alice, "keep")
	if err != nil {
		t.Fatal(err)
	}
	before, err := s.GetUser(ctx, bob.UserID)
	if err != nil || before.Issuer != "https://idp.example" {
		t.Fatalf("%+v %v", before, err)
	}
	if err := s.SetUserDisabled(ctx, alice, bob.UserID, true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetUserDisabled(ctx, alice, bob.UserID, false); err != nil {
		t.Fatal(err)
	}
	after, err := s.GetUser(ctx, bob.UserID)
	if err != nil || after.SessionEpoch != before.SessionEpoch+1 {
		t.Fatalf("epoch %d -> %d: %v", before.SessionEpoch, after.SessionEpoch, err)
	}
	if _, _, err := s.AuthenticateAPIToken(ctx, bobToken); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("token survived disable+enable: %v", err)
	}
	if tokens, err := s.ListAPITokens(ctx, bob.UserID); err != nil || len(tokens) != 0 {
		t.Fatalf("%+v %v", tokens, err)
	}
	if _, u, err := s.AuthenticateAPIToken(ctx, aliceToken); err != nil || u.Issuer != "https://idp.example" {
		t.Fatalf("other user's token affected: %+v %v", u, err)
	}
}
