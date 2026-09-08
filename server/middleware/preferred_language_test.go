package middleware

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"go.ripls.org/ripls/server/auth"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/l10n"
	"go.ripls.org/ripls/server/storage"

	"connectrpc.com/authn"
)

// withAuth sets *auth.Info into ctx the way connectrpc/authn does it
// at the production wire boundary, so the interceptor's
// auth.GetAuthInfo lookup works under test.
func withAuth(ctx context.Context, info *auth.Info) context.Context {
	return authn.SetInfo(ctx, info)
}

// callInterceptor runs the interceptor with a no-op next and returns
// the ctx the next would have observed.
func callInterceptor(t *testing.T, store *storage.ProtoSQLStorage, ctx context.Context) context.Context {
	t.Helper()
	interceptor := PreferredLanguageInterceptor(store)
	var observed context.Context
	next := connect.UnaryFunc(func(c context.Context, _ connect.AnyRequest) (connect.AnyResponse, error) {
		observed = c
		return nil, nil
	})
	// connect.AnyRequest is internalOnly; use a real Request[T] for any T.
	req := connect.NewRequest(&api.GetUserRequest{})
	if _, err := interceptor(next)(ctx, req); err != nil {
		t.Fatalf("interceptor returned error: %v", err)
	}
	if observed == nil {
		t.Fatal("interceptor did not invoke next")
	}
	return observed
}

func TestPreferredLanguageInterceptor_InjectsWhenUserHasPref(t *testing.T) {
	store, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	es := "es"
	user := &models.User{
		Id:                uuid.New().String(),
		Email:             "es-user@example.com",
		Role:              models.Role_ROLE_USER,
		AuthMethod:        models.AuthMethod_AUTH_METHOD_EMAIL_PASSWORD,
		PasswordHash:      "hashed",
		PreferredLanguage: &es,
	}
	if _, err := store.Insert(context.Background(), user); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	ctx := withAuth(context.Background(), &auth.Info{UserID: user.Id, Email: user.Email})
	observed := callInterceptor(t, store, ctx)

	if got := l10n.LocaleFromContext(observed); got.String() != "es" {
		t.Errorf("LocaleFromContext = %s; want es (interceptor must inject preferred_language)", got)
	}
}

func TestPreferredLanguageInterceptor_SkipsWhenAcceptLanguageSet(t *testing.T) {
	store, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	// User has stored es; Accept-Language overrides to en.
	es := "es"
	user := &models.User{
		Id:                uuid.New().String(),
		Email:             "user@example.com",
		Role:              models.Role_ROLE_USER,
		AuthMethod:        models.AuthMethod_AUTH_METHOD_EMAIL_PASSWORD,
		PasswordHash:      "hashed",
		PreferredLanguage: &es,
	}
	if _, err := store.Insert(context.Background(), user); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	ctx := l10n.WithAcceptLanguage(context.Background(), "en")
	ctx = withAuth(ctx, &auth.Info{UserID: user.Id, Email: user.Email})
	observed := callInterceptor(t, store, ctx)

	// Tier 2 (Accept-Language) wins; the interceptor must not
	// overwrite with tier 3.
	if got := l10n.LocaleFromContext(observed); got.String() != "en" {
		t.Errorf("LocaleFromContext = %s; want en (Accept-Language must win)", got)
	}
}

func TestPreferredLanguageInterceptor_NoOpForUnauthenticatedRequest(t *testing.T) {
	store, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	// No auth info in ctx — the interceptor should pass through
	// without touching storage and without altering the locale chain.
	observed := callInterceptor(t, store, context.Background())
	if got := l10n.LocaleFromContext(observed); got != l10n.DefaultTag {
		t.Errorf("LocaleFromContext = %s; want default (unauthenticated request)", got)
	}
}

func TestPreferredLanguageInterceptor_StorageFailureFallsBackToDefault(t *testing.T) {
	store, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	// User id refers to a row that doesn't exist — the storage
	// lookup will fail. The interceptor logs and continues.
	ctx := withAuth(context.Background(), &auth.Info{UserID: "missing-user-id"})
	observed := callInterceptor(t, store, ctx)
	if got := l10n.LocaleFromContext(observed); got != l10n.DefaultTag {
		t.Errorf("LocaleFromContext = %s; want default (missing user)", got)
	}
}
