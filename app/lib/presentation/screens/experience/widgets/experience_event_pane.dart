import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:intl/intl.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/core/utils/navigation_helpers.dart';
import 'package:ripls/data/gen/ripls/api/experience.pb.dart' show Experience;
import 'package:ripls/data/gen/ripls/api/experience.pbenum.dart' as proto;
import 'package:ripls/data/gen/ripls/api/experience_service.pb.dart' show RSVP;
import 'package:ripls/data/gen/ripls/api/experience_service.pbenum.dart'
    show RSVPIntention;
import 'package:ripls/data/gen/ripls/api/location.pb.dart'
    show Location, LocationProposal;
import 'package:ripls/data/gen/ripls/api/time.pb.dart';
import 'package:ripls/presentation/models/item_metric_data_base.dart';
import 'package:ripls/presentation/screens/experience/mark_completed_modal.dart';
import 'package:ripls/presentation/screens/experience/widgets/experience_manage_menu_sheet.dart';
import 'package:ripls/presentation/screens/experience/widgets/experience_rsvp_button.dart';
import 'package:ripls/presentation/screens/item/item_metrics_screen.dart';
import 'package:ripls/presentation/viewmodels/experience_needs_view_model.dart';
import 'package:ripls/presentation/viewmodels/experience_view_model.dart';
import 'package:ripls/presentation/viewmodels/needs_scope.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/content/close_item_modal.dart';
import 'package:ripls/presentation/widgets/content/content_impact_row.dart';
import 'package:ripls/presentation/widgets/content/content_info_row.dart';
import 'package:ripls/presentation/widgets/content/content_shared_widgets.dart'
    show ContentExpandableDescription;
import 'package:ripls/presentation/widgets/content/content_top_rows.dart';
import 'package:ripls/presentation/widgets/content/content_view_builders.dart';
import 'package:ripls/presentation/widgets/content/row_call_to_action_pill.dart';
import 'package:ripls/presentation/widgets/experience/compose/experience_compose_sheet.dart';
import 'package:ripls/presentation/widgets/location/location_picker_helpers.dart'
    show openDirections;
import 'package:ripls/presentation/widgets/needs/needs_actions.dart';
import 'package:ripls/presentation/widgets/planning/poll_banner.dart'
    show PollBanner;
import 'package:ripls/presentation/widgets/user_avatar.dart';
import 'package:ripls/presentation/widgets/weather/weather_chip.dart';
import 'package:url_launcher/url_launcher.dart';

/// ExperienceEventPane renders the "Event" tab pane for an experience.
///
/// View mode shows the title (with an owner edit-pencil that opens the
/// frosted Manage sheet), an expandable description, and a five-row info
/// stack: location, time, needs, owner, RSVP. Edit mode renders editable
/// fields for name, description, and event link.
class ExperienceEventPane extends ConsumerWidget {
  final String experienceId;
  final ExperienceState state;
  final Color accentColor;
  final String editingTitle;
  final String editingDescription;
  final String editingSourceUrl;
  final bool hasInitializedSourceUrl;
  final void Function(String) onTitleChanged;
  final void Function(String) onDescriptionChanged;
  final void Function(String) onSourceUrlChanged;
  final VoidCallback onShowTimeModal;
  final void Function(bool pollActive) onShowTimeModalNonOwner;
  final void Function(String? locationId) onShowLocationPickerModal;
  final VoidCallback onShowAttendeeSheet;
  final Future<void> Function() onDeleteExperience;
  final VoidCallback? onCancelExperience;
  final VoidCallback? onUnshareExperience;
  final VoidCallback onToggleEditMode;
  final VoidCallback onAddToCalendar;

