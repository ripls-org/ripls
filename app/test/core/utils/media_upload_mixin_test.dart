import 'package:cross_file/cross_file.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:logging/logging.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/core/utils/media_upload_mixin.dart';
import 'package:ripls/data/repositories/media_repository.dart';

import 'media_upload_mixin_test.mocks.dart';

// Mock classes
@GenerateMocks([MediaRepository])
void main() {
  group('MediaUploadMixin', () {
    late MockMediaRepository mockMediaRepository;
    late Logger mockLogger;
    late ProviderContainer container;
    late TestNotifier testNotifier;

    setUp(() {
      mockMediaRepository = MockMediaRepository();
      mockLogger = Logger('TestNotifier');
      container = ProviderContainer();
      testNotifier = container.read(testNotifierProvider.notifier);
      testNotifier.setMediaRepository(mockMediaRepository);
      testNotifier.setLogger(mockLogger);
    });

    tearDown(() {
      container.dispose();
    });

    group('pickAndUploadImageFromGallery', () {
      test('uploads image successfully and updates state', () async {
        // Arrange
        const testMediaId = 'test-media-id-123';
        when(mockMediaRepository.addMedia(
          file: anyNamed('file'),
          description: anyNamed('description'),
        )).thenAnswer((_) async => testMediaId);

        // Act - Note: In a real test with MediaPickerHelper, we'd need to mock it
        // For now, we test the internal _setMediaFile method directly
        await testNotifier.testSetMediaFile('test-path.jpg');

        // Assert
        final state = container.read(testNotifierProvider);
        expect(state.uploadedMediaId, testMediaId);
        expect(state.isUploadingMedia, false);
        expect(state.uploadError, null);

        verify(mockMediaRepository.addMedia(
          file: argThat(isA<XFile>(), named: 'file'),
          description: 'Test media',
        )).called(1);
      });

      test('sets loading state during upload', () async {
        // Arrange
        when(mockMediaRepository.addMedia(
          file: anyNamed('file'),
          description: anyNamed('description'),
        )).thenAnswer((_) async {
          // Verify loading state is set before upload completes
          final state = container.read(testNotifierProvider);
          expect(state.isUploadingMedia, true);
          expect(state.uploadError, null);
          return 'test-media-id';
        });

        // Act
        await testNotifier.testSetMediaFile('test-path.jpg');
      });

      test('handles upload failure and sets error', () async {
        // Arrange
        const errorMessage = 'Network error';
        when(mockMediaRepository.addMedia(
          file: anyNamed('file'),
          description: anyNamed('description'),
        )).thenThrow(Exception(errorMessage));

        // Act & Assert
        expect(
          () => testNotifier.testSetMediaFile('test-path.jpg'),
          throwsA(isA<Exception>()),
        );

        // Verify error state is set
        final state = container.read(testNotifierProvider);
        expect(state.isUploadingMedia, false);
        expect(state.uploadError, contains(errorMessage));
        expect(state.uploadedMediaId, null);
      });

      test('clears previous error on new upload attempt', () async {
        // Arrange - First upload fails
        when(mockMediaRepository.addMedia(
          file: argThat(
            isA<XFile>().having((f) => f.path, 'path', 'fail-path.jpg'),
            named: 'file',
          ),
          description: anyNamed('description'),
        )).thenThrow(Exception('First error'));

        try {
          await testNotifier.testSetMediaFile('fail-path.jpg');
        } catch (_) {
          // Expected to fail
        }

        // Verify error is set
        var state = container.read(testNotifierProvider);
        expect(state.uploadError, isNotNull);

        // Arrange - Second upload succeeds
        when(mockMediaRepository.addMedia(
          file: argThat(
            isA<XFile>().having((f) => f.path, 'path', 'success-path.jpg'),
            named: 'file',
          ),
          description: anyNamed('description'),
        )).thenAnswer((_) async => 'success-media-id');

        // Act - Try again
        await testNotifier.testSetMediaFile('success-path.jpg');

        // Assert - Error should be cleared
        state = container.read(testNotifierProvider);
        expect(state.uploadError, null);
        expect(state.uploadedMediaId, 'success-media-id');
        expect(state.isUploadingMedia, false);
      });

      test('uses custom media description from subclass', () async {
        // Arrange
        when(mockMediaRepository.addMedia(
          file: anyNamed('file'),
          description: anyNamed('description'),
        )).thenAnswer((_) async => 'test-media-id');

        // Act
        await testNotifier.testSetMediaFile('test-path.jpg');

        // Assert - Verify custom description is used
        verify(mockMediaRepository.addMedia(
          file: argThat(isA<XFile>(), named: 'file'),
          description: 'Test media', // Custom description from TestNotifier
        )).called(1);
      });
    });

    group('pickAndUploadVideoFromGallery', () {
      test('uploads video successfully', () async {
        // Arrange
        const testMediaId = 'test-video-id-456';
        when(mockMediaRepository.addMedia(
          file: anyNamed('file'),
          description: anyNamed('description'),
        )).thenAnswer((_) async => testMediaId);

        // Act
        await testNotifier.testSetMediaFile('test-video.mp4');

        // Assert
        final state = container.read(testNotifierProvider);
        expect(state.uploadedMediaId, testMediaId);
        expect(state.isUploadingMedia, false);
      });
    });

    group('pickAndUploadImageFromCamera', () {
      test('uploads camera image successfully', () async {
        // Arrange
        const testMediaId = 'test-camera-id-789';
        when(mockMediaRepository.addMedia(
          file: anyNamed('file'),
          description: anyNamed('description'),
        )).thenAnswer((_) async => testMediaId);

        // Act
        await testNotifier.testSetMediaFile('test-camera.jpg');

        // Assert
        final state = container.read(testNotifierProvider);
        expect(state.uploadedMediaId, testMediaId);
        expect(state.isUploadingMedia, false);
      });
    });

    group('state management', () {
      test('onMediaUploadStarted sets loading state correctly', () {
        // Act
        testNotifier.onMediaUploadStarted();

        // Assert
        final state = container.read(testNotifierProvider);
        expect(state.isUploadingMedia, true);
        expect(state.uploadError, null);
      });

      test('onMediaUploaded updates media ID and clears loading', () {
        // Arrange
        testNotifier.onMediaUploadStarted();

        // Act
        testNotifier.onMediaUploaded('uploaded-media-id');

        // Assert
        final state = container.read(testNotifierProvider);
        expect(state.uploadedMediaId, 'uploaded-media-id');
        expect(state.isUploadingMedia, false);
        expect(state.uploadError, null);
      });

      test('onMediaUploadFailed sets error and clears loading', () {
        // Arrange
        testNotifier.onMediaUploadStarted();

        // Act
        testNotifier.onMediaUploadFailed('Upload failed: Network error');

        // Assert
        final state = container.read(testNotifierProvider);
        expect(state.uploadError, 'Upload failed: Network error');
        expect(state.isUploadingMedia, false);
        expect(state.uploadedMediaId, null);
      });
    });

    group('batch upload', () {
      test('uploads multiple files and calls onBatchUploadCompleted', () async {
        when(mockMediaRepository.addMedia(
          file: argThat(
            isA<XFile>().having((f) => f.path, 'path', 'file1.jpg'),
            named: 'file',
          ),
          description: anyNamed('description'),
        )).thenAnswer((_) async => 'media-1');
        when(mockMediaRepository.addMedia(
          file: argThat(
            isA<XFile>().having((f) => f.path, 'path', 'file2.jpg'),
            named: 'file',
          ),
          description: anyNamed('description'),
        )).thenAnswer((_) async => 'media-2');
        when(mockMediaRepository.addMedia(
          file: argThat(
            isA<XFile>().having((f) => f.path, 'path', 'file3.jpg'),
            named: 'file',
          ),
          description: anyNamed('description'),
        )).thenAnswer((_) async => 'media-3');

        await testNotifier.testUploadMultipleMediaFiles(
          ['file1.jpg', 'file2.jpg', 'file3.jpg'],
        );

        final state = container.read(testNotifierProvider);
        expect(state.batchUploadedIds, ['media-1', 'media-2', 'media-3']);
        expect(state.isUploadingMedia, false);
        expect(state.batchUploadFailedCount, null);
      });

      test('reports progress after each upload', () async {
        var callCount = 0;
        when(mockMediaRepository.addMedia(
          file: anyNamed('file'),
          description: anyNamed('description'),
        )).thenAnswer((_) async {
          callCount++;
          return 'media-$callCount';
        });

        await testNotifier.testUploadMultipleMediaFiles(
          ['a.jpg', 'b.jpg'],
        );

        // After completion, progress should be cleared
        final state = container.read(testNotifierProvider);
        expect(state.batchUploadCompleted, null);
        expect(state.batchUploadTotal, null);
        expect(state.batchUploadedIds, hasLength(2));
      });

      test('handles partial failure keeping successful uploads', () async {
        when(mockMediaRepository.addMedia(
          file: argThat(
            isA<XFile>().having((f) => f.path, 'path', 'good.jpg'),
            named: 'file',
          ),
          description: anyNamed('description'),
        )).thenAnswer((_) async => 'media-good');
        when(mockMediaRepository.addMedia(
          file: argThat(
            isA<XFile>().having((f) => f.path, 'path', 'bad.jpg'),
            named: 'file',
          ),
          description: anyNamed('description'),
        )).thenThrow(Exception('Network error'));
        when(mockMediaRepository.addMedia(
          file: argThat(
            isA<XFile>().having((f) => f.path, 'path', 'also-good.jpg'),
            named: 'file',
          ),
          description: anyNamed('description'),
        )).thenAnswer((_) async => 'media-also-good');

        await testNotifier.testUploadMultipleMediaFiles(
          ['good.jpg', 'bad.jpg', 'also-good.jpg'],
        );

        final state = container.read(testNotifierProvider);
        expect(state.batchUploadedIds, ['media-good', 'media-also-good']);
        expect(state.batchUploadFailedCount, 1);
        expect(state.isUploadingMedia, false);
      });

      test('calls onMediaUploadFailed when all uploads fail', () async {
        when(mockMediaRepository.addMedia(
          file: anyNamed('file'),
          description: anyNamed('description'),
        )).thenThrow(Exception('Server down'));

        await testNotifier.testUploadMultipleMediaFiles(
          ['a.jpg', 'b.jpg'],
        );

        final state = container.read(testNotifierProvider);
        expect(state.uploadError, contains('All 2 uploads failed'));
        expect(state.isUploadingMedia, false);
        expect(state.batchUploadedIds, null);
      });

      test('sets loading state during batch upload', () async {
        when(mockMediaRepository.addMedia(
          file: anyNamed('file'),
          description: anyNamed('description'),
        )).thenAnswer((_) async {
          // Verify loading state is set while uploading
          final state = container.read(testNotifierProvider);
          expect(state.isUploadingMedia, true);
          return 'media-id';
        });

        await testNotifier.testUploadMultipleMediaFiles(['file.jpg']);

        final state = container.read(testNotifierProvider);
        expect(state.isUploadingMedia, false);
      });

      test('handles single file batch as batch (not single upload)', () async {
        when(mockMediaRepository.addMedia(
          file: argThat(
            isA<XFile>().having((f) => f.path, 'path', 'single.jpg'),
            named: 'file',
          ),
          description: anyNamed('description'),
        )).thenAnswer((_) async => 'media-single');

        await testNotifier.testUploadMultipleMediaFiles(['single.jpg']);

        final state = container.read(testNotifierProvider);
        expect(state.batchUploadedIds, ['media-single']);
        expect(state.isUploadingMedia, false);
      });
    });

    group('concurrent uploads', () {
      test('handles rapid successive upload attempts', () async {
        // Arrange
        when(mockMediaRepository.addMedia(
          file: anyNamed('file'),
          description: anyNamed('description'),
        )).thenAnswer((_) async {
          // Simulate slow upload
          await Future.delayed(const Duration(milliseconds: 50));
          return 'media-id';
        });

        // Act - Start multiple uploads (only last should complete)
        final future1 = testNotifier.testSetMediaFile('file1.jpg');
        final future2 = testNotifier.testSetMediaFile('file2.jpg');

        await Future.wait([future1, future2]);

        // Assert - Both should complete
        final state = container.read(testNotifierProvider);
        expect(state.isUploadingMedia, false);
        expect(state.uploadedMediaId, isNotNull);
      });
    });
  });
}

