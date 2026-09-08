import 'dart:io';

import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/observability/logging/logger.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/data/gen/ripls/api/location.pb.dart'
    show Location, GeocodedLocation;
import 'package:url_launcher/url_launcher.dart';

final _log = ObservableLogger.named('OpenDirections');

/// Format a GeocodedLocation with complete details in a single-line format.
/// Similar to LocationFormatter.formatLocationDetailed but for GeocodedLocation.
String formatGeocodedLocationDetailed(GeocodedLocation location) {
  final parts = <String>[];

  // Priority 1: Place name
  if (location.name.isNotEmpty) {
    parts.add(location.name);
  }

  // Priority 2: Street address (always add if available, even with place name)
  if (location.addressLines.isNotEmpty) {
    parts.add(location.addressLines.first);
  }

  // Add locality (city) - always include
  if (location.locality.isNotEmpty) {
    parts.add(location.locality);
  }

  // Add region code (state/province)
  if (location.regionCode.isNotEmpty) {
    parts.add(location.regionCode);
  }

  // Add postal code
  if (location.postalCode.isNotEmpty) {
    parts.add(location.postalCode);
  }

  return parts.isEmpty ? 'Unknown Location' : parts.join(', ');
}

/// Pure URL builder for the platform's directions deep link.
///
/// Returns `null` when neither coords nor a name are usable — callers should
/// treat that as a programming error (the directions affordance should not
/// have been reachable) and surface a failure toast instead of launching.
///
/// On iOS this produces an `http://maps.apple.com/` URL (Apple Maps handles
/// the open + offers third-party app picker). On Android coords use the
/// `geo:` scheme so the user's default maps app handles it; name-only falls
/// back to a Google Maps HTTPS URL so devices without Google Maps still
/// resolve through the browser.
String? buildDirectionsUrl({
  required bool isIOS,
  double? latitudeDeg,
  double? longitudeDeg,
  String? name,
}) {
  final hasCoords = latitudeDeg != null &&
      longitudeDeg != null &&
      (latitudeDeg != 0 || longitudeDeg != 0);
  final trimmedName = name?.trim();
  final hasName = trimmedName != null && trimmedName.isNotEmpty;

  if (isIOS) {
    if (hasCoords) {
      return 'http://maps.apple.com/?daddr=$latitudeDeg,$longitudeDeg';
    }
    if (hasName) {
      return 'http://maps.apple.com/?daddr=${Uri.encodeComponent(trimmedName)}';
    }
    return null;
  }

  if (hasCoords) {
    return 'geo:$latitudeDeg,$longitudeDeg?q=$latitudeDeg,$longitudeDeg';
  }
  if (hasName) {
    return 'https://www.google.com/maps/search/?api=1'
        '&query=${Uri.encodeComponent(trimmedName)}';
  }
  return null;
}

/// Open the platform's maps app with directions to [location] (or the maps
/// app home if [location] is null). Shows a toast via [ToastHelper] on failure.
///
/// Prefers the location's lat/long when both are set; otherwise falls back
/// to the [Location.name] (or [fallbackName]) as a search query so callers
/// that only know the place name can still launch directions.
///
/// Caller is responsible for `mounted` checks before passing the context.
Future<void> openDirections(
  BuildContext context,
  Location? location, {
  String? fallbackName,
}) async {
  final name =
      location?.name.isNotEmpty ?? false ? location!.name : fallbackName;
  final url = buildDirectionsUrl(
    isIOS: Platform.isIOS,
    latitudeDeg: location?.latitudeDeg,
    longitudeDeg: location?.longitudeDeg,
    name: name,
  );

  if (url == null) {
    _log.warning('No coords or name available for directions');
    if (context.mounted) {
      ToastHelper.showError(
          context, context.l10n.locationPickerDirectionsLaunchFailed);
    }
    return;
  }

  final uri = Uri.parse(url);
  try {
    if (await canLaunchUrl(uri)) {
      await launchUrl(uri, mode: LaunchMode.externalApplication);
      return;
    }
    _log.warning('canLaunchUrl returned false', {
      'platform': Platform.isIOS ? 'ios' : 'android',
      'uri': url,
    });
  } catch (e) {
    _log.warning('launchUrl threw', {
      'platform': Platform.isIOS ? 'ios' : 'android',
      'uri': url,
      'error': e.toString(),
    });
  }

  if (context.mounted) {
    ToastHelper.showError(
        context, context.l10n.locationPickerDirectionsLaunchFailed);
  }
}
