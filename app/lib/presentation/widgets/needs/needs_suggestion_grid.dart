import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// Suggestion category. Maps to a theme-aware background/accent pair on
/// [AppColors] so the grid stays readable in both light and dark modes
/// without hardcoded hex at the call site.
enum NeedsCategory { gear, food, help, personal }

/// A single suggestion entry rendered in [NeedsSuggestionGrid]. The
/// [name] is what gets pre-filled into the picker on tap.
class NeedsSuggestion {
  final String name;
  final NeedsCategory category;

  const NeedsSuggestion({required this.name, required this.category});
}

/// NK1's category-tinted suggestion grid. Renders as a wrap of pill-shaped
/// tiles, each tinted by its category. Tapping a tile fires [onSelect]
/// with the suggestion — the parent decides whether to commit it or open
/// the Picker pre-populated.
class NeedsSuggestionGrid extends StatelessWidget {
  final List<NeedsSuggestion> suggestions;
  final ValueChanged<NeedsSuggestion> onSelect;

  const NeedsSuggestionGrid({
    super.key,
    required this.suggestions,
    required this.onSelect,
  });

  @override
  Widget build(BuildContext context) {
    if (suggestions.isEmpty) return const SizedBox.shrink();
    return Wrap(
      spacing: 8,
      runSpacing: 8,
      children: [
        for (final s in suggestions) _SuggestionTile(suggestion: s, onTap: () => onSelect(s)),
      ],
    );
  }
}

class _SuggestionTile extends StatelessWidget {
  final NeedsSuggestion suggestion;
  final VoidCallback onTap;

  const _SuggestionTile({required this.suggestion, required this.onTap});

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    final (bg, accent) = switch (suggestion.category) {
      NeedsCategory.gear => (
          AppColors.needsCategoryGearBg(context),
          AppColors.needsCategoryGearAccent(context)
        ),
      NeedsCategory.food => (
          AppColors.needsCategoryFoodBg(context),
          AppColors.needsCategoryFoodAccent(context)
        ),
      NeedsCategory.help => (
          AppColors.needsCategoryHelpBg(context),
          AppColors.needsCategoryHelpAccent(context)
        ),
      NeedsCategory.personal => (
          AppColors.needsCategoryPersonalBg(context),
          AppColors.needsCategoryPersonalAccent(context)
        ),
    };
    return Tappable(
      semanticsLabel: l10n.a11yNeedsPickerSuggestion(suggestion.name),
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
        decoration: BoxDecoration(
          color: bg,
          borderRadius: BorderRadius.circular(20),
          border: Border.all(color: accent.withValues(alpha: 0.4), width: 1),
        ),
        child: Text(
          suggestion.name,
          style: TextStyle(
            fontSize: 13,
            fontWeight: FontWeight.w600,
            color: accent,
          ),
        ),
      ),
    );
  }
}
