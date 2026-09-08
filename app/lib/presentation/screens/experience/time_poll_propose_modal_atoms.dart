// Part of [time_poll_propose_modal.dart] — holds the small private
// widget "atoms" (eyebrow, title, row cards, primary button, etc.) that
// make up the propose modal's body. Split out so the main file stays
// under the 1000-line lint gate; nothing here is exported. Mirrors
// [location_poll_propose_modal_atoms.dart].
part of 'time_poll_propose_modal.dart';

/* ── Atoms matched to the design ────────────────────────────────────────── */

class _Eyebrow extends StatelessWidget {
  final String text;
  const _Eyebrow({required this.text});

  @override
  Widget build(BuildContext context) {
    return Text(
      text.toUpperCase(),
      style: TextStyle(
        color: AppColors.lightAccent,
        fontSize: 11,
        fontWeight: FontWeight.w700,
        letterSpacing: 1.6,
      ),
    );
  }
}

class _Title extends StatelessWidget {
  final String text;
  const _Title({required this.text});

  @override
  Widget build(BuildContext context) {
    return Semantics(
      header: true,
      child: Text(
        text,
        style: TextStyle(
          fontFamily: AppTheme.headingFont,
          color: AppColors.modalTextPrimary,
          fontSize: 27,
          fontWeight: FontWeight.w600,
          height: 1.1,
          letterSpacing: -0.4,
        ),
      ),
    );
  }
}

class _Subtitle extends StatelessWidget {
  final String text;
  const _Subtitle({required this.text});

  @override
  Widget build(BuildContext context) {
    return Text(
      text,
      style: TextStyle(
        color: AppColors.modalTextSecondary,
        fontSize: 14,
        height: 1.4,
      ),
    );
  }
}

class _NumberCircle extends StatelessWidget {
  final int n;
  const _NumberCircle({required this.n});

  @override
  Widget build(BuildContext context) {
    return Container(
      width: 30,
      height: 30,
      decoration: BoxDecoration(
        color: AppColors.lightAccent,
        shape: BoxShape.circle,
      ),
      alignment: Alignment.center,
      child: Text(
        '$n',
        style: const TextStyle(
          color: Color(0xFF11201A),
          fontWeight: FontWeight.w700,
          fontSize: 13,
        ),
      ),
    );
  }
}

class _StagedTimeRow extends StatelessWidget {
  final int n;
  final DateTime dateTime;
  final VoidCallback onRemove;
  const _StagedTimeRow({
    required this.n,
    required this.dateTime,
    required this.onRemove,
  });

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.fromLTRB(12, 10, 4, 10),
      decoration: BoxDecoration(
        color: AppColors.surface(context).withValues(alpha: 0.06),
        border: Border.all(color: AppColors.border(context)),
        borderRadius: BorderRadius.circular(16),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.center,
        children: [
          _NumberCircle(n: n),
          const SizedBox(width: 11),
          Expanded(child: _TimeDetails(dateTime: dateTime)),
          IconAction(
            icon: Icons.close,
            tooltip: context.l10n.commonRemove,
            semanticsLabel: context.l10n.commonRemove,
            onPressed: onRemove,
          ),
        ],
      ),
    );
  }
}

class _ExistingProposalRow extends StatelessWidget {
  final int n;
  final TimeProposal proposal;
  final VoidCallback onRemove;
  const _ExistingProposalRow({
    required this.n,
    required this.proposal,
    required this.onRemove,
  });

  @override
  Widget build(BuildContext context) {
    final DateTime? dt = proposal.time.hasSpecific()
        ? DateTime.fromMillisecondsSinceEpoch(
            proposal.time.specific.unixTimestampSec.toInt() * 1000)
        : null;
    return Container(
      padding: const EdgeInsets.fromLTRB(12, 10, 4, 10),
      decoration: BoxDecoration(
        color: AppColors.surface(context).withValues(alpha: 0.06),
        border: Border.all(color: AppColors.border(context)),
        borderRadius: BorderRadius.circular(16),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.center,
        children: [
          _NumberCircle(n: n),
          const SizedBox(width: 11),
          Expanded(
            child: dt == null
                ? Text(
                    'TBD',
                    style: TextStyle(color: AppColors.modalTextPrimary),
                  )
                : _TimeDetails(dateTime: dt),
          ),
          IconAction(
            icon: Icons.close,
            tooltip: context.l10n.commonRemove,
            semanticsLabel: context.l10n.commonRemove,
            onPressed: onRemove,
          ),
        ],
      ),
    );
  }
}

class _TimeDetails extends StatelessWidget {
  final DateTime dateTime;
  const _TimeDetails({required this.dateTime});

  @override
  Widget build(BuildContext context) {
    final dateLabel = DateFormat('EEEE, MMM d').format(dateTime);
    final timeLabel = DateFormat('h:mm a').format(dateTime);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: [
        Text(
          dateLabel,
          style: TextStyle(
            color: AppColors.modalTextPrimary,
            fontWeight: FontWeight.w700,
            fontSize: 15,
            height: 1.15,
          ),
          overflow: TextOverflow.ellipsis,
        ),
        const SizedBox(height: 2),
        Text(
          timeLabel,
          style: TextStyle(
            color: AppColors.modalTextSecondary,
            fontSize: 12,
          ),
          overflow: TextOverflow.ellipsis,
        ),
      ],
    );
  }
}

