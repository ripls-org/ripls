import 'package:flutter_riverpod/flutter_riverpod.dart';

/// Tracks whether a full-screen morph-reveal content panel is currently open
/// for a given content item (keyed by its id) — e.g. an experience's "Who's
/// pitching in?" roster / conversation, or a request's location / helpers
/// panel.
///
/// While true, the owning read surface hides its body so the panel — pushed
/// over the content via `morphRevealRoute` — overlays the **existing** hero
/// background alone, with none of the other content showing through its
/// semi-transparent surface. The hero keeps playing underneath rather than
/// being re-rendered (which would restart the video).
/// See docs/client/modals.md (Morph-reveal content panels).
class ContentExpandedNotifier extends Notifier<bool> {
  ContentExpandedNotifier(this.contentId);

  /// The content id this flag belongs to (from the family modifier).
  final String contentId;

  @override
  bool build() => false;

  /// Sets whether a content panel is expanded.
  void set(bool value) => state = value;
}

/// Per-experience flag for an open morph-reveal content panel. Not autoDispose:
/// the read shell (in the feed) and the pushed panel both depend on it across
/// the open/close interaction.
final experienceContentExpandedProvider =
    NotifierProvider.family<ContentExpandedNotifier, bool, String>(
      ContentExpandedNotifier.new,
    );

/// Per-request flag for an open morph-reveal content panel (location, helpers).
/// Mirrors [experienceContentExpandedProvider]; kept separate so an open panel
/// on one surface never hides the other.
final requestContentExpandedProvider =
    NotifierProvider.family<ContentExpandedNotifier, bool, String>(
      ContentExpandedNotifier.new,
    );

/// Per-gear flag for an open morph-reveal content panel (the conversation grown
/// from the discussion card). Mirrors [requestContentExpandedProvider]; kept
/// separate so an open panel on one surface never hides the others.
final gearContentExpandedProvider =
    NotifierProvider.family<ContentExpandedNotifier, bool, String>(
      ContentExpandedNotifier.new,
    );

/// Flag for an open morph-reveal panel on the Workshop overview (#2447) — e.g.
/// the community conversation grown from the chat glimpse. While true, the
/// overview hides its scrolling body so the panel overlays the community-photo
/// backdrop alone. Single instance (one Workshop surface), so not a family.
final workshopContentExpandedProvider =
    NotifierProvider<ContentExpandedNotifier, bool>(
      () => ContentExpandedNotifier('workshop'),
    );

/// Per-target-user flag for an open morph-reveal panel on the person profile
/// (#2568/#2634) — the standing conversation grown from the profile's message
/// bubble. Mirrors [gearContentExpandedProvider]; keyed by the target user id
/// so a panel open on one person's profile never hides another's.
final userProfileContentExpandedProvider =
    NotifierProvider.family<ContentExpandedNotifier, bool, String>(
      ContentExpandedNotifier.new,
    );

/// Per-community flag for an open morph-reveal panel on the community
/// profile (#2568/#2634) — the crew's standing conversation grown from the
/// profile's message bubble. Mirrors [userProfileContentExpandedProvider];
/// kept separate so an open panel on one surface never hides the others.
final communityProfileContentExpandedProvider =
    NotifierProvider.family<ContentExpandedNotifier, bool, String>(
      ContentExpandedNotifier.new,
    );