  const ExperienceEventPane({
    super.key,
    required this.experienceId,
    required this.state,
    required this.accentColor,
    required this.editingTitle,
    required this.editingDescription,
    required this.editingSourceUrl,
    required this.hasInitializedSourceUrl,
    required this.onTitleChanged,
    required this.onDescriptionChanged,
    required this.onSourceUrlChanged,
    required this.onShowTimeModal,
    required this.onShowTimeModalNonOwner,
    required this.onShowLocationPickerModal,
    required this.onShowAttendeeSheet,
    required this.onDeleteExperience,
    this.onCancelExperience,
    this.onUnshareExperience,
    required this.onToggleEditMode,
    required this.onAddToCalendar,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return SingleChildScrollView(
      padding: const EdgeInsets.fromLTRB(16, 14, 16, 8),
      child: state.isEditing
          ? _buildEditContent(context)
          : _buildReadContent(context, ref),
    );
  }

  Widget _buildReadContent(BuildContext context, WidgetRef ref) {
    final exp = state.experienceDetails?.experience;
    if (exp == null) return const SizedBox.shrink();

    final sourceUrl = exp.hasSourceUrl() && exp.sourceUrl.isNotEmpty
        ? exp.sourceUrl
        : null;
    final isCompleted =
        exp.state == proto.ExperienceState.EXPERIENCE_STATE_COMPLETED;
    final isCancelled =
        exp.state == proto.ExperienceState.EXPERIENCE_STATE_CANCELLED;
    final isTerminal = isCompleted || isCancelled;

    final locationRow = _buildLocationRow(context, exp, isCompleted);
    final timeRow = _buildTimeRow(context, exp, isCompleted);
    final needsRow = _buildNeedsRow(context, ref, isTerminal: isTerminal);
    final rsvpRow = _buildRsvpRow(context, isTerminal: isTerminal);
    final impactRow = isCompleted
        ? ContentImpactRow.build(
            context: context,
            impact: state.experienceStats?.impact,
            // Inline entry point to the full impact receipt — completed
            // events have no manage menu left to hide it in (#2724).
            onViewImpact: () => NavigationHelpers.pushWithSlide(
              context: context,
              screen: ItemMetricsScreen(
                itemType: ItemType.experience,
                itemId: experienceId,
                communityId: state.communityId,
              ),
              routeName: 'item_metrics',
            ),
          )
        : null;

    final rows = <ContentInfoRow>[
      ?impactRow,
      timeRow,
      locationRow,
      needsRow,
      rsvpRow,
    ];
    for (var i = 0; i < rows.length; i++) {
      rows[i] = _withDivider(rows[i], i < rows.length - 1);
    }

    final showEditPencil = state.isOwner && !isTerminal;

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: [
        ContentTopRows(
          title: exp.name,
          titleTrailing: showEditPencil
              ? IconAction(
                  icon: Icons.edit_outlined,
                  semanticsLabel: context.l10n.a11yExpOpenManageSheet,
                  iconSize: 20,
                  color: GlassTokens.textSecondary,
                  padding: const EdgeInsets.all(4),
                  constraints:
                      const BoxConstraints.tightFor(width: 36, height: 36),
                  onPressed: () => _showManageSheet(context),
                )
              : null,
          descriptionSlot: ContentExpandableDescription(
            description: exp.description,
            style: Theme.of(context).textTheme.bodyMedium!.copyWith(
                  color: GlassTokens.textSecondary,
                  height: 1.55,
                ),
          ),
          rows: rows,
        ),
        if (sourceUrl != null) ...[
          const SizedBox(height: 6),
          _buildSourceUrlRow(context, sourceUrl),
        ],
        const SizedBox(height: 8),
      ],
    );
  }

