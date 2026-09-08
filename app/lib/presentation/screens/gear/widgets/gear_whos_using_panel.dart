import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/core/utils/date_time_formatter.dart';
import 'package:ripls/data/gen/ripls/api/gear.pb.dart' show Availability;
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart'
    show TransferRequest;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/viewmodels/gear_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';
import 'package:ripls/presentation/widgets/chat/action_dropdown_menu.dart'
    show ActionDropdownItem, ActionDropdownMenu;
import 'package:ripls/presentation/widgets/content/content_morph_panel.dart';
import 'package:ripls/presentation/widgets/user_avatar.dart';

/// GearWhosUsingPanel is the full-screen "story" surface the gear read shell's
/// WHO'S-USING card morph-expands into (matching the gear-screen-redesign-v2
/// mock's expand overlay). It shows the gear's usage metrics (times shared /
/// friends / value saved), who has it now + the queue, the owner/borrower
/// per-borrower workflow actions, and the list of past borrowers.
///
/// Per-borrower actions are rendered inline by reusing the existing
/// `GearMenuItems` checklist via [ActionDropdownMenu] — the same items the
/// sticky action button uses — so there is a single source of truth for the
/// transfer workflow. [actionItemsBuilder] is passed (not a pre-built list) so
/// the checklist's completed states stay live as the panel rebuilds after an
/// action mutates gear state.
class GearWhosUsingPanel extends ConsumerWidget {
  final String gearId;
  final Color accentColor;
  final List<ActionDropdownItem> Function(GearState) actionItemsBuilder;

