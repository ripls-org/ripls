import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/observability/events.dart';
import 'package:ripls/core/utils/gear_metadata_formatter.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart'
    show
        Estimate,
        MaterialCategory,
        Provenance,
        ProvenanceSource,
        TrackedEstimate,
        TrackedMaterialCategory,
        TrackedString;
import 'package:ripls/data/gen/ripls/api/gear.pbenum.dart' show Availability;
import 'package:ripls/data/gen/ripls/api/gear_service.pb.dart' show GearMetadata;
import 'package:ripls/data/gen/ripls/api/unified_create_service.pb.dart'
    show DetectedContentType;
import 'package:ripls/data/gen/ripls/api/value.pb.dart' show ValueEstimate;
import 'package:ripls/presentation/viewmodels/unified_create_state.dart';
import 'package:ripls/services/providers.dart' show observabilityServiceProvider;
import 'package:ripls/services/providers/experience_providers.dart';
import 'package:ripls/services/providers/gear_providers.dart';
import 'package:ripls/services/providers/location_providers.dart';
import 'package:ripls/services/providers/request_providers.dart';

final _log = Logger('UnifiedCreateSaveActions');

class UnifiedSaveResult {
  const UnifiedSaveResult({
    required this.entityId,
    required this.type,
    required this.itemName,
  });
  final String entityId;
  final DetectedContentType type;

  /// Display name of the saved item, forwarded to the Share sheet so it can
  /// render the item's title without a refetch.
  final String itemName;
}

/// Production save dispatch for the unified-create flow. Creation is now
/// just create — no audience is picked here. Routes completed state to the
/// matching per-type `Save*` RPC via the existing per-type service Riverpod
/// providers; the per-item community is provisioned server-side (events /
/// requests at creation, gear on first ShareItem) and audience expansion is
/// handled by the Share sheet the modal opens after Save.
///
/// Per-type call shapes differ:
///   * Event — `saveExperience()` (returns id).
///   * Gear  — `saveGear()` (returns id). The Lend/Give toggle is applied
///     later by the Share sheet / ShareItem, not here.
///   * Request — `submitRequest()` (returns id), with no community id so the
///     server defaults it to the request's per-item community. Going through
///     `RequestRepository` (not the service directly) so feed / search /
///     daily caches invalidate.
class UnifiedCreateSaveActions {
  UnifiedCreateSaveActions(this._ref);
  final Ref _ref;

  Future<UnifiedSaveResult> save(UnifiedCreateState state) async {
    if (!state.isSaveable) {
      throw StateError('save called before isSaveable=true');
    }
    final type = state.type;
    if (type == null) {
      throw StateError('save called with null type');
    }
    _log.info('save dispatch type=${type.name} '
        'transfer_intent=${state.transferIntent.name}');
    final result = switch (type) {
      DetectedContentType.DETECTED_CONTENT_TYPE_EVENT =>
        await _saveEvent(state),
      DetectedContentType.DETECTED_CONTENT_TYPE_GEAR =>
        await _saveGear(state),
      DetectedContentType.DETECTED_CONTENT_TYPE_REQUEST =>
        await _saveRequest(state),
      _ => throw StateError('save: unhandled type $type'),
    };
    // Note: post-creation (feed refresh + navigate to feed tab) is run by
    // the caller AFTER the unified-create modal has popped. Running it
    // here — while the modal is still in the route stack — races with
    // the new ContentView mount and leaves the per-item provider stuck.
    return result;
  }