  ContentInfoRow _buildLocationRow(
      BuildContext context, Experience exp, bool isCompleted) {
    final locationName = state.locationName ?? '';
    final locationPollActive = exp.locationPollActive && !isCompleted;
    final activeLocationPollId =
        exp.hasCurrentLocationPollId() ? exp.currentLocationPollId : '';
    final activeLocationProposals = locationPollActive
        ? (activeLocationPollId.isEmpty
            ? exp.locationProposals.toList()
            : exp.locationProposals
                .where((p) => p.hasPollId() && p.pollId == activeLocationPollId)
                .toList())
        : const <LocationProposal>[];
    final locationProposalCount = activeLocationProposals.length;
    final hasVotedOnLocation = locationPollActive &&
        state.currentUserId != null &&
        activeLocationProposals
            .any((p) => p.votes.any((v) => v.user.id == state.currentUserId));
    final hasLocation = locationName.isNotEmpty;

    return ContentInfoRow(
      icon: Icons.location_on_outlined,
      value: locationPollActive
          ? context.l10n.locationPollEventRowChoose(locationProposalCount)
          : (hasLocation
              ? locationName
              : context.l10n.experienceEventRowLocationTbd),
      rowSemanticsLabel: context.l10n.contentRowsEditLocation,
      onTap: isCompleted
          ? null
          : () => onShowLocationPickerModal(exp.locationId),
      actionIcon: !locationPollActive && hasLocation ? Icons.directions : null,
      actionSemanticsLabel: !locationPollActive && hasLocation
          ? context.l10n.contentRowsGetDirections
          : null,
      onAction: !locationPollActive && hasLocation
          ? () => openDirections(
                context,
                Location()
                  ..name = locationName
                  ..latitudeDeg = state.locationLatitude ?? 0
                  ..longitudeDeg = state.locationLongitude ?? 0,
                fallbackName: locationName,
              )
          : null,
      trailing: locationPollActive
          ? RowCallToActionPill(
              chosen: hasVotedOnLocation,
              actionLabel: context.l10n.locationPollEventRowChooseCta,
              chosenLabel: context.l10n.locationPollEventRowChosenCta,
              onTap: () => onShowLocationPickerModal(exp.locationId),
            )
          : null,
    );
  }

  ContentInfoRow _buildTimeRow(
      BuildContext context, Experience exp, bool isCompleted) {
    final timeDate = exp.hasTime() ? _formatExperienceDate(exp.time) : null;
    final timeSub =
        exp.hasTime() ? _formatExperienceTimeSubtitle(exp.time) : null;
    final canAddToCalendar =
        exp.hasTime() && (exp.time.hasSpecific() || exp.time.hasRange());

    final pollActive = exp.timePollActive && !isCompleted;
    final activePollId = exp.hasCurrentPollId() ? exp.currentPollId : '';
    final activeProposals = activePollId.isEmpty
        ? exp.timeProposals.toList()
        : exp.timeProposals
            .where((p) => p.hasPollId() && p.pollId == activePollId)
            .toList();
    final hasVoted = pollActive &&
        state.currentUserId != null &&
        PollBanner.hasVotedYes(activeProposals, state.currentUserId!);
    final activeProposalCount = activeProposals.length;

    // The event-day forecast (server-computed for the event's location), shown
    // as a chip on the WHEN row when there's no active time poll.
    final details = state.experienceDetails;
    final forecast =
        (details != null && details.hasForecast()) ? details.forecast : null;

    return ContentInfoRow(
      icon: Icons.access_time_rounded,
      value: pollActive
          ? context.l10n.timePollEventRowChoose(activeProposalCount)
          : timeDate != null
              ? (timeSub != null ? '$timeDate · $timeSub' : timeDate)
              : context.l10n.experienceEventRowTimeTbd,
      rowSemanticsLabel: context.l10n.contentRowsEditTime,
      onTap: isCompleted
          ? null
          : state.isOwner
              ? onShowTimeModal
              : () => onShowTimeModalNonOwner(exp.timePollActive),
      actionIcon: !pollActive && canAddToCalendar
          ? Icons.calendar_today_outlined
          : null,
      actionSemanticsLabel: !pollActive && canAddToCalendar
          ? context.l10n.contentRowsAddToCalendar
          : null,
      onAction: !pollActive && canAddToCalendar ? onAddToCalendar : null,
      trailing: pollActive
          ? RowCallToActionPill(
              chosen: hasVoted,
              actionLabel: context.l10n.timePollEventRowChooseCta,
              chosenLabel: context.l10n.timePollEventRowChosenCta,
              onTap: state.isOwner
                  ? onShowTimeModal
                  : () => onShowTimeModalNonOwner(true),
            )
          : forecast != null
              ? WeatherChip(forecast: forecast)
              : null,
    );
  }

