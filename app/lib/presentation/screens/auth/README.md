# Auth Screens

Screens for user authentication and account registration.

## Purpose

These screens handle all pre-authenticated flows: sign-in, phone verification, registration, password reset, and invitation landing. They are shown before the main `HomeScreen` shell.

## Key Files

- **`login_screen.dart`** — entry point for returning users; supports OIDC sign-in and phone-based login. Accepts an optional `redirectTo` path for deep-link handling.
- **`phone_auth_screen.dart`** — phone number entry and SMS verification code input.
- **`register_screen.dart`** — new user profile setup (name, photo) after initial auth.
- **`invitation_landing_screen.dart`** — resolves an invite short code, prompts unauthenticated users to sign in or register, then adds them to the invited community.
- **`forgot_password_screen.dart`** — initiates a password reset email.
- **`reset_password_screen.dart`** — accepts and submits a new password from a reset link.

## When to add here vs. elsewhere

Code here covers flows that run before the user has an authenticated session. Anything that requires a valid session belongs under `screens/` at the appropriate feature directory (e.g., `profile/` for account settings).
