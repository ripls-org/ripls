import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/screens/request/request_content_view.dart';

void main() {
  group('RequestContentView Feed Header API', () {
    test('accepts all feed header parameters in constructor', () {
      final testActor = User(
        id: 'actor789',
        name: 'Request Actor',
        mediaId: 'media456',
      );

      final timestamp = DateTime.now().millisecondsSinceEpoch ~/ 1000;

      // Verify constructor accepts all parameters without errors
      final widget = RequestContentView(
        requestId: 'test-request-id',
        showEditControls: false,
        showFloatingActions: false,
        showOwnerInfo: false,
        showFeedHeader: true,
        feedActor: testActor,
        feedOccurredAtUnixSec: timestamp,
        feedActionText: 'Posted request',
      );

      expect(widget.requestId, equals('test-request-id'));
      expect(widget.showEditControls, isFalse);
      expect(widget.showFloatingActions, isFalse);
      expect(widget.showOwnerInfo, isFalse);
      expect(widget.showFeedHeader, isTrue);
      expect(widget.feedActor, equals(testActor));
      expect(widget.feedOccurredAtUnixSec, equals(timestamp));
      expect(widget.feedActionText, equals('Posted request'));
    });

    test('feed header parameters default to null/false', () {
      final widget = RequestContentView(
        requestId: 'test-request-id',
      );

      // Verify defaults
      expect(widget.requestId, equals('test-request-id'));
      expect(widget.showEditControls, isTrue); // default
      expect(widget.showFloatingActions, isTrue); // default
      expect(widget.showOwnerInfo, isTrue); // default
      expect(widget.showFeedHeader, isFalse); // default for feed header
      expect(widget.feedActor, isNull);
      expect(widget.feedOccurredAtUnixSec, isNull);
      expect(widget.feedActionText, isNull);
    });

    test('can enable feed header without providing parameters', () {
      final widget = RequestContentView(
        requestId: 'test-request-id',
        showFeedHeader: true,
        // Not providing feedActor, feedOccurredAtUnixSec, feedActionText
      );

      expect(widget.showFeedHeader, isTrue);
      expect(widget.feedActor, isNull);
      expect(widget.feedOccurredAtUnixSec, isNull);
      expect(widget.feedActionText, isNull);
      // This is valid - the widget will just not render the header
    });

    test('can provide partial feed header parameters', () {
      final testActor = User(
        id: 'actor999',
        name: 'Partial Actor',
        mediaId: '',
      );

      final widget = RequestContentView(
        requestId: 'test-request-id',
        showFeedHeader: true,
        feedActor: testActor,
        // Missing feedOccurredAtUnixSec and feedActionText
      );

      expect(widget.showFeedHeader, isTrue);
      expect(widget.feedActor, equals(testActor));
      expect(widget.feedOccurredAtUnixSec, isNull);
      expect(widget.feedActionText, isNull);
      // Widget should handle null checks gracefully
    });
  });
}
