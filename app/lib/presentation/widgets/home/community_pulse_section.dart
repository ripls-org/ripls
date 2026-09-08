import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:intl/intl.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/core/theme/gen/overlay_tokens.gen.dart';
import 'package:ripls/core/utils/date_time_formatter.dart';
import 'package:ripls/core/utils/image_cache_keys.dart';
import 'package:ripls/core/utils/navigation_helpers.dart';
import 'package:ripls/data/gen/ripls/api/feed_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/screens/story/story_detail_screen.dart';
import 'package:ripls/presentation/viewmodels/feed_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/chat/cached_media_image.dart';
import 'package:ripls/presentation/widgets/content/content_view_helpers.dart';
import 'package:ripls/services/providers.dart';

/// CommunityPulseSection is the Home tab's "In your communities" block
/// (#2634 v3, `bottom-nav-liquid-glass-mock-v3.html`): the feed as a
/// single-column stream of full-width snapshot posts. Every post shares
/// one anatomy —
///
/// 1. **Snapshot** — the full-width photo running edge to edge, with a
///    skeleton shimmer until it paints, an optional day pill, and the
///    title, body, and status overlaid on the standard bottom shim in
///    white.
/// 2. **Footer strip** — who/where: the asker's avatar fronts a
///    request (people ask), the owner's avatar fronts gear (things
///    lend), a glyph fronts stories — with the action CTA in the
///    corner (solid = give/ask verbs, ghost = join) and the unread dot
///    on the face.
///
/// Posts without a photo carry the caption on the card surface
/// instead. Two tap targets per post: the snapshot/caption (and the
/// CTA) opens the item's full-screen view, while the footer's
/// who-strip — the avatar and the person's name — opens that person's
/// profile. Stories have nobody fronting them, so their who-strip
/// falls back to the item.
///
/// The section renders every loaded feed item (it is the Home scroll's
/// long tail; the host screen drives [FeedNotifier.loadMore] as the user
/// nears the bottom) and shows a loading tail while the next page is in
/// flight. Only feed items that map to an openable full-screen view
/// become posts; system items (nudges, milestone celebrations,
/// notification clusters) stay feed-only.
class CommunityPulseSection extends ConsumerStatefulWidget {
  const CommunityPulseSection({super.key});

  @override
  ConsumerState<CommunityPulseSection> createState() =>
      _CommunityPulseSectionState();
}

class _CommunityPulseSectionState extends ConsumerState<CommunityPulseSection> {
  Set<String>? _initializedCommunityIds;

