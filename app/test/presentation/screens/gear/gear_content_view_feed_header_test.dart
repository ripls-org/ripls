import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/screens/gear/gear_content_view.dart';

void main() {
  group('GearContentView Feed Header API', () {
    test('accepts all feed header parameters in constructor', () {
      final testActor = User(
        id: 'actor456',
        name: 'Another Actor',
        mediaId: 'media123',
      );

      final timestamp = DateTime.now().millisecondsSinceEpoch ~/ 1000;

      // Verify constructor accepts all parameters without errors
      final widget = GearContentView(
        gearId: 'test-gear-id',
        showEditControls: false,
        showOwnerInfo: false,
        showFeedHeader: true,
        feedActor: testActor,
        feedOccurredAtUnixSec: timestamp,
        feedActionText: 'Posted gear',
      );

      expect(widget.gearId, equals('test-gear-id'));
      expect(widget.showEditControls, isFalse);
      expect(widget.showOwnerInfo, isFalse);
      expect(widget.showFeedHeader, isTrue);
      expect(widget.feedActor, equals(testActor));
      expect(widget.feedOccurredAtUnixSec, equals(timestamp));
      expect(widget.feedActionText, equals('Posted gear'));
    });

    test('feed header parameters default to null/false', () {
      final widget = GearContentView(
        gearId: 'test-gear-id',
      );

      // Verify defaults
      expect(widget.gearId, equals('test-gear-id'));
      expect(widget.showEditControls, isTrue); // default
      expect(widget.showOwnerInfo, isTrue); // default
      expect(widget.showFeedHeader, isFalse); // default for feed header
      expect(widget.feedActor, isNull);
      expect(widget.feedOccurredAtUnixSec, isNull);
      expect(widget.feedActionText, isNull);
    });

    test('can enable feed header without providing parameters', () {
      final widget = GearContentView(
        gearId: 'test-gear-id',
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
        id: 'actor123',
        name: 'Test Actor',
        mediaId: '',
      );

      final widget = GearContentView(
        gearId: 'test-gear-id',
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
