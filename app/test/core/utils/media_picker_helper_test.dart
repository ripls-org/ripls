import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/utils/media_picker_helper.dart';

void main() {
  group('MediaPickerHelper', () {
    group('Quality Constants', () {
      test('imageMaxWidth is 1920', () {
        expect(MediaPickerHelper.imageMaxWidth, equals(1920.0));
      });

      test('imageMaxHeight is 1080', () {
        expect(MediaPickerHelper.imageMaxHeight, equals(1080.0));
      });

      test('imageQuality is 85', () {
        expect(MediaPickerHelper.imageQuality, equals(85));
      });

      test('videoMaxDuration is 5 minutes', () {
        expect(
          MediaPickerHelper.videoMaxDuration,
          equals(const Duration(minutes: 5)),
        );
      });
    });

    // Note: Testing the actual picker methods requires mocking ImagePicker
    // which is complex and would require additional dependencies.
    // The constants tests above verify the critical configuration values.
    // Integration tests would be needed to verify the picker methods work
    // correctly with the ImagePicker plugin.
  });
}
