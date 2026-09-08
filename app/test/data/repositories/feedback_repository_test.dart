import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/feedback_service.pb.dart';
import 'package:ripls/data/repositories/feedback_repository.dart';
import 'package:ripls/services/feedback_service.dart';

import 'feedback_repository_test.mocks.dart';

@GenerateMocks([FeedbackService])
void main() {
  late FeedbackRepository repository;
  late MockFeedbackService mockService;

  setUp(() {
    mockService = MockFeedbackService();
    repository = FeedbackRepository(mockService);
  });

  group('FeedbackRepository', () {
    test('submitFeedback calls service and returns response', () async {
      // Arrange
      final request = SubmitFeedbackRequest(
        type: FeedbackType.FEEDBACK_TYPE_BUG_REPORT,
        title: 'Test bug',
        description: 'This is a test bug report',
      );

      final expectedResponse = SubmitFeedbackResponse(
        issueUrl: 'https://github.com/test/repo/issues/123',
        issueNumber: 123,
        message: 'Thank you for your feedback!',
      );

      when(mockService.submitFeedback(any))
          .thenAnswer((_) async => expectedResponse);

      // Act
      final response = await repository.submitFeedback(request);

      // Assert
      expect(response, equals(expectedResponse));
      expect(response.issueUrl, equals('https://github.com/test/repo/issues/123'));
      expect(response.issueNumber, equals(123));
      verify(mockService.submitFeedback(request)).called(1);
    });

    test('submitFeedback propagates service exceptions', () async {
      // Arrange
      final request = SubmitFeedbackRequest(
        type: FeedbackType.FEEDBACK_TYPE_FEATURE_REQUEST,
        title: 'Test feature',
        description: 'This is a test feature request',
      );

      when(mockService.submitFeedback(any))
          .thenThrow(Exception('Network error'));

      // Act & Assert
      expect(
        () => repository.submitFeedback(request),
        throwsA(isA<Exception>()),
      );
      verify(mockService.submitFeedback(request)).called(1);
    });
  });
}
