import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/location_formatter.dart';
import 'package:ripls/data/gen/ripls/api/location.pb.dart' show Location;
import 'package:ripls/data/gen/ripls/api/location_service.pb.dart' show UserLocationWithDetails;
import 'package:ripls/presentation/viewmodels/profile_locations_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:timeago/timeago.dart' as timeago;

/// Screen for managing user's saved locations
class ProfileLocationsScreen extends ConsumerWidget {
  const ProfileLocationsScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(profileLocationsProvider);
    final notifier = ref.read(profileLocationsProvider.notifier);

    return Scaffold(
      appBar: AppBar(
        title: const Text('My Locations'),
        backgroundColor: AppColors.surface(context),
      ),
      body: RefreshIndicator(
        onRefresh: notifier.refresh,
        child: _buildBody(context, state, notifier),
      ),
    );
  }

  Widget _buildBody(
    BuildContext context,
    ProfileLocationsState state,
    ProfileLocationsNotifier notifier,
  ) {
    // Show loading state
    if (state.isLoading) {
      return const Center(child: CircularProgressIndicator());
    }

    // Show error state
    if (state.hasError) {
      return Center(
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Icon(
              Icons.error_outline,
              size: 64,
              color: AppColors.textSecondary(context),
            ),
            const SizedBox(height: 16),
            Text(
              state.error == null
                  ? 'An error occurred'
                  : RpcErrorHandler.localize(state.error!, context.l10n),
              style: TextStyle(
                color: AppColors.textSecondary(context),
                fontSize: 16,
              ),
              textAlign: TextAlign.center,
            ),
            const SizedBox(height: 24),
            ElevatedButton(
              onPressed: notifier.refresh,
              child: Text(context.l10n.commonRetry),
            ),
          ],
        ),
      );
    }

    // Show empty state (wrapped in ListView to support pull-to-refresh)
    if (state.isEmpty) {
      return ListView(
        physics: const AlwaysScrollableScrollPhysics(),
        children: [
          SizedBox(
            height: MediaQuery.of(context).size.height - 200,
            child: Center(
              child: Column(
                mainAxisAlignment: MainAxisAlignment.center,
                children: [
                  Icon(
                    Icons.location_off,
                    size: 64,
                    color: AppColors.textSecondary(context),
                  ),
                  const SizedBox(height: 16),
                  Text(
                    'No saved locations',
                    style: TextStyle(
                      color: AppColors.textSecondary(context),
                      fontSize: 16,
                    ),
                  ),
                  const SizedBox(height: 8),
                  Text(
                    'Locations will appear here when you\nadd them to gear, requests, or experiences',
                    style: TextStyle(
                      color: AppColors.textSecondary(context),
                      fontSize: 14,
                    ),
                    textAlign: TextAlign.center,
                  ),
                ],
              ),
            ),
          ),
        ],
      );
    }

    // Show locations list
    return ListView.builder(
      padding: const EdgeInsets.all(0),
      itemCount: state.locations.length,
      itemBuilder: (context, index) {
        final userLocation = state.locations[index];
        final location = userLocation.location;
        final isPrimary = location.id == state.primaryLocationId;

        return _buildLocationCard(
          context,
          userLocation,
          isPrimary,
          notifier,
        );
      },
    );
  }

  Widget _buildLocationCard(
    BuildContext context,
    UserLocationWithDetails userLocation,
    bool isPrimary,
    ProfileLocationsNotifier notifier,
  ) {
    final location = userLocation.location;
    final lastUsedAt = DateTime.fromMillisecondsSinceEpoch(
      userLocation.lastUsedAtUnixSec.toInt() * 1000,
    );
    final lastUsedAgo = timeago.format(lastUsedAt);

    return Dismissible(
      key: Key(location.id),
      direction: DismissDirection.endToStart,
      background: Container(
        alignment: Alignment.centerRight,
        padding: const EdgeInsets.only(right: 20),
        color: Colors.red,
        child: const Icon(
          Icons.delete,
          color: Colors.white,
          size: 28,
        ),
      ),
      confirmDismiss: (direction) async {
        return showDialog<bool>(
          context: context,
          builder: (context) => AlertDialog(
            title: const Text('Delete Location'),
            content: Text(
              'Remove "${_buildLocationName(location)}" from your saved locations?',
            ),
            actions: [
              TextButton(
                onPressed: () => Navigator.of(context).pop(false),
                child: Text(context.l10n.commonCancel),
              ),
              TextButton(
                onPressed: () => Navigator.of(context).pop(true),
                style: TextButton.styleFrom(
                  foregroundColor: Colors.red,
                ),
                child: Text(context.l10n.commonDelete),
              ),
            ],
          ),
        );
      },
      onDismissed: (direction) {
        notifier.deleteLocation(location.id);
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text('Deleted ${_buildLocationName(location)}'),
            duration: accessibleDuration(context, const Duration(seconds: 2)),
          ),
        );
      },
      child: Tappable(
        semanticsLabel: _buildLocationName(location),
        onTap: () => _handleCardTap(context, location, isPrimary, notifier),
        excludeChildSemantics: false,
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
          decoration: BoxDecoration(
            color: AppColors.cardBackground(context),
            border: Border(
              bottom: BorderSide(color: AppColors.divider(context), width: 1),
            ),
          ),
          child: Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              // Leading icon with primary label
              SizedBox(
                width: 60,
                child: Column(
                  children: [
                    Icon(
                      isPrimary ? Icons.star : Icons.location_on,
                      color: isPrimary
                          ? AppColors.statusWarning(context)
                          : AppColors.primary(context),
                      size: 32,
                    ),
                    if (isPrimary) ...[
                      const SizedBox(height: 4),
                      Text(
                        'Primary',
                        style: TextStyle(
                          color: AppColors.textSecondary(context),
                          fontSize: 11,
                        ),
                      ),
                    ],
                  ],
                ),
              ),
              const SizedBox(width: 12),

              // Location details
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    // Location name (POI name or street address)
                    Text(
                      LocationFormatter.formatLocationNameShort(location),
                      style: TextStyle(
                        color: AppColors.textPrimary(context),
                        fontSize: 15,
                        fontWeight: FontWeight.w600,
                      ),
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                    ),

                    // Address line and city, state, postal
                    const SizedBox(height: 4),
                    Text(
                      _buildLocationSubtitle(location),
                      style: TextStyle(
                        color: AppColors.textSecondary(context),
                        fontSize: 13,
                      ),
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                    ),
                  ],
                ),
              ),

              // Timestamp (bottom-right)
              Padding(
                padding: const EdgeInsets.only(left: 8),
                child: Text(
                  lastUsedAgo,
                  style: TextStyle(
                    color: AppColors.textSecondary(context),
                    fontSize: 12,
                  ),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }

  void _handleCardTap(
    BuildContext context,
    Location location,
    bool isPrimary,
    ProfileLocationsNotifier notifier,
  ) {
    if (isPrimary) {
      // Show info that this is already primary
      showDialog(
        context: context,
        builder: (context) => AlertDialog(
          title: const Text('Primary Location'),
          content: const Text('This is your primary location.'),
          actions: [
            TextButton(
              onPressed: () => Navigator.of(context).pop(),
              child: const Text('OK'),
            ),
          ],
        ),
      );
    } else {
      // Ask to set as primary
      showDialog(
        context: context,
        builder: (context) => AlertDialog(
          title: const Text('Set as Primary Location'),
          content: Text(
            'Set "${_buildLocationName(location)}" as your primary location?',
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.of(context).pop(),
              child: Text(context.l10n.commonCancel),
            ),
            TextButton(
              onPressed: () {
                Navigator.of(context).pop();
                notifier.setPrimaryLocation(location.id);
              },
              child: const Text('Set as Primary'),
            ),
          ],
        ),
      );
    }
  }

  String _buildLocationName(Location location) {
    if (location.addressLines.isNotEmpty) {
      return location.addressLines.first;
    }
    if (location.name.isNotEmpty) {
      return location.name;
    }
    return location.locality;
  }

  String _buildLocationSubtitle(Location location) {
    final parts = <String>[];

    // If the title is using the POI name, show the address on the second line
    if (location.name.isNotEmpty && location.addressLines.isNotEmpty) {
      parts.add(location.addressLines.first);
    }

    // Always add city
    if (location.locality.isNotEmpty) {
      parts.add(location.locality);
    }

    // Add state and postal code
    if (location.regionCode.isNotEmpty && location.postalCode.isNotEmpty) {
      parts.add('${location.regionCode} ${location.postalCode}');
    } else if (location.regionCode.isNotEmpty) {
      parts.add(location.regionCode);
    } else if (location.postalCode.isNotEmpty) {
      parts.add(location.postalCode);
    }

    return parts.join(', ');
  }
}
