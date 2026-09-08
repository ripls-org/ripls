import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// One row in [ExperienceWhenAgenda]: another commitment on the viewed day.
class WhenAgendaRow {
  /// The event/item name.
  final String name;

  /// Its time, plus any conflict suffix ("2:30 PM · overlaps 2–3 PM").
  final String time;

  /// True when this commitment overlaps the event's window — styled as a warn.
  final bool conflict;

  /// Optional semantics label + tap to open the item.
  final String? semanticsLabel;
  final VoidCallback? onTap;

  const WhenAgendaRow({
    required this.name,
    required this.time,
    this.conflict = false,
    this.semanticsLabel,
    this.onTap,
  });
}

/// ExperienceWhenAgenda is the "On Jun 24" list of the viewer's other
/// commitments on the day they're looking at in the event "When" screen — so a
/// host can spot conflicts before locking a time. Empty days show a clear-day
/// note. Conflict rows (a commitment overlapping the event window) read in the
/// warn accent.
class ExperienceWhenAgenda extends StatelessWidget {
  /// Kicker, e.g. "On Jun 24".
  final String dayLabel;
  final List<WhenAgendaRow> rows;

  /// Shown when [rows] is empty.
  final String emptyText;

  const ExperienceWhenAgenda({
    super.key,
    required this.dayLabel,
    required this.rows,
    required this.emptyText,
  });

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          dayLabel.toUpperCase(),
          style: const TextStyle(
            fontSize: 10,
            fontWeight: FontWeight.w800,
            letterSpacing: 0.8,
            color: AppColors.darkTextTertiary,
          ),
        ),
        const SizedBox(height: 6),
        if (rows.isEmpty)
          Padding(
            padding: const EdgeInsets.symmetric(vertical: 4),
            child: Text(
              emptyText,
              style: const TextStyle(
                fontSize: 12.5,
                fontStyle: FontStyle.italic,
                color: AppColors.darkTextSecondary,
              ),
            ),
          )
        else
          for (var i = 0; i < rows.length; i++) _row(rows[i], first: i == 0),
      ],
    );
  }

  Widget _row(WhenAgendaRow r, {required bool first}) {
    final content = Container(
      padding: const EdgeInsets.symmetric(vertical: 8),
      decoration: BoxDecoration(
        border: first
            ? null
            : const Border(top: BorderSide(color: GlassTokens.hairline)),
      ),
      child: Row(
        children: [
          Expanded(
            child: Text(
              r.name,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: const TextStyle(
                fontSize: 13.5,
                fontWeight: FontWeight.w600,
                color: AppColors.onContentImage,
              ),
            ),
          ),
          const SizedBox(width: 10),
          Text(
            r.time,
            style: TextStyle(
              fontSize: 12.5,
              fontWeight: FontWeight.w600,
              color: r.conflict
                  ? AppColors.statusWarningOnDark
                  : AppColors.darkTextSecondary,
            ),
          ),
        ],
      ),
    );
    if (r.onTap == null) return content;
    return Tappable(
      semanticsLabel: r.semanticsLabel ?? r.name,
      onTap: r.onTap!,
      child: content,
    );
  }
}
