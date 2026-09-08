// dart-line-count-allow: pushed over by needs-v2 additions
// (updateNeed + nudgeUncoveredNeedClaimers). A clean split (e.g.
// extracting the needs-and-contributions surface into a mixin) is
// tracked in #2148 — the repository's other concerns
// (impact / RSVP / time-poll / location-poll) sit elsewhere so the
// natural cut is by RPC family.
import 'package:flutter/foundation.dart' show VoidCallback;
import 'package:ripls/data/cache/cache_manager.dart';
import 'package:ripls/data/cache/cache_service.dart';
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart'
    show MoneySavings, PreventedEmissions, QualityTimeAttributes;
import 'package:ripls/data/repositories/chat_repository.dart';
import 'package:ripls/data/repositories/feed_repository.dart';
import 'package:ripls/data/repositories/search_repository.dart';
import 'package:ripls/services/experience_service.dart';

// Export types that ViewModels need
export 'package:ripls/services/experience_service.dart'
    show
        GenExperienceResponse,
        Experience,
        ExperienceTime,
        ExperienceMetadata,
        GetExperienceResponse,
        RSVPIntention,
        AttendedStatus,
        AttendanceRecord,
        GetExperiencePeopleResponse,
        GetExperienceStatsResponse,
        CompleteExperienceResponse,
        ImpactEstimate,
        TimeProposal,
        TimeVote,
        TimeVoteStatus,
        ListExperienceNeedsAndContributionsResponse,
        ExperienceNeedResponse,
        ExperienceContributionResponse,
        BatchNeedItem,
        BatchContributionItem,
        StreamGenExperienceRequest,
        StreamGenExperienceResponse,
        StreamGenExperienceResponse_Event,
        GenStreamError,
        GenStreamErrorCode,
        MediaReady;

// Location-poll mutation methods (propose/vote/confirm/cancel/lock/nudge/
// extract) live in a sibling part file to keep this file under the
// 1000-line size gate (`npm run lint:dart:size`).
part 'experience_repository_location_poll.dart';

/// Repository for experience data with transparent caching.
///
/// This repository wraps ExperienceService and provides caching for experience
/// items using the global TTL from environment (CACHE_TTL_MINUTES).
class ExperienceRepository {
  final CacheService _cache;
  final ExperienceService _service;
  final FeedRepository _feedRepository;
  final ChatRepository _chatRepository;
  final SearchRepository _searchRepository;
  final VoidCallback? _onDailyInvalidated;
  final VoidCallback? _onContentInvalidated;
  final VoidCallback? _onImpactInvalidated;
  final VoidCallback? _onProfileInvalidated;

  ExperienceRepository(
    CacheManager cacheManager,
    this._service,
    this._feedRepository,
    this._chatRepository,
    this._searchRepository, {
    VoidCallback? onDailyInvalidated,
    VoidCallback? onContentInvalidated,
    VoidCallback? onImpactInvalidated,
    VoidCallback? onProfileInvalidated,
  }) : _cache = CacheService(cacheManager, 'experience'),
       _onDailyInvalidated = onDailyInvalidated,
       _onContentInvalidated = onContentInvalidated,
       _onImpactInvalidated = onImpactInvalidated,
       _onProfileInvalidated = onProfileInvalidated;

  /// Gets an experience by ID with caching.
  ///
  /// [communityId] is optional - when provided, returns community-specific conversation_id.
  Future<GetExperienceResponse> get(
    String experienceId, {
    String? communityId,
  }) async {
    // Use community-specific cache key when community context is provided
    final cacheKey = communityId != null
        ? 'experience:$experienceId:community:$communityId'
        : experienceId;

    return _cache.get(
      key: cacheKey,
      fetch: () =>
          _service.getExperience(experienceId, communityId: communityId),
    );
  }

  /// Lists experiences for a specific community.
  ///
  /// Use [refreshCommunityExperiences] to force a refresh.
  Future<List<Experience>> listCommunityExperiences(String communityId) async {
    return _cache.getList(
      listKey: 'community:$communityId:list',
      fetch: () => _service.listExperiences(communityId),
    );
  }

  /// Refreshes community experiences by invalidating the cache.
  ///
  /// Call this after sharing or deleting experiences to ensure
  /// the next fetch gets fresh data.
  Future<List<Experience>> refreshCommunityExperiences(
    String communityId,
  ) async {
    await _cache.invalidate('community:$communityId:list');
    return listCommunityExperiences(communityId);
  }