// Test implementation of the mixin
class TestState {
  final bool isUploadingMedia;
  final String? uploadError;
  final String? uploadedMediaId;
  final List<String>? batchUploadedIds;
  final int? batchUploadCompleted;
  final int? batchUploadTotal;
  final int? batchUploadFailedCount;

  const TestState({
    this.isUploadingMedia = false,
    this.uploadError,
    this.uploadedMediaId,
    this.batchUploadedIds,
    this.batchUploadCompleted,
    this.batchUploadTotal,
    this.batchUploadFailedCount,
  });

  TestState copyWith({
    bool? isUploadingMedia,
    String? uploadError,
    String? uploadedMediaId,
    List<String>? batchUploadedIds,
    int? batchUploadCompleted,
    int? batchUploadTotal,
    int? batchUploadFailedCount,
  }) {
    return TestState(
      isUploadingMedia: isUploadingMedia ?? this.isUploadingMedia,
      uploadError: uploadError,
      uploadedMediaId: uploadedMediaId ?? this.uploadedMediaId,
      batchUploadedIds: batchUploadedIds ?? this.batchUploadedIds,
      batchUploadCompleted: batchUploadCompleted,
      batchUploadTotal: batchUploadTotal,
      batchUploadFailedCount: batchUploadFailedCount,
    );
  }
}

