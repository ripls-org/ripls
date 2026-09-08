import 'dart:async';

import 'package:cross_file/cross_file.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/observability/service.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart' show Estimate;
import 'package:ripls/data/gen/ripls/api/experience_service.pb.dart'
    show GenExperienceResponse;
import 'package:ripls/data/gen/ripls/api/gear_service.pb.dart'
    show DetectedGearItem, GenGearResponse;
import 'package:ripls/data/gen/ripls/api/gen_stream.pb.dart'
    show MediaCandidate;
import 'package:ripls/data/gen/ripls/api/media_service.pb.dart'
    show StockImageProvider;
import 'package:ripls/data/gen/ripls/api/request_service.pb.dart'
    show GenRequestResponse;
import 'package:ripls/data/gen/ripls/api/unified_create_service.pb.dart'
    show DetectedContentType, StreamGenUnifiedCreateFinal;
import 'package:ripls/data/gen/ripls/api/value.pb.dart' show ValueEstimate;
import 'package:ripls/data/repositories/media_repository.dart';
import 'package:ripls/presentation/viewmodels/unified_create_state.dart';
import 'package:ripls/presentation/viewmodels/unified_create_streaming_actions.dart';
import 'package:ripls/presentation/viewmodels/unified_create_view_model.dart';
import 'package:ripls/services/providers/auth_providers.dart'
    show observabilityServiceProvider;
import 'package:ripls/services/providers/media_providers.dart';

/// Hand-rolled fake for [MediaRepository] that only implements
/// `addMedia` — the single method exercised by the unified-create
/// camera tests. Uses [Fake] (from mockito) so any other call surfaces
/// as a clear `UnimplementedError` rather than a silent null return.
class _FakeMediaRepository extends Fake implements MediaRepository {
  /// `addMedia` returns this value (or throws [_addMediaError] when
  /// non-null). Set via [stubReturns] / [stubThrows].
  String _addMediaReturn = 'media-stub';
  Exception? _addMediaError;

  /// Records of every `addMedia` call so tests can assert.
  final List<({String filePath, String? description})> addMediaCalls = [];

  /// Optional gate that delays `addMedia` until the test completes the
  /// returned future. Used by the disposal-safety test to keep the
  /// upload mid-flight while the provider container is disposed.
  Completer<void>? addMediaGate;

  void stubReturns(String id) {
    _addMediaReturn = id;
    _addMediaError = null;
  }

  void stubThrows(Exception e) {
    _addMediaError = e;
  }

  @override
  Future<String> addMedia({
    required XFile file,
    String? description,
  }) async {
    addMediaCalls.add((filePath: file.path, description: description));
    if (addMediaGate != null) {
      await addMediaGate!.future;
    }
    if (_addMediaError != null) {
      throw _addMediaError!;
    }
    return _addMediaReturn;
  }

  /// `addMediaFromUrl` returns this value (or throws [_addMediaFromUrlError]
  /// when non-null).
  String _addMediaFromUrlReturn = 'media-from-url-stub';
  Exception? _addMediaFromUrlError;
  final List<({String url, String? description})> addMediaFromUrlCalls = [];
  Completer<void>? addMediaFromUrlGate;

  void stubAddMediaFromUrlReturns(String id) {
    _addMediaFromUrlReturn = id;
    _addMediaFromUrlError = null;
  }

  void stubAddMediaFromUrlThrows(Exception e) {
    _addMediaFromUrlError = e;
  }

  @override
  Future<String> addMediaFromUrl({
    required String url,
    String? description,
    StockImageProvider? provider,
    String? providerPhotoId,
  }) async {
    addMediaFromUrlCalls.add((url: url, description: description));
    if (addMediaFromUrlGate != null) {
      await addMediaFromUrlGate!.future;
    }
    if (_addMediaFromUrlError != null) {
      throw _addMediaFromUrlError!;
    }
    return _addMediaFromUrlReturn;
  }
}

/// Scripted streaming actions for tests. Emits the provided event list
/// then closes the stream.
class _ScriptedActions implements UnifiedCreateStreamingActions {
  _ScriptedActions(this._events);
  final List<UnifiedCreateEvent> _events;

  /// Records the force_type value passed by the view model so tests can
  /// assert re-stream behavior.
  DetectedContentType? lastForceType;
  int callCount = 0;

  @override
  Stream<UnifiedCreateEvent> stream({
    required UnifiedCreateState state,
    DetectedContentType? forceType,
  }) {
    callCount += 1;
    lastForceType = forceType;
    final controller = StreamController<UnifiedCreateEvent>();
    Future<void>.microtask(() async {
      for (final ev in _events) {
        controller.add(ev);
        await Future<void>.delayed(Duration.zero);
      }
      await controller.close();
    });
    return controller.stream;
  }
}