  ContentInfoRow _buildNeedsRow(
    BuildContext context,
    WidgetRef ref, {
    required bool isTerminal,
  }) {
    final l10n = context.l10n;
    final needsState = ref.watch(experienceNeedsProvider(experienceId));
    final openNeeds = [
      for (final n in needsState.needs)
        if (n.slotsRemaining > 0) n,
    ];
    final fulfilledNeeds = [
      for (final n in needsState.needs)
        if (n.slotsRemaining == 0) n,
    ];
    final thingsNeeded = openNeeds.length;
    final thingsProvided = needsState.contributions.length;
    final isTbd =
        needsState.needs.isEmpty && needsState.contributions.isEmpty;
    final String value;
    if (isTbd) {
      value = l10n.needsEventRowTbd;
    } else if (openNeeds.isNotEmpty) {
      // Name the first open need; remaining open needs become "and N
      // more needed". Singular / no-others fall through ICU plural.
      value = l10n.needsEventRowNamedOpen(
        openNeeds.first.name,
        openNeeds.length - 1,
      );
    } else if (fulfilledNeeds.isNotEmpty) {
      // Every Need is covered — surface the first fulfilled Need's
      // name + count of the others so the row reads like the
      // open-state copy, just past tense.
      value = l10n.needsEventRowNamedDone(
        fulfilledNeeds.first.name,
        fulfilledNeeds.length - 1,
      );
    } else {
      // Edge: only freestanding contributions, no Needs. Fall back to
      // the legacy "N contributions. All set!" copy.
      value = l10n.needsEventRowNothingNeeded(thingsProvided);
    }

    final exp = state.experienceDetails!.experience;
    final rsvps = state.experienceDetails!.rsvps;
    final myRsvp = rsvps.firstWhere(
      (r) => r.user.id == state.currentUserId,
      orElse: () => RSVP(),
    );
    final isRsvpedYes =
        myRsvp.intention == RSVPIntention.RSVP_INTENTION_YES;
    final isRsvpedMaybe =
        myRsvp.intention == RSVPIntention.RSVP_INTENTION_MAYBE;

    final scope = NeedsScope.experience(
      experienceId: experienceId,
      currentUserId: state.currentUserId,
      isTerminal: isTerminal,
      communityId: state.communityId ?? '',
      isRsvped: isRsvpedYes || isRsvpedMaybe,
      isRsvpedMaybe: isRsvpedMaybe,
      ownerId: exp.owner.id,
      experienceName: exp.name,
    );
    final actions = NeedsActions(scope: scope);

    final myId = state.currentUserId;
    final hasOwnContribution = myId != null &&
        needsState.contributions.any((c) => c.contributor.id == myId);
    final showHelpPill =
        !isTerminal && (thingsNeeded > 0 || hasOwnContribution);

    // Host on a not-yet-broken-down experience: route to the compose
    // sheet (chips, paste-list, inline-add, pre-claim) — same modal
    // the Request side opens for TBD + owner. Non-hosts and any
    // already-broken-down experience keep the existing dispatcher.
    final useComposeSheet = isTbd && state.isOwner && !isTerminal;
    return ContentInfoRow(
      icon: Icons.checklist_rtl_rounded,
      value: value,
      rowSemanticsLabel: l10n.contentRowsEditNeeds,
      onTap: () => useComposeSheet
          ? ExperienceComposeSheet.show(
              context,
              experienceId: experienceId,
              communityId: state.communityId ?? '',
              needsRsvp: !(isRsvpedYes || isRsvpedMaybe),
            )
          : actions.openPlanTabDispatcher(context, ref),
      trailing: showHelpPill
          ? RowCallToActionPill(
              chosen: hasOwnContribution,
              actionLabel: l10n.needsEventRowHelpCta,
              chosenLabel: l10n.needsEventRowHelpedCta,
              onTap: () => actions.openPlanTabDispatcher(context, ref),
            )
          : null,
    );
  }

