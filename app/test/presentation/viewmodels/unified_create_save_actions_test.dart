import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/observability/events.dart';
import 'package:ripls/core/observability/service.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart'
    show Estimate, MaterialCategory, ProvenanceSource;
import 'package:ripls/data/gen/ripls/api/gear.pbenum.dart' show Availability;
import 'package:ripls/data/gen/ripls/api/request.pb.dart' show Request;
import 'package:ripls/data/gen/ripls/api/request_service.pb.dart'
    show RequestMetadata;
import 'package:ripls/data/gen/ripls/api/unified_create_service.pb.dart'
    show DetectedContentType;
import 'package:ripls/data/gen/ripls/api/value.pb.dart' show ValueEstimate;
import 'package:ripls/data/repositories/request_repository.dart';
import 'package:ripls/presentation/viewmodels/unified_create_save_actions.dart';
import 'package:ripls/presentation/viewmodels/unified_create_state.dart';
import 'package:ripls/services/gear_service.dart';
import 'package:ripls/services/providers.dart' show observabilityServiceProvider;
import 'package:ripls/services/providers/gear_providers.dart';
import 'package:ripls/services/providers/request_providers.dart';

/// Hand-rolled fake repository that records every submit / share call.
///
/// CREATE-1 (#2492): creation is now just create — `_saveRequest` makes
/// exactly one `submitRequest` with no community id (the server defaults the
/// request to its per-item community) and never calls `shareRequest`. Audience
/// expansion happens later in the Share sheet.
class _FakeRequestRepository extends Fake implements RequestRepository {
  final List<_SubmitCall> submitCalls = [];
  final List<_ShareCall> shareCalls = [];
  String returnedRequestId = 'req-stub';

  @override
  Future<String> submitRequest({
    String communityId = '',
    required String title,
    required String description,
    List<String>? mediaIds,
    String? locationId,
    RequestMetadata? metadata,
    int? neededByUnixSec,
    List<String>? seedNeedNames,
  }) async {
    submitCalls.add(_SubmitCall(
      communityId: communityId,
      title: title,
      description: description,
      mediaIds: mediaIds,
      locationId: locationId,
      seedNeedNames: seedNeedNames,
    ));
    return returnedRequestId;
  }

  @override
  Future<Request> shareRequest({
    required String requestId,
    required List<String> communityIds,
  }) async {
    shareCalls.add(_ShareCall(
      requestId: requestId,
      communityIds: communityIds,
    ));
    return Request();
  }
}

class _SubmitCall {
  _SubmitCall({
    required this.communityId,
    required this.title,
    required this.description,
    required this.mediaIds,
    required this.locationId,
    required this.seedNeedNames,
  });
  final String communityId;
  final String title;
  final String description;
  final List<String>? mediaIds;
  final String? locationId;
  final List<String>? seedNeedNames;
}

class _ShareCall {
  _ShareCall({required this.requestId, required this.communityIds});
  final String requestId;
  final List<String> communityIds;
}

class _FakeObservabilityService extends Fake implements ObservabilityService {
  final List<AnalyticsEvent> events = [];

  @override
  Future<void> logAnalyticsEvent(AnalyticsEvent event) async {
    events.add(event);
  }
}

/// Hand-rolled fake gear service recording every saveGear call, so the
/// Lend/Give plumb (#2687) and the metadata plumb (#2702) can be asserted:
/// the preview toggle must reach SaveGear's availability field, and the
/// AI detection (value/material/weight) must reach its metadata field.
class _FakeGearService extends Fake implements GearService {
  final List<Availability?> saveGearAvailabilities = [];
  final List<GearMetadata?> saveGearMetadatas = [];
  final List<String?> saveGearGenerationModes = [];

  @override
  Future<String> saveGear({
    String? id,
    String? name,
    String? description,
    List<String>? mediaIds,
    String? locationId,
    GearMetadata? metadata,
    String? generationMode,
    String? sourceUrl,
    Availability? availability,
  }) async {
    saveGearAvailabilities.add(availability);
    saveGearMetadatas.add(metadata);
    saveGearGenerationModes.add(generationMode);
    return 'gear-stub';
  }
}

UnifiedCreateState _requestState({
  String? locationId,
  List<String> mediaIds = const [],
  List<String> requestSeedNeeds = const [],
}) {
  return UnifiedCreateState(
    type: DetectedContentType.DETECTED_CONTENT_TYPE_REQUEST,
    title: 'Need a ladder',
    description: 'For weekend project.',
    mediaIds: mediaIds,
    locationId: locationId,
    requestSeedNeeds: requestSeedNeeds,
    streamComplete: true,
  );
}