  /// Lists all experiences created by the current user.
  ///
  /// Use [refreshMyExperiences] to force a refresh.
  Future<List<Experience>> listMyExperiences() async {
    return _cache.getList(
      listKey: 'user:list',
      fetch: () => _service.listMyExperiences(),
    );
  }

  /// Refreshes the user experience list by invalidating the cache.
  ///
  /// Call this after creating, updating, or deleting experiences to ensure
  /// the next fetch gets fresh data.
  Future<List<Experience>> refreshMyExperiences() async {
    await _cache.invalidate('user:list');
    return listMyExperiences();
  }

  /// Gets an experience with full details.
  ///
  /// [communityId] is optional - when provided, returns community-specific conversation_id.
  /// This wraps [get] with a more descriptive name.
  Future<GetExperienceResponse> getExperienceDetails(
    String experienceId, {
    String? communityId,
  }) async {
    return get(experienceId, communityId: communityId);
  }

  /// Invalidates a specific experience entry.
  ///
  /// If [communityId] is provided, invalidates the community-specific cache entry.
  /// Otherwise, invalidates both the base entry and all community-specific entries.
  Future<void> invalidate(String experienceId, {String? communityId}) async {
    if (communityId != null) {
      await _cache.invalidate(
        'experience:$experienceId:community:$communityId',
      );
    } else {
      await _cache.invalidate(experienceId);
      await _cache.invalidatePattern('experience:$experienceId:*');
    }
  }

  /// Invalidates all cached experiences.
  Future<void> invalidateAll() async {
    await _cache.invalidateAll();
  }

  /// Invalidates every cached entry scoped to a single community.
  ///
  /// Use this after a community-level lifecycle change (e.g. restoring
  /// a previously soft-deleted community) so the next read fetches
  /// the freshly un-soft-deleted experiences rather than the cached
  /// "no experiences visible" snapshot from the deleted period.
  Future<void> invalidateForCommunity(String communityId) async {
    await _cache.invalidatePattern('community:$communityId:*');
  }

  /// Saves experience details (update operation).
  ///
  /// Supports partial updates - only provided fields are updated.
  /// Invalidates the experience item cache, user experience list, and search cache after save.
  Future<void> saveExperience({
    required String id,
    String? name,
    String? description,
    List<String>? mediaIds,
    String? locationId,
    ExperienceTime? time,
    int? maxParticipants,
    String? sourceUrl,
    ExperienceMetadata? metadata,
    SocialContext? socialContext,
  }) async {
    await _service.saveExperience(
      id: id,
      name: name,
      description: description,
      mediaIds: mediaIds,
      locationId: locationId,
      time: time,
      maxParticipants: maxParticipants,
      sourceUrl: sourceUrl,
      metadata: metadata,
      socialContext: socialContext,
    );

    // Invalidate caches after mutation
    // Use pattern to invalidate both regular and community-specific cache keys
    // Regular key: "experienceId", Community key: "experience:experienceId:community:communityId"
    await _cache.invalidatePattern('experience:$id*');
    await invalidate(id);
    await refreshMyExperiences();

    // Invalidate search cache so updates appear in discover screen
    await _searchRepository.invalidateSearches();
    _onDailyInvalidated?.call();

    // Fire content invalidation so experience detail ViewModels reload and
    // reflect LLM-inferred QT attributes updated server-side on save.
    _onContentInvalidated?.call();
  }

  /// Creates a new experience item.
  ///
  /// Returns the ID of the created experience.
  /// Invalidates the user experience list cache after creation.
  /// Does not cache (write operation).
  Future<String> createExperience({
    required String name,
    String? description,
    List<String>? mediaIds,
    String? locationId,
    ExperienceTime? time,
    int? maxParticipants,
    String? sourceUrl,
    ExperienceMetadata? metadata,
  }) async {
    final experienceId = await _service.saveExperience(
      name: name,
      description: description,
      mediaIds: mediaIds,
      locationId: locationId,
      time: time,
      maxParticipants: maxParticipants,
      sourceUrl: sourceUrl,
      metadata: metadata,
    );

    // Invalidate user experience list to ensure fresh data on next fetch
    await refreshMyExperiences();
    _onDailyInvalidated?.call();

    return experienceId;
  }