  /// New RSVP row — last row in the stack. Renders an attendee
  /// stack + "N going" through the row's [valueBuilder] slot, with the
  /// existing [ExperienceRsvpButton] in the trailing slot for active
  /// events. Terminal events drop the RSVP button.
  ContentInfoRow _buildRsvpRow(
    BuildContext context, {
    required bool isTerminal,
  }) {
    // Authoritative cross-community YES count is on the experience itself
    // (server dedups across communities). The `rsvps` list is scoped to the
    // current community and may omit the owner's auto-RSVP when it was
    // registered in a different community — so it's the right shape for the
    // avatar stack but the wrong source for "N going".
    final exp = state.experienceDetails!.experience;
    final rsvps = state.experienceDetails!.rsvps;
    final yesRsvps = rsvps
        .where((r) => r.intention == RSVPIntention.RSVP_INTENTION_YES)
        .toList();
    final currentUserRsvp = rsvps.firstWhere(
      (r) => r.user.id == state.currentUserId,
      orElse: () => RSVP(),
    );
    final rsvpIntention =
        currentUserRsvp.hasIntention() ? currentUserRsvp.intention : null;
    final goingCount = exp.rsvpYesCount;

    return ContentInfoRow(
      icon: Icons.event_available_outlined,
      value: context.l10n.experienceAttendeeCount(goingCount),
      valueBuilder: (ctx) => _AttendeeStackValue(
        yesRsvps: yesRsvps,
        goingCount: goingCount,
      ),
      rowSemanticsLabel: context.l10n.a11yExpViewAttendees,
      onTap: onShowAttendeeSheet,
      trailing: isTerminal
          ? null
          : ExperienceRsvpButton(
              intention: rsvpIntention,
              onTap: onShowAttendeeSheet,
            ),
    );
  }

  ContentInfoRow _withDivider(ContentInfoRow row, bool showDivider) {
    return ContentInfoRow(
      icon: row.icon,
      leading: row.leading,
      value: row.value,
      valueBuilder: row.valueBuilder,
      rowSemanticsLabel: row.rowSemanticsLabel,
      trailingText: row.trailingText,
      onTap: row.onTap,
      actionIcon: row.actionIcon,
      actionSemanticsLabel: row.actionSemanticsLabel,
      onAction: row.onAction,
      trailing: row.trailing,
      showDivider: showDivider,
    );
  }

