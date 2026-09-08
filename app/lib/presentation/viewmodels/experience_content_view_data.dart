import 'package:ripls/data/gen/ripls/api/experience.pb.dart' as pb;
import 'package:ripls/data/gen/ripls/api/experience_service.pb.dart'
    show RSVPIntention;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/viewmodels/experience_view_model.dart';
import 'package:ripls/presentation/widgets/content/content_edge_data.dart';
import 'package:ripls/presentation/widgets/content/content_lifecycle_phase.dart';

/// Distance beyond which the in-person "you-gap" prompt is muted on the
/// redesigned content view: attending in person stops being a suggestion and
/// the surface should offer remote-friendly alternatives instead. ~100 miles.
const double kFarAwayThresholdMeters = 160000;

/// Presentation derivations for the redesigned content view
/// (docs/issues/2278-experience-content-redesign.md), layered onto
/// [ExperienceState] as an extension so the core view-model file stays focused.
/// Re-exported from `experience_view_model.dart`, so importers of the
/// view-model get these for free.
extension ExperienceContentViewData on ExperienceState {
  /// Derives the presentation [ContentLifecyclePhase] from the experience state
  /// and [nowUnixSec]. IN_PROCESS splits at the recorded start time: before it
  /// the event is "leaving" (imminent), at/after it the event is "on the road"
  /// (underway). Pure for a fixed [nowUnixSec] so it can be unit-tested without
  /// a clock.
  ContentLifecyclePhase lifecyclePhaseAt(int nowUnixSec) {
    final exp = experienceDetails?.experience;
    if (exp == null) return ContentLifecyclePhase.sharing;
    switch (exp.state) {
      case pb.ExperienceState.EXPERIENCE_STATE_CANCELLED:
        return ContentLifecyclePhase.cancelled;
      case pb.ExperienceState.EXPERIENCE_STATE_COMPLETED:
        return ContentLifecyclePhase.wrapped;
      case pb.ExperienceState.EXPERIENCE_STATE_IN_PROCESS:
        if (exp.hasStartedAtUnixSec() &&
            nowUnixSec >= exp.startedAtUnixSec.toInt()) {
          return ContentLifecyclePhase.onTheRoad;
        }
        return ContentLifecyclePhase.leaving;
      case pb.ExperienceState.EXPERIENCE_STATE_JOINED:
        return ContentLifecyclePhase.confirmed;
      default:
        return ContentLifecyclePhase.sharing;
    }
  }

  /// [lifecyclePhaseAt] evaluated against the system clock.
  ContentLifecyclePhase get lifecyclePhase =>
      lifecyclePhaseAt(DateTime.now().millisecondsSinceEpoch ~/ 1000);

  /// True when the viewer is far enough away that in-person participation is no
  /// longer the suggested action. Null distance (unknown) is treated as near.
  bool get isFarAway =>
      locationDistanceMeters != null &&
      locationDistanceMeters! > kFarAwayThresholdMeters;

  /// Assembles the contribution graph ("who's bringing what"). The host is
  /// listed first, followed by everyone who RSVP'd yes or maybe (declines are
  /// omitted); each row's status pill is derived from the RSVP intention,
  /// except once the event has wrapped, when every row reads as
  /// [EdgeStatus.wrapped].
  ///
  /// Contribution text is injected via [contributionsByUserId] (keyed by user
  /// id) because contributions live in a separate provider; callers pass the
  /// resolved map, and it defaults to empty so the method stays pure and
  /// unit-testable.
  List<EdgeViewData> edges({
    Map<String, String> contributionsByUserId = const {},
  }) {
    final details = experienceDetails;
    if (details == null) return const [];
    final isWrapped = lifecyclePhase == ContentLifecyclePhase.wrapped;
    final result = <EdgeViewData>[];
    final seen = <String>{};

    final owner = details.experience.owner;
    if (owner.id.isNotEmpty) {
      seen.add(owner.id);
      result.add(
        EdgeViewData(
          userId: owner.id,
          name: owner.name,
          initials: _edgeInitials(owner.name),
          mediaId: owner.hasMediaId() ? owner.mediaId : null,
          contribution: contributionsByUserId[owner.id] ?? '',
          status: isWrapped ? EdgeStatus.wrapped : EdgeStatus.host,
          isYou: owner.id == currentUserId,
        ),
      );
    }

    for (final rsvp in details.rsvps) {
      final user = rsvp.user;
      if (user.id.isEmpty || !seen.add(user.id)) continue;
      if (rsvp.intention == RSVPIntention.RSVP_INTENTION_NO) continue;
      final EdgeStatus status;
      if (isWrapped) {
        status = EdgeStatus.wrapped;
      } else if (rsvp.intention == RSVPIntention.RSVP_INTENTION_MAYBE) {
        status = EdgeStatus.maybe;
      } else {
        status = EdgeStatus.going;
      }
      result.add(
        EdgeViewData(
          userId: user.id,
          name: user.name,
          initials: _edgeInitials(user.name),
          mediaId: user.hasMediaId() ? user.mediaId : null,
          contribution: contributionsByUserId[user.id] ?? '',
          status: status,
          isYou: user.id == currentUserId,
        ),
      );
    }

    return result;
  }