  @override
  Widget build(BuildContext context) {
    final feedState = ref.watch(feedProvider);
    final communityIds = ref.watch(communitiesProvider).communityIds;

    _initializeFeedIfNeeded(feedState, communityIds);

    final posts = feedState.items
        .map(_postData)
        .whereType<_PostData>()
        .toList();
    if (posts.isEmpty) return const SizedBox.shrink();

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(0, 22, 0, 10),
          child: Text(
            context.l10n.homePulseTitle.toUpperCase(),
            style: TextStyle(
              fontSize: 11.5,
              fontWeight: FontWeight.w600,
              letterSpacing: 1.3,
              color: AppColors.primary(context),
            ),
          ),
        ),
        for (final post in posts)
          Padding(
            padding: const EdgeInsets.only(bottom: 14),
            child: _PostCard(data: post),
          ),
        if (feedState.isLoadingMore)
          const Padding(
            padding: EdgeInsets.symmetric(vertical: 16),
            child: Center(child: CircularProgressIndicator()),
          ),
      ],
    );
  }

  /// The pulse reuses the feed's data path. When the Home tab renders
  /// before anything initialized the feed (the Feed tab no longer does),
  /// kick the load once per community set.
  void _initializeFeedIfNeeded(FeedState feedState, List<String> communityIds) {
    if (communityIds.isEmpty || feedState.isLoading) return;
    final idSet = communityIds.toSet();
    if (_initializedCommunityIds != null &&
        _initializedCommunityIds!.containsAll(idSet) &&
        idSet.containsAll(_initializedCommunityIds!)) {
      return;
    }
    _initializedCommunityIds = idSet;
    if (feedState.items.isEmpty) {
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (!mounted) return;
        unawaited(ref.read(feedProvider.notifier).initialize(communityIds));
      });
    }
  }

  String? _timeAgo(FeedItem item) =>
      DateTimeFormatter.formatTimeAgo(item.occurredAtUnixSec.toInt());

  _PostData? _postData(FeedItem item) {
    final l10n = context.l10n;
    switch (item.itemType) {
      case FeedItemType.FEED_ITEM_TYPE_GEAR_SHARED:
        final p = item.gearShared;
        if (p.gearName.isEmpty) return null;
        // Things lend: the owner's face fronts the gear post.
        return _PostData(
          headerName: p.actor.name,
          headerSub: _timeAgo(item) ?? '',
          headerUser: p.actor,
          title: p.gearName,
          meta: '',
          statusLine: '',
          mediaId: p.mediaIds.isEmpty ? '' : p.mediaIds.first,
          isUnread: item.isUnread,
          onTap: () => _openItem(p.gearId, 'gear'),
          onPersonTap: _openPerson(p.actor),
        );
      case FeedItemType.FEED_ITEM_TYPE_REQUEST_CREATED:
        final p = item.requestCreated;
        if (p.title.isEmpty) return null;
        // People ask: the asker's face fronts the request, the amber
        // tag says the card wants something from the viewer.
        final ago = _timeAgo(item);
        final askStatus = _askStatusLabel(p);
        return _PostData(
          headerName: l10n.homePostAsks(p.requester.name),
          headerSub: p.locationName,
          headerUser: p.requester,
          title: p.title,
          meta: p.description,
          statusLine: ago == null ? '' : l10n.homePostAskedAgo(ago),
          mediaId: p.mediaIds.isEmpty ? '' : p.mediaIds.first,
          ctaLabel: askStatus ?? l10n.homePulseAskCta,
          ctaIsSolid: true,
          ctaIsStatus: askStatus != null,
          isUnread: item.isUnread,
          onTap: () => _openItem(p.requestId, 'request'),
          onPersonTap: _openPerson(p.requester),
        );
      case FeedItemType.FEED_ITEM_TYPE_EXPERIENCE_CREATED:
        final p = item.experienceCreated;
        if (p.name.isEmpty) return null;
        final eventStatus = _eventStatusLabel(p);
        return _PostData(
          headerName: p.creator.name,
          headerSub: _eventMeta(p),
          headerUser: p.creator,
          title: p.name,
          meta: p.description,
          statusLine: _eventStatusLine(p),
          mediaId: p.mediaIds.isEmpty ? '' : p.mediaIds.first,
          snapPill: _eventDayLabel(p),
          ctaLabel: eventStatus ?? l10n.homePulseEventCta,
          ctaIsSolid: false,
          ctaIsStatus: eventStatus != null,
          isUnread: item.isUnread,
          onTap: () => _openItem(p.experienceId, 'experience'),
          onPersonTap: _openPerson(p.creator),
        );
      case FeedItemType.FEED_ITEM_TYPE_STORY:
        final p = item.story;
        if (p.title.isEmpty || p.mediaIds.isEmpty) return null;
        return _PostData(
          headerName: _participantNames(p),
          headerSub: _timeAgo(item) ?? '',
          headerUser: null,
          headerGlyph: Icons.auto_stories_outlined,
          title: p.title,
          meta: p.description,
          statusLine: '',
          mediaId: p.mediaIds.first,
          isUnread: item.isUnread,
          onTap: () => unawaited(
            NavigationHelpers.pushScreen(
              context: context,
              screen: StoryDetailScreen(story: p),
              routeName: 'pulse_story',
            ),
          ),
        );
      default:
        return null;
    }
  }

  /// "Betty, Alfred +2" — the story's featured people, or the story
  /// tag as a fallback when nobody is named.
  String _participantNames(StoryPayload p) {
    final names = p.participants
        .map((u) => u.name.split(' ').first)
        .where((n) => n.isNotEmpty)
        .toList();
    if (names.isEmpty) return context.l10n.homePulseTagStory;
    final shown = names.take(2).join(', ');
    final extra = names.length - 2;
    return extra > 0 ? '$shown +$extra' : shown;
  }

  /// The viewer's own standing on an event ("Going", "Hosting"), or null
  /// when they have not answered and the card should still invite them to
  /// join. Hosting wins over any RSVP — you do not join your own event.
  ///
  /// An absent `viewerRsvp` means the server could not determine the answer,
  /// which is not the same as "no". Falling back to the join CTA there is the
  /// honest default: it is what the card said before it knew anything.
  String? _eventStatusLabel(ExperienceCreatedPayload p) {
    final l10n = context.l10n;
    if (p.canEdit) return l10n.homePulseEventCtaHosting;
    if (!p.hasViewerRsvp()) return null;
    return switch (p.viewerRsvp) {
      FeedRSVPIntention.FEED_RSVP_INTENTION_YES => l10n.homePulseEventCtaGoing,
      FeedRSVPIntention.FEED_RSVP_INTENTION_MAYBE =>
        l10n.homePulseEventCtaMaybe,
      FeedRSVPIntention.FEED_RSVP_INTENTION_NO =>
        l10n.homePulseEventCtaNotGoing,
      _ => null,
    };
  }

  /// The viewer's own standing on a request ("Offered", "Your request"), or
  /// null when they have not answered and the card should still ask. Same
  /// unknown-means-invite rule as events.
  String? _askStatusLabel(RequestCreatedPayload p) {
    final l10n = context.l10n;
    if (p.canEdit) return l10n.homePulseAskCtaOwn;
    if (p.hasViewerHasOffered() && p.viewerHasOffered) {
      return l10n.homePulseAskCtaOffered;
    }
    return null;
  }

  /// Weekday pill label ("SUN") when the event has a specific time.
  String? _eventDayLabel(ExperienceCreatedPayload p) {
    if (!p.time.hasSpecific() || p.time.specific.unixTimestampSec == 0) {
      return null;
    }
    final when = DateTime.fromMillisecondsSinceEpoch(
      p.time.specific.unixTimestampSec.toInt() * 1000,
    );
    return DateFormat('EEE').format(when);
  }

  /// "Sun 10:00 AM · Millbrook" — the informal time as typed when there
  /// is one, else the formatted specific time, plus the place.
  String _eventMeta(ExperienceCreatedPayload p) {
    String? time;
    if (p.time.informalDescription.isNotEmpty) {
      time = p.time.informalDescription;
    } else if (p.time.hasSpecific() && p.time.specific.unixTimestampSec > 0) {
      final when = DateTime.fromMillisecondsSinceEpoch(
        p.time.specific.unixTimestampSec.toInt() * 1000,
      );
      time = p.time.specific.isAllDay
          ? DateFormat('EEE, MMM d').format(when)
          : DateFormat('EEE, MMM d · h:mm a').format(when);
    }
    return [?time, if (p.locationName.isNotEmpty) p.locationName].join(' · ');
  }

  /// "5 going · Room for 3" — the joinable line, from RSVP counts and
  /// capacity; empty when there are no counts to show yet (the header
  /// already attributes the event).
  String _eventStatusLine(ExperienceCreatedPayload p) {
    final l10n = context.l10n;
    final remaining = p.maxParticipants > 0
        ? p.maxParticipants - p.yesCount
        : 0;
    return [
      if (p.yesCount > 0) l10n.homePulseEventGoing(p.yesCount),
      if (remaining > 0) l10n.homePulseEventRoomFor(remaining),
    ].join(' · ');
  }

  void _openItem(String itemId, String itemType) {
    unawaited(
      NavigationHelpers.pushToItemScreen(
        context: context,
        itemId: itemId,
        itemType: itemType,
      ),
    );
  }

  /// The who-strip's tap — opens the fronting person's profile. Null
  /// (falls back to the item) when the payload carries no user id.
  VoidCallback? _openPerson(User user) {
    if (user.id.isEmpty) return null;
    return () => ContentViewHelpers.openUserScreen(context, user.id);
  }
}