/// Compact dashed "+ Add a time" row, matching the prototype's `.addrow`:
/// a dashed sage-neutral border, a sage "+" glyph, and a centered label.
class _AddTimeCard extends StatelessWidget {
  final String label;
  final VoidCallback? onTap;
  const _AddTimeCard({required this.label, required this.onTap});

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: label,
      onTap: onTap,
      child: CustomPaint(
        painter: _DashedRRectPainter(
          color: AppColors.modalTextPrimary.withValues(alpha: 0.22),
          radius: 14,
        ),
        child: Container(
          padding: const EdgeInsets.symmetric(vertical: 14, horizontal: 14),
          alignment: Alignment.center,
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(Icons.add, size: 18, color: AppColors.lightAccent),
              const SizedBox(width: 8),
              Text(
                label,
                style: TextStyle(
                  color: AppColors.modalTextPrimary,
                  fontSize: 13.5,
                  fontWeight: FontWeight.w600,
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

/// Paints a dashed rounded-rectangle border (Flutter's [Border] has no dashed
/// style). Shared by the add-row and any other dashed affordance here.
class _DashedRRectPainter extends CustomPainter {
  _DashedRRectPainter({required this.color, required this.radius});

  final Color color;
  final double radius;

  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = color
      ..style = PaintingStyle.stroke
      ..strokeWidth = 1.2;
    final path = Path()
      ..addRRect(RRect.fromRectAndRadius(
        Offset.zero & size,
        Radius.circular(radius),
      ));
    const dash = 6.0;
    const gap = 4.0;
    for (final metric in path.computeMetrics()) {
      var distance = 0.0;
      while (distance < metric.length) {
        canvas.drawPath(
          metric.extractPath(
              distance, math.min(distance + dash, metric.length)),
          paint,
        );
        distance += dash + gap;
      }
    }
  }

  @override
  bool shouldRepaint(covariant _DashedRRectPainter old) =>
      old.color != color || old.radius != radius;
}

class _PrimaryButton extends StatelessWidget {
  final String label;
  final bool enabled;
  final VoidCallback? onTap;
  const _PrimaryButton({
    required this.label,
    required this.enabled,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    final accent = AppColors.lightAccent;
    return Tappable(
      semanticsLabel: label,
      onTap: onTap,
      child: Container(
        height: 52,
        decoration: BoxDecoration(
          color: enabled ? accent : accent.withValues(alpha: 0.30),
          borderRadius: BorderRadius.circular(16),
          boxShadow: enabled
              ? [
                  BoxShadow(
                    color: accent.withValues(alpha: 0.30),
                    offset: const Offset(0, 8),
                    blurRadius: 20,
                  ),
                ]
              : null,
        ),
        alignment: Alignment.center,
        child: Text(
          label,
          style: TextStyle(
            color: enabled
                ? AppColors.cardBackground(context)
                : AppColors.modalTextMuted,
            fontWeight: FontWeight.w700,
            fontSize: 16,
            letterSpacing: 0.1,
          ),
        ),
      ),
    );
  }
}

/// Amber-tinted "Replies due …" strip with a single inline Change button.
/// Mirrors location's `_DeadlineStrip` exactly; clearing the deadline is
/// reached from inside the picker modal's Remove action.
class _DeadlineStrip extends StatelessWidget {
  final int? deadlineUnixSec;
  final VoidCallback onChange;
  const _DeadlineStrip({
    required this.deadlineUnixSec,
    required this.onChange,
  });

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    const amber = AppColors.statusWarningOnDark;
    final whenText = deadlineUnixSec == null
        ? null
        : DateFormat('EEE, MMM d · h:mm a').format(
            DateTime.fromMillisecondsSinceEpoch(deadlineUnixSec! * 1000),
          );
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
      decoration: BoxDecoration(
        color: amber.withValues(alpha: 0.08),
        border: Border.all(color: amber.withValues(alpha: 0.30)),
        borderRadius: BorderRadius.circular(14),
      ),
      child: Row(
        children: [
          Icon(Icons.schedule, size: 14, color: amber),
          const SizedBox(width: 8),
          Expanded(
            child: Text(
              whenText == null
                  ? l10n.locationPollDeadlineSet
                  : l10n.locationPollDeadlineLabel(whenText),
              style: TextStyle(
                color: AppColors.modalTextPrimary,
                fontSize: 13,
                height: 1.35,
              ),
            ),
          ),
          const SizedBox(width: 8),
          Tappable(
            semanticsLabel: whenText == null
                ? l10n.locationPollDeadlineSet
                : l10n.locationPollDeadlineChange,
            onTap: onChange,
            child: Padding(
              padding: const EdgeInsets.symmetric(horizontal: 6),
              child: Text(
                whenText == null
                    ? l10n.locationPollDeadlineSet
                    : l10n.locationPollDeadlineChange,
                style: TextStyle(
                  color: AppColors.lightAccent,
                  fontSize: 13,
                  fontWeight: FontWeight.w600,
                  decoration: TextDecoration.underline,
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }
}
