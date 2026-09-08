import 'dart:io';

import 'package:cross_file/cross_file.dart';
import 'package:device_info_plus/device_info_plus.dart';
import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:mime/mime.dart';
import 'package:package_info_plus/package_info_plus.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/data/gen/ripls/api/feedback_service.pb.dart';
import 'package:ripls/data/repositories/feedback_repository.dart';
import 'package:ripls/services/providers.dart';

part 'feedback_sheet_view_model.freezed.dart';

/// State for the feedback bottom-sheet modal.
@freezed
sealed class FeedbackSheetState with _$FeedbackSheetState {
  const FeedbackSheetState._();

  const factory FeedbackSheetState({
    @Default(FeedbackType.FEEDBACK_TYPE_BUG_REPORT) FeedbackType feedbackType,
    @Default('') String title,
    @Default('') String description,
    @Default(<XFile>[]) List<XFile> screenshots,
    @Default(false) bool isSubmitting,
    @Default(false) bool isSubmitted,
    UserError? error,
  }) = _FeedbackSheetState;

  /// canSubmit returns true when the form is ready to submit.
  bool get canSubmit =>
      title.trim().isNotEmpty && description.trim().isNotEmpty && !isSubmitting;
}

/// FeedbackSheetNotifier manages state for the feedback bottom-sheet modal.
///
/// Architecture: ViewModel → FeedbackRepository → FeedbackService
class FeedbackSheetNotifier extends Notifier<FeedbackSheetState> {
  static const int _maxScreenshots = 3;
  static const int _maxTitleLength = 100;
  static const int _maxDescriptionLength = 5000;

  @override
  FeedbackSheetState build() => const FeedbackSheetState();

  FeedbackRepository get _feedbackRepository =>
      ref.read(feedbackRepositoryProvider);

  /// setFeedbackType updates the selected feedback type.
  void setFeedbackType(FeedbackType type) {
    state = state.copyWith(feedbackType: type);
  }

  /// setTitle updates the title, clamping to [_maxTitleLength] characters.
  void setTitle(String title) {
    final clamped = title.length > _maxTitleLength
        ? title.substring(0, _maxTitleLength)
        : title;
    state = state.copyWith(title: clamped);
  }

  /// setDescription updates the description, clamping to [_maxDescriptionLength] characters.
  void setDescription(String description) {
    final clamped = description.length > _maxDescriptionLength
        ? description.substring(0, _maxDescriptionLength)
        : description;
    state = state.copyWith(description: clamped);
  }

  /// addScreenshot appends a picked file to the list, up to [_maxScreenshots].
  ///
  /// Takes an [XFile] (the cross-platform handle returned by `image_picker`)
  /// so the bytes can be read uniformly on mobile and web; on web, `XFile.path`
  /// is a `blob:` URL that is not valid for `dart:io File`.
  void addScreenshot(XFile file) {
    if (state.screenshots.length >= _maxScreenshots) return;
    state = state.copyWith(screenshots: [...state.screenshots, file]);
  }

  /// removeScreenshot removes the screenshot at [index] from the list.
  void removeScreenshot(int index) {
    final updated = List<XFile>.from(state.screenshots)..removeAt(index);
    state = state.copyWith(screenshots: updated);
  }

  /// submitFeedback collects device context and submits feedback to the repository.
  ///
  /// Sets [isSubmitting] during the request, then [isSubmitted] on success or
  /// [error] on failure.
  Future<void> submitFeedback() async {
    if (!state.canSubmit) return;

    state = state.copyWith(isSubmitting: true, error: null);

    try {
      final deviceContext = await _collectDeviceContext();
      if (!ref.mounted) return;

      final screenshots = await _buildScreenshots(state.screenshots);
      if (!ref.mounted) return;

      await _feedbackRepository.submitFeedback(
        SubmitFeedbackRequest(
          type: state.feedbackType,
          title: state.title.trim(),
          description: state.description.trim(),
          stepsToReproduce: '',
          contactEmail: '',
          deviceContext: deviceContext,
          screenshots: screenshots,
        ),
      );

      if (!ref.mounted) return;
      state = state.copyWith(isSubmitting: false, isSubmitted: true);
    } catch (e) {
      if (!ref.mounted) return;
      state = state.copyWith(
        isSubmitting: false,
        error: RpcErrorHandler.classify(e, fallback: 'Could not submit feedback. Please try again.'),
      );
    }
  }

  /// reset returns the state to its defaults.
  void reset() {
    state = const FeedbackSheetState();
  }

  Future<DeviceContext> _collectDeviceContext() async {
    final packageInfo = await PackageInfo.fromPlatform();
    final deviceInfo = DeviceInfoPlugin();

    String platform = 'Unknown';
    String osVersion = 'Unknown';
    String deviceModel = 'Unknown';

    // kIsWeb must be checked before any dart:io Platform access — Platform.isIOS
    // throws UnsupportedError on Flutter Web.
    if (kIsWeb) {
      final webInfo = await deviceInfo.webBrowserInfo;
      platform = 'Web';
      osVersion = webInfo.platform ?? 'Unknown';
      deviceModel = webInfo.userAgent ?? webInfo.browserName.name;
    } else if (Platform.isIOS) {
      final iosInfo = await deviceInfo.iosInfo;
      platform = 'iOS';
      osVersion = iosInfo.systemVersion;
      deviceModel = iosInfo.model;
    } else if (Platform.isAndroid) {
      final androidInfo = await deviceInfo.androidInfo;
      platform = 'Android';
      osVersion = 'Android ${androidInfo.version.release}';
      deviceModel = androidInfo.model;
    }

    const environment = String.fromEnvironment(
      'ENVIRONMENT',
      defaultValue: 'dev',
    );

    return DeviceContext(
      appVersion: packageInfo.version,
      platform: platform,
      osVersion: osVersion,
      deviceModel: deviceModel,
      buildNumber: packageInfo.buildNumber,
      environment: environment.toUpperCase(),
    );
  }

  Future<List<FeedbackScreenshot>> _buildScreenshots(List<XFile> files) async {
    final screenshots = <FeedbackScreenshot>[];
    for (var i = 0; i < files.length; i++) {
      final file = files[i];
      final bytes = await file.readAsBytes();
      // Prefer the XFile-reported mime type (set by image_picker on web from
      // the browser's File object); fall back to sniffing path + bytes.
      final mimeType = file.mimeType ??
          lookupMimeType(file.path, headerBytes: bytes) ??
          'image/png';
      final extension = _extensionFromMimeType(mimeType);
      screenshots.add(
        FeedbackScreenshot(
          data: bytes,
          filename: 'screenshot_${i + 1}.$extension',
          contentType: mimeType,
        ),
      );
    }
    return screenshots;
  }

  String _extensionFromMimeType(String mimeType) {
    switch (mimeType) {
      case 'image/jpeg':
        return 'jpg';
      case 'image/gif':
        return 'gif';
      case 'image/webp':
        return 'webp';
      case 'image/heic':
      case 'image/heif':
        return 'heic';
      default:
        return 'png';
    }
  }
}

/// Provider for the feedback sheet ViewModel.
///
/// Uses autoDispose so state is reset each time the sheet is opened and closed.
final feedbackSheetProvider =
    NotifierProvider.autoDispose<FeedbackSheetNotifier, FeedbackSheetState>(
      FeedbackSheetNotifier.new,
    );
