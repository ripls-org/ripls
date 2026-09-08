// Package rsvpstate holds shared RSVP-intention semantics for
// models.ExperienceRSVP — currently the single rule for which intentions
// count as "going". It gives cross-cutting consumers (experience service,
// notifications, portfolio, impact metrics) one definition instead of each
// re-spelling the YES-or-MAYBE check.
package rsvpstate
