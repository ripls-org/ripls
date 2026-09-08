import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/data/repositories/experience_repository.dart';
import 'package:ripls/presentation/viewmodels/experience_creation_view_model.dart';
import 'package:ripls/presentation/widgets/creation/input_mode_toggle.dart';
import 'package:ripls/services/providers.dart';

import 'experience_creation_view_model_test.mocks.dart';

@GenerateMocks([ExperienceRepository])
void main() {
  late MockExperienceRepository mockExperienceRepository;
  late ProviderContainer container;

  setUp(() {
    mockExperienceRepository = MockExperienceRepository();

    container = ProviderContainer(
      overrides: [
        experienceRepositoryProvider.overrideWithValue(mockExperienceRepository),
      ],
    );
  });

  tearDown(() {
    container.dispose();
    reset(mockExperienceRepository);
  });

  group('ExperienceCreationState', () {
    test('initial state is correct', () {
      final state = container.read(experienceCreationProvider);

      expect(state.inputMode, CreationInputMode.text);
      expect(state.textInput, isEmpty);
      expect(state.selectedImagePath, isNull);
      expect(state.isLoading, isFalse);
      expect(state.generatedExperience, isNull);
      expect(state.error, isNull);
    });
  });

  group('setInputMode', () {
    test('switches to image mode', () {
      final notifier = container.read(experienceCreationProvider.notifier);

      notifier.setInputMode(CreationInputMode.image);

      final state = container.read(experienceCreationProvider);
      expect(state.inputMode, CreationInputMode.image);
    });

    test('does not change state when setting same mode', () {
      final notifier = container.read(experienceCreationProvider.notifier);
      notifier.setTextInput('test');

      notifier.setInputMode(CreationInputMode.text);

      final state = container.read(experienceCreationProvider);
      expect(state.textInput, 'test');
    });

    test('clears error when switching modes', () {
      final notifier = container.read(experienceCreationProvider.notifier);
      notifier.state = notifier.state.copyWith(
          error: const UserError.generic(fallback: 'Previous error'));

      notifier.setInputMode(CreationInputMode.image);

      final state = container.read(experienceCreationProvider);
      expect(state.error, isNull);
    });
  });

  group('setTextInput', () {
    test('sets text input correctly', () {
      final notifier = container.read(experienceCreationProvider.notifier);

      notifier.setTextInput('Board game night this Friday');

      final state = container.read(experienceCreationProvider);
      expect(state.textInput, 'Board game night this Friday');
    });

    test('clears error when setting text', () {
      final notifier = container.read(experienceCreationProvider.notifier);
      notifier.state = notifier.state.copyWith(
          error: const UserError.generic(fallback: 'Previous error'));

      notifier.setTextInput('New text');

      final state = container.read(experienceCreationProvider);
      expect(state.error, isNull);
    });
  });

  group('extractUrl', () {
    test('extracts HTTPS URL from text', () {
      final notifier = container.read(experienceCreationProvider.notifier);

      final url = notifier.extractUrl('Check out this event https://eventbrite.com/e/test-event');

      expect(url, 'https://eventbrite.com/e/test-event');
    });

    test('extracts HTTP URL from text', () {
      final notifier = container.read(experienceCreationProvider.notifier);

      final url = notifier.extractUrl('Event link: http://example.com/event');

      expect(url, 'http://example.com/event');
    });

    test('extracts URL with path and query params', () {
      final notifier = container.read(experienceCreationProvider.notifier);

      final url = notifier.extractUrl('https://meetup.com/events/123?source=app');

      expect(url, 'https://meetup.com/events/123?source=app');
    });

    test('extracts first URL when multiple present', () {
      final notifier = container.read(experienceCreationProvider.notifier);

      final url = notifier.extractUrl(
        'First: https://first.com/event Second: https://second.com/event',
      );

      expect(url, 'https://first.com/event');
    });

    test('extracts URL at start of text', () {
      final notifier = container.read(experienceCreationProvider.notifier);

      final url = notifier.extractUrl('https://eventbrite.com/e/12345 is the event');

      expect(url, 'https://eventbrite.com/e/12345');
    });

    test('extracts URL at end of text', () {
      final notifier = container.read(experienceCreationProvider.notifier);

      final url = notifier.extractUrl('The event is at https://example.com/event');

      expect(url, 'https://example.com/event');
    });

    test('extracts URL that is the entire text', () {
      final notifier = container.read(experienceCreationProvider.notifier);

      final url = notifier.extractUrl('https://eventbrite.com/e/game-night-tickets-123456');

      expect(url, 'https://eventbrite.com/e/game-night-tickets-123456');
    });

    test('returns null for text without URL', () {
      final notifier = container.read(experienceCreationProvider.notifier);

      final url = notifier.extractUrl('Board game night this Friday');

      expect(url, isNull);
    });

    test('returns null for empty text', () {
      final notifier = container.read(experienceCreationProvider.notifier);

      final url = notifier.extractUrl('');

      expect(url, isNull);
    });

    test('returns null for invalid URL scheme', () {
      final notifier = container.read(experienceCreationProvider.notifier);

      final url = notifier.extractUrl('ftp://example.com/file');

      expect(url, isNull);
    });

    test('returns null for mailto links', () {
      final notifier = container.read(experienceCreationProvider.notifier);

      final url = notifier.extractUrl('Contact us at mailto:test@example.com');

      expect(url, isNull);
    });

    test('returns null for text that looks like URL but is not', () {
      final notifier = container.read(experienceCreationProvider.notifier);

      final url = notifier.extractUrl('Visit example.com for more info');

      expect(url, isNull);
    });

    test('handles URL with special characters in path', () {
      final notifier = container.read(experienceCreationProvider.notifier);

      final url = notifier.extractUrl('https://example.com/events/game-night-2025');

      expect(url, 'https://example.com/events/game-night-2025');
    });

    test('handles URL with port number', () {
      final notifier = container.read(experienceCreationProvider.notifier);

      final url = notifier.extractUrl('http://localhost:8080/event');

      expect(url, 'http://localhost:8080/event');
    });

    test('handles URL with subdomain', () {
      final notifier = container.read(experienceCreationProvider.notifier);

      final url = notifier.extractUrl('https://events.example.com/my-event');

      expect(url, 'https://events.example.com/my-event');
    });

    test('handles Eventbrite URL', () {
      final notifier = container.read(experienceCreationProvider.notifier);

      final url = notifier.extractUrl(
        'https://www.eventbrite.com/e/community-game-night-tickets-123456789',
      );

      expect(url, 'https://www.eventbrite.com/e/community-game-night-tickets-123456789');
    });

    test('handles Meetup URL', () {
      final notifier = container.read(experienceCreationProvider.notifier);

      final url = notifier.extractUrl(
        'https://www.meetup.com/seattle-board-games/events/298765432/',
      );

      expect(url, 'https://www.meetup.com/seattle-board-games/events/298765432/');
    });

    test('handles Facebook Events URL', () {
      final notifier = container.read(experienceCreationProvider.notifier);

      final url = notifier.extractUrl(
        'https://www.facebook.com/events/123456789012345',
      );

      expect(url, 'https://www.facebook.com/events/123456789012345');
    });
  });

  group('hasUrl', () {
    test('returns true when text contains URL', () {
      final notifier = container.read(experienceCreationProvider.notifier);
      notifier.setTextInput('Check out https://eventbrite.com/e/test');

      expect(notifier.hasUrl(), isTrue);
    });

    test('returns false when text has no URL', () {
      final notifier = container.read(experienceCreationProvider.notifier);
      notifier.setTextInput('Board game night this Friday');

      expect(notifier.hasUrl(), isFalse);
    });

    test('returns false for empty text', () {
      final notifier = container.read(experienceCreationProvider.notifier);

      expect(notifier.hasUrl(), isFalse);
    });
  });

  group('canGenerate', () {
    test('returns true for text mode with text input', () {
      final notifier = container.read(experienceCreationProvider.notifier);
      notifier.setTextInput('Board game night');

      expect(notifier.canGenerate(), isTrue);
    });

    test('returns true for text mode with URL input', () {
      final notifier = container.read(experienceCreationProvider.notifier);
      notifier.setTextInput('https://eventbrite.com/e/test');

      expect(notifier.canGenerate(), isTrue);
    });

    test('returns false for text mode with empty input', () {
      final notifier = container.read(experienceCreationProvider.notifier);

      expect(notifier.canGenerate(), isFalse);
    });

    test('returns false for text mode with whitespace only', () {
      final notifier = container.read(experienceCreationProvider.notifier);
      notifier.setTextInput('   \n\t  ');

      expect(notifier.canGenerate(), isFalse);
    });

    test('returns true for image mode even without image', () {
      final notifier = container.read(experienceCreationProvider.notifier);
      notifier.setInputMode(CreationInputMode.image);

      expect(notifier.canGenerate(), isTrue);
    });
  });

  group('reset', () {
    test('resets to initial state', () {
      final notifier = container.read(experienceCreationProvider.notifier);
      notifier.setTextInput('Some text');
      notifier.setImagePath('/path/to/image.jpg');
      notifier.setInputMode(CreationInputMode.image);
      notifier.state = notifier.state.copyWith(
        isLoading: true,
        error: const UserError.generic(fallback: 'Some error'),
      );

      notifier.reset();

      final state = container.read(experienceCreationProvider);
      expect(state.inputMode, CreationInputMode.text);
      expect(state.textInput, isEmpty);
      expect(state.selectedImagePath, isNull);
      expect(state.isLoading, isFalse);
      expect(state.error, isNull);
    });
  });
}
