import '../../data/gen/ripls/api/feedback_service.pb.dart';
import '../../services/feedback_service.dart';

/// FeedbackRepository handles feedback submission operations.
///
/// This repository provides a layer between the UI and the FeedbackService,
/// maintaining architectural consistency even for simple write-only operations.
class FeedbackRepository {
  final FeedbackService _service;

  FeedbackRepository(this._service);

  /// SubmitFeedback submits user feedback (bug report or feature request).
  ///
  /// Returns the response with GitHub issue URL and number.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<SubmitFeedbackResponse> submitFeedback(
    SubmitFeedbackRequest request,
  ) async {
    return _service.submitFeedback(request);
  }
}
