import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/data/gen/ripls/api/experience.pbenum.dart' as proto;
import 'package:ripls/data/gen/ripls/api/experience_service.pb.dart'
    show RSVP, RSVPIntention;
import 'package:ripls/presentation/viewmodels/experience_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/toggle.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass.dart';
import 'package:ripls/presentation/widgets/user_avatar.dart';

/// Returns true when [state] means the experience no longer accepts RSVPs.
///
/// Mirrors the server's RSVP allow-list (ACTIVE, JOINED, IN_PROCESS): only
/// terminal states (COMPLETED, CANCELLED) lock the toggles. IN_PROCESS is
/// explicitly **not** read-only — attendees who arrive after start time can
/// still confirm they're going. See `server/services/experience/rsvp.go`.
bool isExperienceRsvpReadOnly(proto.ExperienceState state) {
  return state == proto.ExperienceState.EXPERIENCE_STATE_COMPLETED ||
      state == proto.ExperienceState.EXPERIENCE_STATE_CANCELLED;
}

/// Attendee list bottom sheet with inline RSVP toggle buttons.
///
/// Watches [experienceProvider] so the attendee list, counts, and current user's
/// RSVP intention rebuild live when RSVPs change. RSVP button taps call the
/// notifier directly — no callback plumbing needed.
///
/// Pass [isReadOnly] to hide the RSVP toggle section (e.g. for completed or
/// cancelled experiences where RSVPs are no longer accepted).
class AttendeeListSheet extends ConsumerWidget {
  const AttendeeListSheet._({required this.experienceId});

  final String experienceId;

  /// Shows the attendee list as a glass bottom sheet.
  ///
  /// When [isReadOnly] is true the RSVP toggle buttons are hidden and the sheet
  /// only displays the attendee list.
  static Future<void> show(
    BuildContext context,
    String experienceId, {
    bool isReadOnly = false,
  }) {
    return showAccessibleModal(context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (context) => _AttendeeListBody(
        experienceId: experienceId,
        isReadOnly: isReadOnly,
      ),
    );
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return const SizedBox.shrink();
  }
}

/// Body of the attendee list sheet. Watches [experienceProvider] for live
/// updates and manages local RSVP selection state.
class _AttendeeListBody extends ConsumerStatefulWidget {
  const _AttendeeListBody({
    required this.experienceId,
    this.isReadOnly = false,
  });

  final String experienceId;
  final bool isReadOnly;

  @override
  ConsumerState<_AttendeeListBody> createState() => _AttendeeListBodyState();
}

class _AttendeeListBodyState extends ConsumerState<_AttendeeListBody> {
  RSVPIntention? _selectedIntention;
  bool _hasInitialized = false;

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(experienceProvider(widget.experienceId));
    final rsvps = state.experienceDetails?.rsvps ?? [];
    final ownerId = state.experienceDetails?.experience.owner.id ?? '';

    // Sync local selection with server state on first load
    if (!_hasInitialized && state.currentUserIntention != null) {
      _selectedIntention = state.currentUserIntention;
      _hasInitialized = true;
    }

    final going = rsvps
        .where((r) => r.intention == RSVPIntention.RSVP_INTENTION_YES)
        .toList();
    final maybe = rsvps
        .where((r) => r.intention == RSVPIntention.RSVP_INTENTION_MAYBE)
        .toList();
    final cantGo = rsvps
        .where((r) => r.intention == RSVPIntention.RSVP_INTENTION_NO)
        .toList();

    final totalResponses = rsvps.length;
    final parts = <String>[];
    if (going.isNotEmpty) {
      parts.add(context.l10n.experienceGoingCount(going.length));
    }
    if (maybe.isNotEmpty) {
      parts.add(context.l10n.experienceMaybeCount(maybe.length));
    }
    if (cantGo.isNotEmpty) {
      parts.add(context.l10n.experienceCantCount(cantGo.length));
    }
    final subtitle = parts.join(' · ');

