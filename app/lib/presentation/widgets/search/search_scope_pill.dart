import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/search/close_search_button.dart';

/// The search-bar remnant a dock tab shows while scoped to a
/// universal-search query: styled like the search field (glyph + the
/// query text) so the user can see what's filtering the tab, with the
/// close affordance cancelling the scope and restoring the full tab.
class SearchScopePill extends StatelessWidget {
  const SearchScopePill({
    super.key,
    required this.query,
    required this.onClear,
  });

  /// The active query the tab is filtered to.
  final String query;

  /// Cancels the search scope.
  final VoidCallback onClear;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 6),
      decoration: BoxDecoration(
        color: AppColors.cardBackground(context),
        borderRadius: BorderRadius.circular(22),
        border: Border.all(color: AppColors.border(context)),
      ),
      child: Row(
        children: [
          Icon(Icons.search, size: 18, color: AppColors.textSecondary(context)),
          const SizedBox(width: 8),
          Expanded(
            child: Text(
              query,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: TextStyle(
                fontSize: 13.5,
                fontWeight: FontWeight.w500,
                color: AppColors.textPrimary(context),
              ),
            ),
          ),
          CloseSearchButton(
            onTap: onClear,
            semanticsLabel: context.l10n.searchScopeCancel,
          ),
        ],
      ),
    );
  }
}