  /// Returns a usable saved locationId for [state], materializing the
  /// AI-predicted [UnifiedCreateState.location] (a `GeocodedLocation` that
  /// has not yet been persisted) when the user never opened the location
  /// picker. Without this, every Save* call would drop the predicted spot
  /// and the new entity would land with no location ("TBD"). Failures are
  /// logged and swallowed — Save proceeds with no location rather than
  /// blocking the share.
  Future<String?> _resolveLocationId(UnifiedCreateState state) async {
    final picked = state.locationId;
    if (picked != null && picked.isNotEmpty) return picked;
    final geocoded = state.location;
    if (geocoded == null) return null;
    if (geocoded.latitudeDeg == 0 && geocoded.longitudeDeg == 0) return null;
    try {
      final repo = _ref.read(locationRepositoryProvider);
      return await repo.saveLocation(
        latitudeDeg: geocoded.latitudeDeg,
        longitudeDeg: geocoded.longitudeDeg,
        regionCode: geocoded.regionCode,
        postalCode: geocoded.postalCode,
        locality: geocoded.locality,
        addressLines:
            geocoded.addressLines.isNotEmpty ? geocoded.addressLines : null,
        name: geocoded.name.isNotEmpty ? geocoded.name : null,
        neighborhood:
            geocoded.neighborhood.isNotEmpty ? geocoded.neighborhood : null,
        county: geocoded.county.isNotEmpty ? geocoded.county : null,
        administrativeArea: geocoded.administrativeArea.isNotEmpty
            ? geocoded.administrativeArea
            : null,
        externalPlaceId: geocoded.externalPlaceId.isNotEmpty
            ? geocoded.externalPlaceId
            : null,
        externalPlaceProvider: geocoded.externalPlaceProvider.isNotEmpty
            ? geocoded.externalPlaceProvider
            : null,
      );
    } catch (e, s) {
      _log.warning('failed to materialize predicted location', e, s);
      return null;
    }
  }

  Future<UnifiedSaveResult> _saveEvent(UnifiedCreateState state) async {
    final svc = _ref.read(experienceServiceProvider);
    final locationId = await _resolveLocationId(state);
    final name = state.title ?? '';
    final id = await svc.saveExperience(
      name: name,
      description: state.description ?? '',
      locationId: locationId,
      time: state.eventTime?.suggestedTime,
      mediaIds: state.mediaIds.isEmpty ? null : state.mediaIds,
    );
    return UnifiedSaveResult(
      entityId: id,
      type: DetectedContentType.DETECTED_CONTENT_TYPE_EVENT,
      itemName: name,
    );
  }

  Future<UnifiedSaveResult> _saveGear(UnifiedCreateState state) async {
    final gearSvc = _ref.read(gearServiceProvider);
    final locationId = await _resolveLocationId(state);
    final name = state.title ?? '';
    final sourceUrl = state.detectedGear?.sourceUrl;
    final id = await gearSvc.saveGear(
      name: name,
      description: state.description ?? '',
      locationId: locationId,
      mediaIds: state.mediaIds.isEmpty ? null : state.mediaIds,
      metadata: _gearMetadata(state),
      // The server assigns LLM provenance to unstamped metadata fields on
      // insert based on this mode (see SaveGearRequest.generation_mode).
      generationMode: _generationMode(state),
      sourceUrl: sourceUrl,
      // The preview's Lend/Give toggle: the per-item community is shared
      // with this availability at creation, and every ShareItem community
      // invite inherits it (#2687).
      availability: state.transferIntent == TransferIntent.give
          ? Availability.AVAILABILITY_FOR_GIVEAWAY
          : Availability.AVAILABILITY_FOR_LOAN,
    );
    return UnifiedSaveResult(
      entityId: id,
      type: DetectedContentType.DETECTED_CONTENT_TYPE_GEAR,
      itemName: name,
    );
  }

  Future<UnifiedSaveResult> _saveRequest(UnifiedCreateState state) async {
    final repo = _ref.read(requestRepositoryProvider);
    // Pass the stock-imagery media the unified-create stream already
    // attached to state. Without this the server sees empty MediaIds
    // and spawns its own async Pexels fetch (lifecycle.go), which (a)
    // lands later than the feed-refresh so the new request shows up
    // imageless, and (b) usually picks a different image than the
    // preview because Pexels is re-queried server-side with different
    // parameters.
    final mediaIds = state.mediaIds.isEmpty ? null : state.mediaIds;
    final locationId = await _resolveLocationId(state);
    final name = state.title ?? '';
    // No community id — the server defaults the request to its per-item
    // community; the Share sheet expands the audience afterward.
    final id = await repo.submitRequest(
      title: name,
      description: state.description ?? '',
      mediaIds: mediaIds,
      locationId: locationId,
      // Send the names only when there are any; an empty list means the
      // request is born with no needs, same as omitting the field.
      seedNeedNames:
          state.requestSeedNeeds.isEmpty ? null : state.requestSeedNeeds,
    );
    unawaited(_ref.read(observabilityServiceProvider).logAnalyticsEvent(
          RequestCreatedEvent(
            communityId: '',
            hasLocation: locationId != null,
          ),
        ));
    return UnifiedSaveResult(
      entityId: id,
      type: DetectedContentType.DETECTED_CONTENT_TYPE_REQUEST,
      itemName: name,
    );
  }
}