  Widget _buildSourceUrlRow(BuildContext context, String sourceUrl) {
    return Tappable(
      semanticsLabel: context.l10n.a11yExpOpenSourceUrl,
      isLink: true,
      onTap: () {
        final uri = Uri.tryParse(sourceUrl);
        if (uri != null) {
          launchUrl(uri, mode: LaunchMode.externalApplication);
        }
      },
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          const Icon(
            Icons.link_rounded,
            size: 14,
            color: GlassTokens.textMuted,
          ),
          const SizedBox(width: 4),
          Flexible(
            child: Text(
              sourceUrl,
              style: const TextStyle(
                fontSize: 12,
                color: GlassTokens.textMuted,
                decoration: TextDecoration.underline,
                decorationColor: GlassTokens.textFaint,
                decorationStyle: TextDecorationStyle.solid,
              ),
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
            ),
          ),
        ],
      ),
    );
  }

  Future<void> _showManageSheet(BuildContext context) async {
    final exp = state.experienceDetails?.experience;
    if (exp == null) return;
    final communityId = state.communityId ?? '';

    final result = await showAccessibleModal<ExperienceManageAction>(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (_) => const ExperienceManageMenuSheet(),
    );
    if (result == null) return;
    if (!context.mounted) return;

    switch (result) {
      case ExperienceManageAction.editDetails:
        onToggleEditMode();
        break;
      case ExperienceManageAction.markCompleted:
        await MarkCompletedModal.show(
          context,
          exp.id,
          communityId,
          sharedCommunityIds: exp.sharedCommunityIds.toSet(),
        );
        break;
      case ExperienceManageAction.closeEvent:
        await CloseItemModal.show(
          context,
          contentType: CloseItemContentType.event,
          onUnshare: onUnshareExperience,
          onCancel: onCancelExperience,
          onDelete: onDeleteExperience,
        );
        break;
    }
  }

  Widget _buildEditContent(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: [
        ContentViewBuilders.buildEditableTitle(
          value: editingTitle,
          onChanged: onTitleChanged,
          enabled: true,
          label: 'Event Name',
          hintText: 'Event name',
        ),
        const SizedBox(height: 8),
        ContentViewBuilders.buildEditableDescription(
          value: editingDescription,
          onChanged: onDescriptionChanged,
          enabled: true,
          label: 'Description',
          hintText: 'Experience description',
        ),
        const SizedBox(height: 8),
        ContentViewBuilders.buildEditableTitle(
          value: editingSourceUrl,
          onChanged: onSourceUrlChanged,
          enabled: true,
          label: 'Event Link (optional)',
          hintText: 'https://eventbrite.com/...',
        ),
      ],
    );
  }

  String? _formatExperienceDate(ExperienceTime time) {
    if (time.hasInformalDescription() && time.informalDescription.isNotEmpty) {
      return time.informalDescription;
    }
    if (time.hasSpecific()) {
      final dt = DateTime.fromMillisecondsSinceEpoch(
        time.specific.unixTimestampSec.toInt() * 1000,
      );
      return DateFormat('EEE, MMM d').format(dt);
    }
    if (time.hasRange() && time.range.description.isNotEmpty) {
      return time.range.description;
    }
    return null;
  }

  String? _formatExperienceTimeSubtitle(ExperienceTime time) {
    if (!time.hasSpecific()) return null;
    final dt = DateTime.fromMillisecondsSinceEpoch(
      time.specific.unixTimestampSec.toInt() * 1000,
    );
    final timeStr = DateFormat('h:mm a').format(dt);
    final mins = time.specific.durationMinutes;
    if (mins <= 0) return timeStr;
    final hours = mins / 60.0;
    final durationStr = hours == hours.truncateToDouble()
        ? '${hours.toInt()} ${hours == 1 ? 'hr' : 'hrs'}'
        : '${hours.toStringAsFixed(1)} hrs';
    return '$timeStr · $durationStr';
  }
}

/// Renders an overlapping avatar stack (up to 4 faces) followed by a
/// "{N} going" label. Used inside the [ContentInfoRow] valueBuilder slot
/// on the RSVP row.
class _AttendeeStackValue extends StatelessWidget {
  final List<RSVP> yesRsvps;
  final int goingCount;

  const _AttendeeStackValue({
    required this.yesRsvps,
    required this.goingCount,
  });

  static const double _avatarSize = 24;
  static const double _overlap = 8;

  @override
  Widget build(BuildContext context) {
    final shown = yesRsvps.take(4).toList();
    final stackWidth = shown.isEmpty
        ? 0.0
        : _avatarSize + (shown.length - 1) * (_avatarSize - _overlap);

    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        if (shown.isNotEmpty) ...[
          SizedBox(
            width: stackWidth,
            height: _avatarSize,
            child: Stack(
              children: [
                for (var i = 0; i < shown.length; i++)
                  Positioned(
                    left: i * (_avatarSize - _overlap),
                    child: Container(
                      decoration: BoxDecoration(
                        shape: BoxShape.circle,
                        border: Border.all(
                          color: Colors.black.withValues(alpha: 0.5),
                          width: 2,
                        ),
                      ),
                      child: UserAvatar(
                        user: shown[i].user,
                        radius: _avatarSize / 2,
                      ),
                    ),
                  ),
              ],
            ),
          ),
          const SizedBox(width: 8),
        ],
        Flexible(
          child: Text(
            context.l10n.experienceAttendeeCount(goingCount),
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
            style: const TextStyle(
              color: GlassTokens.textPrimary,
              fontSize: 14,
              fontWeight: FontWeight.w500,
            ),
          ),
        ),
      ],
    );
  }
}

