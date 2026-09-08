import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'mention_suggestion_item.dart';
import 'mention_types.dart';

/// An overlay that displays @-mention suggestions above the text input.
///
/// Shows a list of up to 5 suggestions, with users displayed first, then entities.
/// Supports keyboard navigation and selection.
class MentionSuggestionOverlay extends StatelessWidget {
  /// The list of suggestions to display.
  final List<MentionSuggestion> suggestions;

  /// The currently highlighted index (for keyboard navigation).
  final int highlightedIndex;

  /// Callback when a suggestion is selected.
  final ValueChanged<MentionSuggestion>? onSelect;

  /// Whether suggestions are currently loading.
  final bool isLoading;

  /// The query text that was searched for.
  final String query;

  /// Optional callback to load media URLs for thumbnails.
  final MediaUrlLoader? mediaUrlLoader;

  /// Error message to display when suggestion fetch failed.
  final String? errorMessage;

  /// Callback to retry fetching suggestions after an error.
  final VoidCallback? onRetry;

  const MentionSuggestionOverlay({
    super.key,
    required this.suggestions,
    this.highlightedIndex = -1,
    this.onSelect,
    this.isLoading = false,
    this.query = '',
    this.mediaUrlLoader,
    this.errorMessage,
    this.onRetry,
  });

  @override
  Widget build(BuildContext context) {
    return Material(
      elevation: 8,
      borderRadius: BorderRadius.circular(12),
      color: AppColors.cardBackground(context),
      child: Container(
        constraints: const BoxConstraints(
          maxHeight: 280, // Approximately 5 items
          minWidth: 200,
        ),
        decoration: BoxDecoration(
          borderRadius: BorderRadius.circular(12),
          border: Border.all(
            color: AppColors.border(context),
            width: 1,
          ),
        ),
        child: ClipRRect(
          borderRadius: BorderRadius.circular(12),
          child: _buildContent(context),
        ),
      ),
    );
  }

  Widget _buildContent(BuildContext context) {
    if (isLoading) {
      return _buildLoadingState(context);
    }

    if (errorMessage != null) {
      return _buildErrorState(context);
    }

    if (suggestions.isEmpty) {
      return _buildEmptyState(context);
    }

    return _buildSuggestionsList(context);
  }

  Widget _buildLoadingState(BuildContext context) {
    return const Padding(
      padding: EdgeInsets.symmetric(vertical: 24, horizontal: 16),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          SizedBox(
            width: 16,
            height: 16,
            child: CircularProgressIndicator(strokeWidth: 2),
          ),
          SizedBox(width: 12),
          Text('Searching...'),
        ],
      ),
    );
  }

  Widget _buildEmptyState(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 24, horizontal: 16),
      child: Row(
        children: [
          Icon(
            Icons.search_off,
            size: 20,
            color: AppColors.textTertiary(context),
          ),
          const SizedBox(width: 8),
          Expanded(
            child: Text(
              query.isEmpty ? 'Type to search' : 'No results for "@$query"',
              style: TextStyle(
                color: AppColors.textSecondary(context),
                fontSize: 14,
              ),
              overflow: TextOverflow.ellipsis,
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildErrorState(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 16, horizontal: 16),
      child: Row(
        children: [
          Icon(
            Icons.error_outline,
            size: 20,
            color: AppColors.statusError(context),
          ),
          const SizedBox(width: 8),
          Expanded(
            child: Text(
              errorMessage!,
              style: TextStyle(
                color: AppColors.textSecondary(context),
                fontSize: 14,
              ),
            ),
          ),
          if (onRetry != null)
            TextButton(
              onPressed: onRetry,
              style: TextButton.styleFrom(
                padding: const EdgeInsets.symmetric(horizontal: 8),
                minimumSize: const Size(0, 32),
              ),
              child: Text(context.l10n.commonRetry),
            ),
        ],
      ),
    );
  }

  Widget _buildSuggestionsList(BuildContext context) {
    // Sort: users first, then entities, alphabetically within each group
    final sortedSuggestions = List<MentionSuggestion>.from(suggestions);
    sortedSuggestions.sort((a, b) {
      // Users first
      if (a.type == MentionType.user && b.type != MentionType.user) return -1;
      if (a.type != MentionType.user && b.type == MentionType.user) return 1;
      // Then alphabetically by name
      return a.displayName.toLowerCase().compareTo(b.displayName.toLowerCase());
    });

    // Limit to 5 results
    final displaySuggestions = sortedSuggestions.take(5).toList();

    return ListView.builder(
      shrinkWrap: true,
      padding: const EdgeInsets.symmetric(vertical: 4),
      itemCount: displaySuggestions.length,
      itemBuilder: (context, index) {
        final suggestion = displaySuggestions[index];
        return MentionSuggestionItem(
          suggestion: suggestion,
          isHighlighted: index == highlightedIndex,
          onTap: () => onSelect?.call(suggestion),
          imageProvider: suggestion.imageProvider,
          mediaUrlLoader: mediaUrlLoader,
        );
      },
    );
  }
}