class _PostData {
  /// Who-line ("Carmen asks", "Betty Cho", "Betty, Alfred +2").
  final String headerName;

  /// Where/when-line under the name (location, time, or relative age).
  final String headerSub;

  /// The person whose avatar fronts the post; null renders
  /// [headerGlyph] instead.
  final User? headerUser;

  /// Glyph for posts nobody fronts (stories).
  final IconData headerGlyph;

  final String title;

  /// Caption body under the title.
  final String meta;

  /// Status line ("Asked 2h ago", "5 going · Room for 3").
  final String statusLine;

  /// Snapshot media; empty renders the post without a photo block and
  /// the caption on the card surface instead.
  final String mediaId;

  /// Dark overlay pill on the snapshot ("SUN").
  final String? snapPill;

  /// Optional header-corner CTA; solid for give/ask verbs, ghost for
  /// join. Names the action the item's full-screen view opens onto and
  /// taps through to it.
  final String? ctaLabel;
  final bool ctaIsSolid;

  /// Whether [ctaLabel] states what the viewer has already done ("Going",
  /// "Offered") rather than inviting them to act. Status labels render as a
  /// third treatment — neither solid nor ghost — and are **not** wrapped in a
  /// tap target: a chip that says "Going" but announces as a button named
  /// "Going" promises an action it does not perform. The card body still
  /// opens the item, so changing the answer stays one tap away.
  final bool ctaIsStatus;

