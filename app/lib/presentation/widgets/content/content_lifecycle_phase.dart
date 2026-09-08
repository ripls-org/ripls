/// ContentLifecyclePhase is the presentation-neutral lifecycle a content view
/// moves through. It drives the hero treatment, chip copy, and CTA across the
/// redesigned sheet-over-hero surface (see docs/issues/2278-experience-content-redesign.md).
///
/// It is deliberately decoupled from any proto enum: the experience (and, later,
/// request) view-models derive it from their own state + the current time, and
/// the shared content widgets map it to copy via `context.l10n`. Keeping the enum
/// free of Flutter and proto imports lets both the view-model and the widget
/// layer depend on it without a layering violation.
enum ContentLifecyclePhase {
  /// Open and gathering responses (experience: ACTIVE).
  sharing,

  /// Enough participation to be happening (experience: JOINED).
  confirmed,

  /// Underway but not yet started — the imminent window (experience:
  /// IN_PROCESS before the recorded start time).
  leaving,

  /// Started and ongoing (experience: IN_PROCESS at/after the start time).
  onTheRoad,

  /// Finished (experience: COMPLETED).
  wrapped,

  /// Called off (experience: CANCELLED).
  cancelled,
}