  const GearWhosUsingPanel({
    super.key,
    required this.gearId,
    required this.accentColor,
    required this.actionItemsBuilder,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(gearProvider(gearId));
    final gear = state.gearDetails;
    if (gear == null) {
      return const ContentMorphPanel(child: SizedBox.shrink());
    }
    final isGiveaway =
        gear.availability == Availability.AVAILABILITY_FOR_GIVEAWAY;

    return ContentMorphPanel(
      child: SafeArea(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            _header(context, isGiveaway: isGiveaway),
            Expanded(
              child: ListView(
                padding: const EdgeInsets.fromLTRB(20, 0, 20, 28),
                children: _body(context, state, isGiveaway: isGiveaway),
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _header(BuildContext context, {required bool isGiveaway}) {
    final l10n = context.l10n;
    return Padding(
      padding: const EdgeInsets.fromLTRB(20, 8, 12, 8),
      child: Row(
        children: [
          Expanded(
            child: Semantics(
              header: true,
              child: Text(
                isGiveaway
                    ? l10n.gearSectionUpForGrabs
                    : l10n.gearWhosUsingPanelTitle,
                style: const TextStyle(
                  fontFamily: AppTheme.headingFont,
                  fontSize: 24,
                  fontWeight: FontWeight.w600,
                  color: AppColors.onContentImage,
                ),
              ),
            ),
          ),
          IconAction(
            icon: Icons.close_rounded,
            semanticsLabel: l10n.a11yClose,
            color: AppColors.onContentImage,
            onPressed: () => Navigator.of(context).pop(),
          ),
        ],
      ),
    );
  }

  List<Widget> _body(
    BuildContext context,
    GearState state, {
    required bool isGiveaway,
  }) {
    final l10n = context.l10n;
    return [
      // Loan gear hides the share-stats story (matches the read-shell trail,
      // which is also loan-only); a giveaway has no return loop to measure.
      if (!isGiveaway) ...[
        _storyTiles(context, state),
        ?_calcNote(context, state),
        const SizedBox(height: 18),
      ],
      _sectionHeader(l10n.gearWhoHasItNow),
      ..._holderRows(context, state, isGiveaway: isGiveaway),
      ?_actions(context, state),
      if (!isGiveaway) ...[
        const SizedBox(height: 18),
        ?_pastBorrowers(context, state),
      ],
    ];
  }

  // ── Story metric tiles ─────────────────────────────────────────────────────

  Widget _storyTiles(BuildContext context, GearState state) {
    final l10n = context.l10n;
    final stats = state.gearStats;
    final timesShared =
        stats?.timesLoaned ?? state.gearDetails?.timesLoaned ?? 0;
    final friends = stats?.peopleHelped ?? 0;
    final saved = stats?.valueSharedUsd ?? 0;
    return Padding(
      padding: const EdgeInsets.only(top: 4),
      child: Row(
        children: [
          Expanded(
            child: _StoryTile(
              value: '$timesShared',
              label: l10n.gearStoryTimesShared,
            ),
          ),
          const SizedBox(width: 10),
          Expanded(
            child: _StoryTile(
              value: '$friends',
              label: l10n.gearStoryFriends,
            ),
          ),
          const SizedBox(width: 10),
          Expanded(
            child: _StoryTile(
              value: saved > 0 ? '~\$${saved.round()}' : '–',
              label: l10n.gearStorySavedTogether,
            ),
          ),
        ],
      ),
    );
  }

  Widget? _calcNote(BuildContext context, GearState state) {
    final gear = state.gearDetails;
    if (gear == null || !gear.hasValueEstimate()) return null;
    final estimate = gear.valueEstimate;
    if (!estimate.hasEstimatedValueUsd()) return null;
    final stats = state.gearStats;
    final count = stats?.timesLoaned ?? gear.timesLoaned;
    if (count <= 0) return null;
    final value = '\$${estimate.estimatedValueUsd.round()}';
    return Padding(
      padding: const EdgeInsets.only(top: 10),
      child: Text(
        context.l10n.gearStoryCalcNote(value, count),
        textAlign: TextAlign.center,
        style: const TextStyle(
          color: AppColors.darkTextTertiary,
          fontSize: 11.5,
          height: 1.4,
        ),
      ),
    );
  }

  // ── Who has it now + queue ─────────────────────────────────────────────────

  List<Widget> _holderRows(
    BuildContext context,
    GearState state, {
    required bool isGiveaway,
  }) {
    final l10n = context.l10n;
    if (isGiveaway) {
      final ctx = state.transferContext;
      final selected = ctx?.hasSelectedRecipient() ?? false
          ? ctx!.selectedRecipient.borrower
          : null;
      final interested = state.gearPeople?.interestedParties ?? const <User>[];
      final others = [
        for (final u in interested)
          if (selected == null || u.id != selected.id) u,
      ];
      if (selected == null && others.isEmpty) {
        return [_emptyRow(l10n.gearWhosUsingEmptyGiveaway)];
      }
      return [
        if (selected != null)
          _personRow(context, selected,
              badge: l10n.gearWhosUsingSelected),
        for (final u in others) _personRow(context, u),
      ];
    }

    final gear = state.gearDetails!;
    final holder = gear.hasActiveLoan()
        ? gear.activeLoan.borrower
        : (state.gearPeople?.hasCurrentBorrower() ?? false
            ? state.gearPeople!.currentBorrower
            : null);
    final queue = [
      for (final r in state.transferContext?.pendingRequests ?? <TransferRequest>[])
        if (r.hasBorrower()) r,
    ];
    if (holder == null && queue.isEmpty) {
      return [_emptyRow(l10n.gearWhosUsingEmptyAvailable)];
    }
    return [
      if (holder != null)
        _personRow(context, holder, badge: l10n.gearWhosUsingHasItNow),
      for (final r in queue)
        _personRow(context, r.borrower, dateBadge: _bookingWindow(r)),
    ];
  }

  /// Compact booking window ("Jul 18", "Jul 18–20") for a dated request, so
  /// multiple bookings by the same borrower read as separate reservations
  /// rather than duplicated rows (#2638). Null for undated interest requests.
  String? _bookingWindow(TransferRequest r) {
    if (!r.hasEstimatedPickupUnixSec()) return null;
    final start = r.estimatedPickupUnixSec.toInt();
    final end =
        r.hasExpectedReturnUnixSec() ? r.expectedReturnUnixSec.toInt() : start;
    return DateTimeFormatter.formatBookingWindow(start, end);
  }

  // ── Per-borrower workflow actions (reused checklist) ───────────────────────

  Widget? _actions(BuildContext context, GearState state) {
    final items = actionItemsBuilder(state);
    if (items.isEmpty) return null;
    return Padding(
      padding: const EdgeInsets.only(top: 14),
      child: ActionDropdownMenu(items: items),
    );
  }

  // ── Past borrowers ─────────────────────────────────────────────────────────

  Widget? _pastBorrowers(BuildContext context, GearState state) {
    final past = state.gearPeople?.pastBorrowers ?? const <User>[];
    if (past.isEmpty) return null;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        _sectionHeader(context.l10n.gearPastBorrowers),
        Opacity(
          opacity: 0.7,
          child: Column(
            children: [for (final u in past) _personRow(context, u)],
          ),
        ),
      ],
    );
  }

  // ── Shared row helpers ─────────────────────────────────────────────────────

  Widget _sectionHeader(String label) {
    return Padding(
      padding: const EdgeInsets.only(top: 6, bottom: 2),
      child: Text(
        label.toUpperCase(),
        style: const TextStyle(
          color: AppColors.darkTextTertiary,
          fontSize: 11,
          fontWeight: FontWeight.w700,
          letterSpacing: 1.2,
        ),
      ),
    );
  }

  Widget _personRow(BuildContext context, User user,
      {String? badge, String? dateBadge}) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 9),
      child: Row(
        children: [
          UserAvatar(user: user, radius: 16),
          const SizedBox(width: 10),
          Expanded(
            child: Text(
              user.name,
              overflow: TextOverflow.ellipsis,
              style: const TextStyle(
                color: AppColors.onContentImage,
                fontSize: 15,
                fontWeight: FontWeight.w600,
              ),
            ),
          ),
          if (badge != null) _pill(badge, AppColors.statusWarningOnDark),
          if (dateBadge != null) _pill(dateBadge, AppColors.darkTextSecondary),
        ],
      ),
    );
  }

  Widget _pill(String label, Color color) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 7, vertical: 2),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.18),
        borderRadius: BorderRadius.circular(999),
      ),
      child: Text(
        label.toUpperCase(),
        style: TextStyle(
          color: color,
          fontSize: 9,
          fontWeight: FontWeight.w700,
          letterSpacing: 0.6,
        ),
      ),
    );
  }

  Widget _emptyRow(String text) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 9),
      child: Row(
        children: [
          Container(
            width: 8,
            height: 8,
            decoration: const BoxDecoration(
              color: AppColors.experienceSageGreen,
              shape: BoxShape.circle,
            ),
          ),
          const SizedBox(width: 10),
          Expanded(
            child: Text(
              text,
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
}

/// A single story metric tile: a serif value over a small dim caps label.
class _StoryTile extends StatelessWidget {
  final String value;
  final String label;

  const _StoryTile({required this.value, required this.label});

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(vertical: 14, horizontal: 8),
      decoration: BoxDecoration(
        color: AppColors.darkTextPrimary.withValues(alpha: 0.04),
        borderRadius: BorderRadius.circular(14),
        border: Border.all(color: AppColors.darkBorder),
      ),
      child: Column(
        children: [
          Text(
            value,
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
            style: const TextStyle(
              fontFamily: AppTheme.headingFont,
              fontSize: 24,
              fontWeight: FontWeight.w600,
              color: AppColors.experienceSageGreen,
              height: 1,
            ),
          ),
          const SizedBox(height: 6),
          Text(
            label,
            textAlign: TextAlign.center,
            style: const TextStyle(
              color: AppColors.darkTextTertiary,
              fontSize: 11,
            ),
          ),
        ],
      ),
    );
  }
}
