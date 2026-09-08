import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/location_formatter.dart';
import 'package:ripls/core/utils/map_avatar_renderer.dart';
import 'package:ripls/presentation/models/discover_item.dart';
import 'package:ripls/services/providers.dart';

part 'discover_view_model.freezed.dart';

final _log = Logger('DiscoverViewModel');

/// State for the discover screen
@freezed
sealed class DiscoverState with _$DiscoverState {
  const factory DiscoverState({
    /// Unified list of discover items (gear, requests, and experiences)
    @Default([]) List<DiscoverItem> items,
    @Default(true) bool isLoading,
    UserError? error,
    /// Currently selected item for map view overlay
    DiscoverItem? selectedItem,
    // Cache for owner names by owner ID
    @Default({}) Map<String, String> ownerNames,
    // Track which items are currently loading owner info
    @Default({}) Set<String> loadingOwnerIds,
    // Cache for resolved location names by location ID
    @Default({}) Map<String, String> locationNames,
  }) = _DiscoverState;

  const DiscoverState._();

  /// Returns whether the state has an error
  bool get hasError => error != null;

  /// Returns whether the list is empty (and not loading)
  bool get isEmpty => items.isEmpty && !isLoading;

  /// Returns items with valid location coordinates for map display.
  /// Only includes items that have actual lat/lon coordinates.
  List<DiscoverItem> get itemsWithLocations {
    return items.where((item) {
      return item.when(
        gear: (g) => g.latitudeDeg != 0 && g.longitudeDeg != 0,
        request: (r) => r.latitudeDeg != 0 && r.longitudeDeg != 0,
        experience: (e) => e.latitudeDeg != 0 && e.longitudeDeg != 0,
      );
    }).toList();
  }

  /// Returns the index of the selected item in the items list.
  /// Returns null if no item is selected or if the selected item is not in the list.
  int? get selectedItemIndex {
    if (selectedItem == null) return null;
    final index = items.indexWhere((item) => item.id == selectedItem!.id);
    return index >= 0 ? index : null;
  }
}

/// Notifier for managing discover screen state
class DiscoverNotifier extends Notifier<DiscoverState> {
  @override
  DiscoverState build() {
    return const DiscoverState();
  }

  /// resolveItemThumbnails resolves thumbnail PNG bytes for each item in [items].
  ///
  /// Fetches thumbnail URLs via [MediaRepository] and renders rounded-square
  /// bitmaps via [MapAvatarRenderer].  Returns a map of itemId → PNG bytes.
  /// Items without a thumbnail media ID fall back to an initials bitmap using
  /// the item name.
  Future<Map<String, Uint8List>> resolveItemThumbnails(
    List<DiscoverItem> items,
    BuildContext context,
  ) async {
    final mediaRepository = ref.read(mediaRepositoryProvider);
    // Capture context-dependent value before any async gaps.
    final fallbackColor = AppColors.secondary(context);
    final result = <String, Uint8List>{};

    for (final item in items) {
      final itemId = item.id;
      if (itemId.isEmpty) continue;

      final mediaId = item.thumbnailMediaId;
      try {
        if (mediaId.isNotEmpty) {
          final mediaUrl = await mediaRepository.getMediaUrl(mediaId);
          if (mediaUrl.url.isEmpty) {
            // Video without a thumbnail (or deleted media) — fall through to
            // initials rather than passing an empty URL to renderThumbnail.
            throw ArgumentError('no displayable thumbnail for $mediaId');
          }
          final bytes = await MapAvatarRenderer.renderThumbnail(
            imageUrl: mediaUrl.url,
          );
          result[itemId] = bytes;
        } else {
          // No thumbnail — render initials fallback using item name
          final bytes = await MapAvatarRenderer.renderInitials(
            name: item.name,
            backgroundColor: fallbackColor,
          );
          result[itemId] = bytes;
        }
      } catch (e, stack) {
        // Expected cases: video without thumbnail, deleted media, network errors.
        // Log at fine level to avoid noise; fall back to initials marker.
        _log.fine(
          'No thumbnail for map marker $itemId, using initials: $e',
          e,
          stack,
        );
        // Fallback to initials on error
        try {
          result[itemId] = await MapAvatarRenderer.renderInitials(
            name: item.name,
            backgroundColor: fallbackColor,
          );
        } catch (_) {
          // Ignore — marker will be skipped for this item
        }
      }
    }

    return result;
  }

