package auth

import (
	"context"

	"github.com/mr-karan/lil/internal/store"
)

type ctxKey struct{}

type identity struct {
	actor store.Actor
	user  store.User
}

func WithIdentity(ctx context.Context, actor store.Actor, user store.User) context.Context {
	return context.WithValue(ctx, ctxKey{}, identity{actor: actor, user: user})
}

func ActorFrom(ctx context.Context) (store.Actor, bool) {
	id, ok := ctx.Value(ctxKey{}).(identity)
	return id.actor, ok
}

func UserFrom(ctx context.Context) (store.User, bool) {
	id, ok := ctx.Value(ctxKey{}).(identity)
	return id.user, ok
}
