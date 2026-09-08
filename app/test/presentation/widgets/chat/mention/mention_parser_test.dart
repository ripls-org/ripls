import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/widgets/chat/mention/mention_parser.dart';
import 'package:ripls/presentation/widgets/chat/mention/mention_types.dart';

void main() {
  group('MentionParser', () {
    group('parseMentions', () {
      test('returns empty list for text without mentions', () {
        final mentions = MentionParser.parseMentions('Hello world!');
        expect(mentions, isEmpty);
      });

      test('parses single user mention', () {
        final text = 'Hello @[user:abc123:John Doe]!';
        final mentions = MentionParser.parseMentions(text);

        expect(mentions, hasLength(1));
        expect(mentions[0].type, MentionType.user);
        expect(mentions[0].id, 'abc123');
        expect(mentions[0].displayName, 'John Doe');
        expect(mentions[0].startIndex, 6);
        expect(mentions[0].endIndex, 29);
      });

      test('parses loan mention', () {
        final text = 'Check out @[loan:xyz789:Camping Tent]';
        final mentions = MentionParser.parseMentions(text);

        expect(mentions, hasLength(1));
        expect(mentions[0].type, MentionType.loan);
        expect(mentions[0].id, 'xyz789');
        expect(mentions[0].displayName, 'Camping Tent');
      });

      test('parses giveaway mention', () {
        final text = 'Free stuff: @[giveaway:def456:Old Bike]';
        final mentions = MentionParser.parseMentions(text);

        expect(mentions, hasLength(1));
        expect(mentions[0].type, MentionType.giveaway);
        expect(mentions[0].id, 'def456');
        expect(mentions[0].displayName, 'Old Bike');
      });

      test('parses request mention', () {
        final text = 'Can anyone help with @[request:ghi012:Need a ladder]?';
        final mentions = MentionParser.parseMentions(text);

        expect(mentions, hasLength(1));
        expect(mentions[0].type, MentionType.request);
        expect(mentions[0].id, 'ghi012');
        expect(mentions[0].displayName, 'Need a ladder');
      });

      test('parses experience mention', () {
        final text = 'Join us for @[experience:jkl345:Weekend Hike]!';
        final mentions = MentionParser.parseMentions(text);

        expect(mentions, hasLength(1));
        expect(mentions[0].type, MentionType.experience);
        expect(mentions[0].id, 'jkl345');
        expect(mentions[0].displayName, 'Weekend Hike');
      });

      test('parses multiple mentions', () {
        final text =
            'Hey @[user:u1:Alice] and @[user:u2:Bob], check @[loan:l1:Tent]!';
        final mentions = MentionParser.parseMentions(text);

        expect(mentions, hasLength(3));
        expect(mentions[0].type, MentionType.user);
        expect(mentions[0].displayName, 'Alice');
        expect(mentions[1].type, MentionType.user);
        expect(mentions[1].displayName, 'Bob');
        expect(mentions[2].type, MentionType.loan);
        expect(mentions[2].displayName, 'Tent');
      });

      test('handles IDs with hyphens and underscores', () {
        final text = '@[user:user-id_123:Test User]';
        final mentions = MentionParser.parseMentions(text);

        expect(mentions, hasLength(1));
        expect(mentions[0].id, 'user-id_123');
      });

      test('handles display names with special characters', () {
        final text = '@[user:u1:John O\'Brien]';
        final mentions = MentionParser.parseMentions(text);

        expect(mentions, hasLength(1));
        expect(mentions[0].displayName, "John O'Brien");
      });

      test('ignores invalid mention types', () {
        final text = '@[invalid:123:Test]';
        final mentions = MentionParser.parseMentions(text);

        expect(mentions, isEmpty);
      });

      test('ignores malformed mentions', () {
        final texts = [
          '@[user:123]', // Missing display name
          '@[user::Name]', // Missing ID
          '@user:123:Name]', // Missing opening bracket
          '@[user:123:Name', // Missing closing bracket
          '[@user:123:Name]', // Wrong order
        ];

        for (final text in texts) {
          final mentions = MentionParser.parseMentions(text);
          expect(mentions, isEmpty, reason: 'Should not parse: $text');
        }
      });

      test('returns mentions sorted by position', () {
        final text = '@[user:u2:Second] text @[user:u1:First]';
        final mentions = MentionParser.parseMentions(text);

        expect(mentions, hasLength(2));
        expect(mentions[0].startIndex, lessThan(mentions[1].startIndex));
        expect(mentions[0].displayName, 'Second');
        expect(mentions[1].displayName, 'First');
      });
    });

    group('hasMentions', () {
      test('returns true when text contains mentions', () {
        expect(
          MentionParser.hasMentions('Hello @[user:123:John]!'),
          isTrue,
        );
      });

      test('returns false when text has no mentions', () {
        expect(MentionParser.hasMentions('Hello world!'), isFalse);
      });

      test('returns false for malformed mentions', () {
        expect(MentionParser.hasMentions('@[invalid:123:Test]'), isFalse);
      });
    });

    group('toPlainText', () {
      test('converts mention to @displayName format', () {
        final text = 'Hello @[user:123:John Doe]!';
        final plain = MentionParser.toPlainText(text);

        expect(plain, 'Hello @John Doe!');
      });

      test('converts multiple mentions', () {
        final text = '@[user:u1:Alice] and @[user:u2:Bob]';
        final plain = MentionParser.toPlainText(text);

        expect(plain, '@Alice and @Bob');
      });

      test('preserves text without mentions', () {
        final text = 'Hello world!';
        final plain = MentionParser.toPlainText(text);

        expect(plain, 'Hello world!');
      });

      test('handles different mention types', () {
        final text = 'Check @[loan:l1:Camping Tent] from @[user:u1:John]';
        final plain = MentionParser.toPlainText(text);

        expect(plain, 'Check @Camping Tent from @John');
      });
    });

    group('segmentText', () {
      test('returns single text segment for text without mentions', () {
        final segments = MentionParser.segmentText('Hello world!');

        expect(segments, hasLength(1));
        expect(segments[0].isMention, isFalse);
        expect(segments[0].text, 'Hello world!');
      });

      test('returns empty list for empty text', () {
        final segments = MentionParser.segmentText('');

        expect(segments, isEmpty);
      });

      test('segments text with single mention at start', () {
        final segments = MentionParser.segmentText('@[user:123:John] hello');

        expect(segments, hasLength(2));
        expect(segments[0].isMention, isTrue);
        expect(segments[0].mention!.displayName, 'John');
        expect(segments[1].isMention, isFalse);
        expect(segments[1].text, ' hello');
      });

      test('segments text with single mention at end', () {
        final segments = MentionParser.segmentText('Hello @[user:123:John]');

        expect(segments, hasLength(2));
        expect(segments[0].isMention, isFalse);
        expect(segments[0].text, 'Hello ');
        expect(segments[1].isMention, isTrue);
        expect(segments[1].mention!.displayName, 'John');
      });

      test('segments text with mention in middle', () {
        final segments =
            MentionParser.segmentText('Hello @[user:123:John] there!');

        expect(segments, hasLength(3));
        expect(segments[0].isMention, isFalse);
        expect(segments[0].text, 'Hello ');
        expect(segments[1].isMention, isTrue);
        expect(segments[1].mention!.displayName, 'John');
        expect(segments[2].isMention, isFalse);
        expect(segments[2].text, ' there!');
      });

      test('segments text with multiple mentions', () {
        final segments = MentionParser.segmentText(
          '@[user:u1:Alice] and @[user:u2:Bob] are here',
        );

        expect(segments, hasLength(4));
        expect(segments[0].isMention, isTrue);
        expect(segments[0].mention!.displayName, 'Alice');
        expect(segments[1].isMention, isFalse);
        expect(segments[1].text, ' and ');
        expect(segments[2].isMention, isTrue);
        expect(segments[2].mention!.displayName, 'Bob');
        expect(segments[3].isMention, isFalse);
        expect(segments[3].text, ' are here');
      });

      test('segments consecutive mentions without text between', () {
        final segments =
            MentionParser.segmentText('@[user:u1:Alice]@[user:u2:Bob]');

        expect(segments, hasLength(2));
        expect(segments[0].isMention, isTrue);
        expect(segments[0].mention!.displayName, 'Alice');
        expect(segments[1].isMention, isTrue);
        expect(segments[1].mention!.displayName, 'Bob');
      });
    });
  });

  group('MentionSegment', () {
    test('text factory creates non-mention segment', () {
      final segment = MentionSegment.text('hello');

      expect(segment.isMention, isFalse);
      expect(segment.text, 'hello');
      expect(segment.mention, isNull);
    });

    test('mention factory creates mention segment', () {
      final mention = ParsedMention(
        type: MentionType.user,
        id: '123',
        displayName: 'John',
        startIndex: 0,
        endIndex: 10,
      );
      final segment = MentionSegment.mention(mention);

      expect(segment.isMention, isTrue);
      expect(segment.mention, mention);
      expect(segment.text, isNull);
    });
  });
}
