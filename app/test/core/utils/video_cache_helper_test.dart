import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/utils/video_cache_helper_io.dart';

void main() {
  group('isLocalPath', () {
    test('empty string is local', () {
      expect(isLocalPath(''), isTrue);
    });

    test('absolute POSIX path is local', () {
      expect(isLocalPath('/Users/test/Library/tmp/image_picker_video.mp4'), isTrue);
    });

    test('relative path is local', () {
      expect(isLocalPath('tmp/video.mp4'), isTrue);
    });

    test('file:// URI is local', () {
      expect(isLocalPath('file:///Users/test/video.mp4'), isTrue);
    });

    test('malformed input that fails URI parsing is local', () {
      // Uri.tryParse is permissive; the few inputs it rejects (e.g. unbalanced
      // percent-encoded sequences) must still be classified as local so the
      // helper falls through to File() rather than HTTP.
      expect(isLocalPath('not a uri'), isTrue);
    });

    test('https URL with host is remote', () {
      expect(
        isLocalPath('https://signed-bucket.googleapis.com/v.mp4?Signature=abc'),
        isFalse,
      );
    });

    test('http URL with host is remote', () {
      expect(isLocalPath('http://localhost:8080/v.mp4'), isFalse);
    });

    test('http URL with no host is local', () {
      // A non-file scheme without an authority can't be fetched; defer to
      // File() and let the player surface the failure naturally.
      expect(isLocalPath('http:///v.mp4'), isTrue);
    });

    test('blob: URL is classified as local on the IO target', () {
      // `blob:` is a web-picker construct; the IO impl is only reached on
      // mobile/desktop, where a blob URL would have nowhere to live. This
      // test documents that the IO classifier rejects it as "remote" — if
      // a blob ever leaks into a mobile build, File() will throw and the
      // VideoBackgroundHost catch-block surfaces the failure rather than
      // silently producing a black frame. (Web does not reach this branch:
      // the web impl in video_cache_helper_web.dart bypasses isLocalPath
      // entirely and uses VideoPlayerController.networkUrl for every URL.)
      expect(isLocalPath('blob:abc-123'), isTrue);
    });
  });
}