  /// Aggregates the "stocked" progress across an event's open needs from each
  /// need's slot counts. The needs live in a separate provider, so callers pass
  /// the list and this stays pure / unit-testable.
  StockedSummary stockedSummaryFrom(List<pb.ExperienceNeedResponse> needs) {
    var total = 0;
    var remaining = 0;
    for (final need in needs) {
      total += need.slots;
      remaining += need.slotsRemaining;
    }
    return StockedSummary(total: total, remaining: remaining);
  }

  /// Splits everyone who has responded into participation groups for the
  /// expanded pitching-in panel. The host is bucketed by **their own RSVP**
  /// (defaulting to going when they haven't responded) and pinned first within
  /// that group — so changing the host's RSVP moves their row to the matching
  /// section, while still flagging them as the host. Within a group, most-recent
  /// responders come first. [RosterGroups.noReply] surfaces server-supplied
  /// invitees who haven't responded. Pure for a fixed roster, like [edges].
  RosterGroups rosterGroups() {
    final details = experienceDetails;
    if (details == null) return RosterGroups.empty;

    final going = <RosterEntry>[];
    final maybe = <RosterEntry>[];
    final notGoing = <RosterEntry>[];
    final noReply = <RosterEntry>[];
    final seen = <String>{};

    void place(RosterEntry entry, RosterStatus bucket) {
      switch (bucket) {
        case RosterStatus.going:
          going.add(entry);
        case RosterStatus.maybe:
          maybe.add(entry);
        case RosterStatus.notGoing:
          notGoing.add(entry);
        case RosterStatus.noReply:
          noReply.add(entry);
      }
    }

    final owner = details.experience.owner;
    if (owner.id.isNotEmpty) {
      seen.add(owner.id);
      // The host's section follows their own RSVP (going when they haven't
      // replied), but their row keeps the host flag.
      RSVPIntention? ownerIntention;
      for (final rsvp in details.rsvps) {
        if (rsvp.user.id == owner.id) {
          ownerIntention = rsvp.intention;
          break;
        }
      }
      final ownerBucket = ownerIntention == null
          ? RosterStatus.going
          : _rosterStatusForIntention(ownerIntention);
      place(
        RosterEntry(
          user: owner,
          status: ownerBucket,
          isHost: true,
          isYou: owner.id == currentUserId,
          timeUnixSec: details.experience.createdAtUnixSec.toInt(),
        ),
        ownerBucket,
      );
    }

    for (final rsvp in details.rsvps) {
      final user = rsvp.user;
      if (user.id.isEmpty || !seen.add(user.id)) continue;
      final status = _rosterStatusForIntention(rsvp.intention);
      place(
        RosterEntry(
          user: user,
          status: status,
          isYou: user.id == currentUserId,
          timeUnixSec: rsvp.rsvpedAtUnixSec.toInt(),
        ),
        status,
      );
    }

    // Directly-invited individuals (the event's own ad-hoc community members
    // the host invited one by one) who haven't responded — server-supplied,
    // already excluding the host and anyone with an RSVP. Named-community
    // members stay collapsed to a no-reply count instead of being exploded
    // here. Skip ids already listed above so a stale overlap never
    // double-counts (#2492).
    for (final user in details.invitedIndividuals) {
      if (user.id.isEmpty || !seen.add(user.id)) continue;
      noReply.add(
        RosterEntry(
          user: user,
          status: RosterStatus.noReply,
          isYou: user.id == currentUserId,
          timeUnixSec: 0,
        ),
      );
    }

    int byRecency(RosterEntry a, RosterEntry b) =>
        b.timeUnixSec.compareTo(a.timeUnixSec);
    // Keep the host pinned to the top of whichever section they're in.
    List<RosterEntry> hostFirst(List<RosterEntry> entries) {
      final hosts = entries.where((e) => e.isHost).toList();
      final rest = entries.where((e) => !e.isHost).toList()..sort(byRecency);
      return [...hosts, ...rest];
    }

    return RosterGroups(
      going: hostFirst(going),
      maybe: hostFirst(maybe),
      notGoing: hostFirst(notGoing),
      noReply: noReply,
    );
  }
}

