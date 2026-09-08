import 'package:cached_network_image/cached_network_image.dart';
import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/image_cache_keys.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'mention_types.dart';

/// Callback to get a media URL from a media ID.
typedef MediaUrlLoader = Future<String?> Function(String mediaId);

/// A single suggestion item in the @-mention autocomplete dropdown.
///
/// Displays an icon/avatar, the entity name, and optionally a type badge.
class MentionSuggestionItem extends StatelessWidget {
  /// The suggestion to display.
  final MentionSuggestion suggestion;

  /// Whether this item is currently highlighted/selected.
  final bool isHighlighted;

  /// Callback when this item is tapped.
  final VoidCallback? onTap;

  /// Optional image provider for the thumbnail. If provided, shows the image
  /// instead of the emoji icon for entity types.
  final ImageProvider? imageProvider;

  /// Optional callback to load media URLs. If provided and the suggestion has
  /// a mediaId but no imageProvider, this will be used to load the thumbnail.
  final MediaUrlLoader? mediaUrlLoader;

  const MentionSuggestionItem({
    super.key,
    required this.suggestion,
    this.isHighlighted = false,
    this.onTap,
    this.imageProvider,
    this.mediaUrlLoader,
  });

  @override
  Widget build(BuildContext context) {
    return Material(
      color: isHighlighted
          ? AppColors.primary(context).withValues(alpha: 0.1)
          : Colors.transparent,
      child: Tappable(
        semanticsLabel: suggestion.displayName,
        onTap: onTap,
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
          child: Row(
            children: [
              _buildLeadingIcon(context),
              const SizedBox(width: 12),
              Expanded(child: _buildContent(context)),
              if (suggestion.type != MentionType.user) _buildTypeBadge(context),
            ],
          ),
        ),
      ),
    );
  }

  Widget _buildLeadingIcon(BuildContext context) {
    // Try to load image from mediaId if available
    if (suggestion.mediaId != null &&
        suggestion.mediaId!.isNotEmpty &&
        mediaUrlLoader != null) {
      return _buildAsyncImage(context);
    }

    // Use provided imageProvider
    if (imageProvider != null) {
      return _buildImageFromProvider(context);
    }

    // Fall back to placeholder
    return _buildPlaceholder(context);
  }

  Widget _buildAsyncImage(BuildContext context) {
    final isCircle = suggestion.type == MentionType.user;
    final borderRadius = isCircle ? null : BorderRadius.circular(6);

    return FutureBuilder<String?>(
      future: mediaUrlLoader!(suggestion.mediaId!),
      builder: (context, snapshot) {
        if (snapshot.hasData && snapshot.data != null) {
          return ClipRRect(
            borderRadius: borderRadius ?? BorderRadius.circular(16),
            child: CachedNetworkImage(
              imageUrl: snapshot.data!,
              cacheKey: ImageCacheKeys.thumbnail(suggestion.mediaId),
              width: 32,
              height: 32,
              fit: BoxFit.cover,
              placeholder: (context, url) => _buildPlaceholder(context),
              errorWidget: (context, url, error) => _buildPlaceholder(context),
            ),
          );
        }
        return _buildPlaceholder(context);
      },
    );
  }

  Widget _buildImageFromProvider(BuildContext context) {
    if (suggestion.type == MentionType.user) {
      return Container(
        width: 32,
        height: 32,
        decoration: BoxDecoration(
          shape: BoxShape.circle,
          image: DecorationImage(
            image: imageProvider!,
            fit: BoxFit.cover,
          ),
        ),
      );
    }

    return ClipRRect(
      borderRadius: BorderRadius.circular(6),
      child: Image(
        image: imageProvider!,
        width: 32,
        height: 32,
        fit: BoxFit.cover,
      ),
    );
  }

  Widget _buildPlaceholder(BuildContext context) {
    if (suggestion.type == MentionType.user) {
      return Container(
        width: 32,
        height: 32,
        decoration: BoxDecoration(
          color: AppColors.primary(context).withValues(alpha: 0.2),
          shape: BoxShape.circle,
        ),
        child: Center(
          child: Text(
            suggestion.displayName.isNotEmpty
                ? suggestion.displayName[0].toUpperCase()
                : '?',
            style: TextStyle(
              color: AppColors.primary(context),
              fontWeight: FontWeight.w600,
              fontSize: 14,
            ),
          ),
        ),
      );
    }

    return Container(
      width: 32,
      height: 32,
      decoration: BoxDecoration(
        color: _getIconBackgroundColor(context),
        borderRadius: BorderRadius.circular(6),
      ),
      child: Center(
        child: Text(
          suggestion.type.icon,
          style: const TextStyle(fontSize: 16),
        ),
      ),
    );
  }

  Widget _buildContent(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: [
        Text(
          suggestion.displayName,
          style: TextStyle(
            color: AppColors.textPrimary(context),
            fontSize: 14,
            fontWeight: FontWeight.w500,
          ),
          maxLines: 1,
          overflow: TextOverflow.ellipsis,
        ),
        if (suggestion.subtitle != null) ...[
          const SizedBox(height: 2),
          Text(
            suggestion.subtitle!,
            style: TextStyle(
              color: AppColors.textSecondary(context),
              fontSize: 12,
            ),
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
          ),
        ],
      ],
    );
  }

  Widget _buildTypeBadge(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
      decoration: BoxDecoration(
        color: _getBadgeColor(context),
        borderRadius: BorderRadius.circular(12),
      ),
      child: Text(
        suggestion.type.label,
        style: TextStyle(
          color: AppColors.textPrimary(context),
          fontSize: 10,
          fontWeight: FontWeight.w600,
        ),
      ),
    );
  }

  Color _getIconBackgroundColor(BuildContext context) {
    switch (suggestion.type) {
      case MentionType.user:
        return AppColors.primary(context).withValues(alpha: 0.2);
      case MentionType.loan:
        return AppColors.loanColorOnDark.withValues(alpha: 0.2);
      case MentionType.giveaway:
        return AppColors.giveawayColorOnDark.withValues(alpha: 0.2);
      case MentionType.request:
        return AppColors.requestColorOnDark.withValues(alpha: 0.2);
      case MentionType.experience:
        return AppColors.experienceColorOnDark.withValues(alpha: 0.2);
    }
  }

  Color _getBadgeColor(BuildContext context) {
    switch (suggestion.type) {
      case MentionType.user:
        return AppColors.primary(context).withValues(alpha: 0.15);
      case MentionType.loan:
        return AppColors.loanColorOnDark.withValues(alpha: 0.15);
      case MentionType.giveaway:
        return AppColors.giveawayColorOnDark.withValues(alpha: 0.15);
      case MentionType.request:
        return AppColors.requestColorOnDark.withValues(alpha: 0.15);
      case MentionType.experience:
        return AppColors.experienceColorOnDark.withValues(alpha: 0.15);
    }
  }
}