  final bool isUnread;

  /// Opens the item's full-screen view (the snapshot/caption and CTA).
  final VoidCallback onTap;

  /// Opens the fronting person's profile (the footer's who-strip:
  /// avatar + name). Null — stories, or payloads with no user id —
  /// makes the who-strip fall back to [onTap].
  final VoidCallback? onPersonTap;

  const _PostData({
    required this.headerName,
    required this.headerSub,
    required this.headerUser,
    this.headerGlyph = Icons.group_outlined,
    required this.title,
    required this.meta,
    required this.statusLine,
    required this.mediaId,
    this.snapPill,
    this.ctaLabel,
    this.ctaIsSolid = true,
    this.ctaIsStatus = false,
    required this.isUnread,
    required this.onTap,
    this.onPersonTap,
  });

  bool get hasImage => mediaId.isNotEmpty;
}

/// One full-width snapshot post: the snapshot with the title, body,
/// and status overlaid on the standard bottom shim in white — the
/// content-view pattern — then the footer strip (face + who/where +
/// CTA) along the card's bottom. Posts without a photo carry the
/// caption on the card surface instead.
class _PostCard extends ConsumerWidget {
  final _PostData data;

  const _PostCard({required this.data});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    // Sibling tap targets, never nested (a nested Tappable loses its
    // semantics node — the HomeEditorialRow pill / gear who-card CTA
    // failure class): the snapshot/caption opens the item, the footer's
    // who-strip opens the person, the CTA opens the item.
    return Container(
      width: double.infinity,
      clipBehavior: Clip.antiAlias,
      decoration: BoxDecoration(
        color: AppColors.cardBackground(context),
        borderRadius: BorderRadius.circular(22),
        border: Border.all(color: AppColors.border(context)),
        boxShadow: [
          BoxShadow(
            color: Colors.black.withValues(alpha: 0.08),
            blurRadius: 15,
            offset: const Offset(0, 3),
          ),
        ],
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // The post's spoken label is its title; the status line is
          // visual detail the full-screen view repeats.
          Tappable(
            semanticsLabel: data.title,
            onTap: data.onTap,
            inkBorderRadius: BorderRadius.zero,
            child: data.hasImage ? _Snapshot(data: data) : _caption(context),
          ),
          _header(context),
        ],
      ),
    );
  }

  Widget _header(BuildContext context) {
    // The who-strip: its own target opening the person's profile
    // (stories and id-less payloads fall back to the item).
    final personLabel = (data.headerUser?.name.isNotEmpty ?? false)
        ? data.headerUser!.name
        : data.headerName;
    return Row(
      children: [
        Expanded(
          child: Tappable(
            semanticsLabel: personLabel,
            onTap: data.onPersonTap ?? data.onTap,
            inkBorderRadius: BorderRadius.zero,
            child: Padding(
              padding: const EdgeInsets.fromLTRB(13, 10, 9, 9),
              child: Row(
                children: [
                  _HeaderFace(
                    user: data.headerUser,
                    glyph: data.headerGlyph,
                    unread: data.isUnread,
                  ),
                  const SizedBox(width: 9),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          data.headerName,
                          maxLines: 1,
                          overflow: TextOverflow.ellipsis,
                          style: TextStyle(
                            fontSize: 13,
                            fontWeight: FontWeight.w600,
                            color: AppColors.textPrimary(context),
                          ),
                        ),
                        if (data.headerSub.isNotEmpty)
                          Text(
                            data.headerSub,
                            maxLines: 1,
                            overflow: TextOverflow.ellipsis,
                            style: TextStyle(
                              fontSize: 11,
                              color: AppColors.textSecondary(context),
                            ),
                          ),
                      ],
                    ),
                  ),
                ],
              ),
            ),
          ),
        ),
        if (data.ctaLabel != null)
          if (data.ctaIsStatus)
            // Stated, not offered — so no tap target (see [ctaIsStatus]).
            Padding(
              padding: const EdgeInsets.fromLTRB(0, 10, 10, 9),
              child: _StatusChip(label: data.ctaLabel!),
            )
          else
            Tappable(
              semanticsLabel: data.ctaLabel!,
              onTap: data.onTap,
              inkBorderRadius: BorderRadius.circular(12),
              child: Padding(
                padding: const EdgeInsets.fromLTRB(0, 10, 10, 9),
                child: _CtaButton(
                  label: data.ctaLabel!,
                  solid: data.ctaIsSolid,
                ),
              ),
            ),
      ],
    );
  }

  /// Card-surface caption for posts without a photo.
  Widget _caption(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(14, 13, 14, 0),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            data.title,
            maxLines: 2,
            overflow: TextOverflow.ellipsis,
            style: TextStyle(
              fontFamily: AppTheme.headingFont,
              fontSize: 16,
              fontWeight: FontWeight.w500,
              height: 1.25,
              color: AppColors.textPrimary(context),
            ),
          ),
          if (data.meta.isNotEmpty) ...[
            const SizedBox(height: 3),
            Text(
              data.meta,
              maxLines: 2,
              overflow: TextOverflow.ellipsis,
              style: TextStyle(
                fontSize: 11.5,
                color: AppColors.textSecondary(context),
              ),
            ),
          ],
          if (data.statusLine.isNotEmpty) ...[
            const SizedBox(height: 6),
            Text(
              data.statusLine,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: TextStyle(
                fontSize: 10,
                fontWeight: FontWeight.w600,
                color: AppColors.primary(context),
              ),
            ),
          ],
        ],
      ),
    );
  }
}

