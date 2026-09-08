import 'package:flutter/widgets.dart';
import 'package:ripls/data/gen/ripls/api/gear.pb.dart' show Availability;
import 'package:ripls/services/search_service.dart';
import 'mention_types.dart';

/// Converts search results to MentionSuggestion objects.
///
/// Handles the mapping from SearchResultItem types to MentionType and extracts
/// the relevant IDs and display names for each entity type.
class MentionSuggestionConverter {
  /// Converts a list of search results to mention suggestions.
  ///
  /// The [imageProviderFactory] is called for each suggestion that has a media ID,
  /// allowing the caller to provide image providers from their media repository.
  static List<MentionSuggestion> fromSearchResults(
    List<SearchResultItem> results, {
    ImageProvider? Function(String mediaId)? imageProviderFactory,
  }) {
    final suggestions = <MentionSuggestion>[];

    for (final result in results) {
      final suggestion = _convertResult(result, imageProviderFactory);
      if (suggestion != null) {
        suggestions.add(suggestion);
      }
    }

    return suggestions;
  }

  static MentionSuggestion? _convertResult(
    SearchResultItem result,
    ImageProvider? Function(String mediaId)? imageProviderFactory,
  ) {
    switch (result.itemType) {
      case SearchItemType.SEARCH_ITEM_TYPE_USER:
        return _convertUser(result, imageProviderFactory);

      case SearchItemType.SEARCH_ITEM_TYPE_GEAR:
        return _convertGear(result, imageProviderFactory);

      case SearchItemType.SEARCH_ITEM_TYPE_REQUEST:
        return _convertRequest(result, imageProviderFactory);

      case SearchItemType.SEARCH_ITEM_TYPE_EXPERIENCE:
        return _convertExperience(result, imageProviderFactory);

      default:
        return null;
    }
  }

  static MentionSuggestion _convertUser(
    SearchResultItem result,
    ImageProvider? Function(String mediaId)? imageProviderFactory,
  ) {
    final user = result.user;
    final mediaId = user.mediaId.isNotEmpty ? user.mediaId : null;

    return MentionSuggestion(
      type: MentionType.user,
      id: user.id,
      displayName: user.name,
      mediaId: mediaId,
      imageProvider: mediaId != null ? imageProviderFactory?.call(mediaId) : null,
    );
  }

  static MentionSuggestion _convertGear(
    SearchResultItem result,
    ImageProvider? Function(String mediaId)? imageProviderFactory,
  ) {
    final gear = result.gear;

    // Determine type based on availability
    final mentionType = gear.availability == Availability.AVAILABILITY_FOR_GIVEAWAY
        ? MentionType.giveaway
        : MentionType.loan;

    // Use transferId from activeLoan if available, otherwise use gear.id
    final id = gear.hasActiveLoan() && gear.activeLoan.transferId.isNotEmpty
        ? gear.activeLoan.transferId
        : gear.id;

    // Get first media ID if available
    final mediaId = gear.mediaIds.isNotEmpty ? gear.mediaIds.first : null;

    // Subtitle shows loan status if available
    final subtitle = gear.hasActiveLoan() && gear.activeLoan.status.isNotEmpty
        ? gear.activeLoan.status
        : null;

    return MentionSuggestion(
      type: mentionType,
      id: id,
      displayName: gear.name,
      mediaId: mediaId,
      subtitle: subtitle,
      imageProvider: mediaId != null ? imageProviderFactory?.call(mediaId) : null,
    );
  }

  static MentionSuggestion _convertRequest(
    SearchResultItem result,
    ImageProvider? Function(String mediaId)? imageProviderFactory,
  ) {
    final request = result.request;

    // Get first media ID if available
    final mediaId = request.mediaIds.isNotEmpty ? request.mediaIds.first : null;

    return MentionSuggestion(
      type: MentionType.request,
      id: request.id,
      displayName: request.title,
      mediaId: mediaId,
      imageProvider: mediaId != null ? imageProviderFactory?.call(mediaId) : null,
    );
  }

  static MentionSuggestion _convertExperience(
    SearchResultItem result,
    ImageProvider? Function(String mediaId)? imageProviderFactory,
  ) {
    final experience = result.experience;

    // Get first media ID if available (Experience has mediaIds list)
    final mediaId = experience.mediaIds.isNotEmpty ? experience.mediaIds.first : null;

    return MentionSuggestion(
      type: MentionType.experience,
      id: experience.id,
      displayName: experience.name,
      mediaId: mediaId,
      imageProvider: mediaId != null ? imageProviderFactory?.call(mediaId) : null,
    );
  }
}