    return GlassSheet(
      child: SingleChildScrollView(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          mainAxisSize: MainAxisSize.min,
          children: [
            // Header
            Text(
              totalResponses == 1
                  ? context.l10n.experienceResponseCount
                  : context.l10n.experienceResponsesCount(totalResponses),
              style: TextStyle(
                fontSize: 22,
                fontWeight: FontWeight.w700,
                color: AppColors.modalTextPrimary,
              ),
            ),
            if (subtitle.isNotEmpty) ...[
              const SizedBox(height: 4),
              Text(
                subtitle,
                style: TextStyle(
                  fontSize: 14,
                  color: AppColors.modalTextSecondary,
                ),
              ),
            ],
            const SizedBox(height: 20),
            if (!widget.isReadOnly) ...[
              _buildToggleSection(context),
              const SizedBox(height: 20),
            ],
            if (going.isNotEmpty)
              _buildSection(context, context.l10n.experienceSectionGoing,
                  going.length, going, ownerId,
                  opacity: 1),
            if (maybe.isNotEmpty)
              _buildSection(context, context.l10n.experienceSectionMaybe,
                  maybe.length, maybe, ownerId,
                  opacity: 1),
            if (cantGo.isNotEmpty)
              _buildSection(context, context.l10n.experienceSectionCantGo,
                  cantGo.length, cantGo, ownerId,
                  opacity: 0.5),
          ],
        ),
      ),
    );
  }

  Widget _buildToggleSection(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: [
        Text(
          context.l10n.experienceCanYouMakeIt,
          style: TextStyle(
            fontSize: 15,
            fontWeight: FontWeight.w600,
            color: AppColors.modalTextSecondary,
          ),
        ),
        const SizedBox(height: 10),
        Row(
          children: [
            _buildToggle(
              context: context,
              label: context.l10n.experienceRsvpYes,
              intention: RSVPIntention.RSVP_INTENTION_YES,
              selectedColor: AppColors.transferSage,
            ),
            const SizedBox(width: 8),
            _buildToggle(
              context: context,
              label: context.l10n.experienceRsvpMaybe,
              intention: RSVPIntention.RSVP_INTENTION_MAYBE,
              selectedColor: AppColors.transferCoral,
            ),
            const SizedBox(width: 8),
            _buildToggle(
              context: context,
              label: context.l10n.experienceRsvpNo,
              intention: RSVPIntention.RSVP_INTENTION_NO,
              selectedColor: AppColors.modalTextMuted,
            ),
          ],
        ),
      ],
    );
  }

  Widget _buildToggle({
    required BuildContext context,
    required String label,
    required RSVPIntention intention,
    required Color selectedColor,
  }) {
    final isSelected = _selectedIntention == intention;

    return Expanded(
      child: Toggle(
        semanticsLabel: label,
        selected: isSelected,
        onTap: () {
          setState(() => _selectedIntention = intention);
          ref
              .read(experienceProvider(widget.experienceId).notifier)
              .updateRSVP(intention);
        },
        child: Container(
          padding: const EdgeInsets.symmetric(vertical: 10),
          decoration: BoxDecoration(
            color: isSelected
                ? selectedColor.withValues(alpha: 0.30)
                : AppColors.modalInsetCardBg,
            borderRadius: BorderRadius.circular(12),
            border: Border.all(
              color: isSelected
                  ? selectedColor.withValues(alpha: 0.60)
                  : AppColors.modalInsetCardBorder,
            ),
          ),
          child: Text(
            label,
            textAlign: TextAlign.center,
            style: TextStyle(
              fontSize: 14,
              fontWeight: FontWeight.w600,
              color: isSelected
                  ? AppColors.modalTextPrimary
                  : AppColors.modalTextSecondary,
            ),
          ),
        ),
      ),
    );
  }

  Widget _buildSection(
    BuildContext context,
    String label,
    int count,
    List<RSVP> group,
    String ownerId, {
    required double opacity,
  }) {
    return Opacity(
      opacity: opacity,
      child: Padding(
        padding: const EdgeInsets.only(bottom: 16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          mainAxisSize: MainAxisSize.min,
          children: [
            Row(
              mainAxisAlignment: MainAxisAlignment.spaceBetween,
              children: [
                Text(
                  label,
                  style: TextStyle(
                    fontSize: 11,
                    fontWeight: FontWeight.w700,
                    letterSpacing: 0.8,
                    color: AppColors.modalTextMuted,
                  ),
                ),
                Text(
                  '$count',
                  style: TextStyle(
                    fontSize: 13,
                    fontWeight: FontWeight.w600,
                    color: AppColors.modalTextMuted,
                  ),
                ),
              ],
            ),
            const SizedBox(height: 8),
            for (final rsvp in group)
              _AttendeeRow(
                rsvp: rsvp,
                isOrganizer: rsvp.user.id == ownerId,
              ),
          ],
        ),
      ),
    );
  }
}

class _AttendeeRow extends ConsumerWidget {
  const _AttendeeRow({
    required this.rsvp,
    required this.isOrganizer,
  });

  final RSVP rsvp;
  final bool isOrganizer;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 6),
      child: Row(
        children: [
          UserAvatar(user: rsvp.user, radius: 18),
          const SizedBox(width: 10),
          Expanded(
            child: Text(
              rsvp.user.name,
              style: TextStyle(
                fontSize: 14,
                fontWeight: FontWeight.w500,
                color: AppColors.modalTextPrimary,
              ),
            ),
          ),
          if (isOrganizer)
            Container(
              padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
              decoration: BoxDecoration(
                color: AppColors.modalInsetCardBg,
                borderRadius: BorderRadius.circular(10),
                border: Border.all(color: AppColors.modalInsetCardBorder),
              ),
              child: Text(
                context.l10n.experienceOrganizerBadge,
                style: TextStyle(
                  fontSize: 11,
                  fontWeight: FontWeight.w600,
                  color: AppColors.modalTextSecondary,
                ),
              ),
            ),
        ],
      ),
    );
  }
}
