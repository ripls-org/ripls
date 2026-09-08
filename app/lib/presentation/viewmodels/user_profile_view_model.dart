import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:image_picker/image_picker.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/utils/safe_notifier.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/data/repositories/community_repository.dart';
import 'package:ripls/data/repositories/user_repository.dart';
import 'package:ripls/presentation/viewmodels/feed_view_model.dart';
import 'package:ripls/presentation/viewmodels/profile_metrics_view_model.dart';
import 'package:ripls/presentation/viewmodels/user_profile_state.dart';
import 'package:ripls/services/providers.dart';

/// ViewModel for user profile screen.
///
/// This ViewModel manages the state for viewing a user's profile, including:
/// - User profile data
/// - User's communities
class UserProfileViewModel extends Notifier<UserProfileState>
    with SafeNotifierMixin<UserProfileState> {
  late UserRepository _userRepository;
  late CommunityRepository _communityRepository;
  String? _userId;
  final ImagePicker _imagePicker = ImagePicker();

  @override
  UserProfileState build() {
    // Get repositories from providers
    _userRepository = ref.watch(userRepositoryProvider);
    _communityRepository = ref.watch(communityRepositoryProvider);

    // Set up cleanup when provider is disposed
    ref.onDispose(() {
      _userId = null;
    });

    return const UserProfileState();
  }

  /// Initialize the viewmodel with a user ID.
  ///
  /// Must be called before using the viewmodel.
  Future<void> initialize(String userId) async {
    _userId = userId;
    await _loadAllData(userId);
  }

  /// Loads all user profile data in parallel.
  Future<void> _loadAllData(String userId) async {
    await Future.wait([
      _loadUser(userId),
      _loadCommunities(),
    ]);

    // Load activities after communities are loaded (needs community IDs)
    await _loadActivities(userId);
  }

  /// Loads user profile data with media URL and all media URLs for carousel.
  Future<void> _loadUser(String userId) async {
    state = state.copyWith(isLoadingUser: true, userError: null);
    try {
      final userProfile = await _userRepository.getUserProfile(userId);
      final mediaUrls = await _userRepository.getUserMediaUrls(userId);
      safeUpdateState(
        (s) => s.copyWith(
          user: userProfile.user,
          mediaUrl: userProfile.mediaUrl,
          mediaUrls: mediaUrls,
          isLoadingUser: false,
        ),
      );
    } catch (e) {
      safeUpdateState(
        (s) => s.copyWith(
          userError: RpcErrorHandler.classify(e, fallback: 'Could not load user profile'),
          isLoadingUser: false,
        ),
      );
    }
  }

  /// Loads user's communities.
  Future<void> _loadCommunities() async {
    state = state.copyWith(isLoadingCommunities: true, communitiesError: null);
    try {
      safeUpdateState((s) => s.copyWith(isLoadingCommunities: false));
    } catch (e) {
      safeUpdateState(
        (s) => s.copyWith(
          communitiesError: RpcErrorHandler.classify(e, fallback: 'Could not load communities'),
          isLoadingCommunities: false,
        ),
      );
    }
  }

  /// Loads recent activities (events) across all user's communities.
  ///
  /// Fetches events from each community in parallel and filters for events
  /// where the user is the actor (events they triggered).
  ///
  /// Note: Only loads events from communities the authenticated user has access to.
  Future<void> _loadActivities(String userId) async {
    state = state.copyWith(isLoadingActivities: true, activitiesError: null);

    try {
      final communities = state.communities;
      if (communities.isEmpty) {
        safeUpdateState(
          (s) => s.copyWith(recentActivities: [], isLoadingActivities: false),
        );
        return;
      }

      // Fetch events from each community, ignoring permission errors
      final eventResults = await Future.wait(
        communities.map((community) async {
          try {
            return await _communityRepository.getEvents(community.id);
          } catch (e) {
            // Ignore permission errors for communities user can't access
            return <CommunityEventItem>[];
          }
        }),
      );

      if (!ref.mounted) return;

      // Flatten all events into a single list
      final flatEvents = eventResults.expand((events) => events).toList();

      // Filter for events where this user is the actor
      final userEvents = flatEvents
          .where((event) => event.actor.id == userId)
          .toList();

      // Sort by timestamp descending (most recent first)
      userEvents.sort(
        (a, b) => b.occurredAtUnixSec.compareTo(a.occurredAtUnixSec),
      );

      // Take top 20 most recent activities
      final recentActivities = userEvents.take(20).toList();

      safeUpdateState(
        (s) => s.copyWith(
          recentActivities: recentActivities,
          isLoadingActivities: false,
        ),
      );
    } catch (e) {
      safeUpdateState(
        (s) => s.copyWith(
          activitiesError: RpcErrorHandler.classify(e, fallback: 'Could not load activities'),
          isLoadingActivities: false,
        ),
      );
    }
  }

  /// Refreshes all data by invalidating caches and reloading.
  Future<void> refresh() async {
    if (_userId == null) return;

    // Invalidate caches
    await _communityRepository.refreshUserCommunities();

    // Reload data (user data doesn't have a refresh method, so just reload)
    await _loadAllData(_userId!);
  }

  /// Refreshes only user data.
  Future<void> refreshUser() async {
    if (_userId != null) {
      await _loadUser(_userId!);
    }
  }

  
  /// Saves user profile changes (name, description, mediaIds).
  ///
  /// This method handles the mutation and automatically refreshes the user data.
  Future<void> saveUserProfile({
    String? name,
    String? description,
    List<String>? mediaIds,
  }) async {
    if (_userId == null) return;

    state = state.copyWith(isSaving: true);

    try {
      // Get current auth state to check if we're editing the current user
      final authState = ref.read(authStateProvider);
      final isCurrentUser = authState.user?.id == _userId;
      final oldMediaId = state.user?.mediaId;

      // Invalidate old media cache if mediaIds are being updated
      if (mediaIds != null && oldMediaId != null && oldMediaId.isNotEmpty) {
        final mediaRepository = ref.read(mediaRepositoryProvider);
        await mediaRepository.invalidate(oldMediaId);
        ref.invalidate(mediaUrlProvider(oldMediaId));
      }

      await _userRepository.saveUser(
        userId: _userId!,
        name: name,
        description: description,
        mediaIds: mediaIds,
      );

      // Invalidate the userProfileProvider cache (used by UserAvatar widgets)
      ref.invalidate(userProfileProvider(_userId!));

      // Invalidate profileImpactMetricsProvider to refresh profile background image
      ref.invalidate(profileImpactMetricsProvider(_userId!));

      // Refresh user data to get updated profile
      await refreshUser();

      // If editing current user, update auth state and invalidate new media cache
      if (isCurrentUser && state.user != null) {
        final authNotifier = ref.read(authStateProvider.notifier);

        // Construct User object from GetUserResponse
        final updatedUser = User(
          id: state.user!.userId,
          name: state.user!.name,
          mediaId: state.user!.mediaId,
        );
        await authNotifier.updateUser(updatedUser);

        // Invalidate new media cache to ensure fresh image loads
        final newMediaId = state.user!.mediaId;
        if (newMediaId.isNotEmpty) {
          final mediaRepository = ref.read(mediaRepositoryProvider);
          await mediaRepository.invalidate(newMediaId);
          ref.invalidate(mediaUrlProvider(newMediaId));
        }

        // Invalidate feed to update user avatars in all feed items
        // This ensures that gear/experiences/etc. posted by current user show updated avatar
        unawaited(ref.read(feedProvider.notifier).refresh());
      }

      state = state.copyWith(isSaving: false);
    } catch (e) {
      state = state.copyWith(isSaving: false);
      rethrow;
    }
  }

  
  /// Gets media URL for a media ID.
  Future<String?> getMediaUrl(String mediaId) async {
    final mediaRepo = ref.read(mediaRepositoryProvider);
    final mediaUrl = await mediaRepo.getMediaUrl(mediaId);
    return mediaUrl.url;
  }

  /// Updates the selected tab index.
  ///
  /// This method maintains tab selection in the ViewModel state rather than
  /// local widget state, following the single source of truth principle.
  void selectTab(int index) {
    if (index >= 0 && index <= 2) {
      state = state.copyWith(selectedTabIndex: index);
    }
  }

  
  
  
  /// Picks image from gallery for profile photo.
  ///
  /// `image_picker_for_web` handles the gallery picker on web via the
  /// browser's file chooser. The XFile flows through unchanged on
  /// every platform — `MediaRepository.addMedia` reads it via
  /// `XFile.readAsBytes()`. See #2157.
  Future<void> pickImageFromGallery() async {
    final XFile? image = await _imagePicker.pickImage(
      source: ImageSource.gallery,
      imageQuality: 85,
    );

    if (image != null) {
      await _uploadMedia(image);
    }
  }

  /// Picks image from camera for profile photo.
  ///
  /// On mobile, opens the device camera. On web, image_picker's
  /// `source: camera` triggers the browser's camera-aware file
  /// picker — works on iOS Safari and Android Chrome; on desktop
  /// browsers it falls back to the file chooser.
  Future<void> pickImageFromCamera() async {
    final XFile? image = await _imagePicker.pickImage(
      source: ImageSource.camera,
      imageQuality: 85,
    );

    if (image != null) {
      await _uploadMedia(image);
    }
  }

  /// Uploads media and sets it as the profile photo.
  ///
  /// Uploads the image to the server, then adds it to the user's mediaIds list.
  /// The new image becomes the primary profile photo (first in the list).
  Future<void> _uploadMedia(XFile file) async {
    if (_userId == null || state.user == null) return;

    state = state.copyWith(isUploadingMedia: true);

    try {
      // Upload media to server
      final mediaRepository = ref.read(mediaRepositoryProvider);
      final mediaId = await mediaRepository.addMedia(
        file: file,
        description: 'Profile image for ${state.user!.name}',
      );

      // Fetch the media URL
      final mediaUrl = await mediaRepository.getFullMediaUrl(mediaId);

      // Add the new mediaId to the beginning of the list (becomes primary photo)
      final currentMediaIds = List<String>.from(state.user!.mediaIds);
      currentMediaIds.insert(0, mediaId);

      // Update local state immediately with the new photo
      state = state.copyWith(mediaUrl: mediaUrl.url, isUploadingMedia: false);

      // Save the updated mediaIds list to the server
      await saveUserProfile(mediaIds: currentMediaIds);
    } catch (e) {
      state = state.copyWith(isUploadingMedia: false);
      rethrow;
    }
  }
}

/// Provider for UserProfileViewModel.
///
/// This provider uses autoDispose to clean up when no longer used.
final userProfileViewModelProvider =
    NotifierProvider.autoDispose<UserProfileViewModel, UserProfileState>(
      UserProfileViewModel.new,
    );