final unifiedCreateSaveActionsProvider =
    Provider<UnifiedCreateSaveActions>((ref) {
  return UnifiedCreateSaveActions(ref);
});

/// Generation mode forwarded to SaveGear for provenance stamping. Null when
/// nothing was AI-detected (fully manual entry).
String? _generationMode(UnifiedCreateState state) {
  final detected = state.detectedGear;
  if (detected == null) return null;
  if (detected.sourceUrl.isNotEmpty) return 'web';
  return switch (state.inputMode) {
    CreateInputMode.image => 'image',
    CreateInputMode.url => 'web',
    CreateInputMode.text => 'text',
  };
}

/// Build the [GearMetadata] to persist: the full AI detection (category,
/// material, weight distribution, value estimate) with any user edits from
/// the item-details sheet overlaid on top with USER provenance. Persisting
/// only brand/model here was the #2702 reel's "$0 saved" bug — impact
/// estimation derives money and emissions from exactly these fields, so a
/// photo-captured item lent through a request counted as worthless.
GearMetadata? _gearMetadata(UnifiedCreateState state) {
  final md = GearMetadata();

  final detected = state.detectedGear;
  if (detected != null) {
    if (!GearMetadataFormatter.isUnknownOrEmpty(detected.category)) {
      md.category = TrackedString(value: detected.category);
    }
    if (!GearMetadataFormatter.isUnknownOrEmpty(detected.brand)) {
      md.brand = TrackedString(value: detected.brand);
    }
    if (!GearMetadataFormatter.isUnknownOrEmpty(detected.model)) {
      md.model = TrackedString(value: detected.model);
    }
    if (detected.materialCategory !=
        MaterialCategory.MATERIAL_CATEGORY_UNSPECIFIED) {
      md.materialCategory =
          TrackedMaterialCategory(value: detected.materialCategory);
    }
    if (detected.hasWeightGrams() && detected.weightGrams.mean > 0) {
      md.weightGrams = TrackedEstimate(value: detected.weightGrams);
    }
    if (detected.hasValueEstimate() &&
        detected.valueEstimate.estimatedValueUsd > 0) {
      md.valueEstimate = detected.valueEstimate;
    }
  }

  // Item-details sheet edits overlay the detection. The sheet returns the
  // whole value set on save, so every non-empty field it carries is a
  // user-confirmed value (and an emptied field is an explicit clear).
  final v = state.itemDetails;
  if (v != null &&
      state.userEditedFields.contains(UserEditedField.itemDetails)) {
    final userProv =
        Provenance(source: ProvenanceSource.PROVENANCE_SOURCE_USER);
    v.brand.isEmpty
        ? md.clearBrand()
        : md.brand = TrackedString(value: v.brand, provenance: userProv);
    v.model.isEmpty
        ? md.clearModel()
        : md.model = TrackedString(value: v.model, provenance: userProv);
    final value = double.tryParse(v.estValueUsd);
    if (v.estValueUsd.isEmpty) {
      md.clearValueEstimate();
    } else if (value != null && value > 0) {
      md.valueEstimate =
          ValueEstimate(estimatedValueUsd: value, provenance: userProv);
    }
    final weight = double.tryParse(v.weight);
    if (v.weight.isEmpty) {
      md.clearWeightGrams();
    } else if (weight != null && weight > 0) {
      final grams = switch (v.weightUnit) {
        'kg' => weight * 1000,
        'lbs' => weight * 453.592,
        _ => weight,
      };
      md.weightGrams = TrackedEstimate(
        value: Estimate(mean: grams),
        provenance: userProv,
      );
    }
  }

  final hasAnyField = md.hasCategory() ||
      md.hasBrand() ||
      md.hasModel() ||
      md.hasMaterialCategory() ||
      md.hasWeightGrams() ||
      md.hasValueEstimate();
  return hasAnyField ? md : null;
}