  /// Shares an experience with a community.
  ///
  /// Invalidates the community experiences list, feed cache, and search cache.
  /// Does NOT invalidate experience details (which includes RSVPs) — sharing
  /// state is managed independently by ExperienceSharingNotifier.
  Future<void> shareExperience({
    required String experienceId,
    required String communityId,
  }) async {
    await _service.shareExperience(
      experienceId: experienceId,
      communityId: communityId,
    );

    // Invalidate community list, feed, and search — but NOT the experience details
    // cache (which contains RSVPs). Sharing state is tracked separately in the
    // sharing notifier; invalidating experience details would cause RSVPs to
    // appear/disappear as the server returns community-scoped results on refetch.
    await refreshCommunityExperiences(communityId);
    await _feedRepository.invalidateFeed();

    // Invalidate search cache so new experience appears in discover screen
    await _searchRepository.invalidateSearchesForCommunity(communityId);
    _onDailyInvalidated?.call();
  }

  /// Unshares an experience from a specific community.
  ///
  /// Removes the experience from that community's feed.
  /// Invalidates the community experiences list, feed cache, and search cache.
  /// Does NOT invalidate experience details (which includes RSVPs) — sharing
  /// state is managed independently by ExperienceSharingNotifier.
  Future<void> unshareExperience({
    required String experienceId,
    required String communityId,
  }) async {
    await _service.unshareExperience(
      experienceId: experienceId,
      communityId: communityId,
    );

    // Invalidate community list, feed, and search — but NOT the experience details
    // cache (which contains RSVPs). Sharing state is tracked separately in the
    // sharing notifier; invalidating experience details would cause RSVPs to
    // appear/disappear as the server returns community-scoped results on refetch.
    await refreshCommunityExperiences(communityId);
    await _feedRepository.invalidateFeed();

    // Invalidate search cache so removed experience disappears from discover screen
    await _searchRepository.invalidateSearchesForCommunity(communityId);
    _onDailyInvalidated?.call();
  }

  /// Host sets an invitee's RSVP from the Who's In roster (owner-only).
  /// Invalidates the experience details so the roster reflects the change.
  Future<void> setMemberRsvp({
    required String experienceId,
    required String memberUserId,
    required RSVPIntention intention,
  }) async {
    await _service.setMemberRsvp(
      experienceId: experienceId,
      memberUserId: memberUserId,
      intention: intention,
    );
    await invalidate(experienceId);
  }

  /// Host uninvites a directly-invited individual (owner-only). Invalidates the
  /// experience details so the roster reflects the removal.
  Future<void> removeMember({
    required String experienceId,
    required String memberUserId,
  }) async {
    await _service.removeMember(
      experienceId: experienceId,
      memberUserId: memberUserId,
    );
    await invalidate(experienceId);
  }

  /// Updates the user's RSVP intention for an experience.
  ///
  /// Invalidates the experience cache to ensure fresh RSVP data.
  /// Also invalidates the chat inbox cache since RSVP may create a conversation.
  Future<void> rsvp({
    required String experienceId,
    required String communityId,
    required RSVPIntention intention,
  }) async {
    await _service.rsvp(
      experienceId: experienceId,
      communityId: communityId,
      intention: intention,
    );

    // Invalidate experience cache to get updated RSVPs
    await _cache.invalidatePattern('experience:$experienceId*');
    await invalidate(experienceId);

    // Invalidate chat inbox cache since RSVP may have created a new conversation
    await _chatRepository.refreshConversations();
    _onDailyInvalidated?.call();
    // Notify content views (ExperienceContentView) to reload fresh RSVP data
    _onContentInvalidated?.call();
  }

  /// Marks an experience as in process (owner only).
  ///
  /// Invalidates the experience cache to reflect state change.
  Future<void> markInProcess(String experienceId) async {
    await _service.markInProcess(experienceId);

    // Invalidate experience cache to get updated state
    await _cache.invalidatePattern('experience:$experienceId*');
    await invalidate(experienceId);
    _onDailyInvalidated?.call();
  }

