// Coordination pattern: [ConversationNotifier] is the single Riverpod provider.
// It composes behaviour from five mixins:
//   ConversationMessagesMixin     — stream, send, retry, reactions
//   ConversationTransferActionsMixin — transfer/loan lifecycle
//   ConversationRequestActionsMixin  — request lifecycle
//   ConversationExperienceActionsMixin — experience lifecycle
//   ConversationAttachmentsMixin  — pending attachment staging/upload
//
// All mixins share [state] (the coordinator's own [Notifier.state]) and
// communicate via non-private abstract method declarations. There is exactly
// one [conversationProvider] family so call sites are unchanged.
import 'dart:async';

import 'package:flutter/material.dart' show ImageProvider;
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/utils/video_cache_helper.dart';
import 'package:ripls/data/gen/ripls/api/chat_service.pb.dart'
    show ConversationItem;
import 'package:ripls/data/gen/ripls/api/conversation.pb.dart';
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart'
    show ImpactEstimate;
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart' show TransferState;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/data/repositories/media_url.dart';
import 'package:ripls/data/repositories/search_repository.dart';
import 'package:ripls/presentation/viewmodels/community_edit_view_model.dart'
    show communityEditProvider;
import 'package:ripls/presentation/viewmodels/conversation_attachments_notifier.dart';
import 'package:ripls/presentation/viewmodels/conversation_experience_actions.dart';
import 'package:ripls/presentation/viewmodels/conversation_messages_notifier.dart';
import 'package:ripls/presentation/viewmodels/conversation_request_actions.dart';
// Mixin imports — one per concern.
import 'package:ripls/presentation/viewmodels/conversation_state.dart';
import 'package:ripls/presentation/viewmodels/conversation_transfer_actions.dart';
import 'package:ripls/presentation/viewmodels/gear_view_model.dart'
    show gearProvider;
import 'package:ripls/presentation/widgets/chat/mention/mention_suggestion_converter.dart';
import 'package:ripls/presentation/widgets/chat/mention/mention_types.dart';
import 'package:ripls/services/providers.dart';
import 'package:video_player/video_player.dart';

// Re-export ConversationState so existing importers of this file work unchanged.
export 'package:ripls/presentation/viewmodels/conversation_state.dart'
    show ConversationState;

final _log = Logger('ConversationViewModel');

