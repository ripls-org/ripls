import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/data/gen/ripls/api/location_service.pb.dart';
import 'package:ripls/data/repositories/location_repository.dart';
import 'package:ripls/services/providers.dart';
import 'package:ripls/services/user_service.dart';

part 'profile_locations_view_model.freezed.dart';

final _log = Logger('ProfileLocationsViewModel');

/// State for the Profile Locations screen
@freezed
sealed class ProfileLocationsState with _$ProfileLocationsState {
  const factory ProfileLocationsState({
    @Default([]) List<UserLocationWithDetails> locations,
    String? primaryLocationId,
    @Default(true) bool isLoading,
    UserError? error,
  }) = _ProfileLocationsState;

  const ProfileLocationsState._();

  bool get hasError => error != null;
  bool get isEmpty => locations.isEmpty && !isLoading;
  bool get hasLocations => locations.isNotEmpty;
}

/// Notifier for managing the Profile Locations screen state
class ProfileLocationsNotifier extends Notifier<ProfileLocationsState> {
  @override
  ProfileLocationsState build() {
    // Defer loading until after build completes to avoid circular dependency
    Future.microtask(() => _loadLocations());
    return const ProfileLocationsState();
  }

  LocationRepository get _locationRepository =>
      ref.read(locationRepositoryProvider);

  UserService get _userService => ref.read(userServiceProvider);

  /// Loads user's locations sorted by last_used_at
  Future<void> _loadLocations() async {
    state = state.copyWith(isLoading: true, error: null);

    try {
      // Invalidate cache to ensure fresh data
      await _locationRepository.invalidateUserLocationsCache();

      // Get authenticated user to fetch primary location ID
      final authState = ref.read(authStateProvider);
      final userId = authState.user?.id;

      if (userId == null) {
        state = state.copyWith(
          isLoading: false,
          error: const UserError.generic(fallback: 'User not authenticated'),
        );
        return;
      }

      // Fetch user to get primary location ID
      final user = await _userService.getUser(userId);
      final primaryLocationId = user.primaryResidenceLocationId;

      // Fetch user locations sorted by last_used_at (limit: 50)
      final locations = await _locationRepository.getUserLocations(limit: 50);

      if (!ref.mounted) return;

      state = state.copyWith(
        locations: locations,
        primaryLocationId: primaryLocationId,
        isLoading: false,
      );
    } catch (e, stackTrace) {
      _log.severe('Failed to load locations', e, stackTrace);
      if (!ref.mounted) return;

      state = state.copyWith(
        isLoading: false,
        error: RpcErrorHandler.classify(e, fallback: 'Could not load locations'),
      );
    }
  }

  /// Sets a location as the primary location
  Future<void> setPrimaryLocation(String locationId) async {
    try {
      final authState = ref.read(authStateProvider);
      final userId = authState.user?.id;

      if (userId == null) {
        state = state.copyWith(
          error: const UserError.generic(fallback: 'User not authenticated'),
        );
        return;
      }

      // Update user's primary residence location
      await _userService.saveUser(
        userId: userId,
        primaryResidenceLocationId: locationId,
      );

      // Refresh locations to reflect new primary
      await refresh();
    } catch (e) {
      _log.severe('Failed to set primary location', e);
      state = state.copyWith(
        error: RpcErrorHandler.classify(e, fallback: 'Could not set primary location'),
      );
    }
  }

  /// Deletes a location from the user's list
  Future<void> deleteLocation(String locationId) async {
    try {
      // Remove from user's location list (soft delete UserLocation entry)
      await _locationRepository.removeUserLocation(locationId);

      // Reload locations
      await _loadLocations();
    } catch (e) {
      _log.severe('Failed to delete location', e);
      state = state.copyWith(
        error: RpcErrorHandler.classify(e, fallback: 'Could not delete location'),
      );
    }
  }

  /// Refreshes the location list (pull-to-refresh)
  Future<void> refresh() async {
    await _loadLocations();
  }

  /// Clears any error messages
  void clearError() {
    state = state.copyWith(error: null);
  }
}

/// Provider for the Profile Locations screen state
final profileLocationsProvider =
    NotifierProvider.autoDispose<ProfileLocationsNotifier, ProfileLocationsState>(
      ProfileLocationsNotifier.new,
    );
