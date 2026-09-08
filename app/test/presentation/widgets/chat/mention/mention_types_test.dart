import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/widgets/chat/mention/mention_types.dart';

void main() {
  group('MentionType', () {
    group('icon', () {
      test('returns correct icon for each type', () {
        expect(MentionType.user.icon, '👤');
        expect(MentionType.loan.icon, '🔄');
        expect(MentionType.giveaway.icon, '🎁');
        expect(MentionType.request.icon, '❓');
        expect(MentionType.experience.icon, '📅');
      });
    });

    group('label', () {
      test('returns correct label for each type', () {
        expect(MentionType.user.label, 'User');
        expect(MentionType.loan.label, 'Loan');
        expect(MentionType.giveaway.label, 'Giveaway');
        expect(MentionType.request.label, 'Request');
        expect(MentionType.experience.label, 'Event');
      });
    });

    group('encodedType', () {
      test('returns correct encoded string for each type', () {
        expect(MentionType.user.encodedType, 'user');
        expect(MentionType.loan.encodedType, 'loan');
        expect(MentionType.giveaway.encodedType, 'giveaway');
        expect(MentionType.request.encodedType, 'request');
        expect(MentionType.experience.encodedType, 'experience');
      });
    });

    group('fromString', () {
      test('parses valid type strings', () {
        expect(MentionTypeDisplay.fromString('user'), MentionType.user);
        expect(MentionTypeDisplay.fromString('loan'), MentionType.loan);
        expect(MentionTypeDisplay.fromString('giveaway'), MentionType.giveaway);
        expect(MentionTypeDisplay.fromString('request'), MentionType.request);
        expect(
          MentionTypeDisplay.fromString('experience'),
          MentionType.experience,
        );
      });

      test('returns null for invalid type strings', () {
        expect(MentionTypeDisplay.fromString('invalid'), isNull);
        expect(MentionTypeDisplay.fromString(''), isNull);
        expect(MentionTypeDisplay.fromString('USER'), isNull); // Case-sensitive
        expect(MentionTypeDisplay.fromString('users'), isNull);
      });
    });
  });

  group('MentionSuggestion', () {
    test('creates suggestion with required fields', () {
      final suggestion = MentionSuggestion(
        type: MentionType.user,
        id: 'abc123',
        displayName: 'John Doe',
      );

      expect(suggestion.type, MentionType.user);
      expect(suggestion.id, 'abc123');
      expect(suggestion.displayName, 'John Doe');
      expect(suggestion.mediaId, isNull);
      expect(suggestion.subtitle, isNull);
    });

    test('creates suggestion with optional fields', () {
      final suggestion = MentionSuggestion(
        type: MentionType.loan,
        id: 'xyz789',
        displayName: 'Camping Tent',
        mediaId: 'media123',
        subtitle: 'Active',
      );

      expect(suggestion.mediaId, 'media123');
      expect(suggestion.subtitle, 'Active');
    });

    group('toEncodedMention', () {
      test('encodes user mention correctly', () {
        final suggestion = MentionSuggestion(
          type: MentionType.user,
          id: 'abc123',
          displayName: 'John Doe',
        );

        expect(suggestion.toEncodedMention(), '@[user:abc123:John Doe]');
      });

      test('encodes loan mention correctly', () {
        final suggestion = MentionSuggestion(
          type: MentionType.loan,
          id: 'xyz789',
          displayName: 'Camping Tent',
        );

        expect(suggestion.toEncodedMention(), '@[loan:xyz789:Camping Tent]');
      });

      test('encodes giveaway mention correctly', () {
        final suggestion = MentionSuggestion(
          type: MentionType.giveaway,
          id: 'def456',
          displayName: 'Old Bike',
        );

        expect(suggestion.toEncodedMention(), '@[giveaway:def456:Old Bike]');
      });

      test('encodes request mention correctly', () {
        final suggestion = MentionSuggestion(
          type: MentionType.request,
          id: 'ghi012',
          displayName: 'Need a ladder',
        );

        expect(suggestion.toEncodedMention(), '@[request:ghi012:Need a ladder]');
      });

      test('encodes experience mention correctly', () {
        final suggestion = MentionSuggestion(
          type: MentionType.experience,
          id: 'jkl345',
          displayName: 'Weekend Hike',
        );

        expect(
          suggestion.toEncodedMention(),
          '@[experience:jkl345:Weekend Hike]',
        );
      });

      test('handles special characters in display name', () {
        final suggestion = MentionSuggestion(
          type: MentionType.user,
          id: 'u1',
          displayName: "John O'Brien",
        );

        expect(suggestion.toEncodedMention(), "@[user:u1:John O'Brien]");
      });
    });

    group('equality', () {
      test('equal suggestions are equal', () {
        final s1 = MentionSuggestion(
          type: MentionType.user,
          id: 'abc123',
          displayName: 'John Doe',
        );
        final s2 = MentionSuggestion(
          type: MentionType.user,
          id: 'abc123',
          displayName: 'John Doe',
        );

        expect(s1, equals(s2));
        expect(s1.hashCode, equals(s2.hashCode));
      });

      test('different IDs are not equal', () {
        final s1 = MentionSuggestion(
          type: MentionType.user,
          id: 'abc123',
          displayName: 'John Doe',
        );
        final s2 = MentionSuggestion(
          type: MentionType.user,
          id: 'xyz789',
          displayName: 'John Doe',
        );

        expect(s1, isNot(equals(s2)));
      });

      test('different types are not equal', () {
        final s1 = MentionSuggestion(
          type: MentionType.user,
          id: 'abc123',
          displayName: 'John Doe',
        );
        final s2 = MentionSuggestion(
          type: MentionType.loan,
          id: 'abc123',
          displayName: 'John Doe',
        );

        expect(s1, isNot(equals(s2)));
      });

      test('optional fields do not affect equality', () {
        final s1 = MentionSuggestion(
          type: MentionType.user,
          id: 'abc123',
          displayName: 'John Doe',
          mediaId: 'media1',
        );
        final s2 = MentionSuggestion(
          type: MentionType.user,
          id: 'abc123',
          displayName: 'John Doe',
          mediaId: 'media2',
        );

        expect(s1, equals(s2));
      });
    });

    test('toString returns readable format', () {
      final suggestion = MentionSuggestion(
        type: MentionType.user,
        id: 'abc123',
        displayName: 'John Doe',
      );

      expect(
        suggestion.toString(),
        'MentionSuggestion(type: MentionType.user, id: abc123, displayName: John Doe)',
      );
    });
  });

  group('ParsedMention', () {
    test('creates parsed mention with all fields', () {
      final mention = ParsedMention(
        type: MentionType.user,
        id: 'abc123',
        displayName: 'John Doe',
        startIndex: 6,
        endIndex: 29,
      );

      expect(mention.type, MentionType.user);
      expect(mention.id, 'abc123');
      expect(mention.displayName, 'John Doe');
      expect(mention.startIndex, 6);
      expect(mention.endIndex, 29);
    });

    test('length returns correct value', () {
      final mention = ParsedMention(
        type: MentionType.user,
        id: 'abc123',
        displayName: 'John Doe',
        startIndex: 6,
        endIndex: 29,
      );

      expect(mention.length, 23);
    });

    test('toString returns readable format', () {
      final mention = ParsedMention(
        type: MentionType.user,
        id: 'abc123',
        displayName: 'John Doe',
        startIndex: 0,
        endIndex: 10,
      );

      expect(
        mention.toString(),
        'ParsedMention(type: MentionType.user, id: abc123, name: John Doe)',
      );
    });
  });
}