/// The post header's 32px face: the person's avatar, their initial on
/// the sage-soft circle while it loads, or the glyph for posts nobody
/// fronts. Carries the unread dot on its corner.
class _HeaderFace extends ConsumerWidget {
  final User? user;
  final IconData glyph;
  final bool unread;

  const _HeaderFace({
    required this.user,
    required this.glyph,
    required this.unread,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final face = SizedBox(width: 32, height: 32, child: _face(context, ref));
    if (!unread) return face;
    return Stack(
      clipBehavior: Clip.none,
      children: [
        face,
        Positioned(
          top: -2,
          right: -2,
          child: Container(
            width: 10,
            height: 10,
            decoration: BoxDecoration(
              color: AppColors.transferCoral,
              shape: BoxShape.circle,
              border: Border.all(
                color: AppColors.cardBackground(context),
                width: 2,
              ),
            ),
          ),
        ),
      ],
    );
  }

  Widget _face(BuildContext context, WidgetRef ref) {
    Widget fallback() => Container(
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        color: AppColors.primary(context).withAlpha(31),
      ),
      alignment: Alignment.center,
      child: user == null || user!.name.isEmpty
          ? Icon(glyph, size: 17, color: AppColors.primary(context))
          : Text(
              user!.name[0].toUpperCase(),
              style: TextStyle(
                fontSize: 13,
                fontWeight: FontWeight.w700,
                color: AppColors.primary(context),
              ),
            ),
    );
    final mediaId = user?.mediaId ?? '';
    if (mediaId.isEmpty) return fallback();
    final mediaAsync = ref.watch(mediaObjectProvider(mediaId));
    return mediaAsync.when(
      data: (media) {
        if (media.url.isEmpty) return fallback();
        return ClipOval(
          child: CachedMediaImage(
            // Decorative: the who-line names the person right beside it.
            semanticsLabel: null,
            imageUrl: media.url,
            cacheKey: ImageCacheKeys.thumbnail(mediaId),
            width: 32,
            height: 32,
            fit: BoxFit.cover,
          ),
        );
      },
      loading: () => fallback(),
      error: (_, _) => fallback(),
    );
  }
}