  /// Clears all state (useful when logging out or switching contexts)
  void clear() {
    state = const DiscoverState(
      items: [],
      isLoading: false,
      error: null,
      selectedItem: null,
    );
    _log.info('Cleared discover state');
  }

  /// Selects an item for display in map view overlay
  void selectItem(DiscoverItem item) {
    state = state.copyWith(selectedItem: item);
    final name = item.when(
      gear: (g) => g.name,
      request: (r) => r.title,
      experience: (e) => e.name,
    );
    _log.info('Selected item: $name');
  }

  /// Clears the selected item
  void clearSelectedItem() {
    state = state.copyWith(selectedItem: null);
    _log.info('Cleared selected item');
  }

  /// Finds an item by ID
  DiscoverItem? findItemById(String itemId) {
    try {
      return state.items.firstWhere((item) => item.id == itemId);
    } catch (e) {
      return null;
    }
  }

  /// Loads owner name for a given owner ID.
  ///
  /// This method fetches the owner's user profile via UserRepository
  /// and caches the result in state. If already cached or loading, does nothing.
  Future<void> loadOwnerName(String ownerId) async {
    // Skip if already cached or loading
    if (state.ownerNames.containsKey(ownerId) ||
        state.loadingOwnerIds.contains(ownerId)) {
      return;
    }

    // Mark as loading
    state = state.copyWith(
      loadingOwnerIds: {...state.loadingOwnerIds, ownerId},
    );

    try {
      final userRepository = ref.read(userRepositoryProvider);
      final user = await userRepository.get(ownerId);

      // Update state with owner name and remove from loading
      final updatedOwnerNames = Map<String, String>.from(state.ownerNames);
      updatedOwnerNames[ownerId] = user.name;

      final updatedLoadingIds = Set<String>.from(state.loadingOwnerIds);
      updatedLoadingIds.remove(ownerId);

      state = state.copyWith(
        ownerNames: updatedOwnerNames,
        loadingOwnerIds: updatedLoadingIds,
      );

      _log.fine('Loaded owner name for $ownerId: ${user.name}');
    } catch (e, stackTrace) {
      _log.warning('Failed to load owner name for $ownerId: $e', e, stackTrace);

      // Remove from loading on error
      final updatedLoadingIds = Set<String>.from(state.loadingOwnerIds);
      updatedLoadingIds.remove(ownerId);

      state = state.copyWith(
        loadingOwnerIds: updatedLoadingIds,
      );
    }
  }

  /// Resolves a location ID to a human-readable name.
  ///
  /// Fetches the location via LocationRepository, formats it with
  /// LocationFormatter.formatLocationNameShort, and stores the result in state.
  /// If already resolved, does nothing.
  Future<void> resolveLocationName(String locationId) async {
    if (state.locationNames.containsKey(locationId)) return;

    try {
      final repo = ref.read(locationRepositoryProvider);
      final location = await repo.getLocation(locationId);
      final name = LocationFormatter.formatLocationNameShort(location);
      state = state.copyWith(
        locationNames: {...state.locationNames, locationId: name},
      );
    } catch (e, stackTrace) {
      _log.warning(
        'Failed to resolve location $locationId: $e',
        e,
        stackTrace,
      );
    }
  }
}

/// Provider for the discover state
final discoverProvider = NotifierProvider<DiscoverNotifier, DiscoverState>(
  DiscoverNotifier.new,
);