void main() {
  late _FakeRequestRepository repo;
  late _FakeObservabilityService observability;
  late ProviderContainer container;

  setUp(() {
    repo = _FakeRequestRepository();
    observability = _FakeObservabilityService();
    container = ProviderContainer(
      overrides: [
        requestRepositoryProvider.overrideWithValue(repo),
        observabilityServiceProvider.overrideWithValue(observability),
      ],
    );
  });

  tearDown(() {
    container.dispose();
  });

  group('UnifiedCreateSaveActions._saveRequest (CREATE-1 #2492)', () {
    test('submits once with no community id, never shares', () async {
      final actions = container.read(unifiedCreateSaveActionsProvider);
      final result = await actions.save(_requestState());

      expect(repo.submitCalls, hasLength(1));
      expect(repo.submitCalls.single.communityId, '',
          reason: 'creation defers the audience to the server-side per-item '
              'community');
      expect(repo.shareCalls, isEmpty);
      expect(result.entityId, 'req-stub');
      expect(result.type, DetectedContentType.DETECTED_CONTENT_TYPE_REQUEST);
      expect(result.itemName, 'Need a ladder');
    });

    test('emits one RequestCreatedEvent with empty community id', () async {
      final actions = container.read(unifiedCreateSaveActionsProvider);
      await actions.save(_requestState(locationId: 'loc-1'));

      expect(observability.events, hasLength(1));
      final event = observability.events.single;
      expect(event, isA<RequestCreatedEvent>());
      final created = event as RequestCreatedEvent;
      expect(created.communityId, '');
      expect(created.hasLocation, isTrue);
    });

    test('RequestCreatedEvent.hasLocation is false when no location set',
        () async {
      final actions = container.read(unifiedCreateSaveActionsProvider);
      await actions.save(_requestState());

      expect(observability.events, hasLength(1));
      expect(
        (observability.events.single as RequestCreatedEvent).hasLocation,
        isFalse,
      );
    });

    test('passes media IDs as null when state has none', () async {
      final actions = container.read(unifiedCreateSaveActionsProvider);
      await actions.save(_requestState());
      expect(repo.submitCalls.single.mediaIds, isNull);
    });

    test('forwards the seeded needs list from state (#2731)', () async {
      final actions = container.read(unifiedCreateSaveActionsProvider);
      await actions.save(
        _requestState(requestSeedNeeds: ['Picture books', 'Whiteboard']),
      );
      expect(
        repo.submitCalls.single.seedNeedNames,
        ['Picture books', 'Whiteboard'],
      );
    });

    test('forwards media IDs from state', () async {
      final actions = container.read(unifiedCreateSaveActionsProvider);
      await actions.save(
        _requestState(mediaIds: ['m1', 'm2']),
      );
      expect(repo.submitCalls.single.mediaIds, ['m1', 'm2']);
    });
  });

  group('UnifiedCreateSaveActions._saveGear Lend/Give plumb (#2687)', () {
    UnifiedCreateState gearState(TransferIntent intent) => UnifiedCreateState(
          type: DetectedContentType.DETECTED_CONTENT_TYPE_GEAR,
          title: 'Spaghetti (2 boxes)',
          description: 'Two unopened boxes.',
          transferIntent: intent,
          streamComplete: true,
        );

    late _FakeGearService gearService;
    late ProviderContainer gearContainer;

    setUp(() {
      gearService = _FakeGearService();
      gearContainer = ProviderContainer(
        overrides: [
          gearServiceProvider.overrideWithValue(gearService),
          observabilityServiceProvider
              .overrideWithValue(_FakeObservabilityService()),
        ],
      );
    });

    tearDown(() {
      gearContainer.dispose();
    });

    test('Give reaches SaveGear as FOR_GIVEAWAY', () async {
      final actions = gearContainer.read(unifiedCreateSaveActionsProvider);
      await actions.save(gearState(TransferIntent.give));
      expect(gearService.saveGearAvailabilities.single,
          Availability.AVAILABILITY_FOR_GIVEAWAY);
    });

    test('Lend (the default) reaches SaveGear as FOR_LOAN', () async {
      final actions = gearContainer.read(unifiedCreateSaveActionsProvider);
      await actions.save(gearState(TransferIntent.lend));
      expect(gearService.saveGearAvailabilities.single,
          Availability.AVAILABILITY_FOR_LOAN);
    });

    test('the full detection reaches SaveGear metadata (#2702 \$0-value fix)',
        () async {
      final actions = gearContainer.read(unifiedCreateSaveActionsProvider);
      await actions.save(gearState(TransferIntent.lend).copyWith(
        inputMode: CreateInputMode.image,
        detectedGear: DetectedGearItem(
          title: 'Honda Self-Propelled Mower',
          category: 'Lawn & Garden',
          brand: 'Honda',
          materialCategory: MaterialCategory.MATERIAL_CATEGORY_SOLID_METAL,
          weightGrams: Estimate(mean: 30000),
          valueEstimate: ValueEstimate(estimatedValueUsd: 420),
        ),
      ));

      final md = gearService.saveGearMetadatas.single;
      expect(md, isNotNull,
          reason: 'a photo capture must persist its detection — dropping it '
              'is the "\$0 saved" bug');
      expect(md!.valueEstimate.estimatedValueUsd, 420);
      expect(md.weightGrams.value.mean, 30000);
      expect(md.brand.value, 'Honda');
      expect(md.category.value, 'Lawn & Garden');
      expect(md.materialCategory.value,
          MaterialCategory.MATERIAL_CATEGORY_SOLID_METAL);
      expect(gearService.saveGearGenerationModes.single, 'image',
          reason: 'the server stamps LLM provenance from generation_mode');
    });

    test('item-details edits overlay the detection with USER provenance',
        () async {
      final actions = gearContainer.read(unifiedCreateSaveActionsProvider);
      await actions.save(gearState(TransferIntent.lend).copyWith(
        detectedGear: DetectedGearItem(
          brand: 'Honda',
          valueEstimate: ValueEstimate(estimatedValueUsd: 420),
        ),
        itemDetails: const ItemDetailsValue(
          brand: 'Honda',
          estValueUsd: '350',
          weight: '30',
          weightUnit: 'kg',
        ),
        userEditedFields: const {UserEditedField.itemDetails},
      ));

      final md = gearService.saveGearMetadatas.single!;
      expect(md.valueEstimate.estimatedValueUsd, 350,
          reason: 'the sheet edit wins over the detection');
      expect(md.valueEstimate.provenance.source,
          ProvenanceSource.PROVENANCE_SOURCE_USER);
      expect(md.weightGrams.value.mean, 30000);
    });
  });
}
