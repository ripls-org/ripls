import 'package:flutter/material.dart';
import 'package:ripls/core/theme/gen/paper_tokens.gen.dart';
import 'package:ripls/presentation/models/drill_down_data.dart';

/// ReferenceList renders the numbered research reference section.
///
/// Academic-style reference list at the bottom of the drill-down screen.
/// Each reference shows: [id] citation — support level, what it's used for.
class ReferenceList extends StatelessWidget {
  final List<ResearchReference> references;

  const ReferenceList({
    super.key,
    required this.references,
  });

  @override
  Widget build(BuildContext context) {
    if (references.isEmpty) return const SizedBox.shrink();

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const Text(
          'REFERENCES',
          style: TextStyle(
            color: PaperTokens.textMuted,
            fontSize: 11,
            fontWeight: FontWeight.w600,
            letterSpacing: 0.5,
          ),
        ),
        const SizedBox(height: 12),
        ...references.map(_buildReference),
      ],
    );
  }

  Widget _buildReference(ResearchReference ref) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 10),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          SizedBox(
            width: 28,
            child: Text(
              '[${ref.id}]',
              style: const TextStyle(
                color: PaperTokens.accentGreen,
                fontSize: 12,
                fontWeight: FontWeight.w600,
              ),
            ),
          ),
          Expanded(
            child: RichText(
              text: TextSpan(
                style: const TextStyle(
                  color: PaperTokens.textMuted,
                  fontSize: 12,
                  height: 1.4,
                ),
                children: [
                  TextSpan(
                    text: ref.citation,
                    style: const TextStyle(fontWeight: FontWeight.w500),
                  ),
                  TextSpan(text: ' — ${ref.supportLevel} support for '),
                  TextSpan(
                    text: ref.usedFor.endsWith('.')
                        ? ref.usedFor
                        : '${ref.usedFor}.',
                  ),
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }
}
