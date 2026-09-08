import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/widgets/chat/mention/mention_text_controller.dart';
import 'package:ripls/presentation/widgets/chat/mention/mention_types.dart';

void main() {
  group('MentionTextController', () {
    late MentionTextController controller;
    MentionQuery? lastQuery;

    setUp(() {
      lastQuery = null;
      controller = MentionTextController(
        onMentionQueryChanged: (query) => lastQuery = query,
      );
    });

    tearDown(() {
      controller.dispose();
    });

    group('@-detection', () {
      test('detects @ at start of text', () {
        controller.text = '@';
        controller.selection = const TextSelection.collapsed(offset: 1);

        expect(lastQuery, isNotNull);
        expect(lastQuery!.atIndex, 0);
        expect(lastQuery!.queryText, '');
      });

      test('detects @ with query text', () {
        controller.text = '@john';
        controller.selection = const TextSelection.collapsed(offset: 5);

        expect(lastQuery, isNotNull);
        expect(lastQuery!.atIndex, 0);
        expect(lastQuery!.queryText, 'john');
      });

      test('detects @ after space', () {
        controller.text = 'Hello @bob';
        controller.selection = const TextSelection.collapsed(offset: 10);

        expect(lastQuery, isNotNull);
        expect(lastQuery!.atIndex, 6);
        expect(lastQuery!.queryText, 'bob');
      });

      test('detects @ after newline', () {
        controller.text = 'Hello\n@alice';
        controller.selection = const TextSelection.collapsed(offset: 12);

        expect(lastQuery, isNotNull);
        expect(lastQuery!.atIndex, 6);
        expect(lastQuery!.queryText, 'alice');
      });

      test('does not detect @ in middle of word', () {
        controller.text = 'email@test';
        controller.selection = const TextSelection.collapsed(offset: 10);

        expect(lastQuery, isNull);
      });

      test('does not detect @ when query contains space', () {
        controller.text = '@john doe';
        controller.selection = const TextSelection.collapsed(offset: 9);

        expect(lastQuery, isNull);
      });

      test('clears query when cursor moves away', () {
        // First, type @ to start query
        controller.text = '@john';
        controller.selection = const TextSelection.collapsed(offset: 5);
        expect(lastQuery, isNotNull);

        // Move cursor to beginning
        controller.selection = const TextSelection.collapsed(offset: 0);
        expect(lastQuery, isNull);
      });

      test('clears query when text is cleared', () {
        controller.text = '@john';
        controller.selection = const TextSelection.collapsed(offset: 5);
        expect(lastQuery, isNotNull);

        controller.text = '';
        controller.selection = const TextSelection.collapsed(offset: 0);
        expect(lastQuery, isNull);
      });

      test('handles multiple @ symbols, uses last valid one', () {
        controller.text = 'Hi @alice check @bob';
        controller.selection = const TextSelection.collapsed(offset: 20);

        expect(lastQuery, isNotNull);
        expect(lastQuery!.queryText, 'bob');
      });

      group('punctuation triggers', () {
        test('detects @ after opening parenthesis', () {
          controller.text = 'Check out (@john';
          controller.selection = const TextSelection.collapsed(offset: 16);

          expect(lastQuery, isNotNull);
          expect(lastQuery!.atIndex, 11);
          expect(lastQuery!.queryText, 'john');
        });

        test('detects @ after opening bracket', () {
          controller.text = 'See [@alice';
          controller.selection = const TextSelection.collapsed(offset: 11);

          expect(lastQuery, isNotNull);
          expect(lastQuery!.atIndex, 5);
          expect(lastQuery!.queryText, 'alice');
        });

        test('detects @ after opening brace', () {
          controller.text = 'Test {@bob';
          controller.selection = const TextSelection.collapsed(offset: 10);

          expect(lastQuery, isNotNull);
          expect(lastQuery!.atIndex, 6);
          expect(lastQuery!.queryText, 'bob');
        });

        test('detects @ after colon', () {
          controller.text = 'CC:@john';
          controller.selection = const TextSelection.collapsed(offset: 8);

          expect(lastQuery, isNotNull);
          expect(lastQuery!.atIndex, 3);
          expect(lastQuery!.queryText, 'john');
        });

        test('detects @ after comma', () {
          controller.text = 'Hi,@alice';
          controller.selection = const TextSelection.collapsed(offset: 9);

          expect(lastQuery, isNotNull);
          expect(lastQuery!.atIndex, 3);
          expect(lastQuery!.queryText, 'alice');
        });

        test('detects @ after hyphen/dash', () {
          controller.text = 'Ask-@bob';
          controller.selection = const TextSelection.collapsed(offset: 8);

          expect(lastQuery, isNotNull);
          expect(lastQuery!.atIndex, 4);
          expect(lastQuery!.queryText, 'bob');
        });

        test('detects @ after exclamation mark', () {
          controller.text = 'Hey!@john';
          controller.selection = const TextSelection.collapsed(offset: 9);

          expect(lastQuery, isNotNull);
          expect(lastQuery!.atIndex, 4);
          expect(lastQuery!.queryText, 'john');
        });

        test('detects @ after question mark', () {
          controller.text = 'Who?@alice';
          controller.selection = const TextSelection.collapsed(offset: 10);

          expect(lastQuery, isNotNull);
          expect(lastQuery!.atIndex, 4);
          expect(lastQuery!.queryText, 'alice');
        });

        test('detects @ after double quote', () {
          controller.text = 'Said "@bob';
          controller.selection = const TextSelection.collapsed(offset: 10);

          expect(lastQuery, isNotNull);
          expect(lastQuery!.atIndex, 6);
          expect(lastQuery!.queryText, 'bob');
        });

        test('detects @ after single quote', () {
          controller.text = "It's '@john";
          controller.selection = const TextSelection.collapsed(offset: 11);

          expect(lastQuery, isNotNull);
          expect(lastQuery!.atIndex, 6);
          expect(lastQuery!.queryText, 'john');
        });

        test('detects @ after forward slash', () {
          controller.text = 'with/@alice';
          controller.selection = const TextSelection.collapsed(offset: 11);

          expect(lastQuery, isNotNull);
          expect(lastQuery!.atIndex, 5);
          expect(lastQuery!.queryText, 'alice');
        });

        test('detects @ after angle bracket', () {
          controller.text = 'List <@bob';
          controller.selection = const TextSelection.collapsed(offset: 10);

          expect(lastQuery, isNotNull);
          expect(lastQuery!.atIndex, 6);
          expect(lastQuery!.queryText, 'bob');
        });

        test('detects @ after semicolon', () {
          controller.text = 'Done;@john';
          controller.selection = const TextSelection.collapsed(offset: 10);

          expect(lastQuery, isNotNull);
          expect(lastQuery!.atIndex, 5);
          expect(lastQuery!.queryText, 'john');
        });
      });
    });

    group('insertMention', () {
      test('inserts encoded mention at query position', () {
        controller.text = 'Hello @joh';
        controller.selection = const TextSelection.collapsed(offset: 10);

        final suggestion = MentionSuggestion(
          type: MentionType.user,
          id: 'user-123',
          displayName: 'John Doe',
        );

        controller.insertMention(suggestion);

        // Display text shows @DisplayName
        expect(controller.text, 'Hello @John Doe ');
        expect(controller.selection.baseOffset, 16); // After mention + space
        // Encoded text has the full format for sending
        expect(controller.encodedText, 'Hello @[user:user-123:John Doe] ');
      });

      test('preserves text after cursor when inserting', () {
        controller.text = 'Hi @joh world';
        controller.selection = const TextSelection.collapsed(offset: 7);

        final suggestion = MentionSuggestion(
          type: MentionType.user,
          id: 'user-123',
          displayName: 'John',
        );

        controller.insertMention(suggestion);

        // Display text shows @DisplayName
        expect(controller.text, 'Hi @John  world');
        // Encoded text has the full format for sending
        expect(controller.encodedText, 'Hi @[user:user-123:John]  world');
      });

      test('does nothing if not in mention mode', () {
        controller.text = 'Hello world';
        controller.selection = const TextSelection.collapsed(offset: 5);

        final suggestion = MentionSuggestion(
          type: MentionType.user,
          id: 'user-123',
          displayName: 'John',
        );

        controller.insertMention(suggestion);

        expect(controller.text, 'Hello world');
      });

      test('inserts loan mention with correct format', () {
        controller.text = '@tent';
        controller.selection = const TextSelection.collapsed(offset: 5);

        final suggestion = MentionSuggestion(
          type: MentionType.loan,
          id: 'transfer-456',
          displayName: 'Camping Tent',
        );

        controller.insertMention(suggestion);

        // Display text shows @DisplayName
        expect(controller.text, '@Camping Tent ');
        // Encoded text has the full format for sending
        expect(controller.encodedText, '@[loan:transfer-456:Camping Tent] ');
      });

      test('inserts giveaway mention with correct format', () {
        controller.text = '@bike';
        controller.selection = const TextSelection.collapsed(offset: 5);

        final suggestion = MentionSuggestion(
          type: MentionType.giveaway,
          id: 'transfer-789',
          displayName: 'Old Bike',
        );

        controller.insertMention(suggestion);

        // Display text shows @DisplayName
        expect(controller.text, '@Old Bike ');
        // Encoded text has the full format for sending
        expect(controller.encodedText, '@[giveaway:transfer-789:Old Bike] ');
      });

      test('inserts request mention with correct format', () {
        controller.text = '@ladder';
        controller.selection = const TextSelection.collapsed(offset: 7);

        final suggestion = MentionSuggestion(
          type: MentionType.request,
          id: 'req-123',
          displayName: 'Need a ladder',
        );

        controller.insertMention(suggestion);

        // Display text shows @DisplayName
        expect(controller.text, '@Need a ladder ');
        // Encoded text has the full format for sending
        expect(controller.encodedText, '@[request:req-123:Need a ladder] ');
      });

      test('inserts experience mention with correct format', () {
        controller.text = '@hike';
        controller.selection = const TextSelection.collapsed(offset: 5);

        final suggestion = MentionSuggestion(
          type: MentionType.experience,
          id: 'exp-456',
          displayName: 'Weekend Hike',
        );

        controller.insertMention(suggestion);

        // Display text shows @DisplayName
        expect(controller.text, '@Weekend Hike ');
        // Encoded text has the full format for sending
        expect(controller.encodedText, '@[experience:exp-456:Weekend Hike] ');
      });
    });

    group('isInMentionMode', () {
      test('returns true when typing after @', () {
        controller.text = '@';
        controller.selection = const TextSelection.collapsed(offset: 1);

        expect(controller.isInMentionMode, isTrue);
      });

      test('returns false for regular text', () {
        controller.text = 'Hello world';
        controller.selection = const TextSelection.collapsed(offset: 5);

        expect(controller.isInMentionMode, isFalse);
      });
    });

    group('cancelMention', () {
      test('clears current query', () {
        controller.text = '@john';
        controller.selection = const TextSelection.collapsed(offset: 5);
        expect(controller.isInMentionMode, isTrue);

        controller.cancelMention();

        expect(controller.isInMentionMode, isFalse);
        expect(lastQuery, isNull);
      });

      test('does nothing when not in mention mode', () {
        controller.text = 'Hello';
        controller.selection = const TextSelection.collapsed(offset: 5);

        controller.cancelMention();

        expect(controller.text, 'Hello');
      });
    });

    group('atomic mention deletion', () {
      test('backspace at end of mention deletes entire mention', () {
        // Setup: Insert a mention
        controller.text = 'Hello @joh';
        controller.selection = const TextSelection.collapsed(offset: 10);
        controller.insertMention(MentionSuggestion(
          type: MentionType.user,
          id: 'user-123',
          displayName: 'John Doe',
        ));
        expect(controller.text, 'Hello @John Doe ');

        // Simulate backspace at end of mention (cursor after 'e' in 'Doe')
        // First position cursor right after mention
        controller.selection = const TextSelection.collapsed(offset: 15);

        // Simulate single character deletion (backspace)
        controller.value = const TextEditingValue(
          text: 'Hello @John Do ',
          selection: TextSelection.collapsed(offset: 14),
        );

        // Entire mention should be deleted
        expect(controller.text, 'Hello  ');
        expect(controller.selection.baseOffset, 6);
      });

      test('backspace in middle of mention deletes entire mention', () {
        controller.text = '@joh';
        controller.selection = const TextSelection.collapsed(offset: 4);
        controller.insertMention(MentionSuggestion(
          type: MentionType.user,
          id: 'user-123',
          displayName: 'John Doe',
        ));
        expect(controller.text, '@John Doe ');

        // Position cursor in middle of mention
        controller.selection = const TextSelection.collapsed(offset: 5);

        // Simulate backspace (delete 'J')
        controller.value = const TextEditingValue(
          text: '@ohn Doe ',
          selection: TextSelection.collapsed(offset: 4),
        );

        // Entire mention should be deleted
        expect(controller.text, ' ');
        expect(controller.selection.baseOffset, 0);
      });

      test('backspace preserves text around deleted mention', () {
        controller.text = 'Hi @joh there!';
        controller.selection = const TextSelection.collapsed(offset: 7);
        controller.insertMention(MentionSuggestion(
          type: MentionType.user,
          id: 'user-123',
          displayName: 'John Doe',
        ));
        expect(controller.text, 'Hi @John Doe  there!');

        // Position cursor at end of mention
        controller.selection = const TextSelection.collapsed(offset: 12);

        // Simulate backspace
        controller.value = const TextEditingValue(
          text: 'Hi @John Do  there!',
          selection: TextSelection.collapsed(offset: 11),
        );

        // Mention deleted, surrounding text preserved
        expect(controller.text, 'Hi   there!');
        expect(controller.selection.baseOffset, 3);
      });

      test('normal deletion outside mention works normally', () {
        controller.text = 'Hello @joh';
        controller.selection = const TextSelection.collapsed(offset: 10);
        controller.insertMention(MentionSuggestion(
          type: MentionType.user,
          id: 'user-123',
          displayName: 'John Doe',
        ));
        expect(controller.text, 'Hello @John Doe ');

        // Position cursor after trailing space
        controller.selection = const TextSelection.collapsed(offset: 16);

        // Type more text
        controller.value = const TextEditingValue(
          text: 'Hello @John Doe world',
          selection: TextSelection.collapsed(offset: 21),
        );

        // Now backspace the 'd' from 'world' (outside mention)
        controller.value = const TextEditingValue(
          text: 'Hello @John Doe worl',
          selection: TextSelection.collapsed(offset: 20),
        );

        // Regular text deleted, mention preserved
        expect(controller.text, 'Hello @John Doe worl');
        expect(controller.encodedText, 'Hello @[user:user-123:John Doe] worl');
      });

      test('deleting multiple characters at once does not trigger atomic delete', () {
        controller.text = '@joh';
        controller.selection = const TextSelection.collapsed(offset: 4);
        controller.insertMention(MentionSuggestion(
          type: MentionType.user,
          id: 'user-123',
          displayName: 'John Doe',
        ));
        expect(controller.text, '@John Doe ');

        // Select and delete multiple characters (select "ohn D")
        controller.selection = const TextSelection(baseOffset: 2, extentOffset: 7);

        // Simulate deleting selected text (5 chars deleted)
        controller.value = const TextEditingValue(
          text: '@Joe ',
          selection: TextSelection.collapsed(offset: 2),
        );

        // Text should be as provided (not atomic deletion)
        expect(controller.text, '@Joe ');
      });

      test('mentions after deleted mention have positions adjusted', () {
        // Insert first mention
        controller.text = '@joh';
        controller.selection = const TextSelection.collapsed(offset: 4);
        controller.insertMention(MentionSuggestion(
          type: MentionType.user,
          id: 'user-1',
          displayName: 'John',
        ));
        expect(controller.text, '@John ');

        // Add text and insert second mention
        controller.value = const TextEditingValue(
          text: '@John and @ali',
          selection: TextSelection.collapsed(offset: 14),
        );
        controller.insertMention(MentionSuggestion(
          type: MentionType.user,
          id: 'user-2',
          displayName: 'Alice',
        ));
        expect(controller.text, '@John and @Alice ');
        expect(
            controller.encodedText, '@[user:user-1:John] and @[user:user-2:Alice] ');

        // Now delete the first mention
        controller.selection = const TextSelection.collapsed(offset: 5);
        controller.value = const TextEditingValue(
          text: '@Joh and @Alice ',
          selection: TextSelection.collapsed(offset: 4),
        );

        // First mention deleted, second mention still valid
        expect(controller.text, ' and @Alice ');
        expect(controller.encodedText, ' and @[user:user-2:Alice] ');
      });
    });
  });
}