  /// Marks an experience as completed (owner only).
  /// Optionally accepts a [summary] of how the experience went.
  ///
  /// Returns the completion response including savings metrics.
  /// Invalidates the experience cache and stats cache to reflect state change.
  Future<CompleteExperienceResponse> completeExperience(
    String experienceId, {
    String? summary,
    List<String>? confirmedAttendeeIds,
    int? confirmedAttendeeCount,
    QualityTimeAttributes? qualityTimeOverrides,
    MoneySavings? moneySavingsOverrides,
    PreventedEmissions? emissionsOverrides,
  }) async {
    final response = await _service.completeExperience(
      experienceId,
      summary: summary,
      confirmedAttendeeIds: confirmedAttendeeIds,
      confirmedAttendeeCount: confirmedAttendeeCount,
      qualityTimeOverrides: qualityTimeOverrides,
      moneySavingsOverrides: moneySavingsOverrides,
      emissionsOverrides: emissionsOverrides,
    );

    // Invalidate experience cache to get updated state
    await _cache.invalidatePattern('experience:$experienceId*');
    await invalidate(experienceId);

    // Invalidate stats cache since completion calculates new savings
    await invalidateStats(experienceId);
    _onDailyInvalidated?.call();
    // Notify content views so conversation action buttons reflect completed state
    _onContentInvalidated?.call();
    // Notify impact metrics ViewModels (community + portfolio) to refresh in place.
    _onImpactInvalidated?.call();

    return response;
  }

  /// Reverses a prior [completeExperience] using the community_event_id it
  /// returned. Mirrors the completion's cache invalidations (including the
  /// daily/home and impact notifications) so the reverted state reflects
  /// everywhere — the Needs-you decision re-surfaces rather than staying
  /// dismissed on the home view.
  Future<void> undoCompleteExperience({
    required String communityEventId,
  }) async {
    await _service.undoCompleteExperience(communityEventId: communityEventId);
    await _cache.invalidatePattern('experience:*');
    _onDailyInvalidated?.call();
    _onContentInvalidated?.call();
    _onImpactInvalidated?.call();
  }

  /// Marks an experience as cancelled (owner only).
  ///
  /// Invalidates the experience cache to reflect state change.
  Future<void> cancelExperience(String experienceId) async {
    await _service.cancelExperience(experienceId);

    // Invalidate experience cache to get updated state
    await _cache.invalidatePattern('experience:$experienceId*');
    await invalidate(experienceId);
    _onDailyInvalidated?.call();
  }

  /// Deletes an experience (owner only).
  ///
  /// Invalidates the experience item cache, user experience list, feed, search,
  /// conversations, and profile caches after deletion.
  Future<void> deleteExperience(String experienceId) async {
    await _service.deleteExperience(experienceId);

    // Invalidate caches after mutation
    await _cache.invalidatePattern('experience:$experienceId*');
    await invalidate(experienceId);
    await refreshMyExperiences();
    // Invalidate all feed caches since deleted experience should disappear from feed
    await _feedRepository.invalidateAllFeeds();
    // Invalidate search cache so deleted experience disappears from discover
    await _searchRepository.invalidateSearches();
    // Invalidate conversations cache since associated conversation was cascade-deleted
    await _chatRepository.refreshConversations();
    // Invalidate profile caches so profile surfaces drop the deleted experience
    _onProfileInvalidated?.call();
    _onDailyInvalidated?.call();
  }

  /// Records attendance for multiple users (owner only).
  ///
  /// Invalidates the experience cache to reflect updated attendance status.
  Future<void> recordAttendance({
    required String experienceId,
    required String communityId,
    required List<AttendanceRecord> attendance,
  }) async {
    await _service.recordAttendance(
      experienceId: experienceId,
      communityId: communityId,
      attendance: attendance,
    );

    // Invalidate experience cache to get updated attendance
    await _cache.invalidatePattern('experience:$experienceId*');
    await invalidate(experienceId);
  }

  /// Previews the impact estimate for an experience before completing it.
  ///
  /// Calls the real QT formula on the server using the current confirmed attendee
  /// set without persisting anything. Used by the completion modal for live preview
  /// as attendees are toggled.
  ///
  /// [confirmedAttendeeIds] is the list of confirmed registered-user IDs.
  /// [confirmedAttendeeCount] is the total including provisional users.
  ///
  /// Requires `npm run generate` to be run for the underlying RPC type to exist.
  Future<ImpactEstimate> previewExperienceImpact({
    required String experienceId,
    List<String> confirmedAttendeeIds = const [],
    int confirmedAttendeeCount = 0,
  }) async {
    return _service.previewExperienceImpact(
      experienceId: experienceId,
      confirmedAttendeeIds: confirmedAttendeeIds,
      confirmedAttendeeCount: confirmedAttendeeCount,
    );
  }