UnifiedCreateViewModel _vmWith(
  ProviderContainer container,
  UnifiedCreateStreamingActions actions,
) {
  // ignore: invalid_use_of_protected_member
  final vm = container.read(unifiedCreateViewModelProvider.notifier);
  vm.attachActions(actions);
  return vm;
}

Future<void> _drain() async {
  // Allow the StreamController microtasks to flush.
  for (var i = 0; i < 5; i++) {
    await Future<void>.delayed(Duration.zero);
  }
}

void main() {
  group('UnifiedCreateViewModel', () {
    test('reducer applies type and final events', () async {
      final actions = _ScriptedActions([
        const UnifiedCreateEventType(DetectedContentType.DETECTED_CONTENT_TYPE_GEAR),
        const UnifiedCreateEventTitle('Climbing rope'),
        const UnifiedCreateEventDescription('9.6mm'),
        UnifiedCreateEventFinal(StreamGenUnifiedCreateFinal(
          type: DetectedContentType.DETECTED_CONTENT_TYPE_GEAR,
        )),
      ]);
      final container = ProviderContainer();
      addTearDown(container.dispose);
      final vm = _vmWith(container, actions);

      vm.setPrompt('Climbing rope');
      await vm.start();
      await _drain();

      final state = container.read(unifiedCreateViewModelProvider);
      expect(state.type, DetectedContentType.DETECTED_CONTENT_TYPE_GEAR);
      expect(state.title, 'Climbing rope');
      expect(state.description, '9.6mm');
      expect(state.streamComplete, true);
      expect(state.streaming, false);
      expect(state.selectorEnabled, true);
    });

    test('reducer drops events for user-edited fields on re-stream', () async {
      final actions = _ScriptedActions([
        const UnifiedCreateEventType(DetectedContentType.DETECTED_CONTENT_TYPE_GEAR),
        const UnifiedCreateEventTitle('Server title'),
        UnifiedCreateEventFinal(StreamGenUnifiedCreateFinal(
          type: DetectedContentType.DETECTED_CONTENT_TYPE_GEAR,
        )),
      ]);
      final container = ProviderContainer();
      addTearDown(container.dispose);
      final vm = _vmWith(container, actions);
      vm.setPrompt('rope');
      await vm.start();
      await _drain();
      // User overrides title.
      vm.editTitle('User title');
      // Now trigger a flip; reducer should drop the incoming title event.
      await vm.flipType(DetectedContentType.DETECTED_CONTENT_TYPE_EVENT);
      await _drain();

      final state = container.read(unifiedCreateViewModelProvider);
      expect(state.title, 'User title', reason: 'user edit must survive re-stream');
    });

    test('final adopts the extracted seed needs; a user edit survives '
        're-stream (#2702, #2731)', () async {
      final actions = _ScriptedActions([
        const UnifiedCreateEventType(
            DetectedContentType.DETECTED_CONTENT_TYPE_REQUEST),
        UnifiedCreateEventFinal(StreamGenUnifiedCreateFinal(
          type: DetectedContentType.DETECTED_CONTENT_TYPE_REQUEST,
          request: GenRequestResponse(seedNeeds: ['Picture books', 'Whiteboard']),
        )),
      ]);
      final container = ProviderContainer();
      addTearDown(container.dispose);
      final vm = _vmWith(container, actions);
      vm.setPrompt('classroom is bare, need books and a whiteboard');
      await vm.start();
      await _drain();
      expect(
        container.read(unifiedCreateViewModelProvider).requestSeedNeeds,
        ['Picture books', 'Whiteboard'],
        reason: 'the extracted needs should surface on the preview',
      );

      // The requester prunes the list; a later final must not clobber it.
      vm.editRequestSeedNeeds(['Picture books']);
      final secondScript = _ScriptedActions([
        const UnifiedCreateEventType(
            DetectedContentType.DETECTED_CONTENT_TYPE_REQUEST),
        UnifiedCreateEventFinal(StreamGenUnifiedCreateFinal(
          type: DetectedContentType.DETECTED_CONTENT_TYPE_REQUEST,
          request: GenRequestResponse(seedNeeds: ['Server need']),
        )),
      ]);
      vm.attachActions(secondScript);
      await vm.flipType(DetectedContentType.DETECTED_CONTENT_TYPE_REQUEST);
      await _drain();
      expect(
        container.read(unifiedCreateViewModelProvider).requestSeedNeeds,
        ['Picture books'],
        reason: 'user edit must survive the re-stream final',
      );
    });

    test('flipType is a no-op when selector is locked', () async {
      // No type event in the script → selectorEnabled stays false.
      final actions = _ScriptedActions(const [
        UnifiedCreateEventTitle('title only'),
      ]);
      final container = ProviderContainer();
      addTearDown(container.dispose);
      final vm = _vmWith(container, actions);
      vm.setPrompt('something');
      // Don't await start — flip while stream is in flight, before any type event.
      unawaited(vm.start());
      await Future<void>.delayed(Duration.zero);

      // Force selector into locked state by clearing it.
      await vm.flipType(DetectedContentType.DETECTED_CONTENT_TYPE_REQUEST);
      // Confirm a re-stream did NOT fire (callCount stays at 1 from start).
      expect(actions.callCount, 1, reason: 'flipType should be locked out');
    });

    test('flipType sends force_type and unlocks selector after final', () async {
      final firstScript = _ScriptedActions([
        const UnifiedCreateEventType(DetectedContentType.DETECTED_CONTENT_TYPE_GEAR),
        UnifiedCreateEventFinal(StreamGenUnifiedCreateFinal(
          type: DetectedContentType.DETECTED_CONTENT_TYPE_GEAR,
        )),
      ]);
      final container = ProviderContainer();
      addTearDown(container.dispose);
      final vm = _vmWith(container, firstScript);
      vm.setPrompt('thing');
      await vm.start();
      await _drain();
      expect(container.read(unifiedCreateViewModelProvider).selectorEnabled, true);

      // Re-script for the flip stream.
      final secondScript = _ScriptedActions([
        const UnifiedCreateEventType(DetectedContentType.DETECTED_CONTENT_TYPE_EVENT),
        UnifiedCreateEventFinal(StreamGenUnifiedCreateFinal(
          type: DetectedContentType.DETECTED_CONTENT_TYPE_EVENT,
        )),
      ]);
      vm.attachActions(secondScript);
      await vm.flipType(DetectedContentType.DETECTED_CONTENT_TYPE_EVENT);
      await _drain();

      expect(secondScript.lastForceType, DetectedContentType.DETECTED_CONTENT_TYPE_EVENT);
      final state = container.read(unifiedCreateViewModelProvider);
      expect(state.type, DetectedContentType.DETECTED_CONTENT_TYPE_EVENT);
      expect(state.selectorEnabled, true);
      expect(state.streamComplete, true);
    });

    test('isSaveable per-type required fields (CREATE-1: no audience gate)', () {
      // Empty: nothing populated → not saveable.
      const base = UnifiedCreateState(streamComplete: true);
      expect(base.isSaveable, false, reason: 'no type/title');

      // Event: title ≥3 chars + description ≥10 chars. CREATE-1 dropped the
      // at-creation community requirement — the per-item community is
      // server-provisioned and the Share sheet handles audience.
      final eventTitleOnly = base.copyWith(
        type: DetectedContentType.DETECTED_CONTENT_TYPE_EVENT,
        title: 'Hike',
      );
      expect(eventTitleOnly.isSaveable, false,
          reason: 'event requires description ≥10 chars');

      final eventShortDesc = eventTitleOnly.copyWith(description: 'Too short');
      expect(eventShortDesc.isSaveable, false,
          reason: 'event description must be ≥10 chars');

      final eventComplete = eventTitleOnly.copyWith(
        description: 'Mt Sanitas tomorrow morning, all paces welcome',
      );
      expect(eventComplete.isSaveable, true,
          reason: 'no community needed at creation anymore');

      final eventShortTitle = base.copyWith(
        type: DetectedContentType.DETECTED_CONTENT_TYPE_EVENT,
        title: 'Hi',
        description: 'A long enough description to pass the length gate',
      );
      expect(eventShortTitle.isSaveable, false,
          reason: 'event title must be ≥3 chars');

      // Gear: title + description (GearHelper.validateGearFields).
      final gearMissingDescription = base.copyWith(
        type: DetectedContentType.DETECTED_CONTENT_TYPE_GEAR,
        title: 'Climbing rope',
      );
      expect(gearMissingDescription.isSaveable, false,
          reason: 'gear requires description');

      final gearComplete = gearMissingDescription.copyWith(description: '9.6mm');
      expect(gearComplete.isSaveable, true);

      // Request: same shape as gear.
      final requestComplete = base.copyWith(
        type: DetectedContentType.DETECTED_CONTENT_TYPE_REQUEST,
        title: 'Help moving',
        description: 'On Sunday',
      );
      expect(requestComplete.isSaveable, true);

      // streamComplete=false → never saveable.
      expect(
          requestComplete.copyWith(streamComplete: false).isSaveable, false);
    });

    test('initial inputMode defaults to image', () {
      // Fresh state — verifies the @Default(CreateInputMode.image)
      // change on UnifiedCreateState so the modal opens directly to
      // the live camera.
      const initial = UnifiedCreateState();
      expect(initial.inputMode, CreateInputMode.image);
    });

    test('captureAndStartFromCamera uploads and transitions to preview',
        () async {
      final actions = _ScriptedActions([
        const UnifiedCreateEventType(DetectedContentType.DETECTED_CONTENT_TYPE_GEAR),
        UnifiedCreateEventFinal(StreamGenUnifiedCreateFinal(
          type: DetectedContentType.DETECTED_CONTENT_TYPE_GEAR,
        )),
      ]);
      final fakeRepo = _FakeMediaRepository()..stubReturns('media-123');

      final container = ProviderContainer(overrides: [
        mediaRepositoryProvider.overrideWithValue(fakeRepo),
      ]);
      addTearDown(container.dispose);
      final vm = _vmWith(container, actions);

      await vm.captureAndStartFromCamera(XFile('/tmp/capture.jpg'));
      await _drain();

      final state = container.read(unifiedCreateViewModelProvider);
      expect(state.stage, CreateStage.preview);
      expect(state.mediaId, 'media-123');
      expect(state.mediaIds, ['media-123']);
      expect(state.errorMessage, isNull);
      expect(fakeRepo.addMediaCalls, hasLength(1));
      expect(fakeRepo.addMediaCalls.single.filePath, '/tmp/capture.jpg');
      expect(fakeRepo.addMediaCalls.single.description, isNull);
    });

    test('captureAndStartFromCamera surfaces upload errors', () async {
      final actions = _ScriptedActions(const []);
      final fakeRepo = _FakeMediaRepository()
        ..stubThrows(Exception('disk full'));

      final container = ProviderContainer(overrides: [
        mediaRepositoryProvider.overrideWithValue(fakeRepo),
      ]);
      addTearDown(container.dispose);
      final vm = _vmWith(container, actions);

      await vm.captureAndStartFromCamera(XFile('/tmp/capture.jpg'));

      final state = container.read(unifiedCreateViewModelProvider);
      expect(state.streaming, isFalse);
      expect(state.selectorEnabled, isTrue);
      expect(state.errorMessage, contains('disk full'));
    });

    test('captureAndStartFromCamera disposal during in-flight upload '
        'completes without throwing', () async {
      final actions = _ScriptedActions(const []);
      final gate = Completer<void>();
      final fakeRepo = _FakeMediaRepository()
        ..stubReturns('media-late')
        ..addMediaGate = gate;

      final container = ProviderContainer(overrides: [
        mediaRepositoryProvider.overrideWithValue(fakeRepo),
      ]);
      final vm = _vmWith(container, actions);

      // Start the upload, then dispose the container before the
      // upload resolves. The VM's post-await state assignment is
      // expected to no-op rather than throw.
      final future = vm.captureAndStartFromCamera(XFile('/tmp/capture.jpg'));
      container.dispose();
      gate.complete();

      await expectLater(future, completes);
    });

    test('user-supplied photo survives flipType re-stream (#1957)', () async {
      // First stream: type + final for GEAR. No media event — the
      // user's captured photo (set by captureAndStartFromCamera) is the
      // only thing in mediaIds.
      final firstScript = _ScriptedActions([
        const UnifiedCreateEventType(DetectedContentType.DETECTED_CONTENT_TYPE_GEAR),
        UnifiedCreateEventFinal(StreamGenUnifiedCreateFinal(
          type: DetectedContentType.DETECTED_CONTENT_TYPE_GEAR,
        )),
      ]);
      final fakeRepo = _FakeMediaRepository()..stubReturns('user-media-123');
      final container = ProviderContainer(overrides: [
        mediaRepositoryProvider.overrideWithValue(fakeRepo),
      ]);
      addTearDown(container.dispose);
      final vm = _vmWith(container, firstScript);

      await vm.captureAndStartFromCamera(XFile('/tmp/capture.jpg'));
      await _drain();

      var state = container.read(unifiedCreateViewModelProvider);
      expect(state.mediaIds, ['user-media-123']);
      expect(state.userEditedFields, contains(UserEditedField.mediaIds),
          reason: 'user-supplied media must be marked as user-edited');

      // Re-script for the flipType re-stream. The server emits a stock
      // media event for the new type — the reducer must drop it.
      final secondScript = _ScriptedActions([
        const UnifiedCreateEventType(DetectedContentType.DETECTED_CONTENT_TYPE_EVENT),
        UnifiedCreateEventMedia(const ['stock-1'], const []),
        UnifiedCreateEventFinal(StreamGenUnifiedCreateFinal(
          type: DetectedContentType.DETECTED_CONTENT_TYPE_EVENT,
        )),
      ]);
      vm.attachActions(secondScript);
      await vm.flipType(DetectedContentType.DETECTED_CONTENT_TYPE_EVENT);
      await _drain();

      state = container.read(unifiedCreateViewModelProvider);
      expect(state.mediaIds, ['user-media-123'],
          reason: 'stock imagery must not replace the user-supplied photo');
      expect(state.mediaId, 'user-media-123',
          reason: 'singular mediaId records the user-uploaded asset');
    });

    test('gear final populates itemDetails from DetectedGearItem', () async {
      final gear = DetectedGearItem(
        title: 'DeWalt Drill',
        brand: 'DeWalt',
        model: 'DCD771C2',
        valueEstimate: ValueEstimate(estimatedValueUsd: 129.99),
        weightGrams: Estimate(mean: 1500),
      );
      final actions = _ScriptedActions([
        const UnifiedCreateEventType(
            DetectedContentType.DETECTED_CONTENT_TYPE_GEAR),
        UnifiedCreateEventFinal(StreamGenUnifiedCreateFinal(
          type: DetectedContentType.DETECTED_CONTENT_TYPE_GEAR,
          gear: GenGearResponse(detectedGear: gear),
        )),
      ]);
      final container = ProviderContainer();
      addTearDown(container.dispose);
      final vm = _vmWith(container, actions);
      vm.setPrompt('drill');
      await vm.start();
      await _drain();

      final details = container.read(unifiedCreateViewModelProvider).itemDetails;
      expect(details, isNotNull);
      expect(details!.brand, 'DeWalt');
      expect(details.model, 'DCD771C2');
      expect(details.estValueUsd, '129.99',
          reason: 'value is raw numeric — widget renders currency');
      // 1500g rounds up to kg with one decimal.
      expect(details.weight, '1.5');
      expect(details.weightUnit, 'kg');
    });

    test('gear final picks grams when weight < 1000g', () async {
      final gear = DetectedGearItem(
        title: 'Headphones',
        weightGrams: Estimate(mean: 250),
      );
      final actions = _ScriptedActions([
        const UnifiedCreateEventType(
            DetectedContentType.DETECTED_CONTENT_TYPE_GEAR),
        UnifiedCreateEventFinal(StreamGenUnifiedCreateFinal(
          type: DetectedContentType.DETECTED_CONTENT_TYPE_GEAR,
          gear: GenGearResponse(detectedGear: gear),
        )),
      ]);
      final container = ProviderContainer();
      addTearDown(container.dispose);
      final vm = _vmWith(container, actions);
      vm.setPrompt('headphones');
      await vm.start();
      await _drain();

      final details = container.read(unifiedCreateViewModelProvider).itemDetails;
      expect(details, isNotNull);
      expect(details!.weight, '250');
      expect(details.weightUnit, 'g');
    });

    test('gear final with empty metadata leaves itemDetails null', () async {
      final actions = _ScriptedActions([
        const UnifiedCreateEventType(
            DetectedContentType.DETECTED_CONTENT_TYPE_GEAR),
        UnifiedCreateEventFinal(StreamGenUnifiedCreateFinal(
          type: DetectedContentType.DETECTED_CONTENT_TYPE_GEAR,
          gear: GenGearResponse(detectedGear: DetectedGearItem(title: 'Saw')),
        )),
      ]);
      final container = ProviderContainer();
      addTearDown(container.dispose);
      final vm = _vmWith(container, actions);
      vm.setPrompt('saw');
      await vm.start();
      await _drain();

      expect(
        container.read(unifiedCreateViewModelProvider).itemDetails,
        isNull,
        reason:
            'no extractable metadata means we leave the unset semantic alone',
      );
    });

    test('gear final filters placeholder brand/model values', () async {
      final gear = DetectedGearItem(
        title: 'Saw',
        brand: 'UNKNOWN',
        model: '<UNKNOWN>',
      );
      final actions = _ScriptedActions([
        const UnifiedCreateEventType(
            DetectedContentType.DETECTED_CONTENT_TYPE_GEAR),
        UnifiedCreateEventFinal(StreamGenUnifiedCreateFinal(
          type: DetectedContentType.DETECTED_CONTENT_TYPE_GEAR,
          gear: GenGearResponse(detectedGear: gear),
        )),
      ]);
      final container = ProviderContainer();
      addTearDown(container.dispose);
      final vm = _vmWith(container, actions);
      vm.setPrompt('saw');
      await vm.start();
      await _drain();

      expect(
        container.read(unifiedCreateViewModelProvider).itemDetails,
        isNull,
        reason: 'LLM sentinel values must not leak into the editable state',
      );
    });

    test('gear final does not overwrite user-edited itemDetails', () async {
      // First stream populates from AI.
      final firstGear = DetectedGearItem(brand: 'DeWalt', model: 'DCD771C2');
      final firstScript = _ScriptedActions([
        const UnifiedCreateEventType(
            DetectedContentType.DETECTED_CONTENT_TYPE_GEAR),
        UnifiedCreateEventFinal(StreamGenUnifiedCreateFinal(
          type: DetectedContentType.DETECTED_CONTENT_TYPE_GEAR,
          gear: GenGearResponse(detectedGear: firstGear),
        )),
      ]);
      final container = ProviderContainer();
      addTearDown(container.dispose);
      final vm = _vmWith(container, firstScript);
      vm.setPrompt('drill');
      await vm.start();
      await _drain();

      // User overrides.
      vm.setItemDetails(const ItemDetailsValue(brand: 'Milwaukee'));

      // Re-stream emits new AI values.
      final secondGear =
          DetectedGearItem(brand: 'Bosch', model: 'GSR12V-300');
      final secondScript = _ScriptedActions([
        const UnifiedCreateEventType(
            DetectedContentType.DETECTED_CONTENT_TYPE_GEAR),
        UnifiedCreateEventFinal(StreamGenUnifiedCreateFinal(
          type: DetectedContentType.DETECTED_CONTENT_TYPE_GEAR,
          gear: GenGearResponse(detectedGear: secondGear),
        )),
      ]);
      vm.attachActions(secondScript);
      await vm.flipType(DetectedContentType.DETECTED_CONTENT_TYPE_GEAR);
      await _drain();

      final details = container.read(unifiedCreateViewModelProvider).itemDetails;
      expect(details, isNotNull);
      expect(details!.brand, 'Milwaukee',
          reason: 'user edit must survive AI re-stream');
    });

    test('non-gear final leaves itemDetails untouched', () async {
      final actions = _ScriptedActions([
        const UnifiedCreateEventType(
            DetectedContentType.DETECTED_CONTENT_TYPE_EVENT),
        UnifiedCreateEventFinal(StreamGenUnifiedCreateFinal(
          type: DetectedContentType.DETECTED_CONTENT_TYPE_EVENT,
        )),
      ]);
      final container = ProviderContainer();
      addTearDown(container.dispose);
      final vm = _vmWith(container, actions);
      vm.setPrompt('hike sunday');
      await vm.start();
      await _drain();

      expect(
        container.read(unifiedCreateViewModelProvider).itemDetails,
        isNull,
      );
    });

    // Terminal-payload fallback (#2687): unary-wrapped providers may
    // deliver title/description only in the final payload (no mid-stream
    // field events). The reducer must fill unset, un-edited fields from
    // the final so the Save gate can open — most visible in image mode,
    // which has no prompt for the server to echo as a description.
    test('final fills title/description from gear payload when stream '
        'never sent them', () async {
      final actions = _ScriptedActions([
        const UnifiedCreateEventType(
            DetectedContentType.DETECTED_CONTENT_TYPE_GEAR),
        UnifiedCreateEventFinal(StreamGenUnifiedCreateFinal(
          type: DetectedContentType.DETECTED_CONTENT_TYPE_GEAR,
          gear: GenGearResponse(
            detectedGear: DetectedGearItem(
              title: 'Spaghetti',
              description: 'Unopened box of dried spaghetti.',
            ),
          ),
        )),
      ]);
      final container = ProviderContainer();
      addTearDown(container.dispose);
      final vm = _vmWith(container, actions);
      vm.setPrompt('ignored');
      await vm.start();
      await _drain();

      final state = container.read(unifiedCreateViewModelProvider);
      expect(state.title, 'Spaghetti');
      expect(state.description, 'Unopened box of dried spaghetti.');
      expect(state.isSaveable, true,
          reason: 'terminal-only providers must still yield a saveable '
              'preview');
    });

    test('final fills name/description from experience payload', () async {
      final actions = _ScriptedActions([
        const UnifiedCreateEventType(
            DetectedContentType.DETECTED_CONTENT_TYPE_EVENT),
        UnifiedCreateEventFinal(StreamGenUnifiedCreateFinal(
          type: DetectedContentType.DETECTED_CONTENT_TYPE_EVENT,
          experience: GenExperienceResponse(
            name: 'Sunday Dinner',
            description: 'Bring the whole crew, kids cooking.',
          ),
        )),
      ]);
      final container = ProviderContainer();
      addTearDown(container.dispose);
      final vm = _vmWith(container, actions);
      vm.setPrompt('ignored');
      await vm.start();
      await _drain();

      final state = container.read(unifiedCreateViewModelProvider);
      expect(state.title, 'Sunday Dinner');
      expect(state.description, 'Bring the whole crew, kids cooking.');
    });

    test('final does not overwrite mid-stream title/description', () async {
      final actions = _ScriptedActions([
        const UnifiedCreateEventType(
            DetectedContentType.DETECTED_CONTENT_TYPE_GEAR),
        const UnifiedCreateEventTitle('Streamed title'),
        const UnifiedCreateEventDescription('Streamed description'),
        UnifiedCreateEventFinal(StreamGenUnifiedCreateFinal(
          type: DetectedContentType.DETECTED_CONTENT_TYPE_GEAR,
          gear: GenGearResponse(
            detectedGear: DetectedGearItem(
              title: 'Final title',
              description: 'Final description',
            ),
          ),
        )),
      ]);
      final container = ProviderContainer();
      addTearDown(container.dispose);
      final vm = _vmWith(container, actions);
      vm.setPrompt('ignored');
      await vm.start();
      await _drain();

      final state = container.read(unifiedCreateViewModelProvider);
      expect(state.title, 'Streamed title',
          reason: 'mid-stream events take precedence over the terminal '
              'payload');
      expect(state.description, 'Streamed description');
    });

    test('final does not overwrite user-edited description', () async {
      final actions = _ScriptedActions([
        const UnifiedCreateEventType(
            DetectedContentType.DETECTED_CONTENT_TYPE_GEAR),
        UnifiedCreateEventFinal(StreamGenUnifiedCreateFinal(
          type: DetectedContentType.DETECTED_CONTENT_TYPE_GEAR,
          gear: GenGearResponse(
            detectedGear: DetectedGearItem(
              title: 'Final title',
              description: 'Final description',
            ),
          ),
        )),
      ]);
      final container = ProviderContainer();
      addTearDown(container.dispose);
      final vm = _vmWith(container, actions);
      vm.setPrompt('ignored');
      // User clears then types their own description before the final
      // lands (userEditedFields marks it even though the value is empty
      // at final time for the title).
      final startFuture = vm.start();
      vm.editDescription('My own words');
      await startFuture;
      await _drain();

      final state = container.read(unifiedCreateViewModelProvider);
      expect(state.description, 'My own words',
          reason: 'user edits always win over the terminal payload');
    });
  });

  group('UnifiedCreateViewModel.useCandidate', () {
    test('imports candidate and swaps into mediaIds', () async {
      final fakeRepo = _FakeMediaRepository()
        ..stubAddMediaFromUrlReturns('imported-1');
      final container = ProviderContainer(
        overrides: [
          mediaRepositoryProvider.overrideWithValue(fakeRepo),
          observabilityServiceProvider.overrideWithValue(ObservabilityService()),
        ],
      );
      addTearDown(container.dispose);

      final candidate = MediaCandidate(
        url: 'https://example.com/alt.jpg',
        contentType: 'image/jpeg',
        provider: StockImageProvider.STOCK_IMAGE_PROVIDER_PEXELS,
        providerPhotoId: 'pexels-42',
      );
      final script = _ScriptedActions([
        const UnifiedCreateEventType(
          DetectedContentType.DETECTED_CONTENT_TYPE_GEAR,
        ),
        UnifiedCreateEventMedia(const ['picked'], [candidate]),
      ]);
      final vm = _vmWith(container, script);
      vm.setPrompt('drill');
      await vm.start();
      await _drain();

      expect(
        container.read(unifiedCreateViewModelProvider).mediaCandidates.length,
        1,
      );

      await vm.useCandidate(0);

      final state = container.read(unifiedCreateViewModelProvider);
      expect(state.mediaIds, ['imported-1']);
      expect(state.candidateImportingIndex, isNull);
      expect(fakeRepo.addMediaFromUrlCalls.length, 1);
      expect(
        fakeRepo.addMediaFromUrlCalls.first.url,
        'https://example.com/alt.jpg',
      );
    });

    test('surfaces error on import failure and clears spinner', () async {
      final fakeRepo = _FakeMediaRepository()
        ..stubAddMediaFromUrlThrows(Exception('network down'));
      final container = ProviderContainer(
        overrides: [
          mediaRepositoryProvider.overrideWithValue(fakeRepo),
          observabilityServiceProvider.overrideWithValue(ObservabilityService()),
        ],
      );
      addTearDown(container.dispose);

      final script = _ScriptedActions([
        const UnifiedCreateEventType(
          DetectedContentType.DETECTED_CONTENT_TYPE_GEAR,
        ),
        UnifiedCreateEventMedia(const [], [
          MediaCandidate(url: 'https://x', contentType: 'image/jpeg'),
        ]),
      ]);
      final vm = _vmWith(container, script);
      vm.setPrompt('drill');
      await vm.start();
      await _drain();

      await vm.useCandidate(0);

      final state = container.read(unifiedCreateViewModelProvider);
      expect(state.candidateImportingIndex, isNull);
      expect(state.errorMessage, contains('Replace media failed'));
    });

    test('no-op when index out of bounds', () async {
      final fakeRepo = _FakeMediaRepository();
      final container = ProviderContainer(
        overrides: [
          mediaRepositoryProvider.overrideWithValue(fakeRepo),
        ],
      );
      addTearDown(container.dispose);
      final vm = container.read(unifiedCreateViewModelProvider.notifier);

      await vm.useCandidate(0); // no candidates seeded — should no-op
      expect(fakeRepo.addMediaFromUrlCalls, isEmpty);
    });
  });

  // The intent-specific Home affordances ("Plan an event", "Ask for help",
  // "Offer something") each declare their type up front. Without the seed
  // all three open the same undifferentiated composer, because under the
  // unified-create flag every openBlankCreate* helper is the same call
  // (#2936).
  group('UnifiedCreateViewModel.seedTargetType', () {
    test('event seeds the type, opens on text, and forces the type on the '
        'stream', () async {
      final actions = _ScriptedActions([
        UnifiedCreateEventFinal(StreamGenUnifiedCreateFinal(
          type: DetectedContentType.DETECTED_CONTENT_TYPE_EVENT,
        )),
      ]);
      final container = ProviderContainer();
      addTearDown(container.dispose);
      final vm = _vmWith(container, actions);

      vm.seedTargetType(DetectedContentType.DETECTED_CONTENT_TYPE_EVENT);

      var state = container.read(unifiedCreateViewModelProvider);
      expect(state.targetType, DetectedContentType.DETECTED_CONTENT_TYPE_EVENT);
      expect(state.type, DetectedContentType.DETECTED_CONTENT_TYPE_EVENT,
          reason: 'the card should open in the declared shape, not flicker '
              'through "no type" until the first wire event');
      expect(state.inputMode, CreateInputMode.text,
          reason: 'an event is not a thing you can photograph');

      vm.setPrompt('hike at Zilker park Sunday 2pm');
      await vm.start();
      await _drain();

      expect(actions.lastForceType,
          DetectedContentType.DETECTED_CONTENT_TYPE_EVENT,
          reason: 'the classifier must be skipped when the caller already '
              'declared the type');
      state = container.read(unifiedCreateViewModelProvider);
      expect(state.type, DetectedContentType.DETECTED_CONTENT_TYPE_EVENT);
    });

    test('request opens on text', () {
      final container = ProviderContainer();
      addTearDown(container.dispose);
      final vm = _vmWith(container, _ScriptedActions([]));

      vm.seedTargetType(DetectedContentType.DETECTED_CONTENT_TYPE_REQUEST);

      final state = container.read(unifiedCreateViewModelProvider);
      expect(state.inputMode, CreateInputMode.text);
      expect(
          state.targetType, DetectedContentType.DETECTED_CONTENT_TYPE_REQUEST);
    });

    test('gear stays on image — an item is a thing you photograph', () {
      final container = ProviderContainer();
      addTearDown(container.dispose);
      final vm = _vmWith(container, _ScriptedActions([]));

      vm.seedTargetType(DetectedContentType.DETECTED_CONTENT_TYPE_GEAR);

      final state = container.read(unifiedCreateViewModelProvider);
      expect(state.inputMode, CreateInputMode.image);
      expect(state.targetType, DetectedContentType.DETECTED_CONTENT_TYPE_GEAR);
    });

    test('unseeded blank create leaves the type to the classifier', () async {
      final actions = _ScriptedActions([
        const UnifiedCreateEventType(
            DetectedContentType.DETECTED_CONTENT_TYPE_GEAR),
        UnifiedCreateEventFinal(StreamGenUnifiedCreateFinal(
          type: DetectedContentType.DETECTED_CONTENT_TYPE_GEAR,
        )),
      ]);
      final container = ProviderContainer();
      addTearDown(container.dispose);
      final vm = _vmWith(container, actions);

      vm.setPrompt('climbing rope');
      await vm.start();
      await _drain();

      expect(actions.lastForceType, isNull,
          reason: 'the generic + create must still infer the type');
      final state = container.read(unifiedCreateViewModelProvider);
      expect(state.targetType, isNull);
      expect(state.type, DetectedContentType.DETECTED_CONTENT_TYPE_GEAR);
    });
  });
}
