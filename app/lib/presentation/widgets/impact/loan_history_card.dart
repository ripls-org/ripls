import 'package:flutter/material.dart';
import 'package:intl/intl.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/paper_tokens.gen.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/presentation/viewmodels/gear_metric_view_model.dart'
    show LoanSocialAttributeDisplay;
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

import '../user_avatar.dart';

/// LoanHistoryCard displays a single loan entry with an expandable social
/// attributes section.
///
/// Collapsed state shows borrower avatar, name, date range, quality time value,
/// and a chevron. Tapping the value (or anywhere on the row) toggles expansion.
/// Expanded state shows all seven social attributes plus a "How we calculated
/// this →" link.
///
/// [rmValue] is the compact inline format (e.g. "9 mins"). [attributes] may be
/// null when no quality time data is available for this loan.
class LoanHistoryCard extends StatefulWidget {
  final User borrower;
  final DateTime startDate;
  final DateTime? endDate;
  final bool isActive;
  final String? rmValue;
  final LoanSocialAttributeDisplay? attributes;
  final VoidCallback? onDrillDownTap;

  const LoanHistoryCard({
    super.key,
    required this.borrower,
    required this.startDate,
    this.endDate,
    required this.isActive,
    this.rmValue,
    this.attributes,
    this.onDrillDownTap,
  });

  @override
  State<LoanHistoryCard> createState() => _LoanHistoryCardState();
}

class _LoanHistoryCardState extends State<LoanHistoryCard> {
  bool _expanded = false;

  void _toggleExpanded() {
    setState(() => _expanded = !_expanded);
  }

  @override
  Widget build(BuildContext context) {
    final hasRm = widget.rmValue != null && widget.attributes != null;

    return Container(
      margin: const EdgeInsets.only(bottom: 8),
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(
          color: widget.isActive
              ? AppColors.transferCoral
              : PaperTokens.borderSubtle, // borderLt
          width: 1,
        ),
      ),
      child: Column(
        children: [
          _buildCollapsedRow(hasRm),
          if (_expanded && hasRm) ...[
            const Divider(height: 1, color: PaperTokens.borderSubtle),
            _buildAttributesSection(),
          ],
        ],
      ),
    );
  }

  /// _buildCollapsedRow builds the always-visible header row.
  Widget _buildCollapsedRow(bool hasRm) {
    return Semantics(
      expanded: _expanded,
      child: Tappable(
        semanticsLabel: widget.borrower.name,
        onTap: hasRm ? _toggleExpanded : null,
        child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
        child: Row(
          children: [
            UserAvatar(user: widget.borrower, radius: 16),
            const SizedBox(width: 10),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    widget.borrower.name,
                    style: const TextStyle(
                      fontSize: 14,
                      fontWeight: FontWeight.w600,
                      color: PaperTokens.textPrimary, // text
                    ),
                  ),
                  const SizedBox(height: 2),
                  Text(
                    _formatDateRange(),
                    style: const TextStyle(
                      fontSize: 12,
                      color: PaperTokens.textFaint, // textMute
                    ),
                  ),
                ],
              ),
            ),
            if (hasRm) ...[
              const SizedBox(width: 8),
              const Text(
                '•',
                style: TextStyle(color: PaperTokens.textFaint, fontSize: 12),
              ),
              const SizedBox(width: 6),
              Text(
                widget.rmValue!,
                style: const TextStyle(
                  fontSize: 14,
                  fontWeight: FontWeight.w700,
                  color: PaperTokens.accentTeal, // teal
                ),
              ),
              const SizedBox(width: 6),
              AnimatedRotation(
                turns: _expanded ? 0.5 : 0,
                duration: accessibleDuration(context, const Duration(milliseconds: 200)),
                child: const Icon(
                  Icons.keyboard_arrow_down,
                  size: 18,
                  color: PaperTokens.textFaint,
                ),
              ),
            ],
          ],
        ),
        ),
      ),
    );
  }

  /// _buildAttributesSection renders the seven social attribute rows plus
  /// the "How we calculated this →" audit trail link.
  Widget _buildAttributesSection() {
    final attrs = widget.attributes!;
    final rows = [
      ('Duration', attrs.duration),
      ('Modality', attrs.modality),
      ('Group size', attrs.groupSize),
      ('Connection', attrs.connection),
      ('Reciprocity', attrs.reciprocity),
      ('Novelty', attrs.novelty),
      ('Vulnerability', attrs.vulnerability),
    ];

    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 12, 16, 12),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          ...rows.map((row) => _buildAttributeRow(row.$1, row.$2)),
          if (widget.onDrillDownTap != null) ...[
            const SizedBox(height: 10),
            Tappable(
              semanticsLabel: context.l10n.a11yMethodologyLink,
              isLink: true,
              onTap: widget.onDrillDownTap,
              child: const Text(
                'How we calculated this →',
                style: TextStyle(
                  fontSize: 12,
                  color: PaperTokens.textMuted, // textSec
                  decoration: TextDecoration.underline,
                  decorationColor: PaperTokens.textMuted,
                ),
              ),
            ),
          ],
        ],
      ),
    );
  }

  Widget _buildAttributeRow(String label, String value) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 6),
      child: Row(
        children: [
          SizedBox(
            width: 110,
            child: Text(
              label,
              style: const TextStyle(
                fontSize: 13,
                color: PaperTokens.textMuted, // textSec
              ),
            ),
          ),
          Expanded(
            child: Text(
              value,
              style: const TextStyle(
                fontSize: 13,
                fontWeight: FontWeight.w500,
                color: PaperTokens.textPrimary, // text
              ),
            ),
          ),
        ],
      ),
    );
  }

  String _formatDateRange() {
    final fmt = DateFormat('MMM d');
    final start = fmt.format(widget.startDate);
    if (widget.isActive) return 'Since $start';
    if (widget.endDate != null) return '$start – ${fmt.format(widget.endDate!)}';
    return start;
  }
}
