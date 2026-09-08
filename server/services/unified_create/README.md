# unified_create

Server side of the unified-create flow (`StreamGenUnifiedCreate`). See
`docs/design/unified-create.md` for the design.

## Files

- `service.go` — `Service` struct + `New(...)` constructor. Holds the AI
  provider, classifier, and references to the per-type services we
  delegate to.
- `classifier.go` — narrow `Classifier` interface (alias of
  `ai.UnifiedCreateClassifier`), the `providerClassifier` adapter that
  wraps an `ai.Provider`, and the `stubClassifier` fallback for
  no-API-key environments. Production wiring (in `server/main.go`)
  uses `NewProviderClassifier(aiProvider)`, so the classifier rides
  the same primary/fallback chain (Gemini → Anthropic → ...) as every
  other AI surface. Prompt construction lives in
  `server/ai/prompts_unified.go`.
- `stream_gen.go` — the `StreamGenUnifiedCreate` handler. Validates
  input, runs the classifier (or honours `force_type`), emits a `type`
  event, then delegates to the appropriate per-type generator and
  forwards its events through the unified envelope.

## Boundaries

- We do **not** classify lend vs. give. The `gear` final payload is
  whatever the per-type gear generator produced; the client owns the
  Lend/Give toggle (defaulting to Lend) and sends `transfer_intent`
  on the per-type `SaveGear` call.
- We do **not** introduce a new save RPC. After streaming completes,
  the client dispatches to the existing per-type `Save*` RPC.
- The classifier model is the same as the rest of the AI surface —
  per-provider routing is handled by the shared `ai.Provider` chain
  (Gemini primary, Anthropic fallback). See
  `docs/design/unified-create.md` § Decisions #4 for context.

## Phase 1 stub note

The Phase 1 handler runs the classifier and emits the `type` event,
then synthesises a placeholder terminal `final` from the per-type
generator (real per-type delegation is a follow-up — see the TODO in
`stream_gen.go`). This unblocks client implementation against the
streaming envelope while we settle on the cleanest delegation layer.
