import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/feed_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/screens/story/story_content_view.dart';

void main() {
  group('StoryContentView', () {
    test('accepts story payload in constructor', () {
      final story = StoryPayload(
        storyType: StoryType.STORY_TYPE_LOAN_COMPLETED,
        title: 'Loan Completed',
        description: 'Great job completing the loan!',
        mediaIds: ['media1'],
      );

      final widget = StoryContentView(story: story);

      expect(widget.story, equals(story));
      expect(widget.showFeedHeader, isFalse);
      expect(widget.feedActor, isNull);
      expect(widget.feedOccurredAtUnixSec, isNull);
      expect(widget.feedActionText, isNull);
    });

    test('accepts feed header parameters in constructor', () {
      final story = StoryPayload(
        storyType: StoryType.STORY_TYPE_NEW_MEMBER_WELCOME,
        title: 'Welcome!',
        description: 'Welcome to the community',
      );

      final timestamp = DateTime.now().millisecondsSinceEpoch ~/ 1000;

      final widget = StoryContentView(
        story: story,
        showFeedHeader: true,
        feedActor: null, // Stories have no actor
        feedOccurredAtUnixSec: timestamp,
        feedActionText: 'Shared a story',
      );

      expect(widget.story, equals(story));
      expect(widget.showFeedHeader, isTrue);
      expect(widget.feedActor, isNull);
      expect(widget.feedOccurredAtUnixSec, equals(timestamp));
      expect(widget.feedActionText, equals('Shared a story'));
    });

    test('handles story with multiple participants', () {
      final participants = [
        User(id: 'user1', name: 'Alice'),
        User(id: 'user2', name: 'Bob'),
        User(id: 'user3', name: 'Charlie'),
      ];

      final story = StoryPayload(
        storyType: StoryType.STORY_TYPE_EXPERIENCE_CONCLUDED,
        title: 'Experience Concluded',
        description: 'Had a great time at the event!',
        participants: participants,
      );

      final widget = StoryContentView(story: story);

      expect(widget.story.participants, hasLength(3));
      expect(widget.story.participants[0].name, 'Alice');
      expect(widget.story.participants[1].name, 'Bob');
      expect(widget.story.participants[2].name, 'Charlie');
    });

    test('handles story with related entity IDs', () {
      final story = StoryPayload(
        storyType: StoryType.STORY_TYPE_LOAN_COMPLETED,
        title: 'Loan Completed',
        description: 'Test',
        gearId: 'gear123',
        loanId: 'loan456',
      );

      final widget = StoryContentView(story: story);

      expect(widget.story.gearId, 'gear123');
      expect(widget.story.loanId, 'loan456');
    });

    test('handles different story types', () {
      final storyTypes = [
        StoryType.STORY_TYPE_LOAN_COMPLETED,
        StoryType.STORY_TYPE_GIVEAWAY_COMPLETED,
        StoryType.STORY_TYPE_EXPERIENCE_CONCLUDED,
        StoryType.STORY_TYPE_REQUEST_FULFILLED,
        StoryType.STORY_TYPE_NEW_MEMBER_WELCOME,
      ];

      for (final type in storyTypes) {
        final story = StoryPayload(
          storyType: type,
          title: 'Test Story',
          description: 'Test description',
        );

        final widget = StoryContentView(story: story);

        expect(widget.story.storyType, equals(type));
      }
    });

    test('feed header defaults to not shown', () {
      final story = StoryPayload(
        storyType: StoryType.STORY_TYPE_LOAN_COMPLETED,
        title: 'Test',
        description: 'Test',
      );

      final widget = StoryContentView(story: story);

      expect(widget.showFeedHeader, isFalse);
    });
  });
}
