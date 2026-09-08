import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/screens/communities/community_content_view.dart';

void main() {
  group('CommunityContentView Feed Header API', () {
    test('accepts all feed header parameters in constructor', () {
      final testActor = User(
        id: 'actor123',
        name: 'Test Actor',
        mediaId: 'media789',
      );

      final testFeedActor = User(
        id: 'feedActor456',
        name: 'Feed Actor',
        mediaId: 'feedMedia123',
      );

      final timestamp = DateTime.now().millisecondsSinceEpoch ~/ 1000;

      // Verify constructor accepts all parameters without errors
      final widget = CommunityContentView(
        communityId: 'test-community-id',
        communityName: 'Test Community',
        communityDescription: 'Test Description',
        actor: testActor,
        mediaId: 'media123',
        canEdit: true,
        occurredAtUnixSec: timestamp,
        showFeedHeader: true,
        feedActor: testFeedActor,
        feedOccurredAtUnixSec: timestamp,
        feedActionText: 'Created community',
      );

      expect(widget.communityId, equals('test-community-id'));
      expect(widget.showFeedHeader, isTrue);
      expect(widget.feedActor, equals(testFeedActor));
      expect(widget.feedOccurredAtUnixSec, equals(timestamp));
      expect(widget.feedActionText, equals('Created community'));
    });

    test('feed header parameters default to null/false', () {
      final testActor = User(
        id: 'actor789',
        name: 'Another Actor',
        mediaId: '',
      );

      final timestamp = DateTime.now().millisecondsSinceEpoch ~/ 1000;

      final widget = CommunityContentView(
        communityId: 'test-community-id',
        communityName: 'Test Community',
        communityDescription: 'Test Description',
        actor: testActor,
        canEdit: false,
        occurredAtUnixSec: timestamp,
      );

      // Verify defaults
      expect(widget.communityId, equals('test-community-id'));
      expect(widget.showFeedHeader, isFalse); // default for feed header
      expect(widget.feedActor, isNull);
      expect(widget.feedOccurredAtUnixSec, isNull);
      expect(widget.feedActionText, isNull);
    });

    test('can enable feed header without providing parameters', () {
      final testActor = User(
        id: 'actor999',
        name: 'Partial Actor',
        mediaId: '',
      );

      final timestamp = DateTime.now().millisecondsSinceEpoch ~/ 1000;

      final widget = CommunityContentView(
        communityId: 'test-community-id',
        communityName: 'Test Community',
        communityDescription: 'Test Description',
        actor: testActor,
        canEdit: false,
        occurredAtUnixSec: timestamp,
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
        id: 'actor111',
        name: 'Original Actor',
        mediaId: '',
      );

      final testFeedActor = User(
        id: 'feedActor222',
        name: 'Partial Feed Actor',
        mediaId: '',
      );

      final timestamp = DateTime.now().millisecondsSinceEpoch ~/ 1000;

      final widget = CommunityContentView(
        communityId: 'test-community-id',
        communityName: 'Test Community',
        communityDescription: 'Test Description',
        actor: testActor,
        canEdit: false,
        occurredAtUnixSec: timestamp,
        showFeedHeader: true,
        feedActor: testFeedActor,
        // Missing feedOccurredAtUnixSec and feedActionText
      );

      expect(widget.showFeedHeader, isTrue);
      expect(widget.feedActor, equals(testFeedActor));
      expect(widget.feedOccurredAtUnixSec, isNull);
      expect(widget.feedActionText, isNull);
      // Widget should handle null checks gracefully
    });
  });
}