/// The full-width snapshot: skeleton shimmer until the photo paints,
/// the standard bottom shim with the title, body, and status in white,
/// and an optional dark day pill; the photo runs to the card's bottom
/// edge.
class _Snapshot extends ConsumerWidget {
  final _PostData data;

  const _Snapshot({required this.data});

  static const double _height = 240;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final mediaAsync = ref.watch(mediaObjectProvider(data.mediaId));
    final photo = mediaAsync.when(
      data: (media) {
        if (media.url.isEmpty) return const _ShimmerBox();
        return CachedMediaImage(
          semanticsLabel: null,
          imageUrl: media.url,
          cacheKey: ImageCacheKeys.thumbnail(data.mediaId),
          width: double.infinity,
          height: double.infinity,
          fit: BoxFit.cover,
        );
      },
      loading: () => const _ShimmerBox(),
      error: (_, _) => ColoredBox(color: AppColors.surface(context)),
    );
    return SizedBox(
      height: _height,
      width: double.infinity,
      child: Stack(
        fit: StackFit.expand,
        children: [
          photo,
          // The standard content-view shim so the caption reads over
          // any photo.
          const DecoratedBox(
            decoration: BoxDecoration(
              gradient: LinearGradient(
                begin: Alignment.topCenter,
                end: Alignment.bottomCenter,
                colors: [Colors.transparent, OverlayTokens.scrimBottom],
                stops: [0.35, 1.0],
              ),
            ),
          ),
          if (data.snapPill != null)
            Positioned(
              top: 9,
              left: 9,
              child: Container(
                padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
                decoration: BoxDecoration(
                  color: const Color(0x73141810),
                  borderRadius: BorderRadius.circular(7),
                ),
                child: Text(
                  data.snapPill!.toUpperCase(),
                  style: const TextStyle(
                    fontSize: 9,
                    fontWeight: FontWeight.w700,
                    letterSpacing: 0.8,
                    color: OverlayTokens.textPrimary,
                  ),
                ),
              ),
            ),
          Positioned(
            left: 14,
            right: 14,
            bottom: 12,
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  data.title,
                  maxLines: 2,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(
                    fontFamily: AppTheme.headingFont,
                    fontSize: 16,
                    fontWeight: FontWeight.w500,
                    height: 1.25,
                    color: OverlayTokens.textPrimary,
                  ),
                ),
                if (data.meta.isNotEmpty) ...[
                  const SizedBox(height: 3),
                  Text(
                    data.meta,
                    maxLines: 2,
                    overflow: TextOverflow.ellipsis,
                    style: const TextStyle(
                      fontSize: 11.5,
                      color: OverlayTokens.textFaint,
                    ),
                  ),
                ],
                if (data.statusLine.isNotEmpty) ...[
                  const SizedBox(height: 5),
                  Text(
                    data.statusLine,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: const TextStyle(
                      fontSize: 10,
                      fontWeight: FontWeight.w600,
                      color: OverlayTokens.textFaint,
                    ),
                  ),
                ],
              ],
            ),
          ),
        ],
      ),
    );
  }
}