  /// StreamGenExperience emits title / geocoded / media_ready events as each
  /// resolves, then a terminal `final` or `error`. Generation is one-shot and
  /// unique per prompt; this is a cache-bypass pass-through.
  ///
  /// Callers must resolve any image upload to a [mediaId] before invoking.
  Stream<StreamGenExperienceResponse> streamGenExperience({
    String? prompt,
    String? mediaId,
    String? websiteUrl,
    String? locationId,
    double? latitudeDeg,
    double? longitudeDeg,
  }) {
    return _service.streamGenExperience(
      text: prompt,
      mediaId: mediaId,
      websiteUrl: websiteUrl,
      locationId: locationId,
      latitudeDeg: latitudeDeg,
      longitudeDeg: longitudeDeg,
    );
  }

  /// Converts an informal time description to structured ExperienceTime using LLM.
  ///
  /// This method calls the server-side ConvertInformalTime RPC which uses an LLM
  /// to parse natural language time expressions like "tomorrow afternoon" or
  /// "next Friday at 6pm" into structured date/time information.
  ///
  /// Examples:
  /// - "tomorrow afternoon" → SpecificTime with inferred 2pm
  /// - "next weekend" → TimeRange for Saturday-Sunday
  /// - "Friday at 6pm" → SpecificTime with explicit 6pm
  ///
  /// Returns a ConvertInformalTimeResponse containing:
  /// - parsed ExperienceTime (specific/range/TBD)
  /// - confidence level (EXPLICIT/INFERRED/UNKNOWN)
  /// - error message if parsing failed
  Future<ConvertInformalTimeResponse> convertInformalTime(
    String informalDescription,
  ) async {
    return _service.convertInformalTime(informalDescription);
  }

  /// Shares multiple experiences with a community.
  ///
  /// Executes sequential API calls and handles partial failures gracefully.
  /// Returns a list of experience IDs that failed to share.
  /// Invalidates cache after completion for all items (successful and failed).
  Future<List<String>> shareMultipleWithCommunity(
    List<String> experienceIds,
    String communityId,
  ) async {
    final failedIds = <String>[];
    for (final experienceId in experienceIds) {
      try {
        await _service.shareExperience(
          experienceId: experienceId,
          communityId: communityId,
        );
      } catch (e) {
        failedIds.add(experienceId);
      }
    }

    // Invalidate cache after bulk operation
    await _cache.invalidatePattern('*');
    await _searchRepository.invalidateSearches();
    await _feedRepository.invalidateAllFeeds();
    _onDailyInvalidated?.call();

    return failedIds;
  }

  /// Unshares multiple experiences from a community.
  ///
  /// Executes sequential API calls and handles partial failures gracefully.
  /// Returns a list of experience IDs that failed to unshare.
  /// Invalidates cache after completion for all items (successful and failed).
  Future<List<String>> unshareMultipleFromCommunity(
    List<String> experienceIds,
    String communityId,
  ) async {
    final failedIds = <String>[];
    for (final experienceId in experienceIds) {
      try {
        await _service.unshareExperience(
          experienceId: experienceId,
          communityId: communityId,
        );
      } catch (e) {
        failedIds.add(experienceId);
      }
    }

    // Invalidate cache after bulk operation
    await _cache.invalidatePattern('*');
    await _searchRepository.invalidateSearches();
    await _feedRepository.invalidateAllFeeds();
    _onDailyInvalidated?.call();

    return failedIds;
  }

  /// Updates location for multiple experiences.
  ///
  /// Executes sequential API calls and handles partial failures gracefully.
  /// Fetches each item first to preserve all existing fields (name, description, mediaIds, time, maxParticipants).
  /// Returns a list of experience IDs that failed to update.
  /// Invalidates cache after completion for all items (successful and failed).
  Future<List<String>> updateMultipleLocations(
    List<String> experienceIds,
    String locationId,
  ) async {
    final failedIds = <String>[];
    for (final experienceId in experienceIds) {
      try {
        // Fetch current experience to preserve all existing fields
        final response = await get(experienceId);
        final experience = response.experience;

        // Call saveExperience with all fields preserved, only updating location
        await saveExperience(
          id: experienceId,
          name: experience.name,
          description: experience.description,
          mediaIds: experience.mediaIds,
          locationId: locationId,
          time: experience.hasTime() ? experience.time : null,
          maxParticipants: experience.maxParticipants > 0
              ? experience.maxParticipants
              : null,
        );
      } catch (e) {
        failedIds.add(experienceId);
      }
    }

    // Invalidate cache after bulk operation
    await _cache.invalidatePattern('*');
    await _searchRepository.invalidateSearches();
    await _feedRepository.invalidateAllFeeds();

    return failedIds;
  }

