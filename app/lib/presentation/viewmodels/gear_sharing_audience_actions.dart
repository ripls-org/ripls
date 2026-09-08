import 'package:connectrpc/connect.dart' show Code;
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/data/gen/ripls/api/gear.pb.dart' show Availability;
import 'package:ripls/presentation/viewmodels/gear_sharing_state.dart';
import 'package:ripls/services/providers.dart';

final _log = Logger('GearSharingAudienceActions');

/// GearSharingAudienceMixin provides community sharing management for
/// [GearSharingNotifier].
///
/// Covers loading the owner's communities, querying which ones the gear
/// is already shared with, and setting per-community availability.
mixin GearSharingAudienceMixin on Notifier<GearSharingState> {
  /// loadUserCommunities fetches all communities the owner belongs to and
  /// then loads the current sharing status for this gear.
  ///
  /// The community list is user-scoped, so it loads regardless of whether
  /// the notifier has been initialized with gear context. The follow-up
  /// [loadSharingStatus] guards itself for missing gear state.
  Future<void> loadUserCommunities() async {
    _log.info('📥 Loading user communities for gear sharing');

    state = state.copyWith(isLoadingCommunities: true, communitiesError: null);

    try {
      final communityRepository = ref.read(communityRepositoryProvider);
      final communities = await communityRepository.listUserCommunities();

      _log.info('✅ Loaded ${communities.length} user communities');

      state = state.copyWith(
        userCommunities: communities,
        isLoadingCommunities: false,
      );

      // After loading communities, load the sharing status
      await loadSharingStatus();
    } on ServiceException catch (e) {
      _log.severe('❌ ServiceException loading communities: ${e.message}', e);
      state = state.copyWith(
        communitiesError: RpcErrorHandler.classify(e),
        isLoadingCommunities: false,
      );
    } catch (e, stackTrace) {
      _log.severe(
        '❌ Unexpected exception loading communities: $e',
        e,
        stackTrace,
      );
      state = state.copyWith(
        communitiesError: RpcErrorHandler.classify(e),
        isLoadingCommunities: false,
      );
    }
  }

  /// loadSharingStatus determines which of the owner's communities this gear
  /// is currently shared with and what availability settings are configured.
  Future<void> loadSharingStatus() async {
    final gear = state.gear;
    if (gear == null || !state.isOwner) return;

    _log.info('📥 Loading sharing status for gear: ${gear.id}');

    try {
      final communityRepository = ref.read(communityRepositoryProvider);
      final sharedIds = <String>{};
      final availabilityMap = <String, Availability>{};

      // Check each community to see if this gear is shared with it
      for (final community in state.userCommunities) {
        try {
          final gearItems = await communityRepository.listCommunityGear(
            community.id,
          );
          // Check if our gear is in the list
          final matchingGear = gearItems.where((item) => item.id == gear.id);
          if (matchingGear.isNotEmpty) {
            sharedIds.add(community.id);
            availabilityMap[community.id] = matchingGear.first.availability;
            _log.fine(
              '✅ Gear shared with "${community.name}" (availability: ${matchingGear.first.availability.name})',
            );
          }
        } catch (e) {
          // Ignore errors for individual communities
          _log.warning(
            '⚠️ Error loading gear for community ${community.id}: $e',
          );
        }
      }

      _log.info('✅ Gear is shared with ${sharedIds.length} communities');

      state = state.copyWith(
        sharedCommunityIds: sharedIds,
        communityAvailability: availabilityMap,
      );
    } catch (e, stackTrace) {
      _log.severe('❌ Error loading sharing status: $e', e, stackTrace);
    }
  }

  /// setCommunityAvailability shares or unshares this gear with a community.
  ///
  /// If [availability] is null, the gear is unshared from the community.
  /// Otherwise, the gear is shared with the specified availability setting.
  /// Returns null on success, error message on failure.
  Future<String?> setCommunityAvailability({
    required String communityId,
    Availability? availability,
  }) async {
    final gear = state.gear;
    if (gear == null || !state.isOwner) {
      return 'Cannot modify sharing: not owner or gear not initialized';
    }

    // Mark this community as loading
    state = state.copyWith(
      loadingCommunityIds: {...state.loadingCommunityIds, communityId},
    );

    try {
      final communityRepository = ref.read(communityRepositoryProvider);

      if (availability == null) {
        // Unshare from community
        _log.info('🚫 Unsharing gear ${gear.id} from community $communityId');
        await communityRepository.unshareGear(
          gearId: gear.id,
          communityId: communityId,
        );

        // Update local state
        final updatedSharedIds = Set<String>.from(state.sharedCommunityIds);
        updatedSharedIds.remove(communityId);

        final updatedAvailability = Map<String, Availability>.from(
          state.communityAvailability,
        );
        updatedAvailability.remove(communityId);

        state = state.copyWith(
          sharedCommunityIds: updatedSharedIds,
          communityAvailability: updatedAvailability,
        );

        _log.info('✅ Successfully unshared gear from community');
      } else {
        // Share with community
        _log.info(
          '✅ Sharing gear ${gear.id} with community $communityId (${availability.name})',
        );
        await communityRepository.shareGear(
          gearId: gear.id,
          communityId: communityId,
          availability: availability,
        );

        // Update local state
        final updatedSharedIds = Set<String>.from(state.sharedCommunityIds);
        updatedSharedIds.add(communityId);

        final updatedAvailability = Map<String, Availability>.from(
          state.communityAvailability,
        );
        updatedAvailability[communityId] = availability;

        state = state.copyWith(
          sharedCommunityIds: updatedSharedIds,
          communityAvailability: updatedAvailability,
        );

        _log.info('✅ Successfully shared gear with community');
      }

      // Remove from loading
      final updatedLoadingIds = Set<String>.from(state.loadingCommunityIds);
      updatedLoadingIds.remove(communityId);
      state = state.copyWith(loadingCommunityIds: updatedLoadingIds);

      return null; // Success
    } on ServiceException catch (e) {
      // already_exists means the gear is already shared with this community —
      // treat it as success and sync local state rather than surfacing an error.
      if (availability != null && e.code == Code.alreadyExists) {
        _log.info(
          'ℹ️ Gear already shared with $communityId — syncing local state',
        );
        final updatedSharedIds = Set<String>.from(state.sharedCommunityIds)
          ..add(communityId);
        final updatedAvailability =
            Map<String, Availability>.from(state.communityAvailability)
              ..[communityId] = availability;
        final updatedLoadingIds = Set<String>.from(state.loadingCommunityIds)
          ..remove(communityId);
        state = state.copyWith(
          sharedCommunityIds: updatedSharedIds,
          communityAvailability: updatedAvailability,
          loadingCommunityIds: updatedLoadingIds,
        );
        return null;
      }

      _log.severe('❌ ServiceException setting availability: ${e.message}', e);

      // Remove from loading
      final updatedLoadingIds = Set<String>.from(state.loadingCommunityIds);
      updatedLoadingIds.remove(communityId);
      state = state.copyWith(loadingCommunityIds: updatedLoadingIds);

      return e.message;
    } catch (e, stackTrace) {
      _log.severe(
        '❌ Unexpected exception setting availability: $e',
        e,
        stackTrace,
      );

      // Remove from loading
      final updatedLoadingIds = Set<String>.from(state.loadingCommunityIds);
      updatedLoadingIds.remove(communityId);
      state = state.copyWith(loadingCommunityIds: updatedLoadingIds);

      return 'Failed to update sharing: $e';
    }
  }
}