/// ConversationNotifier manages conversation state with real-time messaging.
///
/// Parameterized by conversationId via the family modifier.
/// State mutations from action mixins flow through [state.copyWith].
class ConversationNotifier extends Notifier<ConversationState>
    with
        ConversationMessagesMixin,
        ConversationTransferActionsMixin,
        ConversationRequestActionsMixin,
        ConversationExperienceActionsMixin,
        ConversationAttachmentsMixin {
  /// ConversationNotifier accepts the conversationId parameter from the family modifier.
  ConversationNotifier(this.conversationId);

  /// The conversationId for this specific conversation instance.
  final String conversationId;

  /// maxPendingAttachments is the maximum number of attachments per chat message.
  ///
  /// Exposed here for backwards-compat with callers that reference
  /// [ConversationNotifier.maxPendingAttachments] as a static constant.
  static const int maxPendingAttachments =
      ConversationAttachmentsMixin.maxPendingAttachments;

  // Background video controller (not in Freezed state to avoid equality issues).
  VideoPlayerController? _videoController;

  // Single non-Freezed impact estimate cached after a terminal action
  // (loan completion, request fulfillment, or experience completion).
  // Action mixins write via [setCachedImpact]; readers use [cachedImpact].
  ImpactEstimate? _cachedImpact;

  @override
  ConversationState build() {
    // Register cleanup for stream subscription and timers owned by messages mixin.
    ref.onDispose(() {
      _log.info('🧹 Disposing conversation notifier');
      disposeMessageResources();
      _videoController?.dispose();
    });

    return const ConversationState(
      currentUserId: '',
      currentUserName: '',
      isLoadingMessages: true,
    );
  }

  // ── cachedImpact: shared field set by action mixins ─────────────────────

  /// cachedImpact returns the impact estimate fetched after a terminal action
  /// — loan completion, request fulfillment, or experience completion.
  @override
  ImpactEstimate? get cachedImpact => _cachedImpact;

  /// setCachedImpact stores the impact estimate from a terminal action. Called
  /// by action mixins; not part of the public API.
  @override
  void setCachedImpact(ImpactEstimate? impact) {
    _cachedImpact = impact;
  }

  // ── Initialization ───────────────────────────────────────────────────────

  /// initialize sets up the notifier with required parameters.
  Future<void> initialize({
    required String currentUserId,
    required String currentUserName,
    required ConversationItem conversation,
    int unreadCount = 0,
    ConversationContext? conversationContext,
    Function(String conversationId)? onMessagesMarkedAsRead,
  }) async {
    setOnMessagesMarkedAsRead(onMessagesMarkedAsRead);

    state = ConversationState(
      currentUserId: currentUserId,
      currentUserName: currentUserName,
      conversation: conversation,
      unreadCount: unreadCount,
      conversationContext: conversationContext,
      transferStatusMap: _buildTransferStatusMap(conversationContext),
      isLoadingMessages: true,
    );

    await Future.wait([
      loadMessages(),
      fetchOtherParticipantName(),
      fetchTransferDetails(),
      fetchRequestDetails(),
      fetchExperienceDetails(),
      _resolveBackgroundImage(),
    ]);

    await fetchGearThumbnail();
    await fetchRequestMedia();
    await fetchExperienceMedia();

    _updateParticipants();
    startMessageStream();
  }

  // ── Refresh ──────────────────────────────────────────────────────────────

  /// refresh performs a full invalidation and reload of all conversation data.
  Future<void> refresh() async {
    try {
      await Future.wait([
        ref.read(transferRepositoryProvider).invalidateAll(),
        ref.read(userRepositoryProvider).invalidateAll(),
        ref.read(gearRepositoryProvider).invalidateAll(),
        ref.read(mediaRepositoryProvider).invalidateAll(),
        ref.read(chatRepositoryProvider).invalidateAll(),
        ref.read(requestRepositoryProvider).invalidateAll(),
        ref.read(experienceRepositoryProvider).invalidateAll(),
      ]);

      await loadMessages();

      await Future.wait([
        fetchTransferDetails(),
        fetchRequestDetails(),
        fetchExperienceDetails(),
        fetchGearThumbnail(),
        fetchRequestMedia(),
        fetchExperienceMedia(),
        fetchOtherParticipantName(),
      ]);

      _log.info('✅ Conversation refreshed successfully');
    } catch (e) {
      _log.severe('❌ Failed to refresh conversation: $e');
      rethrow;
    }
  }

  // ── Participant helpers ──────────────────────────────────────────────────

  /// fetchOtherParticipantName fetches the other participant's name from
  /// the UserRepository.
  Future<void> fetchOtherParticipantName() async {
    final conversation = state.conversation;
    if (conversation == null) return;

    final otherParticipant = conversation.participants.firstWhere(
      (participant) => participant.id != state.currentUserId,
      orElse: () => User(),
    );

    if (otherParticipant.id.isEmpty) return;

    try {
      final userRepository = ref.read(userRepositoryProvider);
      final userProfile = await userRepository.getUserProfile(
        otherParticipant.id,
      );

      state = state.copyWith(
        cachedOtherParticipantName: userProfile.user.name.isNotEmpty
            ? userProfile.user.name
            : otherParticipant.id,
      );
    } catch (e) {
      _log.warning('Failed to fetch participant name: $e');
    }
  }

  /// getOtherParticipantName returns the other participant's display name,
  /// or an empty string until [fetchOtherParticipantName] has resolved it.
  String getOtherParticipantName() {
    return state.cachedOtherParticipantName ?? '';
  }

  /// getOtherParticipantId returns the other participant's ID.
  String getOtherParticipantId() {
    return getOtherParticipant()?.id ?? '';
  }

  /// getOtherParticipant returns the other participant's User object.
  User? getOtherParticipant() {
    final conversation = state.conversation;
    if (conversation == null) return null;

    final otherParticipant = conversation.participants.firstWhere(
      (participant) => participant.id != state.currentUserId,
      orElse: () => User(),
    );
    return otherParticipant.id.isNotEmpty ? otherParticipant : null;
  }

  
  // ── Transfer details ─────────────────────────────────────────────────────

  /// fetchTransferDetails fetches transfer details from the repository.
  Future<void> fetchTransferDetails() async {
    final conversation = state.conversation;

    if (conversation == null || !conversation.topic.hasTransferId()) {
      return;
    }

    final transferId = conversation.topic.transferId;
    if (transferId.isEmpty) {
      return;
    }

    try {
      final transferRepository = ref.read(transferRepositoryProvider);
      final transfer = await transferRepository.getTransfer(transferId);

      if (transfer != null) {
        state = state.copyWith(cachedTransfer: transfer);
      } else {
        _log.warning('Transfer not found: $transferId');
      }
    } catch (e) {
      _log.severe('Error fetching transfer details: $e');
    }
  }

  /// fetchGearThumbnail fetches the gear thumbnail URL for the transfer's gear.
  Future<void> fetchGearThumbnail() async {
    final transfer = state.cachedTransfer;
    if (transfer == null || transfer.gearId.isEmpty) {
      _log.info('No gear ID available to fetch thumbnail');
      return;
    }

    try {
      _log.info('Fetching gear details for: ${transfer.gearId}');

      final gearRepository = ref.read(gearRepositoryProvider);
      final gearResponse = await gearRepository.getGearDetails(transfer.gearId);

      _log.info('Gear has ${gearResponse.mediaIds.length} media items');

      if (gearResponse.mediaIds.isEmpty) {
        _log.info('No media associated with gear: ${transfer.gearId}');
        return;
      }

      final firstMediaId = gearResponse.mediaIds.first;
      _log.info('Fetching media thumbnail for: $firstMediaId');

      final mediaRepository = ref.read(mediaRepositoryProvider);
      final mediaUrl = await mediaRepository.getMediaUrl(firstMediaId);

      state = state.copyWith(gearThumbnailUrl: mediaUrl.url);

      _log.info(
        'Gear thumbnail URL fetched - using '
        '${mediaUrl.isThumbnail ? "THUMBNAIL" : "FULL"}: ${mediaUrl.url}',
      );
    } catch (e) {
      _log.warning('Failed to fetch gear thumbnail: $e');
    }
  }

  /// selectRecipient selects a recipient for a transfer (sharer only).
  Future<void> selectRecipient(String recipientId) async {
    final transferId = state.cachedTransfer?.id ?? '';
    if (transferId.isEmpty) {
      throw Exception('Transfer ID not found');
    }

    try {
      final transferRepository = ref.read(transferRepositoryProvider);
      await transferRepository.selectRecipient(
        transferId: transferId,
        recipientId: recipientId,
      );

      await fetchTransferDetails();
      await refreshMessagesAfterMutation();
    } catch (e) {
      _log.severe('Failed to select recipient: $e');
      rethrow;
    }
  }

  // ── Conversation context ─────────────────────────────────────────────────

  /// fetchConversationContext fetches fresh conversation context and updates
  /// the transfer status map.
  @override
  Future<void> fetchConversationContext() async {
    final cId = state.conversation?.conversationId;
    if (cId == null) return;

    try {
      final chatRepository = ref.read(chatRepositoryProvider);
      await chatRepository.refreshConversationContext(cId);

      final updatedContext = await chatRepository.getConversationContext(
        conversationId: cId,
      );

      if (updatedContext.hasGearTransferContext()) {
        final gearContext = updatedContext.gearTransferContext;
        _log.info(
          '🔍 Updated gear context - hasUserTransfer: '
          '${gearContext.hasUserTransfer()}, '
          'pendingRequests: ${gearContext.pendingRequests.length}, '
          'availableActions: ${gearContext.availableActions}',
        );
      }

      state = state.copyWith(
        conversationContext: updatedContext,
        transferStatusMap: _buildTransferStatusMap(updatedContext),
      );

      _log.info('✅ Conversation context refreshed');
    } catch (e) {
      _log.warning('⚠️  Failed to fetch conversation context: $e');
    }
  }

  /// _buildTransferStatusMap builds a map of user IDs to their transfer states
  /// from the conversation context, used for status badges in chat messages.
  Map<String, TransferState> _buildTransferStatusMap(
    ConversationContext? context,
  ) {
    final transferMap = <String, TransferState>{};

    if (context == null || !context.hasGearTransferContext()) {
      return transferMap;
    }

    final gearContext = context.gearTransferContext;

    if (gearContext.hasUserTransfer()) {
      final userTransfer = gearContext.userTransfer;
      transferMap[userTransfer.recipient.id] = userTransfer.state;
    }

    for (final request in gearContext.pendingRequests) {
      transferMap[request.borrower.id] =
          TransferState.TRANSFER_STATE_INTEREST_EXPRESSED;
    }

    return transferMap;
  }

  // ── Parent entity cache invalidation ────────────────────────────────────

  /// invalidateParentEntityCacheIfNeeded invalidates the parent entity cache
  /// when a message with media is received.
  @override
  void invalidateParentEntityCacheIfNeeded() {
    final context = state.conversationContext;
    final conversation = state.conversation;

    if (context == null) {
      if (conversation != null) {
        if (conversation.topic.hasRequestId()) {
          invalidateAndRefreshRequest(conversation.topic.requestId);
          return;
        } else if (conversation.topic.hasGearId()) {
          _invalidateAndRefreshGear(conversation.topic.gearId);
          return;
        } else if (conversation.topic.hasExperienceId()) {
          invalidateAndRefreshExperience();
          return;
        } else if (conversation.topic.hasCommunityId()) {
          _invalidateAndRefreshCommunity(conversation.topic.communityId);
          return;
        } else if (conversation.topic.hasTransferId() &&
            state.cachedTransfer != null) {
          _invalidateAndRefreshGear(state.cachedTransfer!.gearId);
          return;
        }
      }
      return;
    }

    if (context.topic.hasGearId()) {
      _invalidateAndRefreshGear(context.topic.gearId);
    } else if (context.topic.hasTransferId() && state.cachedTransfer != null) {
      _invalidateAndRefreshGear(state.cachedTransfer!.gearId);
    } else if (context.topic.hasExperienceId()) {
      invalidateAndRefreshExperience();
    } else if (context.topic.hasRequestId()) {
      invalidateAndRefreshRequest(context.topic.requestId);
    } else if (context.topic.hasCommunityId()) {
      _invalidateAndRefreshCommunity(context.topic.communityId);
    }
  }

  /// _invalidateAndRefreshCommunity invalidates the community cache and
  /// reloads CommunityContentView's media list. Called after a chat message
  /// with attached media is sent or received in a community-wide conversation
  /// — the server appends those media IDs to the community via
  /// `appendMediaToCommunity`, and this pull syncs the client.
  void _invalidateAndRefreshCommunity(String communityId) {
    if (communityId.isEmpty) return;
    ref.read(communityRepositoryProvider).invalidate(communityId).then((_) {
      if (!ref.mounted) return;
      // Skip if no one is currently watching the provider — ref.read would
      // resurrect it and race the auto-dispose timer (same pattern as gear).
      if (!ref.exists(communityEditProvider(communityId))) return;
      ref
          .read(communityEditProvider(communityId).notifier)
          .reloadMediaFromCommunity();
    }).catchError((Object error) {
      _log.warning('Failed to invalidate community cache: $error');
    });
  }

  /// _invalidateAndRefreshGear invalidates the gear cache and refreshes
  /// GearContentView's media list.
  void _invalidateAndRefreshGear(String gearId) {
    if (gearId.isEmpty) return;
    ref.read(gearRepositoryProvider).invalidate(gearId).then((_) {
      if (!ref.mounted) return;
      // Only refresh if the gear provider is actively watched. Otherwise
      // ref.read resurrects an auto-disposed provider and the resulting
      // refreshGearDetails() races the auto-dispose timer, producing
      // SafeNotifierMixin "State update skipped — notifier disposed" warns.
      if (!ref.exists(gearProvider(gearId))) return;
      ref.read(gearProvider(gearId).notifier).refreshGearDetails();
    }).catchError((Object error) {
      _log.warning('Failed to invalidate gear cache: $error');
    });
  }

  // ── Participant updates ──────────────────────────────────────────────────

  /// _updateParticipants extracts participants from the conversation data and
  /// filters out the current user.
  void _updateParticipants() {
    final conversation = state.conversation;
    if (conversation != null) {
      final otherParticipants = conversation.participants
          .where((p) => p.id != state.currentUserId)
          .toList();
      state = state.copyWith(participants: otherParticipants);
      _log.info(
        '👥 Updated participants: ${otherParticipants.length} '
        '(excluding current user)',
      );
    }
  }

  /// refreshParticipantsFromSystemMessage refreshes the participant list
  /// when a JOINED/LEFT system message is received.
  @override
  void refreshParticipantsFromSystemMessage() async {
    final cId = state.conversation?.conversationId;
    if (cId == null) return;

    _log.info('🔄 Refreshing participants due to membership change');

    try {
      final chatRepository = ref.read(chatRepositoryProvider);
      await chatRepository.refreshConversation(cId);
      final updatedConversation = await chatRepository.getConversation(
        conversationId: cId,
      );

      ConversationContext? updatedContext;
      try {
        updatedContext = await chatRepository.getConversationContext(
          conversationId: cId,
        );
      } catch (e) {
        _log.warning('Failed to fetch conversation context: $e');
        updatedContext = state.conversationContext;
      }

      // Fired from the message stream, so the notifier can be disposed
      // while the two fetches above are in flight.
      if (!ref.mounted) return;

      state = state.copyWith(
        conversation: updatedConversation,
        unreadCount: updatedConversation.unreadCount,
        conversationContext: updatedContext,
        transferStatusMap: _buildTransferStatusMap(updatedContext),
      );

      _updateParticipants();

      _log.info('✅ Participants refreshed successfully');
    } catch (e) {
      _log.warning('⚠️  Failed to refresh participants: $e');
    }
  }

  // ── Background image ─────────────────────────────────────────────────────

  /// _resolveBackgroundImage resolves the full-resolution background media URL.
  ///
  /// When the media is a video, initialises a looping muted [VideoPlayerController]
  /// and stores it in state so the screen can render it as an ambient background.
  Future<void> _resolveBackgroundImage() async {
    final mediaId = state.conversationContext?.topicImageMediaId ?? '';
    if (mediaId.isEmpty) return;

    try {
      final mediaRepository = ref.read(mediaRepositoryProvider);
      final mediaUrl = await mediaRepository.getFullMediaUrl(mediaId);

      if (!ref.mounted) return;

      final isVideo = mediaUrl.contentType?.startsWith('video/') ?? false;

      if (isVideo) {
        await _videoController?.dispose();

        final controller = await createCachedVideoController(
          mediaRepository,
          mediaId,
          mediaUrl.url,
        );
        if (!ref.mounted) {
          await controller.dispose();
          return;
        }
        _videoController = controller;
        state = state.copyWith(
          backgroundImageUrl: mediaUrl.url,
          backgroundImageMediaId: mediaId,
          isVideo: true,
          videoController: controller,
        );
      } else {
        state = state.copyWith(
          backgroundImageUrl: mediaUrl.url,
          backgroundImageMediaId: mediaId,
          isVideo: false,
        );
      }
    } catch (e) {
      _log.warning('Failed to resolve background media: $e');
    }
  }

  // ── Media URL helper ─────────────────────────────────────────────────────

  /// getMediaUrl fetches a media URL for a given media ID.
  ///
  /// Returns null if the media cannot be loaded.
  Future<MediaUrl?> getMediaUrl(String mediaId) async {
    try {
      final mediaRepository = ref.read(mediaRepositoryProvider);
      return await mediaRepository.getMediaUrl(mediaId);
    } catch (e) {
      _log.warning('Failed to load media URL for $mediaId: $e');
      return null;
    }
  }

  // ── Mention suggestions ──────────────────────────────────────────────────

  /// fetchMentionSuggestions fetches mention suggestions for autocomplete.
  ///
  /// Returns a list of [MentionSuggestion] objects sorted with users first,
  /// then other entities alphabetically.
  Future<List<MentionSuggestion>> fetchMentionSuggestions(String query) async {
    final communityId = state.conversation?.communityId;
    if (communityId == null || communityId.isEmpty) {
      _log.warning('Cannot fetch mention suggestions: no community ID');
      return [];
    }

    try {
      final searchRepository = ref.read(searchRepositoryProvider);
      final mediaRepository = ref.read(mediaRepositoryProvider);

      final (latitude, longitude) = await _getUserLocationForSearch();

      final results = await searchRepository.search(
        query: query,
        communityIds: [communityId],
        latitudeDeg: latitude,
        longitudeDeg: longitude,
        maxResults: 10,
        strategy: SearchStrategy.SEARCH_STRATEGY_EXACT,
        itemTypes: [
          SearchItemType.SEARCH_ITEM_TYPE_USER,
          SearchItemType.SEARCH_ITEM_TYPE_GEAR,
          SearchItemType.SEARCH_ITEM_TYPE_REQUEST,
          SearchItemType.SEARCH_ITEM_TYPE_EXPERIENCE,
        ],
      );

      final suggestions = MentionSuggestionConverter.fromSearchResults(
        results,
        imageProviderFactory: (mediaId) {
          return _getMentionImageProvider(mediaId, mediaRepository);
        },
      );

      return suggestions;
    } catch (e) {
      _log.warning('Failed to fetch mention suggestions: $e');
      return [];
    }
  }

  /// _getUserLocationForSearch returns the user's location for search queries.
  ///
  /// Falls back to Austin, TX when the primary residence location is unavailable.
  Future<(double, double)> _getUserLocationForSearch() async {
    try {
      final authState = ref.read(authStateProvider);
      final userId = authState.user?.id;

      if (userId != null && userId.isNotEmpty) {
        final userRepository = ref.read(userRepositoryProvider);
        final userResponse = await userRepository.get(userId);

        if (userResponse.primaryResidenceLocationId.isNotEmpty) {
          final locationRepository = ref.read(locationRepositoryProvider);
          final location = await locationRepository.getLocation(
            userResponse.primaryResidenceLocationId,
          );
          return (location.latitudeDeg, location.longitudeDeg);
        }
      }
    } catch (e) {
      _log.fine('Could not get user location: $e');
    }

    // Fallback to Austin, TX (consistent with discover screen fallback).
    return (30.2672, -97.7431);
  }

  /// _getMentionImageProvider creates an image provider for a mention thumbnail.
  ImageProvider? _getMentionImageProvider(
    String mediaId,
    dynamic mediaRepository,
  ) {
    return null;
  }

  // ── Modal helpers ────────────────────────────────────────────────────────
  
  }

/// conversationProvider provides conversation state, parameterized by conversationId.
///
/// Call [ConversationNotifier.initialize] after reading this provider.
final conversationProvider = NotifierProvider.autoDispose
    .family<ConversationNotifier, ConversationState, String>(
      ConversationNotifier.new,
    );