  /// Gets people associated with an experience (host, RSVPs, attendees).
  ///
  /// If [communityId] is provided, filters results for that specific community.
  /// Results are cached with a key based on experience ID and optional community ID.
  ///
  /// Use [invalidatePeople] to force a refresh after RSVPs or attendance changes.
  Future<GetExperiencePeopleResponse> getPeople(
    String experienceId, {
    String? communityId,
  }) async {
    final cacheKey = communityId != null
        ? 'people:$experienceId:$communityId'
        : 'people:$experienceId';
    return _cache.get(
      key: cacheKey,
      fetch: () => _service.getExperiencePeople(
        experienceId: experienceId,
        communityId: communityId,
      ),
    );
  }

  /// Invalidates the people cache for a specific experience.
  ///
  /// If [communityId] is provided, only invalidates that community's cache.
  /// Otherwise, invalidates all people caches for this experience.
  Future<void> invalidatePeople(
    String experienceId, {
    String? communityId,
  }) async {
    if (communityId != null) {
      await _cache.invalidate('people:$experienceId:$communityId');
    } else {
      await _cache.invalidatePattern('people:$experienceId*');
    }
  }

  /// Gets statistics for an experience (sessions, attendees, value created, upcoming RSVPs).
  ///
  /// If [communityId] is provided, returns stats specific to that community.
  /// Results are cached with a key based on experience ID and optional community ID.
  ///
  /// Use [invalidateStats] to force a refresh after RSVPs or attendance changes.
  Future<GetExperienceStatsResponse> getStats(
    String experienceId, {
    String? communityId,
  }) async {
    final cacheKey = communityId != null
        ? 'stats:$experienceId:$communityId'
        : 'stats:$experienceId';
    return _cache.get(
      key: cacheKey,
      fetch: () => _service.getExperienceStats(
        experienceId: experienceId,
        communityId: communityId,
      ),
    );
  }

  /// Invalidates the stats cache for a specific experience.
  ///
  /// If [communityId] is provided, only invalidates that community's cache.
  /// Otherwise, invalidates all stats caches for this experience.
  Future<void> invalidateStats(
    String experienceId, {
    String? communityId,
  }) async {
    if (communityId != null) {
      await _cache.invalidate('stats:$experienceId:$communityId');
    } else {
      await _cache.invalidatePattern('stats:$experienceId*');
    }
  }

  /// Proposes a new time slot for an experience.
  ///
  /// Returns the created time proposal with votes.
  /// Invalidates the experience cache and notifies content views so the
  /// Plan tab (PollBanner / "Agree on Time" pill / FAB color) and other
  /// surfaces see the new poll state without waiting for the next tab visit.
  Future<TimeProposal> proposeTime({
    required String experienceId,
    required ExperienceTime time,
  }) async {
    final proposal = await _service.proposeTime(
      experienceId: experienceId,
      time: time,
    );
    await _invalidateExperienceAndDaily(experienceId);
    return proposal;
  }

  /// Records a vote on a time proposal. Refreshes PollBanner vote counts.
  Future<void> voteOnTime({
    required String proposalId,
    required TimeVoteStatus status,
    required String experienceId,
  }) async {
    await _service.voteOnTime(proposalId: proposalId, status: status);
    await _invalidateExperience(experienceId);
  }

  /// Confirms a time proposal as the final time (owner only).
  Future<void> confirmTime({
    required String experienceId,
    required String proposalId,
  }) async {
    await _service.confirmTime(
      experienceId: experienceId,
      proposalId: proposalId,
    );
    await _invalidateExperience(experienceId);
  }

  /// Unlocks a previously confirmed time (owner only).
  Future<void> unlockTime({required String experienceId}) async {
    await _service.unlockTime(experienceId: experienceId);
    await _invalidateExperience(experienceId);
  }

