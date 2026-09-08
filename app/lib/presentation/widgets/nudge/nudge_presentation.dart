/// NudgePresentation names the two frames a nudge card can be rendered in.
///
/// [NudgeCardVariants] compositions are `Stack(fit: StackFit.expand)` — they
/// cannot self-size, so every host bounds them. The feed gives a card the whole
/// screen; the calendar's open-day card (200–280 px) and the Home zero state
/// hero (320 px) give it a small box. The editorial spacing and type drawn for
/// the first leave the copy no room in the second, and the overlay chrome's
/// status-bar offset is meaningless there — the host already sits inside the
/// screen's safe area (#2801).
enum NudgePresentation {
  /// The card owns the screen, including its edges.
  fullScreen,

  /// The card lives inside a host-bounded box that owns the screen edges.
  embedded,
}
