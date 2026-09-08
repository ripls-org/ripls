# Auth

The `auth` package provides authentication and identity primitives shared across the server: JWT issuance and verification, OIDC provider management, password hashing, phone-number token verification, and helpers for resolving user identity from an incoming request.

## Key files

- `tokens.go` — JWT generation (`GenerateToken`) and refresh-token utilities (`GenerateRefreshToken`, `HashRefreshToken`). Also exposes `ValidateSigningSecret` for fail-fast startup validation of the HS256 signing key (must be ≥ `MinSigningSecretBytes`). Raw tokens are never stored; always use the hashed form.
- `users.go` — `UserManager`: the primary entry point for user lookups, creation, and role checks.
- `community.go` — helpers for asserting community membership and ownership from an auth context.
- `oidc.go` — `OIDCProviderManager`: verifies ID tokens from third-party identity providers (Google, Apple).
- `password.go` — bcrypt-based password hashing and comparison.
- `firebase.go` — Firebase-specific auth helpers (phone token verification via Firebase Admin SDK).
- `helpers.go` — `RequireAuth` and related context helpers for extracting auth info inside RPC handlers.
- `test_utils.go` — test-only helpers for generating signed tokens without a running server.

## When to add code here vs. elsewhere

This package holds only generic auth primitives — token formats, identity proofs, and access checks that any service may need. Service-specific authorization logic (e.g. "is this user allowed to delete this gear?") belongs in the relevant service package, not here.