  /// Cancels the active time poll for an experience.
  ///
  /// Only the experience owner can cancel a poll.
  /// Deletes all proposals and clears the time_poll_active flag.
  /// Invalidates the experience cache and notifies daily cache.
  Future<void> cancelTimePoll({required String experienceId}) async {
    await _service.cancelTimePoll(experienceId: experienceId);
    await _invalidateExperienceAndDaily(experienceId);
  }

  /// Deletes a proposal from the time poll. Owner may delete any
  /// proposal; participants may delete only their own.
  Future<void> deleteTimeProposal({
    required String experienceId,
    required String proposalId,
  }) async {
    await _service.deleteTimeProposal(
      experienceId: experienceId,
      proposalId: proposalId,
    );
    await _invalidateExperience(experienceId);
  }

  /// Sets the reply-by deadline on the active time poll. Pass
  /// `deadlineUnixSec=0` to clear an existing deadline.
  Future<void> setTimePollDeadline({
    required String experienceId,
    required int deadlineUnixSec,
  }) async {
    await _service.setTimePollDeadline(
      experienceId: experienceId,
      deadlineUnixSec: deadlineUnixSec,
    );
    await _invalidateExperience(experienceId);
  }

  /// Locks or unlocks the proposal list on the active time poll.
  Future<void> lockTimeProposals({
    required String experienceId,
    required bool locked,
  }) async {
    await _service.lockTimeProposals(
      experienceId: experienceId,
      locked: locked,
    );
    await _invalidateExperience(experienceId);
  }

  /// Pings RSVPs who have not voted on the active time poll. Returns the
  /// count of users notified.
  Future<int> nudgeTimePollVoters({required String experienceId}) async {
    return _service.nudgeTimePollVoters(experienceId: experienceId);
  }

  /// Extracts and parses one or more candidate times from a free-form
  /// text message. Used by the propose modal's bulk-paste shortcut.
  Future<List<ExperienceTime>> extractTimeCandidates({
    required String text,
    int? currentTimeUnixSec,
    String? timezone,
  }) {
    return _service.extractTimeCandidates(
      text: text,
      currentTimeUnixSec: currentTimeUnixSec,
      timezone: timezone,
    );
  }

  // Location-poll mutations: see experience_repository_location_poll.dart.

  Future<void> _invalidateExperience(String experienceId) async {
    await _cache.invalidatePattern('experience:$experienceId*');
    await invalidate(experienceId);
    _onContentInvalidated?.call();
  }

  Future<void> _invalidateExperienceAndDaily(String experienceId) async {
    await _invalidateExperience(experienceId);
    _onDailyInvalidated?.call();
  }

  // ── Collaborative lists ──────────────────────────────────────────────────

  /// Lists all needs and contributions for an experience.
  ///
  /// Results are cached under [needs_and_contributions:<experienceId>].
  /// Call [refreshNeedsAndContributions] after any mutation.
  Future<ListExperienceNeedsAndContributionsResponse> listNeedsAndContributions(
    String experienceId,
  ) async {
    return _cache.get(
      key: 'needs_and_contributions:$experienceId',
      fetch: () =>
          _service.listNeedsAndContributions(experienceId: experienceId),
    );
  }

  /// Invalidates and re-fetches needs and contributions for an experience.
  Future<ListExperienceNeedsAndContributionsResponse>
  refreshNeedsAndContributions(String experienceId) async {
    await _cache.invalidate('needs_and_contributions:$experienceId');
    return listNeedsAndContributions(experienceId);
  }

  /// Adds a need to an experience (any participant).
  ///
  /// Invalidates the needs/contributions cache and notifies daily cache.
  Future<ExperienceNeedResponse> addNeed({
    required String experienceId,
    required String name,
    String? note,
    int slots = 1,
  }) async {
    final need = await _service.addNeed(
      experienceId: experienceId,
      name: name,
      note: note,
      slots: slots,
    );
    await _cache.invalidate('needs_and_contributions:$experienceId');
    _onDailyInvalidated?.call();
    return need;
  }

  /// Removes a need from an experience (proposer only).
  ///
  /// Invalidates the needs/contributions cache and notifies daily cache.
  Future<void> removeNeed({
    required String needId,
    required String experienceId,
  }) async {
    await _service.removeNeed(needId: needId, experienceId: experienceId);
    await _cache.invalidate('needs_and_contributions:$experienceId');
    _onDailyInvalidated?.call();
  }

