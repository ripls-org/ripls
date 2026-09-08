import 'dart:async';

import 'package:cross_file/cross_file.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:package_info_plus/package_info_plus.dart';
import 'package:ripls/data/gen/ripls/api/feedback_service.pb.dart';
import 'package:ripls/data/repositories/feedback_repository.dart';
import 'package:ripls/presentation/viewmodels/feedback_sheet_view_model.dart';
import 'package:ripls/services/providers.dart';

import 'feedback_sheet_view_model_test.mocks.dart';

@GenerateMocks([FeedbackRepository])
void main() {
  group('FeedbackSheetNotifier', () {
    late ProviderContainer container;
    late MockFeedbackRepository mockRepository;

    setUp(() {
      PackageInfo.setMockInitialValues(
        appName: 'Ripls Test',
        packageName: 'com.test.ripls',
        version: '1.0.0',
        buildNumber: '42',
        buildSignature: '',
      );
      mockRepository = MockFeedbackRepository();
      container = ProviderContainer(
        overrides: [
          feedbackRepositoryProvider.overrideWithValue(mockRepository),
        ],
      );
    });

    tearDown(() {
      container.dispose();
    });

    group('initial state', () {
      test('starts with default values', () {
        final state = container.read(feedbackSheetProvider);

        expect(state.feedbackType, FeedbackType.FEEDBACK_TYPE_BUG_REPORT);
        expect(state.title, '');
        expect(state.description, '');
        expect(state.screenshots, isEmpty);
        expect(state.isSubmitting, false);
        expect(state.isSubmitted, false);
        expect(state.error, isNull);
      });
    });

    group('setFeedbackType', () {
      test('updates feedback type to feature request', () {
        final notifier = container.read(feedbackSheetProvider.notifier);

        notifier.setFeedbackType(FeedbackType.FEEDBACK_TYPE_FEATURE_REQUEST);

        expect(
          container.read(feedbackSheetProvider).feedbackType,
          FeedbackType.FEEDBACK_TYPE_FEATURE_REQUEST,
        );
      });

      test('updates feedback type to unspecified (Other)', () {
        final notifier = container.read(feedbackSheetProvider.notifier);

        notifier.setFeedbackType(FeedbackType.FEEDBACK_TYPE_UNSPECIFIED);

        expect(
          container.read(feedbackSheetProvider).feedbackType,
          FeedbackType.FEEDBACK_TYPE_UNSPECIFIED,
        );
      });
    });

    group('setTitle', () {
      test('updates title', () {
        final notifier = container.read(feedbackSheetProvider.notifier);

        notifier.setTitle('Button is broken');

        expect(container.read(feedbackSheetProvider).title, 'Button is broken');
      });

      test('clamps title to 100 characters', () {
        final notifier = container.read(feedbackSheetProvider.notifier);

        notifier.setTitle('a' * 150);

        expect(container.read(feedbackSheetProvider).title.length, 100);
      });

      test('accepts title exactly at 100 characters', () {
        final notifier = container.read(feedbackSheetProvider.notifier);

        notifier.setTitle('a' * 100);

        expect(container.read(feedbackSheetProvider).title.length, 100);
      });
    });

    group('setDescription', () {
      test('updates description', () {
        final notifier = container.read(feedbackSheetProvider.notifier);

        notifier.setDescription('Tapping the button causes the app to freeze.');

        expect(
          container.read(feedbackSheetProvider).description,
          'Tapping the button causes the app to freeze.',
        );
      });

      test('clamps description to 5000 characters', () {
        final notifier = container.read(feedbackSheetProvider.notifier);

        notifier.setDescription('a' * 6000);

        expect(
          container.read(feedbackSheetProvider).description.length,
          5000,
        );
      });
    });

    group('canSubmit', () {
      test('is false when title and description are empty', () {
        expect(container.read(feedbackSheetProvider).canSubmit, false);
      });

      test('is false when only title is set', () {
        final notifier = container.read(feedbackSheetProvider.notifier);

        notifier.setTitle('Button is broken');

        expect(container.read(feedbackSheetProvider).canSubmit, false);
      });

      test('is false when only description is set', () {
        final notifier = container.read(feedbackSheetProvider.notifier);

        notifier.setDescription('Tapping the button causes a crash.');

        expect(container.read(feedbackSheetProvider).canSubmit, false);
      });

      test('is true when both title and description are non-empty', () {
        final notifier = container.read(feedbackSheetProvider.notifier);

        notifier.setTitle('Button is broken');
        notifier.setDescription('Tapping the button causes a crash.');

        expect(container.read(feedbackSheetProvider).canSubmit, true);
      });

      test('is false when title is only whitespace', () {
        final notifier = container.read(feedbackSheetProvider.notifier);

        notifier.setTitle('   ');
        notifier.setDescription('Tapping the button causes a crash.');

        expect(container.read(feedbackSheetProvider).canSubmit, false);
      });

      test('is false when isSubmitting is true', () {
        final notifier = container.read(feedbackSheetProvider.notifier);
        notifier.setTitle('Button is broken');
        notifier.setDescription('Tapping the button causes a crash.');
        notifier.state = notifier.state.copyWith(isSubmitting: true);

        expect(container.read(feedbackSheetProvider).canSubmit, false);
      });
    });

    group('addScreenshot / removeScreenshot', () {
      List<String> screenshotPaths() => container
          .read(feedbackSheetProvider)
          .screenshots
          .map((f) => f.path)
          .toList();

      test('adds screenshot to list', () {
        final notifier = container.read(feedbackSheetProvider.notifier);

        notifier.addScreenshot(XFile('/path/to/screenshot1.jpg'));

        expect(screenshotPaths(), ['/path/to/screenshot1.jpg']);
      });

      test('adds up to 3 screenshots', () {
        final notifier = container.read(feedbackSheetProvider.notifier);

        notifier.addScreenshot(XFile('/path/1.jpg'));
        notifier.addScreenshot(XFile('/path/2.jpg'));
        notifier.addScreenshot(XFile('/path/3.jpg'));

        expect(container.read(feedbackSheetProvider).screenshots.length, 3);
      });

      test('ignores add when 3 screenshots already present', () {
        final notifier = container.read(feedbackSheetProvider.notifier);

        notifier.addScreenshot(XFile('/path/1.jpg'));
        notifier.addScreenshot(XFile('/path/2.jpg'));
        notifier.addScreenshot(XFile('/path/3.jpg'));
        notifier.addScreenshot(XFile('/path/4.jpg'));

        expect(container.read(feedbackSheetProvider).screenshots.length, 3);
        expect(screenshotPaths(), isNot(contains('/path/4.jpg')));
      });

      test('removes screenshot at given index', () {
        final notifier = container.read(feedbackSheetProvider.notifier);

        notifier.addScreenshot(XFile('/path/1.jpg'));
        notifier.addScreenshot(XFile('/path/2.jpg'));
        notifier.removeScreenshot(0);

        expect(screenshotPaths(), ['/path/2.jpg']);
      });
    });

    group('submitFeedback', () {
      test('does nothing when canSubmit is false', () async {
        final notifier = container.read(feedbackSheetProvider.notifier);

        await notifier.submitFeedback();

        verifyNever(mockRepository.submitFeedback(any));
        expect(container.read(feedbackSheetProvider).isSubmitting, false);
        expect(container.read(feedbackSheetProvider).isSubmitted, false);
      });

      test('success: sets isSubmitted true and calls repository once', () async {
        when(mockRepository.submitFeedback(any))
            .thenAnswer((_) async => SubmitFeedbackResponse());

        final notifier = container.read(feedbackSheetProvider.notifier);
        notifier.setTitle('Button is broken');
        notifier.setDescription('Tapping the share button causes a crash.');

        await notifier.submitFeedback();

        verify(mockRepository.submitFeedback(any)).called(1);
        final state = container.read(feedbackSheetProvider);
        expect(state.isSubmitted, true);
        expect(state.isSubmitting, false);
        expect(state.error, isNull);
      });

      test('failure: sets errorMessage and keeps isSubmitted false', () async {
        when(mockRepository.submitFeedback(any))
            .thenThrow(Exception('Network error'));

        final notifier = container.read(feedbackSheetProvider.notifier);
        notifier.setTitle('Button is broken');
        notifier.setDescription('Tapping the share button causes a crash.');

        await notifier.submitFeedback();

        final state = container.read(feedbackSheetProvider);
        expect(state.isSubmitted, false);
        expect(state.isSubmitting, false);
        expect(state.error, isNotNull);
      });

      test('sets isSubmitting true while repository call is in progress', () async {
        when(mockRepository.submitFeedback(any)).thenAnswer((_) async {
          expect(container.read(feedbackSheetProvider).isSubmitting, true);
          return SubmitFeedbackResponse();
        });

        final notifier = container.read(feedbackSheetProvider.notifier);
        notifier.setTitle('Button is broken');
        notifier.setDescription('Tapping the share button causes a crash.');

        await notifier.submitFeedback();

        expect(container.read(feedbackSheetProvider).isSubmitting, false);
      });
    });

    group('disposal safety', () {
      test('submitFeedback completes without throwing when disposed mid-await', () async {
        final submitCompleter = Completer<SubmitFeedbackResponse>();
        when(mockRepository.submitFeedback(any))
            .thenAnswer((_) => submitCompleter.future);

        final disposeContainer = ProviderContainer(
          overrides: [
            feedbackRepositoryProvider.overrideWithValue(mockRepository),
          ],
        );

        final notifier = disposeContainer.read(feedbackSheetProvider.notifier);
        notifier.setTitle('Test title');
        notifier.setDescription('Test description text here.');

        final future = notifier.submitFeedback();

        // Allow device context collection to complete before disposal
        await Future.delayed(Duration.zero);

        disposeContainer.dispose();
        submitCompleter.complete(SubmitFeedbackResponse());

        await expectLater(future, completes);
      });
    });

    group('reset', () {
      test('resets all fields to default values', () {
        final notifier = container.read(feedbackSheetProvider.notifier);

        notifier.setFeedbackType(FeedbackType.FEEDBACK_TYPE_FEATURE_REQUEST);
        notifier.setTitle('Some title');
        notifier.setDescription('Some description');
        notifier.addScreenshot(XFile('/path/1.jpg'));

        notifier.reset();

        final state = container.read(feedbackSheetProvider);
        expect(state.feedbackType, FeedbackType.FEEDBACK_TYPE_BUG_REPORT);
        expect(state.title, '');
        expect(state.description, '');
        expect(state.screenshots, isEmpty);
        expect(state.isSubmitting, false);
        expect(state.isSubmitted, false);
        expect(state.error, isNull);
      });
    });
  });
}
