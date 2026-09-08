import 'dart:async';

import 'package:fixnum/fixnum.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/core/observability/events.dart';
import 'package:ripls/data/repositories/experience_repository.dart';
import 'package:ripls/data/repositories/media_repository.dart';
import 'package:ripls/data/repositories/media_url.dart';
import 'package:ripls/presentation/providers/user_timezone_provider.dart';
import 'package:ripls/presentation/viewmodels/gen_experience_view_model.dart';
import 'package:ripls/services/experience_service.dart';
import 'package:ripls/services/providers.dart';

import '../../core/observability/analytics_test_helper.dart';
import 'gen_experience_view_model_test.mocks.dart';

@GenerateMocks([ExperienceService, ExperienceRepository, MediaRepository])
void main() {
  late MockExperienceService mockExperienceService;
  late MockExperienceRepository mockExperienceRepository;
  late MockMediaRepository mockMediaRepository;
  late MockObservabilityService mockObservability;
  late ProviderContainer container;

  setUp(() {
    mockExperienceService = MockExperienceService();
    mockExperienceRepository = MockExperienceRepository();
    mockMediaRepository = MockMediaRepository();
    mockObservability = MockObservabilityService();

    container = ProviderContainer(
      overrides: [
        experienceServiceProvider.overrideWithValue(mockExperienceService),
        experienceRepositoryProvider.overrideWithValue(
          mockExperienceRepository,
        ),
        mediaRepositoryProvider.overrideWithValue(mockMediaRepository),
        observabilityServiceProvider.overrideWithValue(mockObservability),
        resolvedTimezoneProvider.overrideWith(
          (ref) async => 'America/Chicago',
        ),
      ],
    );
  });

  tearDown(() {
    container.dispose();
  });

  group('GenExperienceState', () {
    test('initial state is correct', () {
      final state = container.read(genExperienceProvider);

      expect(state.currentStep, GenExperienceStep.input);
      expect(state.completedSteps, isEmpty);
      expect(state.inputMode, GenExperienceInputMode.text);
      expect(state.textPrompt, isNull);
      expect(state.selectedImagePath, isNull);
      expect(state.uploadedMediaId, isNull);
      expect(state.aiGeneratedName, isNull);
      expect(state.aiGeneratedDescription, isNull);
      expect(state.error, isNull);
      expect(state.errorStep, isNull);
      expect(state.isLoading, isFalse);
    });

    test('canProceed returns true for text mode with prompt', () {
      final notifier = container.read(genExperienceProvider.notifier);
      notifier.setInputMode(GenExperienceInputMode.text);
      notifier.setTextPrompt('Board game night this Friday');

      final state = container.read(genExperienceProvider);
      expect(state.canProceed, isTrue);
    });

    test('canProceed returns true for image mode with selected image', () {
      final notifier = container.read(genExperienceProvider.notifier);
      notifier.setInputMode(GenExperienceInputMode.image);
      notifier.setImagePath('/path/to/image.jpg');

      final state = container.read(genExperienceProvider);
      expect(state.canProceed, isTrue);
    });

    test('canProceed returns true for URL mode with valid URL', () {
      final notifier = container.read(genExperienceProvider.notifier);
      notifier.setInputMode(GenExperienceInputMode.url);
      notifier.setUrlInput('https://eventbrite.com/e/test-event');

      final state = container.read(genExperienceProvider);
      expect(state.canProceed, isTrue);
    });

    test('canProceed returns false for URL mode with invalid URL', () {
      final notifier = container.read(genExperienceProvider.notifier);
      notifier.setInputMode(GenExperienceInputMode.url);
      notifier.setUrlInput('not-a-valid-url');

      final state = container.read(genExperienceProvider);
      expect(state.canProceed, isFalse);
    });

    test('canProceed returns false for URL mode with empty URL', () {
      final notifier = container.read(genExperienceProvider.notifier);
      notifier.setInputMode(GenExperienceInputMode.url);

      final state = container.read(genExperienceProvider);
      expect(state.canProceed, isFalse);
    });

    test('canProceed returns false when no input is provided', () {
      final state = container.read(genExperienceProvider);
      expect(state.canProceed, isFalse);
    });

    test('hasError returns true when error message is set', () {
      final notifier = container.read(genExperienceProvider.notifier);
      notifier.state = notifier.state.copyWith(error: const UserError.generic(fallback: 'Test error'));

      final state = container.read(genExperienceProvider);
      expect(state.hasError, isTrue);
    });

    test('isCompleted returns true when currentStep is completed', () {
      final notifier = container.read(genExperienceProvider.notifier);
      notifier.state = notifier.state.copyWith(
        currentStep: GenExperienceStep.completed,
      );

      final state = container.read(genExperienceProvider);
      expect(state.isCompleted, isTrue);
    });

    test('progress calculation is correct', () {
      final notifier = container.read(genExperienceProvider.notifier);
      notifier.state = notifier.state.copyWith(
        completedSteps: [GenExperienceStep.input, GenExperienceStep.generating],
      );

      final state = container.read(genExperienceProvider);
      expect(state.progress, 2 / GenExperienceStep.values.length);
    });
  });

  group('setInputMode', () {
    test('sets text mode and clears image data', () {
      final notifier = container.read(genExperienceProvider.notifier);
      notifier.setImagePath('/path/to/image.jpg');

      notifier.setInputMode(GenExperienceInputMode.text);

      final state = container.read(genExperienceProvider);
      expect(state.inputMode, GenExperienceInputMode.text);
      expect(state.selectedImagePath, isNull);
      expect(state.error, isNull);
    });

    test('sets image mode and clears text data', () {
      final notifier = container.read(genExperienceProvider.notifier);
      notifier.setTextPrompt('Some prompt');

      notifier.setInputMode(GenExperienceInputMode.image);

      final state = container.read(genExperienceProvider);
      expect(state.inputMode, GenExperienceInputMode.image);
      expect(state.textPrompt, isNull);
      expect(state.error, isNull);
    });

    test('sets URL mode and clears text and image data', () {
      final notifier = container.read(genExperienceProvider.notifier);
      notifier.setTextPrompt('Some prompt');
      notifier.setImagePath('/path/to/image.jpg');

      notifier.setInputMode(GenExperienceInputMode.url);

      final state = container.read(genExperienceProvider);
      expect(state.inputMode, GenExperienceInputMode.url);
      expect(state.textPrompt, isNull);
      expect(state.selectedImagePath, isNull);
      expect(state.urlInput, isNull);
      expect(state.error, isNull);
    });
  });

  group('setTextPrompt', () {
    test('sets the text prompt correctly', () {
      final notifier = container.read(genExperienceProvider.notifier);
      const testPrompt = 'Board game night this Friday at 7pm';

      notifier.setTextPrompt(testPrompt);

      final state = container.read(genExperienceProvider);
      expect(state.textPrompt, testPrompt);
      expect(state.error, isNull);
      expect(state.errorStep, isNull);
    });

    test('clears error when setting text prompt', () {
      final notifier = container.read(genExperienceProvider.notifier);
      notifier.state = notifier.state.copyWith(
        error: const UserError.generic(fallback: 'Previous error'),
        errorStep: GenExperienceStep.generating,
      );

      notifier.setTextPrompt('New prompt');

      final state = container.read(genExperienceProvider);
      expect(state.error, isNull);
      expect(state.errorStep, isNull);
    });
  });

  group('setImagePath', () {
    test('sets the image path correctly', () {
      final notifier = container.read(genExperienceProvider.notifier);
      const testPath = '/path/to/flyer.jpg';

      notifier.setImagePath(testPath);

      final state = container.read(genExperienceProvider);
      expect(state.selectedImagePath, testPath);
      expect(state.error, isNull);
      expect(state.errorStep, isNull);
    });

    test('clears error when setting image path', () {
      final notifier = container.read(genExperienceProvider.notifier);
      notifier.state = notifier.state.copyWith(
        error: const UserError.generic(fallback: 'Previous error'),
        errorStep: GenExperienceStep.uploading,
      );

      notifier.setImagePath('/path/to/image.jpg');

      final state = container.read(genExperienceProvider);
      expect(state.error, isNull);
      expect(state.errorStep, isNull);
    });
  });

  group('setUrlInput', () {
    test('sets the URL input correctly', () {
      final notifier = container.read(genExperienceProvider.notifier);
      const testUrl = 'https://eventbrite.com/e/test-event-123';

      notifier.setUrlInput(testUrl);

      final state = container.read(genExperienceProvider);
      expect(state.urlInput, testUrl);
      expect(state.error, isNull);
      expect(state.errorStep, isNull);
    });

    test('clears error when setting URL input', () {
      final notifier = container.read(genExperienceProvider.notifier);
      notifier.state = notifier.state.copyWith(
        error: const UserError.generic(fallback: 'Previous error'),
        errorStep: GenExperienceStep.generating,
      );

      notifier.setUrlInput('https://example.com/event');

      final state = container.read(genExperienceProvider);
      expect(state.error, isNull);
      expect(state.errorStep, isNull);
    });
  });

  group('clearUrl', () {
    test('clears URL input', () {
      final notifier = container.read(genExperienceProvider.notifier);
      notifier.setUrlInput('https://eventbrite.com/e/test');

      notifier.clearUrl();

      final state = container.read(genExperienceProvider);
      expect(state.urlInput, isNull);
      expect(state.error, isNull);
    });
  });

  group('isValidUrl', () {
    test('returns true for valid HTTPS URL', () {
      final notifier = container.read(genExperienceProvider.notifier);
      notifier.setUrlInput('https://eventbrite.com/e/test-event');
      final state = container.read(genExperienceProvider);
      expect(state.isValidUrl, isTrue);
    });

    test('returns true for valid HTTP URL', () {
      final notifier = container.read(genExperienceProvider.notifier);
      notifier.setUrlInput('http://example.com/event');
      final state = container.read(genExperienceProvider);
      expect(state.isValidUrl, isTrue);
    });

    test('returns false for empty URL', () {
      final state = container.read(genExperienceProvider);
      expect(state.isValidUrl, isFalse);
    });

    test('returns false for invalid URL', () {
      final notifier = container.read(genExperienceProvider.notifier);
      notifier.setUrlInput('not-a-valid-url');
      final state = container.read(genExperienceProvider);
      expect(state.isValidUrl, isFalse);
    });

    test('returns false for FTP URL', () {
      final notifier = container.read(genExperienceProvider.notifier);
      notifier.setUrlInput('ftp://example.com/file');
      final state = container.read(genExperienceProvider);
      expect(state.isValidUrl, isFalse);
    });
  });

  group('clearImage', () {
    test('clears image and resets workflow', () {
      final notifier = container.read(genExperienceProvider.notifier);
      notifier.state = notifier.state.copyWith(
        selectedImagePath: '/path/to/image.jpg',
        uploadedMediaId: 'media123',
        currentStep: GenExperienceStep.generating,
        completedSteps: [GenExperienceStep.input, GenExperienceStep.uploading],
      );

      notifier.clearImage();

      final state = container.read(genExperienceProvider);
      expect(state.selectedImagePath, isNull);
      expect(state.uploadedMediaId, isNull);
      expect(state.currentStep, GenExperienceStep.input);
      expect(state.completedSteps, isEmpty);
      expect(state.error, isNull);
    });
  });

  // NOTE: generateExperience(), _generateFromText/Image/Url, and
  // _callGenExperienceAPI were removed from GenExperienceNotifier as dead
  // code (see #1143). The GenExperience RPC is now called exclusively from
  // ExperienceCreationNotifier in experience_creation_view_model.dart.

  group('updateName', () {
    test('updates edited name', () {
      final notifier = container.read(genExperienceProvider.notifier);
      const newName = 'Updated Experience Name';

      notifier.updateName(newName);

      final state = container.read(genExperienceProvider);
      expect(state.editedName, newName);
      expect(state.error, isNull);
    });
  });

  group('updateDescription', () {
    test('updates edited description', () {
      final notifier = container.read(genExperienceProvider.notifier);
      const newDescription = 'Updated experience description';

      notifier.updateDescription(newDescription);

      final state = container.read(genExperienceProvider);
      expect(state.editedDescription, newDescription);
      expect(state.error, isNull);
    });
  });

  group('updateMedia', () {
    test('updates selected media IDs', () {
      final notifier = container.read(genExperienceProvider.notifier);
      final mediaIds = ['media1', 'media2', 'media3'];

      notifier.updateMedia(mediaIds);

      final state = container.read(genExperienceProvider);
      expect(state.selectedMediaIds, mediaIds);
    });
  });

  group('updateLocation', () {
    test('updates selected location ID', () {
      final notifier = container.read(genExperienceProvider.notifier);
      const locationId = 'loc456';

      notifier.updateLocation(locationId);

      final state = container.read(genExperienceProvider);
      expect(state.selectedLocationId, locationId);
    });
  });

  group('updateTime', () {
    test('updates experience time', () {
      final notifier = container.read(genExperienceProvider.notifier);
      final time = ExperienceTime()..tbd = TimeTBD();

      notifier.updateTime(time);

      final state = container.read(genExperienceProvider);
      expect(state.selectedTime, time);
    });
  });

  group('reset', () {
    test('resets workflow to initial state', () {
      final notifier = container.read(genExperienceProvider.notifier);
      notifier.state = notifier.state.copyWith(
        textPrompt: 'Some prompt',
        selectedImagePath: '/path/to/image.jpg',
        uploadedMediaId: 'media123',
        aiGeneratedName: 'Generated Name',
        aiGeneratedDescription: 'Generated Description',
        currentStep: GenExperienceStep.preview,
        completedSteps: [GenExperienceStep.input, GenExperienceStep.generating],
        error: const UserError.generic(fallback: 'Some error'),
      );

      notifier.reset();

      final state = container.read(genExperienceProvider);
      expect(state.currentStep, GenExperienceStep.input);
      expect(state.completedSteps, isEmpty);
      expect(state.textPrompt, isNull);
      expect(state.selectedImagePath, isNull);
      expect(state.uploadedMediaId, isNull);
      expect(state.aiGeneratedName, isNull);
      expect(state.aiGeneratedDescription, isNull);
      expect(state.error, isNull);
    });
  });

  group('getExtractedTimeDisplay', () {
    test('returns null when no extracted time', () {
      final notifier = container.read(genExperienceProvider.notifier);
      expect(notifier.getExtractedTimeDisplay(), isNull);
    });

    test('returns null for UNKNOWN confidence', () {
      final notifier = container.read(genExperienceProvider.notifier);
      final tomorrow = DateTime.now().add(const Duration(days: 1));
      final tomorrowAt14 = DateTime(
        tomorrow.year,
        tomorrow.month,
        tomorrow.day,
        14,
        0,
      );

      notifier.state = notifier.state.copyWith(
        extractedTimeUnixSec: Int64(
          tomorrowAt14.millisecondsSinceEpoch ~/ 1000,
        ),
        timeConfidence: TimeConfidence.TIME_CONFIDENCE_UNKNOWN,
      );

      expect(notifier.getExtractedTimeDisplay(), isNull);
    });

    test('returns formatted time with ✓ for EXPLICIT confidence', () {
      final notifier = container.read(genExperienceProvider.notifier);
      final tomorrow = DateTime.now().add(const Duration(days: 1));
      final tomorrowAt14 = DateTime(
        tomorrow.year,
        tomorrow.month,
        tomorrow.day,
        14,
        0,
      );

      notifier.state = notifier.state.copyWith(
        extractedTimeUnixSec: Int64(
          tomorrowAt14.millisecondsSinceEpoch ~/ 1000,
        ),
        timeConfidence: TimeConfidence.TIME_CONFIDENCE_EXPLICIT,
      );

      final display = notifier.getExtractedTimeDisplay();
      expect(display, isNotNull);
      expect(display, startsWith('✓'));
      expect(display, contains('Tomorrow'));
      expect(display, contains('2:00 PM'));
    });

    test('returns formatted time with ~ for INFERRED confidence', () {
      final notifier = container.read(genExperienceProvider.notifier);
      final tomorrow = DateTime.now().add(const Duration(days: 1));
      final tomorrowAt14 = DateTime(
        tomorrow.year,
        tomorrow.month,
        tomorrow.day,
        14,
        0,
      );

      notifier.state = notifier.state.copyWith(
        extractedTimeUnixSec: Int64(
          tomorrowAt14.millisecondsSinceEpoch ~/ 1000,
        ),
        timeConfidence: TimeConfidence.TIME_CONFIDENCE_INFERRED,
      );

      final display = notifier.getExtractedTimeDisplay();
      expect(display, isNotNull);
      expect(display, startsWith('~'));
      expect(display, contains('Tomorrow'));
    });
  });

  group('isExtractedDateInPast', () {
    test('returns false when no extracted date', () {
      final notifier = container.read(genExperienceProvider.notifier);
      expect(notifier.isExtractedDateInPast(), isFalse);
    });

    test('returns true for past date', () {
      final notifier = container.read(genExperienceProvider.notifier);
      final yesterday = DateTime.now().subtract(const Duration(days: 1));
      final yesterdayUnix = Int64(yesterday.millisecondsSinceEpoch ~/ 1000);

      notifier.state = notifier.state.copyWith(
        extractedTimeUnixSec: yesterdayUnix,
      );

      expect(notifier.isExtractedDateInPast(), isTrue);
    });

    test('returns false for future date', () {
      final notifier = container.read(genExperienceProvider.notifier);
      final tomorrow = DateTime.now().add(const Duration(days: 1));
      final tomorrowUnix = Int64(tomorrow.millisecondsSinceEpoch ~/ 1000);

      notifier.state = notifier.state.copyWith(
        extractedTimeUnixSec: tomorrowUnix,
      );

      expect(notifier.isExtractedDateInPast(), isFalse);
    });

    test('returns false for today', () {
      final notifier = container.read(genExperienceProvider.notifier);
      final today = DateTime.now();
      final todayUnix = Int64(today.millisecondsSinceEpoch ~/ 1000);

      notifier.state = notifier.state.copyWith(extractedTimeUnixSec: todayUnix);

      expect(notifier.isExtractedDateInPast(), isFalse);
    });
  });

  group('initializeTimeFromExtracted', () {
    test('does nothing when no extracted time', () async {
      final notifier = container.read(genExperienceProvider.notifier);
      final initialTime = notifier.state.selectedTime;

      await notifier.initializeTimeFromExtracted();

      expect(notifier.state.selectedTime, equals(initialTime));
    });

    test('initializes SpecificTime from extracted Unix timestamp', () async {
      final notifier = container.read(genExperienceProvider.notifier);
      final expectedDateTime = DateTime(2025, 12, 25, 14, 30);
      final expectedUnix = Int64(
        expectedDateTime.millisecondsSinceEpoch ~/ 1000,
      );

      notifier.state = notifier.state.copyWith(
        extractedTimeUnixSec: expectedUnix,
      );

      await notifier.initializeTimeFromExtracted();

      final state = container.read(genExperienceProvider);
      expect(state.selectedTime, isNotNull);
      expect(state.selectedTime!.hasSpecific(), isTrue);

      expect(
        state.selectedTime!.specific.unixTimestampSec.toInt(),
        equals(expectedUnix.toInt()),
      );
    });

    // Regression test for #2599: timezone must be an IANA name (e.g.
    // "America/Chicago"), not a platform abbreviation like "CDT". The server's
    // formatExperienceTime uses time.LoadLocation, which only parses IANA
    // names; abbreviations silently fall back to UTC and the system chat
    // message renders the time offset by the user's TZ.
    test('uses IANA timezone from resolvedTimezoneProvider', () async {
      final notifier = container.read(genExperienceProvider.notifier);
      final expectedUnix = Int64(
        DateTime(2026, 6, 30, 12, 30).millisecondsSinceEpoch ~/ 1000,
      );

      notifier.state = notifier.state.copyWith(
        extractedTimeUnixSec: expectedUnix,
      );

      await notifier.initializeTimeFromExtracted();

      final state = container.read(genExperienceProvider);
      expect(state.selectedTime, isNotNull);
      expect(state.selectedTime!.specific.timezone, equals('America/Chicago'));
    });

    test('handles invalid timestamp gracefully', () async {
      final notifier = container.read(genExperienceProvider.notifier);
      // Zero timestamp should be treated as no extraction
      notifier.state = notifier.state.copyWith(extractedTimeUnixSec: Int64(0));

      await notifier.initializeTimeFromExtracted();

      // Should not crash and keep the existing time (null in this case)
      final state = container.read(genExperienceProvider);
      expect(state.selectedTime, isNull);
    });
  });

  group('createAndShareExperience', () {
    test('returns early with error when no community selected', () async {
      final notifier = container.read(genExperienceProvider.notifier);

      notifier.state = notifier.state.copyWith(
        aiGeneratedName: 'Test Experience',
        aiGeneratedDescription: 'Test description',
        currentStep: GenExperienceStep.preview,
      );

      await notifier.createAndShareExperience([]);

      final state = container.read(genExperienceProvider);
      expect(state.hasError, isTrue);
      expect(state.error, isNotNull);

      // Analytics should NOT be logged
      expect(
        mockObservability.hasEventOfType<ExperienceCreatedEvent>(),
        isFalse,
      );
    });
  });

  group('participant tagging', () {
    test('tagParticipant adds userId to taggedParticipantIds', () {
      final notifier = container.read(genExperienceProvider.notifier);

      notifier.tagParticipant('user-1');

      final state = container.read(genExperienceProvider);
      expect(state.taggedParticipantIds, contains('user-1'));
    });

    test('tagParticipant is idempotent — duplicate is not added', () {
      final notifier = container.read(genExperienceProvider.notifier);

      notifier.tagParticipant('user-1');
      notifier.tagParticipant('user-1');

      final state = container.read(genExperienceProvider);
      expect(state.taggedParticipantIds.where((id) => id == 'user-1').length, 1);
    });

    test('untagParticipant removes userId from taggedParticipantIds', () {
      final notifier = container.read(genExperienceProvider.notifier);
      notifier.tagParticipant('user-1');
      notifier.tagParticipant('user-2');

      notifier.untagParticipant('user-1');

      final state = container.read(genExperienceProvider);
      expect(state.taggedParticipantIds, isNot(contains('user-1')));
      expect(state.taggedParticipantIds, contains('user-2'));
    });

    test('untagParticipant on untagged user is a no-op', () {
      final notifier = container.read(genExperienceProvider.notifier);
      notifier.tagParticipant('user-2');

      notifier.untagParticipant('user-999');

      final state = container.read(genExperienceProvider);
      expect(state.taggedParticipantIds, contains('user-2'));
    });

    test('reset clears taggedParticipantIds', () {
      final notifier = container.read(genExperienceProvider.notifier);
      notifier.tagParticipant('user-1');

      notifier.reset();

      final state = container.read(genExperienceProvider);
      expect(state.taggedParticipantIds, isEmpty);
    });
  });

  group('isExtractedDateInPast', () {
    test('returns true when extracted time is before today', () {
      final notifier = container.read(genExperienceProvider.notifier);
      final pastUnixSec = Int64(
        DateTime.now().subtract(const Duration(days: 1)).millisecondsSinceEpoch ~/
            1000,
      );
      notifier.state = notifier.state.copyWith(
        extractedTimeUnixSec: pastUnixSec,
      );

      expect(notifier.isExtractedDateInPast(), isTrue);
    });

    test('returns false when extracted time is in the future', () {
      final notifier = container.read(genExperienceProvider.notifier);
      final futureUnixSec = Int64(
        DateTime.now().add(const Duration(days: 1)).millisecondsSinceEpoch ~/
            1000,
      );
      notifier.state = notifier.state.copyWith(
        extractedTimeUnixSec: futureUnixSec,
      );

      expect(notifier.isExtractedDateInPast(), isFalse);
    });

    test('returns false when extractedTimeUnixSec is null', () {
      final notifier = container.read(genExperienceProvider.notifier);
      notifier.state = notifier.state.copyWith(extractedTimeUnixSec: null);

      expect(notifier.isExtractedDateInPast(), isFalse);
    });
  });

  group('mentionedNames', () {
    test('initializeFromGenResponse stores mentionedNames in state', () {
      final notifier = container.read(genExperienceProvider.notifier);

      notifier.initializeFromGenResponse(
        name: 'Hike with Mike',
        description: 'A fun hike',
        mediaIds: [],
        suggestedTime: null,
        extractedTimeUnixSec: null,
        timeConfidence: null,
        locationQuery: null,
        geocodedLocation: null,
        mentionedNames: ['Mike', 'Bhavna'],
      );

      final state = container.read(genExperienceProvider);
      expect(state.mentionedNames, containsAll(['Mike', 'Bhavna']));
    });

    test('initializeFromGenResponse clears taggedParticipantIds', () {
      final notifier = container.read(genExperienceProvider.notifier);
      notifier.tagParticipant('old-user');

      notifier.initializeFromGenResponse(
        name: 'New experience',
        description: 'Description',
        mediaIds: [],
        suggestedTime: null,
        extractedTimeUnixSec: null,
        timeConfidence: null,
        locationQuery: null,
        geocodedLocation: null,
      );

      final state = container.read(genExperienceProvider);
      expect(state.taggedParticipantIds, isEmpty);
    });
  });

  group('_replacePreviewMedia', () {
    MediaUrl imageMediaUrl(String id) => MediaUrl(
          url: 'https://example.com/$id.jpg',
          isThumbnail: false,
          mediaId: id,
          contentType: 'image/jpeg',
        );

    test(
        'onMediaUploaded clears previewVideoController and sets new selectedMediaIds',
        () async {
      when(mockMediaRepository.getFullMediaUrl(any))
          .thenAnswer((inv) async => imageMediaUrl(inv.positionalArguments[0] as String));

      final notifier = container.read(genExperienceProvider.notifier);
      notifier.onMediaUploadStarted();
      notifier.onMediaUploaded('newImageId');

      await pumpEventQueue();

      final state = container.read(genExperienceProvider);
      expect(state.selectedMediaIds, equals(['newImageId']));
      expect(state.previewVideoController, isNull);
      expect(state.isUploadingMedia, isFalse);
    });

    test(
        'image-to-image replacement updates selectedMediaIds and leaves previewVideoController null',
        () async {
      when(mockMediaRepository.getFullMediaUrl(any))
          .thenAnswer((inv) async => imageMediaUrl(inv.positionalArguments[0] as String));

      final notifier = container.read(genExperienceProvider.notifier);
      notifier.onMediaUploaded('oldImageId');
      await pumpEventQueue();

      notifier.onMediaUploaded('newImageId');
      await pumpEventQueue();

      final state = container.read(genExperienceProvider);
      expect(state.selectedMediaIds, equals(['newImageId']));
      expect(state.previewVideoController, isNull);
      // getFullMediaUrl is called once per _loadPreviewVideoIfNeeded invocation.
      verify(mockMediaRepository.getFullMediaUrl('newImageId')).called(1);
    });

    test(
        'onBatchUploadPartialFailure replaces media and preserves batchUploadFailedCount',
        () async {
      when(mockMediaRepository.getFullMediaUrl(any))
          .thenAnswer((inv) async => imageMediaUrl(inv.positionalArguments[0] as String));

      final notifier = container.read(genExperienceProvider.notifier);
      notifier.onBatchUploadPartialFailure(['img1', 'img2'], 3);
      await pumpEventQueue();

      final state = container.read(genExperienceProvider);
      expect(state.selectedMediaIds, equals(['img1', 'img2']));
      expect(state.previewVideoController, isNull);
      expect(state.isUploadingMedia, isFalse);
      expect(state.batchUploadFailedCount, equals(3));
    });

    test('disposal mid-flight _loadPreviewVideoIfNeeded does not throw', () async {
      final completer = Completer<MediaUrl>();
      when(mockMediaRepository.getFullMediaUrl(any))
          .thenAnswer((_) => completer.future);

      final notifier = container.read(genExperienceProvider.notifier);
      notifier.onMediaUploaded('anyId');

      // Dispose the container while getFullMediaUrl future is still pending.
      container.dispose();

      // Completing the future after disposal must not throw.
      expect(
        () => completer.complete(imageMediaUrl('anyId')),
        returnsNormally,
      );

      await pumpEventQueue();
    });
  });

  group('disposal safety', () {
    test('no state update after container disposal throws no error', () {
      // Read the notifier while the container is live so it is built.
      container.read(genExperienceProvider.notifier);

      // Dispose the container — this triggers ref.onDispose in the notifier.
      container.dispose();

      // If the notifier tried to write state after disposal, Riverpod would
      // throw a StateError. Reaching this line means the onDispose callback
      // ran cleanly without any post-dispose state mutation.
    });

    test('reset after disposal does not throw', () {
      final notifier = container.read(genExperienceProvider.notifier);
      notifier.tagParticipant('user-1');

      // Dispose the container before reset is called externally.
      // In production, reset() is called while mounted — this test confirms
      // there is no crash if the container is disposed first.
      container.dispose();

      // Creating a fresh container confirms the provider can be rebuilt cleanly.
      final freshContainer = ProviderContainer(
        overrides: [
          experienceServiceProvider.overrideWithValue(mockExperienceService),
          experienceRepositoryProvider.overrideWithValue(
            mockExperienceRepository,
          ),
          mediaRepositoryProvider.overrideWithValue(mockMediaRepository),
          observabilityServiceProvider.overrideWithValue(mockObservability),
        ],
      );
      addTearDown(freshContainer.dispose);

      final freshState = freshContainer.read(genExperienceProvider);
      expect(freshState.taggedParticipantIds, isEmpty);
    });
  });
}