RosterStatus _rosterStatusForIntention(RSVPIntention intention) {
  switch (intention) {
    case RSVPIntention.RSVP_INTENTION_YES:
      return RosterStatus.going;
    case RSVPIntention.RSVP_INTENTION_NO:
      return RosterStatus.notGoing;
    default:
      // Maybe and unspecified both read as tentative.
      return RosterStatus.maybe;
  }
}

/// Aggregate "stocked" progress across an event's open needs (the contribution
/// graph's coverage), derived from slot counts. Plain data holder with no
/// Flutter dependency so both the view-model and the widget layer can use it.
class StockedSummary {
  /// Total slots requested across all open needs (Σ slots).
  final int total;

  /// Slots still unclaimed across all open needs (Σ slotsRemaining).
  final int remaining;

  const StockedSummary({required this.total, required this.remaining});

  /// Slots already covered by a contributor.
  int get covered => (total - remaining).clamp(0, total);

  /// Whole-number percent covered; 0 when there is nothing to stock.
  int get percent => total == 0 ? 0 : ((covered / total) * 100).round();

  /// Whether there is anything to stock (drives whether the bar/footer shows).
  bool get hasNeeds => total > 0;

  /// Whether everything that was needed is now covered.
  bool get isFullyStocked => total > 0 && remaining == 0;
}

/// Participation bucket for a person on the expanded pitching-in roster.
enum RosterStatus { going, maybe, notGoing, noReply }

/// One person on the expanded pitching-in roster, grouped by [status]. Holds the
/// [User] so the widget can render an avatar; lives in the view-model layer.
class RosterEntry {
  /// The participant.
  final User user;

  /// Their participation bucket — for the host this is their own RSVP (so the
  /// status pill follows it), with [isHost] flagging the host separately.
  final RosterStatus status;

  /// Whether this entry is the event host. The host is pinned first within
  /// their group and gets the "host" badge, independent of [status].
  final bool isHost;

  /// Whether this entry is the current viewer.
  final bool isYou;

  /// When they responded (RSVP time, or the host's create time), used to order
  /// within a group, most recent first.
  final int timeUnixSec;

  const RosterEntry({
    required this.user,
    required this.status,
    required this.isYou,
    required this.timeUnixSec,
    this.isHost = false,
  });
}

/// The pitching-in roster split into status groups for the expanded panel.
class RosterGroups {
  /// Host + RSVP-yes, host pinned first.
  final List<RosterEntry> going;

  /// Tentative responders.
  final List<RosterEntry> maybe;

  /// Declines.
  final List<RosterEntry> notGoing;

  /// Invited but not yet responded — empty until a backend invitee source
  /// exists; the "still no reply" line is data-gated on this being non-empty.
  final List<RosterEntry> noReply;

  const RosterGroups({
    required this.going,
    required this.maybe,
    required this.notGoing,
    required this.noReply,
  });

  /// An all-empty roster (no details available).
  static const RosterGroups empty = RosterGroups(
    going: [],
    maybe: [],
    notGoing: [],
    noReply: [],
  );

  /// Whether nobody has responded at all.
  bool get isEmpty =>
      going.isEmpty && maybe.isEmpty && notGoing.isEmpty && noReply.isEmpty;

  /// Headcount for the "{N} in" half of the who's-in header: the host plus
  /// everyone who RSVP'd yes. Disjoint from [invitedNoReplyCount] — maybes and
  /// declines are counted in neither (#2724).
  int get goingCount => going.length;

  /// Headcount for the "{M} invited" half of the who's-in header: invitees who
  /// have not replied yet. The server-built [noReply] list already excludes
  /// the host and anyone with an RSVP, so this never overlaps [goingCount] and
  /// the host is never counted as invited (#2724).
  int get invitedNoReplyCount => noReply.length;
}

/// Returns up-to-two uppercase initials for [name], or an empty string when
/// the name has no usable letters.
String _edgeInitials(String name) => name
    .split(' ')
    .where((part) => part.isNotEmpty)
    .take(2)
    .map((part) => part[0])
    .join()
    .toUpperCase();
