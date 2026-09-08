// Package noop implements the notifications.Provider interface as a no-op that
// logs notifications without actually delivering them. It is used in local
// development and end-to-end tests to avoid requiring platform push credentials.
package noop
