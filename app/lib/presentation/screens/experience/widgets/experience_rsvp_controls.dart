import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/data/gen/ripls/api/experience_service.pb.dart'
    show RSVPIntention;
import 'package:ripls/presentation/viewmodels/experience_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/content/content_lifecycle_phase.dart';
import 'package:ripls/presentation/widgets/content/content_rsvp_segmented.dart';

/// ExperienceRsvpControls is the RSVP block at the top of the expanded
/// pitching-in panel (docs/issues/2280-pitching-in-expand.md). It mirrors the
/// v7 reference: once the viewer has replied it shows a compact, tappable
/// status line ("✓ You're going · Tap to change ›"); tapping it — or not having
/// replied yet — reveals the Going / Maybe / Can't go segmented control. The
/// host gets the same control (hosts don't always attend). Hidden once the
/// event is terminal. Every change routes to `ExperienceNotifier.updateRSVP`.
class ExperienceRsvpControls extends ConsumerStatefulWidget {
  final String experienceId;
  final Color accentColor;

  const ExperienceRsvpControls({
    super.key,
    required this.experienceId,
    required this.accentColor,
  });

  @override
  ConsumerState<ExperienceRsvpControls> createState() =>
      _ExperienceRsvpControlsState();
}

class _ExperienceRsvpControlsState
    extends ConsumerState<ExperienceRsvpControls> {
  /// True once a replied viewer taps "Tap to change" to reveal the buttons.
  bool _changing = false;

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    final state = ref.watch(experienceProvider(widget.experienceId));
    if (state.experienceDetails == null) return const SizedBox.shrink();

    final phase = state.lifecyclePhase;
    if (phase == ContentLifecyclePhase.wrapped ||
        phase == ContentLifecyclePhase.cancelled) {
      return const SizedBox.shrink();
    }

    final isOwner = state.isOwner;
    final intention = state.currentUserIntention;
    final replied =
        intention != null &&
        intention != RSVPIntention.RSVP_INTENTION_UNSPECIFIED;

    // Replied and not actively changing → the compact status line.
    if (replied && !_changing) {
      return _statusLine(context, intention);
    }

    // Otherwise the buttons. Prompt only when they haven't replied yet; a
    // viewer who tapped "change" already knows the question.
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      mainAxisSize: MainAxisSize.min,
      children: [
        if (!replied) ...[
          _sectionLabel(
            isOwner
                ? l10n.rosterRsvpQuestionHosting
                : l10n.rosterRsvpQuestionInvited,
          ),
          const SizedBox(height: 8),
        ],
        _segmented(context, intention),
      ],
    );
  }

  /// The compact "✓ You're going · Tap to change ›" line shown once the viewer
  /// has an RSVP. Tapping it reveals the segmented control.
  Widget _statusLine(BuildContext context, RSVPIntention intention) {
    final l10n = context.l10n;
    final (IconData icon, bool filled, String label) = switch (intention) {
      RSVPIntention.RSVP_INTENTION_YES => (
        Icons.check_rounded,
        true,
        l10n.rosterRsvpStatusGoing,
      ),
      RSVPIntention.RSVP_INTENTION_NO => (
        Icons.close_rounded,
        false,
        l10n.rosterRsvpStatusNo,
      ),
      _ => (Icons.question_mark_rounded, false, l10n.rosterRsvpStatusMaybe),
    };

    return Tappable(
      semanticsLabel: l10n.a11yRosterRsvpChange(label),
      onTap: () => setState(() => _changing = true),
      child: Container(
        constraints: const BoxConstraints(minHeight: 52),
        padding: const EdgeInsets.symmetric(vertical: 8),
        child: Row(
          children: [
            _statusIcon(icon, filled),
            const SizedBox(width: 11),
            Text(
              label,
              style: const TextStyle(
                color: AppColors.onContentImage,
                fontSize: 15,
                fontWeight: FontWeight.w700,
              ),
            ),
            const Spacer(),
            Text(
              '${l10n.rosterRsvpTapToChange} ›',
              style: const TextStyle(
                color: AppColors.darkTextTertiary,
                fontSize: 12,
                fontWeight: FontWeight.w500,
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _statusIcon(IconData icon, bool filled) {
    return Container(
      width: 24,
      height: 24,
      alignment: Alignment.center,
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        color: filled ? widget.accentColor : Colors.transparent,
        border: filled
            ? null
            : Border.all(color: AppColors.darkTextTertiary, width: 1.5),
      ),
      child: Icon(
        icon,
        size: 13,
        color: filled ? AppColors.darkBackground : AppColors.darkTextTertiary,
      ),
    );
  }

  Widget _segmented(BuildContext context, RSVPIntention? intention) {
    final l10n = context.l10n;
    void set(RSVPIntention next) {
      ref
          .read(experienceProvider(widget.experienceId).notifier)
          .updateRSVP(next);
      // Collapse back to the status line; it re-reads the new intention once
      // the change settles.
      if (_changing) setState(() => _changing = false);
    }

    return ContentRsvpSegmented(
      accentColor: widget.accentColor,
      segments: [
        ContentSegment(
          label: l10n.experienceGoing,
          semanticsLabel: l10n.a11yExpRsvpGoing,
          selected: intention == RSVPIntention.RSVP_INTENTION_YES,
          onTap: () => set(RSVPIntention.RSVP_INTENTION_YES),
        ),
        ContentSegment(
          label: l10n.experienceRsvpMaybe,
          semanticsLabel: l10n.a11yExpRsvpMaybe,
          selected: intention == RSVPIntention.RSVP_INTENTION_MAYBE,
          onTap: () => set(RSVPIntention.RSVP_INTENTION_MAYBE),
        ),
        ContentSegment(
          label: l10n.experienceRsvpCantGo,
          semanticsLabel: l10n.a11yExpRsvpCantGo,
          selected: intention == RSVPIntention.RSVP_INTENTION_NO,
          onTap: () => set(RSVPIntention.RSVP_INTENTION_NO),
        ),
      ],
    );
  }

  Widget _sectionLabel(String text) {
    return Padding(
      padding: const EdgeInsets.only(top: 8, bottom: 2),
      child: Text(
        text.toUpperCase(),
        style: const TextStyle(
          color: AppColors.darkTextTertiary,
          fontSize: 10,
          fontWeight: FontWeight.w700,
          letterSpacing: 1.2,
        ),
      ),
    );
  }
}
