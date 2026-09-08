import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/overlay_tokens.gen.dart';
import 'package:ripls/core/utils/date_time_formatter.dart';
import 'package:ripls/core/utils/distance_formatter.dart';
import 'package:ripls/data/gen/ripls/api/experience.pbenum.dart' as proto;
import 'package:ripls/data/gen/ripls/api/experience_service.pbenum.dart'
    show RSVPIntention;
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/screens/create/blank_create_dispatcher.dart'
    show openRepeatDraft;
import 'package:ripls/presentation/screens/experience/mark_completed_modal.dart';
import 'package:ripls/presentation/screens/experience/widgets/experience_content_panel_launcher.dart';
import 'package:ripls/presentation/screens/experience/widgets/experience_pitching_in_screen.dart';
import 'package:ripls/presentation/screens/experience/widgets/experience_rsvp_composer_sheet.dart';
import 'package:ripls/presentation/viewmodels/content_expanded_provider.dart';
import 'package:ripls/presentation/viewmodels/experience_needs_view_model.dart';
import 'package:ripls/presentation/viewmodels/experience_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';
import 'package:ripls/presentation/widgets/adaptive/content_column.dart';
import 'package:ripls/presentation/widgets/content/content_action_chip.dart';
import 'package:ripls/presentation/widgets/content/content_dashed_avatar.dart';
import 'package:ripls/presentation/widgets/content/content_discussion_card.dart';
import 'package:ripls/presentation/widgets/content/content_edge_avatar.dart';
import 'package:ripls/presentation/widgets/content/content_edge_data.dart';
import 'package:ripls/presentation/widgets/content/content_edges_card.dart';
import 'package:ripls/presentation/widgets/content/content_facts_row.dart';
import 'package:ripls/presentation/widgets/content/content_headline.dart';
import 'package:ripls/presentation/widgets/content/content_lifecycle_phase.dart';
import 'package:ripls/presentation/widgets/content/content_participation_overflow.dart';
import 'package:ripls/presentation/widgets/content/content_view_helpers.dart';
import 'package:ripls/presentation/widgets/content/hero_content_wash.dart';
import 'package:ripls/presentation/widgets/content/photo_attribution_line.dart';
import 'package:ripls/presentation/widgets/home/calendar/calendar_weather.dart';

/// ExperienceReadShell is the redesigned read-mode layout for an experience: an
/// editorial sheet that scrolls over the full-bleed hero rendered behind it by
/// the content view (docs/issues/2278-experience-content-redesign.md). It
/// composes the shared `widgets/content/` building blocks from the experience
/// view-model's derived view-data (chips, headline, facts, messages card, edges
/// graph, you-gap, CTA, whats-next).
///
/// It is read-only: edit mode still routes through the existing event pane.
/// All mutations flow through callbacks the content view owns (or, for RSVP and
/// contributions, directly through the view-model / NeedsActions via `ref`).
class ExperienceReadShell extends ConsumerWidget {
  final String experienceId;
  final Color accentColor;

  /// Expands the discussion ("comments") card into the full conversation,
  /// morphing from the card's footprint ([Rect]) over the hero — see
  /// docs/client/modals.md (Morph-reveal content panels).
  final ValueChanged<Rect> onExpandConversation;

  /// Expands the time ("WHEN") card into the full-screen time panel, morphing
  /// from the card's footprint ([Rect]) over the hero — see docs/client/modals.md
  /// (Morph-reveal content panels).
  final ValueChanged<Rect> onShowTime;

  /// Expands the location ("WHERE") card into the full-screen location panel,
  /// morphing from the card's footprint ([Rect]) over the hero — see
  /// docs/client/modals.md (Morph-reveal content panels).
  final ValueChanged<Rect> onShowLocation;

  /// Opens the community access sheet ("who can see this").
  final VoidCallback onShowAccess;

  /// Opens the owner manage sheet (owners only).
  final VoidCallback onManage;

  /// Extra bottom inset so the sticky CTA clears the home nav bar when this view
  /// is embedded in the feed (the nav bar overlays the bottom of the screen).
  final double bottomNavInset;

