// Package workshop implements the WorkshopService RPC interface for the
// Workshop tab — the proactive host's curation layer.
//
// The primary surfaces are GetWorkshopBrief (AI-generated daily brief with
// inline tappable levers), GetWorkshopSynthesis (season-totals narrative),
// and GenerateWorkshopDraft (prefill helper for the experience creation modal).
// Cascade priority detection and AI content generation live in the
// momentum subpackage; this service assembles and validates their output.
package workshop