  /// Updates an existing need's name, note, or slot count in place
  /// (proposer only).
  Future<ExperienceNeedResponse> updateNeed({
    required String needId,
    required String experienceId,
    String? name,
    String? note,
    int? slots,
  }) async {
    final need = await _service.updateNeed(
      needId: needId,
      experienceId: experienceId,
      name: name,
      note: note,
      slots: slots,
    );
    await _cache.invalidate('needs_and_contributions:$experienceId');
    _onDailyInvalidated?.call();
    return need;
  }

  /// Nudges YES/MAYBE RSVPs who haven't created a contribution yet
  /// (organizer only). Returns the number of users notified.
  Future<int> nudgeUncoveredNeedClaimers({required String experienceId}) async {
    return _service.nudgeUncoveredNeedClaimers(experienceId: experienceId);
  }

  /// Claims a need slot, creating a linked contribution (any participant).
  ///
  /// Invalidates the needs/contributions cache and notifies daily cache.
  Future<ExperienceContributionResponse> claimNeed({
    required String needId,
    required String experienceId,
    String? note,
    String? gearId,
  }) async {
    final contribution = await _service.claimNeed(
      needId: needId,
      experienceId: experienceId,
      note: note,
      gearId: gearId,
    );
    await _cache.invalidate('needs_and_contributions:$experienceId');
    _onDailyInvalidated?.call();
    return contribution;
  }

  /// Returns a claimed contribution slot (contributor only).
  ///
  /// Invalidates the needs/contributions cache and notifies daily cache.
  Future<void> unclaimNeed({
    required String contributionId,
    required String experienceId,
  }) async {
    await _service.unclaimNeed(
      contributionId: contributionId,
      experienceId: experienceId,
    );
    await _cache.invalidate('needs_and_contributions:$experienceId');
    _onDailyInvalidated?.call();
  }

  /// Adds multiple needs to an experience in a single call (any participant).
  ///
  /// Invalidates the needs/contributions cache and notifies daily cache.
  Future<List<ExperienceNeedResponse>> addNeedsBatch({
    required String experienceId,
    required List<BatchNeedItem> items,
  }) async {
    final needs = await _service.batchAddNeeds(
      experienceId: experienceId,
      items: items,
    );
    await _cache.invalidate('needs_and_contributions:$experienceId');
    _onDailyInvalidated?.call();
    return needs;
  }

  /// Adds a free-form contribution to an experience (any participant).
  ///
  /// Invalidates the needs/contributions cache and notifies daily cache.
  Future<ExperienceContributionResponse> addContribution({
    required String experienceId,
    required String title,
    String? description,
    String? gearId,
  }) async {
    final contribution = await _service.addContribution(
      experienceId: experienceId,
      title: title,
      description: description,
      gearId: gearId,
    );
    await _cache.invalidate('needs_and_contributions:$experienceId');
    _onDailyInvalidated?.call();
    return contribution;
  }

  /// Adds multiple free-form contributions to an experience in a single call (any participant).
  ///
  /// Invalidates the needs/contributions cache and notifies daily cache.
  Future<List<ExperienceContributionResponse>> addContributionsBatch({
    required String experienceId,
    required List<BatchContributionItem> items,
  }) async {
    final contributions = await _service.batchAddContributions(
      experienceId: experienceId,
      items: items,
    );
    await _cache.invalidate('needs_and_contributions:$experienceId');
    _onDailyInvalidated?.call();
    return contributions;
  }

  /// Edits an existing contribution (contributor only).
  ///
  /// Invalidates the needs/contributions cache and notifies daily cache.
  Future<ExperienceContributionResponse> editContribution({
    required String contributionId,
    required String experienceId,
    required String title,
    String? description,
    String? gearId,
    bool clearGearId = false,
  }) async {
    final contribution = await _service.editContribution(
      contributionId: contributionId,
      experienceId: experienceId,
      title: title,
      description: description,
      gearId: gearId,
      clearGearId: clearGearId,
    );
    await _cache.invalidate('needs_and_contributions:$experienceId');
    _onDailyInvalidated?.call();
    return contribution;
  }

  /// Removes a contribution (contributor only).
  ///
  /// Invalidates the needs/contributions cache and notifies daily cache.
  Future<void> removeContribution({
    required String contributionId,
    required String experienceId,
  }) async {
    await _service.removeContribution(
      contributionId: contributionId,
      experienceId: experienceId,
    );
    await _cache.invalidate('needs_and_contributions:$experienceId');
    _onDailyInvalidated?.call();
  }
}