/// The snapshot's skeleton: a slow shimmer sweep while the photo loads,
/// degrading to a static surface tint under Reduce Motion.
class _ShimmerBox extends StatefulWidget {
  const _ShimmerBox();

  @override
  State<_ShimmerBox> createState() => _ShimmerBoxState();
}

class _ShimmerBoxState extends State<_ShimmerBox>
    with SingleTickerProviderStateMixin {
  AnimationController? _controller;

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    final duration = accessibleDuration(
      context,
      const Duration(milliseconds: 1500),
    );
    if (duration == Duration.zero) {
      _controller?.dispose();
      _controller = null;
      return;
    }
    _controller ??= AnimationController(vsync: this, duration: duration)
      ..repeat();
  }

  @override
  void dispose() {
    _controller?.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final base = AppColors.surface(context);
    final controller = _controller;
    if (controller == null) return ColoredBox(color: base);
    final highlight = AppColors.cardBackground(context);
    return AnimatedBuilder(
      animation: controller,
      builder: (context, _) {
        final t = controller.value * 2 - 1;
        return DecoratedBox(
          decoration: BoxDecoration(
            gradient: LinearGradient(
              begin: Alignment(-1 + t * 2, 0),
              end: Alignment(1 + t * 2, 0),
              colors: [base, highlight, base],
            ),
          ),
        );
      },
    );
  }
}

/// The header-corner status: what the viewer has already done ("Going",
/// "Offered", "Hosting"), standing in for the CTA once they have. A third
/// treatment beside the CTA's solid and ghost — a muted fill with a leading
/// check, so it reads as settled rather than as a disabled button.
///
/// The check and the label both carry the meaning; colour only reinforces
/// it. Not wrapped in a tap target — see [_PostData.ctaIsStatus].
class _StatusChip extends StatelessWidget {
  final String label;

  const _StatusChip({required this.label});

  @override
  Widget build(BuildContext context) {
    final tint = AppColors.primary(context);
    return Container(
      height: 36,
      padding: const EdgeInsets.symmetric(horizontal: 12),
      alignment: Alignment.center,
      decoration: BoxDecoration(
        color: tint.withAlpha(26),
        borderRadius: BorderRadius.circular(12),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(Icons.check_rounded, size: 15, color: tint),
          const SizedBox(width: 5),
          Text(
            label,
            style: TextStyle(
              fontSize: 12,
              fontWeight: FontWeight.w600,
              color: tint,
            ),
          ),
        ],
      ),
    );
  }
}

/// The header-corner CTA: names the action the full-screen view opens
/// onto ("I have one", "Join"); solid for give/ask verbs, ghost for
/// join. Wrapped in its own Tappable by the header (sibling of the
/// who-strip's), tapping through to the item.
class _CtaButton extends StatelessWidget {
  final String label;
  final bool solid;

  const _CtaButton({required this.label, required this.solid});

  @override
  Widget build(BuildContext context) {
    return Container(
      height: 36,
      padding: const EdgeInsets.symmetric(horizontal: 14),
      alignment: Alignment.center,
      decoration: BoxDecoration(
        color: solid ? AppColors.primary(context) : Colors.transparent,
        borderRadius: BorderRadius.circular(12),
        border: solid
            ? null
            : Border.all(color: AppColors.primary(context), width: 1.5),
      ),
      child: Text(
        label,
        style: TextStyle(
          fontSize: 12,
          fontWeight: FontWeight.w600,
          color: solid
              ? Theme.of(context).colorScheme.onPrimary
              : AppColors.primary(context),
        ),
      ),
    );
  }
}
