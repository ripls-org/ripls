/// EdgeStatus is the participation state shown as a pill on a content edge row
/// (see docs/issues/2278-experience-content-redesign.md). It is presentation-
/// neutral: the experience (and later request) view-model derives it, and
/// [ContentEdgeRow] maps it to a colored pill + label.
enum EdgeStatus {
  /// The owner/host of the content.
  host,

  /// Confirmed participation (RSVP yes).
  going,

  /// Tentative participation (RSVP maybe).
  maybe,

  /// A past/terminal participation, after the event wrapped.
  wrapped,

  /// Invited but not yet responded.
  invited,
}

/// EdgeViewData is one participant row in a [ContentEdgesCard] — the
/// contribution graph ("who's bringing what"). It is a plain data holder with
/// no Flutter or proto dependency so both the view-model (which assembles it)
/// and the widget layer (which renders it) can depend on it.
class EdgeViewData {
  /// Stable user id (used for keys and dedup).
  final String userId;

  /// Display name.
  final String name;

  /// Up-to-two-letter initials for the fallback avatar.
  final String initials;

  /// Optional avatar media id; null renders the initials avatar.
  final String? mediaId;

  /// What this person is bringing/doing (e.g. "sunscreen, driving 2 seats").
  /// Empty when there is no recorded contribution.
  final String contribution;

  /// Participation state shown as the trailing pill.
  final EdgeStatus status;

  /// Whether this row is the current viewer.
  final bool isYou;

  const EdgeViewData({
    required this.userId,
    required this.name,
    required this.initials,
    required this.status,
    this.mediaId,
    this.contribution = '',
    this.isYou = false,
  });
}