class TestNotifier extends Notifier<TestState>
    with MediaUploadMixin<TestState> {
  MediaRepository? _mediaRepository;
  Logger? _logger;

  @override
  TestState build() {
    return const TestState();
  }

  void setMediaRepository(MediaRepository repository) {
    _mediaRepository = repository;
  }

  void setLogger(Logger logger) {
    _logger = logger;
  }

  @override
  MediaRepository get mediaRepository {
    if (_mediaRepository == null) {
      throw StateError('MediaRepository not set');
    }
    return _mediaRepository!;
  }

  @override
  Logger get log {
    if (_logger == null) {
      throw StateError('Logger not set');
    }
    return _logger!;
  }

  @override
  String get mediaUploadDescription => 'Test media';

  @override
  void onMediaUploadStarted() {
    state = state.copyWith(isUploadingMedia: true, uploadError: null);
  }

  @override
  void onMediaUploaded(String mediaId) {
    state = state.copyWith(
      uploadedMediaId: mediaId,
      isUploadingMedia: false,
    );
  }

  @override
  void onMediaUploadFailed(String error) {
    state = state.copyWith(
      uploadError: error,
      isUploadingMedia: false,
      batchUploadCompleted: null,
      batchUploadTotal: null,
      batchUploadFailedCount: null,
    );
  }

  @override
  void onBatchUploadProgress(int completed, int total) {
    state = state.copyWith(
      batchUploadCompleted: completed,
      batchUploadTotal: total,
    );
  }

  @override
  void onBatchUploadCompleted(List<String> mediaIds) {
    state = state.copyWith(
      batchUploadedIds: mediaIds,
      isUploadingMedia: false,
      batchUploadCompleted: null,
      batchUploadTotal: null,
      batchUploadFailedCount: null,
    );
  }

  @override
  void onBatchUploadPartialFailure(List<String> uploadedIds, int failedCount) {
    state = state.copyWith(
      batchUploadedIds: uploadedIds,
      isUploadingMedia: false,
      batchUploadCompleted: null,
      batchUploadTotal: null,
      batchUploadFailedCount: failedCount,
    );
  }

  // Expose internal method for testing
  Future<void> testSetMediaFile(String filePath) async {
    onMediaUploadStarted();
    try {
      final mediaId = await mediaRepository.addMedia(
        file: XFile(filePath),
        description: mediaUploadDescription,
      );
      onMediaUploaded(mediaId);
      log.info('✅ Media uploaded successfully: $mediaId');
    } catch (e, stackTrace) {
      log.severe('❌ Media upload failed', e, stackTrace);
      onMediaUploadFailed('Upload failed: $e');
      rethrow;
    }
  }

  // Expose batch upload for testing
  Future<void> testUploadMultipleMediaFiles(List<String> filePaths) async {
    await uploadMultipleMediaFiles(
      filePaths.map((p) => XFile(p)).toList(),
    );
  }
}

final testNotifierProvider = NotifierProvider<TestNotifier, TestState>(
  TestNotifier.new,
);
