import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/repositories/request_repository.dart';
import 'package:ripls/presentation/viewmodels/gen_request_view_model.dart';
import 'package:ripls/presentation/viewmodels/request_creation_view_model.dart';
import 'package:ripls/presentation/widgets/creation/input_mode_toggle.dart';
import 'package:ripls/services/providers.dart';

import 'request_creation_view_model_test.mocks.dart';

/// Creates a [StreamGenRequestResponse] with an `error` event.
StreamGenRequestResponse _errorEvent(String message) =>
    StreamGenRequestResponse(error: GenStreamError(message: message));

@GenerateMocks([RequestRepository])
void main() {
  late MockRequestRepository mockRepo;
  late ProviderContainer container;

  setUp(() {
    mockRepo = MockRequestRepository();
    container = ProviderContainer(
      overrides: [
        requestRepositoryProvider.overrideWithValue(mockRepo),
      ],
    );
  });

  tearDown(() {
    container.dispose();
    reset(mockRepo);
  });

  group('initial state', () {
    test('starts in text mode with empty input', () {
      final state = container.read(requestCreationProvider);
      expect(state.inputMode, CreationInputMode.text);
      expect(state.textInput, '');
      expect(state.selectedImagePath, isNull);
      expect(state.isLoading, isFalse);
      expect(state.error, isNull);
    });
  });

  group('setInputMode', () {
    test('switches to image mode', () {
      final notifier = container.read(requestCreationProvider.notifier);
      notifier.setInputMode(CreationInputMode.image);

      expect(
        container.read(requestCreationProvider).inputMode,
        CreationInputMode.image,
      );
    });

    test('no-op when same mode', () {
      final notifier = container.read(requestCreationProvider.notifier);
      notifier.setInputMode(CreationInputMode.text);
      // No state change
      expect(
        container.read(requestCreationProvider).inputMode,
        CreationInputMode.text,
      );
    });

    test('clears errorMessage on switch', () {
      final notifier = container.read(requestCreationProvider.notifier);
      notifier.setTextInput('x'); // short text to trigger error state
      notifier.setInputMode(CreationInputMode.image);
      expect(container.read(requestCreationProvider).error, isNull);
    });
  });

  group('setTextInput', () {
    test('updates textInput', () {
      final notifier = container.read(requestCreationProvider.notifier);
      notifier.setTextInput('I need a ladder');
      expect(
          container.read(requestCreationProvider).textInput, 'I need a ladder');
    });
  });

  group('setImagePath / clearImage', () {
    test('setImagePath updates selectedImagePath', () {
      final notifier = container.read(requestCreationProvider.notifier);
      notifier.setImagePath('/path/to/photo.jpg');
      expect(container.read(requestCreationProvider).selectedImagePath,
          '/path/to/photo.jpg');
    });

    test('clearImage removes selectedImagePath', () {
      final notifier = container.read(requestCreationProvider.notifier);
      notifier.setImagePath('/path/to/photo.jpg');
      notifier.clearImage();
      expect(
          container.read(requestCreationProvider).selectedImagePath, isNull);
    });
  });

  group('canGenerate', () {
    test('text mode: false when empty', () {
      final notifier = container.read(requestCreationProvider.notifier);
      expect(notifier.canGenerate(), isFalse);
    });

    test('text mode: true when non-empty text', () {
      final notifier = container.read(requestCreationProvider.notifier);
      notifier.setTextInput('I need a ladder');
      expect(notifier.canGenerate(), isTrue);
    });

    test('image mode: always true', () {
      final notifier = container.read(requestCreationProvider.notifier);
      notifier.setInputMode(CreationInputMode.image);
      expect(notifier.canGenerate(), isTrue);
    });
  });

  group('generateRequestStreaming', () {
    test('returns false when in image mode', () async {
      final notifier = container.read(requestCreationProvider.notifier);
      notifier.setInputMode(CreationInputMode.image);
      final result = await notifier.generateRequestStreaming();
      expect(result, isFalse);
    });

    test('returns false when text is empty', () async {
      final notifier = container.read(requestCreationProvider.notifier);
      // text mode but empty input
      final result = await notifier.generateRequestStreaming();
      expect(result, isFalse);
    });

    test('starts streaming and returns true for text mode', () async {
      when(mockRepo.streamGenRequest(
        prompt: anyNamed('prompt'),
        locationId: anyNamed('locationId'),
        latitudeDeg: anyNamed('latitudeDeg'),
        longitudeDeg: anyNamed('longitudeDeg'),
      )).thenAnswer((_) => const Stream.empty());

      final notifier = container.read(requestCreationProvider.notifier);
      notifier.setTextInput('I need a hammer');
      final result = await notifier.generateRequestStreaming();
      expect(result, isTrue);
    });
  });

  group('integration: generateRequestStreaming error paths', () {
    // Drains pending microtasks so stream events are processed before assertions.
    Future<void> pump() => Future.delayed(Duration.zero);

    // requestCreationProvider is autoDispose. Subscribe a persistent listener
    // before each test so the provider (and its stream subscription) stays
    // alive across awaits.
    late ProviderSubscription<RequestCreationState> keepAlive;

    setUp(() {
      keepAlive = container.listen<RequestCreationState>(
        requestCreationProvider,
        (_, _) {},
      );
    });

    tearDown(() {
      keepAlive.close();
    });

    test('error event: genRequestProvider.error is non-null and isLoading=false',
        () async {
      final streamController = StreamController<StreamGenRequestResponse>();
      when(mockRepo.streamGenRequest(
        prompt: anyNamed('prompt'),
        mediaId: anyNamed('mediaId'),
        communityId: anyNamed('communityId'),
        locationId: anyNamed('locationId'),
        latitudeDeg: anyNamed('latitudeDeg'),
        longitudeDeg: anyNamed('longitudeDeg'),
      )).thenAnswer((_) => streamController.stream);

      final notifier = container.read(requestCreationProvider.notifier);
      notifier.setTextInput('I need a ladder');

      final ok = await notifier.generateRequestStreaming();
      expect(ok, isTrue);

      streamController.add(_errorEvent('AI service unavailable'));
      await streamController.close();
      await pump();
      await pump();

      final genState = container.read(genRequestProvider);
      expect(genState.error, isNotNull);

      final creationState = container.read(requestCreationProvider);
      expect(creationState.error, isNotNull);
      expect(creationState.isLoading, isFalse);
    });

    test('transport error: stream exception; genRequestProvider.error is non-null',
        () async {
      final streamController = StreamController<StreamGenRequestResponse>();
      addTearDown(streamController.close);
      when(mockRepo.streamGenRequest(
        prompt: anyNamed('prompt'),
        mediaId: anyNamed('mediaId'),
        communityId: anyNamed('communityId'),
        locationId: anyNamed('locationId'),
        latitudeDeg: anyNamed('latitudeDeg'),
        longitudeDeg: anyNamed('longitudeDeg'),
      )).thenAnswer((_) => streamController.stream);

      final notifier = container.read(requestCreationProvider.notifier);
      notifier.setTextInput('I need a power drill');

      await notifier.generateRequestStreaming();

      streamController.addError(Exception('network timeout'));
      await pump();
      await pump();

      final genState = container.read(genRequestProvider);
      expect(genState.error, isNotNull);

      expect(container.read(requestCreationProvider).error, isNotNull);
      expect(container.read(requestCreationProvider).isLoading, isFalse);
    });
  });

  group('reset', () {
    test('resets state to initial values', () {
      final notifier = container.read(requestCreationProvider.notifier);
      notifier.setTextInput('something');
      notifier.setInputMode(CreationInputMode.image);
      notifier.reset();

      final state = container.read(requestCreationProvider);
      expect(state.inputMode, CreationInputMode.text);
      expect(state.textInput, '');
      expect(state.isLoading, isFalse);
    });
  });
}