  const ExperienceReadShell({
    super.key,
    required this.experienceId,
    required this.accentColor,
    required this.onExpandConversation,
    required this.onShowTime,
    required this.onShowLocation,
    required this.onShowAccess,
    required this.onManage,
    this.bottomNavInset = 0,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(experienceProvider(experienceId));
    final exp = state.experienceDetails?.experience;
    if (exp == null) return const SizedBox.shrink();

    // While the roster is expanded into its full-screen panel, hide this whole
    // surface so the panel overlays the (still-playing) hero alone — none of
    // the facts/discussion/collapsed-roster content shows through it.
    if (ref.watch(experienceContentExpandedProvider(experienceId))) {
      return const SizedBox.shrink();
    }

    final phase = state.lifecyclePhase;
    final isTerminal =
        phase == ContentLifecyclePhase.wrapped ||
        phase == ContentLifecyclePhase.cancelled;

    final needsState = ref.watch(experienceNeedsProvider(experienceId));
    final edges = state.edges(
      contributionsByUserId: _contributions(context, needsState),
    );
    final groups = state.rosterGroups();

    // Bottom-anchored, non-scrolling content panel over a dark gradient — the
    // same overlay treatment as the legacy ExperienceBottomContent. No grab
    // handle and no internal scroll view, so the TikTok-style feed's vertical
    // swipe is never captured by this surface.
    return Stack(
      children: [
        // Terminal events (wrapped / cancelled) dim the background imagery.
        // IgnorePointer so the feed's vertical swipe still passes through.
        if (isTerminal)
          const Positioned.fill(
            child: IgnorePointer(
              child: ColoredBox(color: OverlayTokens.scrimFloor),
            ),
          ),
        Positioned(
          left: 0,
          right: 0,
          bottom: 0,
          // The caption column (#2912): on a desktop-wide window the sheet —
          // and the wash, which sizes to it — holds the reading measure,
          // bottom-centered over the full-bleed hero. No-op at phone widths.
          child: ContentColumn(
            child: HeroContentWash(
              child: SafeArea(
                top: false,
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    Padding(
                      padding: const EdgeInsets.fromLTRB(20, 16, 20, 16),
                      child: Column(
                        mainAxisSize: MainAxisSize.min,
                        crossAxisAlignment: CrossAxisAlignment.stretch,
                        children: [
                          if (isTerminal) ...[
                            _terminalStatus(context, state),
                            const SizedBox(height: 10),
                          ],
                          ContentHeadline(title: exp.name),
                          // Discussion (description as the opening quote +
                          // replies) sits just under the title, above the facts.
                          ?_discussion(context, state),
                          const SizedBox(height: 14),
                          _facts(context, state, isTerminal: isTerminal),
                          const SizedBox(height: 12),
                          // The viewer's pinned row carries the RSVP CTA inline,
                          // so there is no separate bottom CTA bar.
                          _edges(
                            context,
                            ref,
                            state,
                            edges,
                            groups,
                            isTerminal: isTerminal,
                          ),
                          ?_completionSummary(context, state),
                        ],
                      ),
                    ),
                    if (bottomNavInset > 0) SizedBox(height: bottomNavInset),
                  ],
                ),
              ),
            ),
          ),
        ),
        _topBar(context, state),
      ],
    );
  }

  /// Per-person contribution summaries for the roster rows. A person with
  /// several claims gets "{first} +{n} more" instead of a comma-joined list:
  /// the row is one line, so a joined list ellipsizes mid-word (#2724).
  Map<String, String> _contributions(
    BuildContext context,
    ExperienceNeedsState needsState,
  ) {
    final titles = <String, List<String>>{};
    for (final contribution in needsState.contributions) {
      final id = contribution.contributor.id;
      if (id.isEmpty || contribution.title.isEmpty) continue;
      titles.putIfAbsent(id, () => []).add(contribution.title);
    }
    final l10n = context.l10n;
    return {
      for (final entry in titles.entries)
        entry.key: entry.value.length == 1
            ? entry.value.first
            : l10n.contentEdgeContributionMore(
                entry.value.first,
                entry.value.length - 1,
              ),
    };
  }

  Widget _facts(
    BuildContext context,
    ExperienceState state, {
    required bool isTerminal,
  }) {
    return ContentFactsRow(
      ctaAccentColor: accentColor,
      facts: [
        _whereFact(context, state, isTerminal: isTerminal),
        _whenFact(context, state, isTerminal: isTerminal),
      ],
    );
  }

  /// The WHERE card: the place name plus how far away it is ("3 mi away"); "TBD"
  /// when unset, or "TBD" + "{n} options" while a location poll is running. When
  /// there is something to do — a running poll the viewer hasn't voted in, or an
  /// unset place the owner can fill — a sage CTA chip sits at the card's foot
  /// (event V6b).
  ContentFactData _whereFact(
    BuildContext context,
    ExperienceState state, {
    required bool isTerminal,
  }) {
    final exp = state.experienceDetails!.experience;
    final l10n = context.l10n;
    String? detail;
    String value;
    ContentFactCta? cta;
    if (exp.locationPollActive) {
      // Poll running: keep "TBD · N options" on the single value line.
      final options = _locationPollOptionCount(state);
      value = options > 0
          ? l10n.contentFactPollOptions(options)
          : l10n.contentFactTbd;
      if (!isTerminal && !_hasVotedLocation(state)) {
        cta = ContentFactCta(
          label: l10n.contentRowsVoteAction,
          semanticsLabel: l10n.a11yContentCtaVoteLocation,
          onTap: onShowLocation,
        );
      }
    } else {
      final where = state.locationName;
      if (where == null || where.isEmpty) {
        value = l10n.contentFactTbd;
        // Unset place with no poll: only the owner can set it.
        if (!isTerminal && state.isOwner) {
          cta = ContentFactCta(
            label: l10n.contentCtaSetLocation,
            semanticsLabel: l10n.a11yContentCtaSetLocation,
            onTap: onShowLocation,
          );
        }
      } else {
        value = where;
        detail = DistanceFormatter.formatWithAway(state.locationDistanceMeters);
      }
    }
    return ContentFactData(
      label: l10n.contentFactWhere,
      value: value,
      detail: detail,
      onTap: onShowLocation,
      semanticsLabel: l10n.a11yExpChangeLocation,
      cta: cta,
    );
  }

  /// The WHEN card: the event time plus how far off it is ("3d 2h from now");
  /// "TBD" when unset, or "TBD" + "{n} options" while a time poll is running.
  /// Carries a sage CTA chip (Vote / Set time) when the viewer has an action.
  ContentFactData _whenFact(
    BuildContext context,
    ExperienceState state, {
    required bool isTerminal,
  }) {
    final details = state.experienceDetails!;
    final exp = details.experience;
    final l10n = context.l10n;
    String? detail;
    String value;
    ContentFactCta? cta;
    String? weatherGlyph;
    String? weatherTemp;
    String semanticsLabel = l10n.a11yExpChangeTime;
    if (exp.timePollActive) {
      // Poll running: keep "TBD · N options" on the single value line.
      final options = _timePollOptionCount(state);
      value = options > 0
          ? l10n.contentFactPollOptions(options)
          : l10n.contentFactTbd;
      if (!isTerminal && !_hasVotedTime(state)) {
        cta = ContentFactCta(
          label: l10n.contentRowsVoteAction,
          semanticsLabel: l10n.a11yContentCtaVoteTime,
          // The time card stays a bottom-sheet modal; ignore the morph rect.
          onTap: onShowTime,
        );
      }
    } else if (exp.hasTime() && !exp.time.hasTbd()) {
      value = DateTimeFormatter.formatExperienceTime(exp.time);
      final eventSec = _eventStartUnixSec(state);
      detail = eventSec != null
          ? DateTimeFormatter.formatRelativeFromNow(eventSec)
          : null;
      // The event-day forecast (server-computed for the event's location): a
      // compact glyph + temp on the card, with the full description folded into
      // the card's screen-reader label since the inline pair is decorative.
      if (details.hasForecast()) {
        final f = details.forecast;
        weatherGlyph = calendarConditionGlyph(f.condition);
        weatherTemp = f.temperatureDisplay;
        final weatherSemantic = calendarWeatherSemantic(context, f);
        if (weatherSemantic.isNotEmpty) {
          semanticsLabel = '$semanticsLabel. $weatherSemantic';
        }
      }
    } else {
      value = l10n.contentFactTbd;
      // Unset time with no poll: only the owner can set it.
      if (!isTerminal && state.isOwner) {
        cta = ContentFactCta(
          label: l10n.contentCtaSetTime,
          semanticsLabel: l10n.a11yContentCtaSetTime,
          onTap: onShowTime,
        );
      }
    }
    return ContentFactData(
      label: l10n.contentFactWhen,
      value: value,
      detail: detail,
      onTap: onShowTime,
      semanticsLabel: semanticsLabel,
      cta: cta,
      weatherGlyph: weatherGlyph,
      weatherTemp: weatherTemp,
    );
  }

  /// Whether the viewer has already voted in the currently-running time poll
  /// (votes from prior polls in `timeProposals` are excluded by the active poll
  /// id when present).
  bool _hasVotedTime(ExperienceState state) {
    final exp = state.experienceDetails!.experience;
    final uid = state.currentUserId;
    if (uid == null) return false;
    final pollId = exp.hasCurrentPollId() ? exp.currentPollId : '';
    return exp.timeProposals
        .where((p) => pollId.isEmpty || (p.hasPollId() && p.pollId == pollId))
        .any((p) => p.votes.any((v) => v.user.id == uid));
  }

  /// Whether the viewer has already voted in the currently-running location
  /// poll.
  bool _hasVotedLocation(ExperienceState state) {
    final exp = state.experienceDetails!.experience;
    final uid = state.currentUserId;
    if (uid == null) return false;
    final pollId = exp.hasCurrentLocationPollId()
        ? exp.currentLocationPollId
        : '';
    return exp.locationProposals
        .where((p) => pollId.isEmpty || (p.hasPollId() && p.pollId == pollId))
        .any((p) => p.votes.any((v) => v.user.id == uid));
  }

  /// The event's representative start timestamp (specific time, else the start
  /// of a range), or null when neither is set.
  int? _eventStartUnixSec(ExperienceState state) {
    final time = state.experienceDetails!.experience.time;
    if (time.hasSpecific() && time.specific.unixTimestampSec > 0) {
      return time.specific.unixTimestampSec.toInt();
    }
    if (time.hasRange() && time.range.hasStartUnixSec()) {
      return time.range.startUnixSec.toInt();
    }
    return null;
  }

  /// Number of options in the currently-running time poll (older proposals from
  /// prior polls are excluded by the active poll id when present).
  int _timePollOptionCount(ExperienceState state) {
    final exp = state.experienceDetails!.experience;
    final pollId = exp.hasCurrentPollId() ? exp.currentPollId : '';
    return exp.timeProposals
        .where((p) => pollId.isEmpty || (p.hasPollId() && p.pollId == pollId))
        .length;
  }

  /// Number of options in the currently-running location poll.
  int _locationPollOptionCount(ExperienceState state) {
    final exp = state.experienceDetails!.experience;
    final pollId = exp.hasCurrentLocationPollId()
        ? exp.currentLocationPollId
        : '';
    return exp.locationProposals
        .where((p) => pollId.isEmpty || (p.hasPollId() && p.pollId == pollId))
        .length;
  }

  /// The discussion block shown where the description used to live: the
  /// description rendered as the opening comment, the reply count, and a preview
  /// of the most recent comment. Null when there is neither a description nor a
  /// conversation. Returns the spacing + card so the caller can null-spread it.
  Widget? _discussion(BuildContext context, ExperienceState state) {
    final exp = state.experienceDetails!.experience;
    final l10n = context.l10n;
    final hasConversation = exp.conversationId.isNotEmpty;
    final hasDescription = exp.description.trim().isNotEmpty;
    if (!hasConversation && !hasDescription) return null;

    // A non-empty description is seeded as the conversation's first comment,
    // so the raw message count includes it — the rendered reply count must
    // not, or a fresh event reads "1 reply" (#2724).
    final replyCount = hasDescription && exp.messageCount > 0
        ? exp.messageCount - 1
        : exp.messageCount;
    final hasRecent = replyCount > 0 && exp.lastMessageText.isNotEmpty;
    String? lastActivityLabel;
    if (hasRecent && exp.lastMessageTimeAgo.isNotEmpty) {
      // The server renders compact tokens ("0m", "2h", "64d"); normalize the
      // broken-reading extremes before display (#2724).
      final ago = DateTimeFormatter.normalizeCompactTimeAgo(
        exp.lastMessageTimeAgo,
      );
      lastActivityLabel = ago == null
          ? l10n.contentDiscussionJustNow
          : l10n.contentDiscussionLastActivity(ago);
    }
    return Padding(
      padding: const EdgeInsets.only(top: 12),
      child: ContentDiscussionCard(
        authorName: exp.owner.name,
        firstComment: exp.description,
        replyCount: replyCount,
        replyCountLabel: l10n.contentReplyCount(replyCount),
        startLabel: l10n.contentMessagesStart,
        lastActivityLabel: lastActivityLabel,
        hasUnread: exp.unreadCount > 0,
        accentColor: accentColor,
        latestLine: hasRecent && exp.lastMessageSender.name.isNotEmpty
            ? l10n.contentDiscussionLatest(
                exp.lastMessageSender.name,
                exp.lastMessageText,
              )
            : null,
        semanticsLabel: l10n.a11yContentOpenConversation,
        onTap: hasConversation ? onExpandConversation : null,
      ),
    );
  }

  Widget _edges(
    BuildContext context,
    WidgetRef ref,
    ExperienceState state,
    List<EdgeViewData> edges,
    RosterGroups groups, {
    required bool isTerminal,
  }) {
    final l10n = context.l10n;

    // A quiet surface: the viewer's status row (tap to expand for the actual
    // controls), then everyone else. No loud call-to-action — the operations
    // live in the expanded panel. Cap the other-participant rows so the card
    // stays short (parity with the request card — see
    // content_participation_overflow.dart); the rest fold behind "+N more".
    final others = [
      for (final edge in edges)
        if (!edge.isYou) edge,
    ];
    final shownOthers = others.take(kParticipationCardMaxItems).toList();
    final hidden = others.skip(kParticipationCardMaxItems).toList();
    final rows = <Widget>[
      if (!isTerminal) _youRow(context, state, edges),
      for (final edge in shownOthers) _collapsedRow(context, edge),
      if (hidden.isNotEmpty)
        contentParticipationMoreRow(_overflowLabel(context, hidden)),
    ];

    return ContentEdgesCard(
      accentColor: accentColor,
      // Match the WHERE/WHEN fact labels' neutral treatment.
      headerColor: AppColors.darkTextTertiary,
      headerLabel: l10n.contentEdgesHeader,
      headerTrailing: _headerTrailing(context, groups),
      onTap: (cardRect) => _openPitchingIn(context, ref, cardRect),
      semanticsLabel: l10n.a11yExpViewAttendees,
      rows: rows,
      footer: _edgesFooter(context, ref, state, isTerminal: isTerminal),
    );
  }

  /// The chip at the foot of the Who's-in card — one slot, three jobs, in the
  /// order a host meets them:
  ///
  /// 1. **Wrap up**, once the event's start time has passed and it is still
  ///    open. Wrapping up is the host's whole next move at that point, and it
  ///    used to live only behind the ⋯ manage sheet.
  /// 2. **Schedule the next one**, once it is wrapped — sitting directly under
  ///    the people who actually came, which is the crew the draft re-invites.
  /// 3. **RSVP**, for a guest who has not replied yet.
  Widget? _edgesFooter(
    BuildContext context,
    WidgetRef ref,
    ExperienceState state, {
    required bool isTerminal,
  }) {
    if (state.isOwner) {
      if (state.lifecyclePhase == ContentLifecyclePhase.wrapped) {
        return _scheduleNextCta(context, ref);
      }
      if (!isTerminal && _hasStarted(state)) {
        return _wrapUpCta(context, state);
      }
    }
    return _rsvpCta(context, state, isTerminal: isTerminal);
  }

  /// Whether the event's scheduled start is in the past — the point from which
  /// "wrap up" is a sensible thing to offer. An event whose time is still a
  /// poll or TBD has no start to be past, so it never prompts.
  bool _hasStarted(ExperienceState state) {
    final start = _eventStartUnixSec(state);
    if (start == null) return false;
    return start <= DateTime.now().millisecondsSinceEpoch ~/ 1000;
  }

  /// The host's "Wrap up" chip: confirm who came and capture the impact.
  ///
  /// This is the same action the ⋯ manage sheet carries, promoted to the card
  /// foot because it is the one thing a host needs from an event that has
  /// already happened — and behind the sheet it was easy to miss entirely.
  Widget _wrapUpCta(BuildContext context, ExperienceState state) {
    final exp = state.experienceDetails!.experience;
    return ContentActionChip(
      label: context.l10n.experienceMenuMarkCompleted,
      semanticsLabel: context.l10n.a11yExpWrapUp,
      accentColor: accentColor,
      fullWidth: true,
      onTap: () => unawaited(
        MarkCompletedModal.show(
          context,
          exp.id,
          state.communityId ?? '',
          sharedCommunityIds: exp.sharedCommunityIds.toSet(),
        ),
      ),
    );
  }

  /// The wrapped-up event's "Schedule the next one" chip (host only).
  ///
  /// Opens the create preview pre-filled by `GenerateWorkshopDraft` from this
  /// very event — same name, place and photo, next week's slot at the same
  /// hour, and the crew confirmed as having come — so keeping a rhythm going
  /// is one tap and an edit rather than retyping an event already run. Without
  /// it the only route to that draft is a workshop-surface `schedule_repeat`
  /// nudge (docs/workshop.md), which the feed and inbox pools filter out and
  /// whose only reader left the nav with #2568. [openRepeatDraft] falls back
  /// to a blank create when the draft can't be built, so it never dead-ends.
  Widget _scheduleNextCta(BuildContext context, WidgetRef ref) {
    return ContentActionChip(
      label: context.l10n.experienceScheduleNextOne,
      semanticsLabel: context.l10n.a11yExpScheduleNextOne,
      accentColor: accentColor,
      fullWidth: true,
      onTap: () => unawaited(openRepeatDraft(context, ref, experienceId)),
    );
  }

  /// The RSVP call-to-action at the foot of the Who's-in card (event V6b): a
  /// roomy sage chip shown only when the viewer has not replied yet. Tapping it
  /// opens the RSVP composer sheet. Null once the viewer has an intention (their
  /// row already reflects it) or on terminal events.
  Widget? _rsvpCta(
    BuildContext context,
    ExperienceState state, {
    required bool isTerminal,
  }) {
    if (isTerminal || state.currentUserIntention != null) return null;
    return ContentActionChip(
      label: context.l10n.contentCtaRsvp,
      semanticsLabel: context.l10n.a11yContentCtaRsvp,
      accentColor: accentColor,
      fullWidth: true,
      onTap: () => ExperienceRsvpComposerSheet.show(
        context,
        experienceId: experienceId,
        accentColor: accentColor,
        initialIntention: RSVPIntention.RSVP_INTENTION_UNSPECIFIED,
      ),
    );
  }

  /// Names the folded-away rows when they all share one status ("1 maybe",
  /// "2 invited") instead of the anonymous "1 more" — otherwise the header's
  /// "5 in · 8 invited" over five rows leaves the reader to guess which bucket
  /// the remainder came from (#2724). Falls back to the plain count when the
  /// hidden rows are mixed.
  String _overflowLabel(BuildContext context, List<EdgeViewData> hidden) {
    final l10n = context.l10n;
    final count = hidden.length;
    final status = hidden.first.status;
    if (hidden.any((e) => e.status != status)) {
      return l10n.experienceMoreParticipants(count);
    }
    return switch (status) {
      EdgeStatus.maybe => l10n.experienceMaybeCount(count),
      EdgeStatus.invited => l10n.contentEdgesInvitedCount(count),
      EdgeStatus.going || EdgeStatus.host => l10n.experienceGoingCount(count),
      // A wrapped event's rows are all "wrapped" — the count is the signal.
      EdgeStatus.wrapped => l10n.experienceMoreParticipants(count),
    };
  }

  /// Header summary: "{N} in · {M} invited" over disjoint buckets — N is the
  /// host + yes-RSVPs, M is invitees who haven't replied yet. The two never
  /// overlap and the host is never counted as invited, so the header math
  /// reconciles with the roster and with "Invited K people" toasts (#2724).
  /// The "· invited" tail is dropped when nobody is awaiting a reply.
  String _headerTrailing(BuildContext context, RosterGroups groups) {
    final l10n = context.l10n;
    final inLabel = l10n.contentEdgesInCount(groups.goingCount);
    if (groups.invitedNoReplyCount <= 0) return inLabel;
    return '$inLabel · ${l10n.contentEdgesInvitedCount(groups.invitedNoReplyCount)}';
  }

  /// The viewer's own row: a status circle + "You" (+ HOST) + their status text
  /// ("You haven't replied yet" before RSVP, else what they're bringing or
  /// their intention). Accent-edged + lightly tinted so it reads as "yours".
  Widget _youRow(
    BuildContext context,
    ExperienceState state,
    List<EdgeViewData> edges,
  ) {
    final l10n = context.l10n;
    final intention = state.currentUserIntention;
    EdgeViewData? youEdge;
    for (final e in edges) {
      if (e.isYou) {
        youEdge = e;
        break;
      }
    }
    final String status;
    if (intention == null) {
      status = l10n.rosterYouNoReply;
    } else if (youEdge != null && youEdge.contribution.isNotEmpty) {
      status = youEdge.contribution;
    } else {
      status = _intentionLabel(context, intention);
    }

    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 10),
      child: Row(
        children: [
          _youAvatar(youEdge, intention),
          const SizedBox(width: 10),
          Text(
            l10n.needsRowContributorYou,
            style: const TextStyle(
              color: AppColors.onContentImage,
              fontSize: 14.5,
              fontWeight: FontWeight.w800,
            ),
          ),
          if (state.isOwner) ...[const SizedBox(width: 8), _hostTag(l10n)],
          const SizedBox(width: 10),
          Expanded(
            child: Text(
              status,
              overflow: TextOverflow.ellipsis,
              style: const TextStyle(
                color: AppColors.darkTextSecondary,
                fontSize: 13.5,
              ),
            ),
          ),
        ],
      ),
    );
  }

  /// A participant row: face + status badge · name (· HOST) · what they're
  /// bringing.
  Widget _collapsedRow(BuildContext context, EdgeViewData edge) {
    final l10n = context.l10n;
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 9),
      child: Row(
        children: [
          ContentEdgeAvatar(edge: edge, accentColor: accentColor),
          const SizedBox(width: 10),
          Flexible(
            child: Text(
              edge.name,
              overflow: TextOverflow.ellipsis,
              style: const TextStyle(
                color: AppColors.onContentImage,
                fontSize: 14,
                fontWeight: FontWeight.w700,
              ),
            ),
          ),
          if (edge.status == EdgeStatus.host) ...[
            const SizedBox(width: 8),
            _hostTag(l10n),
          ],
          if (edge.contribution.isNotEmpty) ...[
            const SizedBox(width: 10),
            Expanded(
              child: Text(
                edge.contribution,
                overflow: TextOverflow.ellipsis,
                style: const TextStyle(
                  color: AppColors.darkTextSecondary,
                  fontSize: 13.5,
                ),
              ),
            ),
          ] else
            const Spacer(),
        ],
      ),
    );
  }

  String _intentionLabel(BuildContext context, RSVPIntention intention) {
    final l10n = context.l10n;
    return switch (intention) {
      RSVPIntention.RSVP_INTENTION_YES => l10n.experienceGoing,
      RSVPIntention.RSVP_INTENTION_NO => l10n.experienceRsvpCantGo,
      _ => l10n.experienceRsvpMaybe,
    };
  }

  Widget _hostTag(AppLocalizations l10n) => Text(
    l10n.experienceHostRole.toUpperCase(),
    style: const TextStyle(
      color: AppColors.darkTextTertiary,
      fontSize: 9,
      fontWeight: FontWeight.w700,
      letterSpacing: 0.8,
    ),
  );

  /// The viewer's own face + status badge. Falls back to the intention-derived
  /// circle when they have no row in the graph — they declined (declines are
  /// omitted from the edges) or their RSVP hasn't round-tripped yet.
  Widget _youAvatar(EdgeViewData? youEdge, RSVPIntention? intention) {
    if (youEdge == null) return _youCircle(intention);
    return ContentEdgeAvatar(edge: youEdge, accentColor: accentColor);
  }

  /// The viewer's status circle, derived from their RSVP intention (a dashed
  /// ring before they reply).
  Widget _youCircle(RSVPIntention? intention) {
    switch (intention) {
      case RSVPIntention.RSVP_INTENTION_YES:
        return _circle(
          fill: accentColor,
          child: const Icon(
            Icons.check_rounded,
            size: 15,
            color: AppColors.darkBackground,
          ),
        );
      case RSVPIntention.RSVP_INTENTION_MAYBE:
        return _circle(
          border: AppColors.statusWarningOnDark,
          child: const Text(
            '?',
            style: TextStyle(
              color: AppColors.statusWarningOnDark,
              fontSize: 13,
              fontWeight: FontWeight.w800,
            ),
          ),
        );
      case RSVPIntention.RSVP_INTENTION_NO:
        return _circle(border: AppColors.darkTextTertiary);
      default:
        // Not replied yet — a dashed ring.
        return ContentDashedAvatar(color: accentColor);
    }
  }

  Widget _circle({Color? fill, Color? border, Widget? child}) {
    return Container(
      width: 26,
      height: 26,
      alignment: Alignment.center,
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        color: fill,
        border: border != null ? Border.all(color: border, width: 1.5) : null,
      ),
      child: child,
    );
  }

  /// Expands the roster into a full-screen panel that grows from the card's
  /// footprint ([cardRect]) via a clip-reveal morph (not a slide, not a modal
  /// sheet — see docs/issues/2280-pitching-in-expand.md). Pushed on the root
  /// navigator so it overlays the feed's home nav bar; the custom
  /// [morphRevealRoute] owns the transition (so this bypasses the slide-based
  /// `NavigationHelpers.pushScreen`), and the reverse morph plays on pop.
  void _openPitchingIn(BuildContext context, WidgetRef ref, Rect cardRect) {
    openExperienceContentPanel(
      context: context,
      ref: ref,
      experienceId: experienceId,
      sourceRect: cardRect,
      routeName: 'experience_pitching_in',
      screen: ExperiencePitchingInScreen(
        experienceId: experienceId,
        accentColor: accentColor,
        onShowAccess: onShowAccess,
      ),
    );
  }

  /// Top-right overlay: the third-party photo-credit line (when the hero image
  /// is from a stock provider) and, to its right, the owner's overflow menu
  /// (edit / mark completed / close). The back button is owned by the parent
  /// screen (top-left); access / invites live in the participating screen's
  /// overflow now.
  Widget _topBar(BuildContext context, ExperienceState state) {
    final attribution = ContentViewHelpers.getBackgroundAttribution(
      mediaId: state.mediaId,
      allMediaItems: state.allMediaItems,
    );
    final showOverflow = state.isOwner;
    if (attribution == null && !showOverflow) return const SizedBox.shrink();
    final topInset = MediaQuery.of(context).padding.top;
    return Positioned(
      top: topInset + 8,
      left: 12,
      right: 12,
      child: Row(
        mainAxisAlignment: MainAxisAlignment.end,
        children: [
          if (attribution != null)
            Expanded(
              child: PhotoAttributionLine(
                attribution: attribution,
                boxed: true,
              ),
            ),
          if (showOverflow) ...[
            const SizedBox(width: 8),
            DecoratedBox(
              decoration: const BoxDecoration(
                color: OverlayTokens.fieldFill,
                shape: BoxShape.circle,
              ),
              child: IconAction(
                icon: Icons.more_horiz,
                semanticsLabel: context.l10n.a11yExpOpenManageSheet,
                color: AppColors.onContentImage,
                onPressed: onManage,
              ),
            ),
          ],
        ],
      ),
    );
  }

  /// Status pill shown above the headline for terminal events: "WRAPPED · {ago}"
  /// or "CANCELLED". Replaces the removed chip row's status affordance.
  Widget _terminalStatus(BuildContext context, ExperienceState state) {
    final exp = state.experienceDetails!.experience;
    final l10n = context.l10n;
    final isCancelled =
        exp.state == proto.ExperienceState.EXPERIENCE_STATE_CANCELLED;
    final color = isCancelled
        ? AppColors.darkTextTertiary
        : AppColors.statusInfoOnDark;
    final String label;
    if (isCancelled) {
      label = l10n.commonCancelled.toUpperCase();
    } else {
      final completed = exp.completedAtUnixSec.toInt();
      final ago = completed > 0
          ? DateTimeFormatter.formatTimeAgo(completed)
          : null;
      final wrapped = l10n.contentEdgeStatusWrapped.toUpperCase();
      label = ago != null ? '$wrapped · $ago' : wrapped;
    }
    return Align(
      alignment: Alignment.centerLeft,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
        decoration: BoxDecoration(
          color: color.withValues(alpha: 0.18),
          borderRadius: BorderRadius.circular(999),
        ),
        child: Text(
          label,
          style: TextStyle(
            color: color,
            fontSize: 10,
            fontWeight: FontWeight.w700,
            letterSpacing: 1,
          ),
        ),
      ),
    );
  }

  /// The AI recap rendered inline once an event has wrapped, in place of the
  /// (now-hidden) you-gap / CTA. Null when not wrapped or there is no summary.
  Widget? _completionSummary(BuildContext context, ExperienceState state) {
    final summary = state.completionSummary?.trim();
    final isWrapped = state.lifecyclePhase == ContentLifecyclePhase.wrapped;
    if (!isWrapped || summary == null || summary.isEmpty) return null;
    return Padding(
      padding: const EdgeInsets.only(top: 12),
      child: Text(
        summary,
        style: const TextStyle(
          color: AppColors.onContentImage,
          fontSize: 13.5,
          height: 1.5,
        ),
      ),
    );
  }
}
