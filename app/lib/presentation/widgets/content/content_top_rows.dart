import 'package:flutter/material.dart';
import 'package:ripls/presentation/widgets/content/content_info_row.dart';
import 'package:ripls/presentation/widgets/content/content_title_row.dart';

/// ContentTopRows composes the first-tab header for all four content
/// views: title, optional description, then an info-row stack of
/// [ContentInfoRow]s (time / location / owner — each optional).
///
/// The caller is responsible for setting `showDivider: false` on the
/// last row in the stack.
class ContentTopRows extends StatelessWidget {
  final String title;

  final String? description;

  /// Optional rich widget rendered between the title row and the info
  /// rows (e.g. a `ContentExpandableDescription`). Takes precedence over
  /// [description] when both are set.
  final Widget? descriptionSlot;

  /// Info rows rendered beneath the title + description. Order matters —
  /// the last row should have `showDivider: false`.
  final List<ContentInfoRow> rows;

  /// Optional widget rendered to the right of the title (e.g. an edit
  /// pencil for owners of an active experience).
  final Widget? titleTrailing;

  const ContentTopRows({
    super.key,
    required this.title,
    this.description,
    this.descriptionSlot,
    this.rows = const [],
    this.titleTrailing,
  });

  @override
  Widget build(BuildContext context) {
    final hasDescriptionSlot = descriptionSlot != null;
    final hasDescriptionText = !hasDescriptionSlot &&
        description != null &&
        description!.isNotEmpty;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        ContentTitleRow(title: title, trailing: titleTrailing),
        if (hasDescriptionSlot) ...[
          const SizedBox(height: 8),
          descriptionSlot!,
        ] else if (hasDescriptionText) ...[
          const SizedBox(height: 8),
          Text(
            description!,
            style: TextStyle(
              fontSize: 13.5,
              height: 1.45,
              color: Colors.white.withValues(alpha: 0.75),
            ),
          ),
        ],
        if (rows.isNotEmpty) ...[
          const SizedBox(height: 8),
          ...rows,
        ],
      ],
    );
  }
}
