// Command seed-legacy-user writes a pre-#2571 account shape directly into the
// database: an account holding a password hash and carrying no verified-email
// stamp.
//
// Registration cannot produce this shape any more. #2864 closed the password
// branch on EmailRegister in every environment, dev included, so that what e2e
// exercises is the same code production runs. The accounts that predate the
// code flow still exist and still sign in, though, and the login screen's
// password fallback has to keep being tested until it is deleted — hence a
// fixture built out of band rather than a carve-out in the server.
//
// It writes through the normal storage layer, so the row it produces is
// byte-identical to one the server would have written (protosql keeps the
// authoritative copy in binary_proto and mirrors the flat columns; a
// hand-written INSERT would have to reproduce both correctly).
//
// TEST FIXTURE ONLY. It is never wired into a server binary and never runs in
// production. It leaves with the password path.
//
// Usage:
//
//	go run ./server/cmd/seed-legacy-user \
//	  -db="postgres://..." \
//	  -email=legacy@example.com \
//	  -name="Marta Olsen" \
//	  -password=legacy-password-1234
//
// Prints the created user's ID on stdout.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

func main() {
	dbURL := flag.String("db",
		"postgres://ripls:ripls_dev@localhost:5432/ripls?sslmode=disable",
		"PostgreSQL connection string")
	email := flag.String("email", "", "Address for the seeded account (required)")
	name := flag.String("name", "Legacy User", "Display name for the seeded account")
	password := flag.String("password", "", "Plaintext password to hash and store (required)")
	flag.Parse()

	if *email == "" || *password == "" {
		fmt.Fprintln(os.Stderr, "both -email and -password are required")
		os.Exit(2)
	}

	ctx := context.Background()

	store, err := storage.InitializePostgreSQLDatabase(ctx, *dbURL, storage.DefaultStorageTypes())
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open database: %v\n", err)
		os.Exit(1)
	}

	hash, err := auth.HashPassword(*password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to hash password: %v\n", err)
		os.Exit(1)
	}

	now := time.Now().Unix()
	user := &models.User{
		Id:         uuid.New().String(),
		Email:      auth.NormalizeEmail(*email),
		Name:       *name,
		CreatedAt:  now,
		UpdatedAt:  now,
		Role:       models.Role_ROLE_USER,
		AuthMethod: models.AuthMethod_AUTH_METHOD_EMAIL_PASSWORD,
		// The two fields that make this the legacy shape: a password to sign in
		// with, and no verified-email stamp. EmailLogin routes an account like
		// this down the password branch.
		PasswordHash: hash,
	}

	if _, err := store.Insert(ctx, user); err != nil {
		fmt.Fprintf(os.Stderr, "failed to insert user: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(user.Id)
}
