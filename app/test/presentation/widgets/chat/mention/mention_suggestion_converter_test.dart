import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/experience.pb.dart';
import 'package:ripls/data/gen/ripls/api/gear.pb.dart';
import 'package:ripls/data/gen/ripls/api/request.pb.dart';
import 'package:ripls/data/gen/ripls/api/search_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/presentation/widgets/chat/mention/mention_suggestion_converter.dart';
import 'package:ripls/presentation/widgets/chat/mention/mention_types.dart';

void main() {
  group('MentionSuggestionConverter', () {
    group('fromSearchResults', () {
      test('returns empty list for empty results', () {
        final suggestions =
            MentionSuggestionConverter.fromSearchResults([]);

        expect(suggestions, isEmpty);
      });

      test('converts user result to MentionSuggestion', () {
        final user = User()
          ..id = 'user-123'
          ..name = 'John Doe'
          ..mediaId = 'media-456';

        final result = SearchResultItem()
          ..itemType = SearchItemType.SEARCH_ITEM_TYPE_USER
          ..user = user;

        final suggestions =
            MentionSuggestionConverter.fromSearchResults([result]);

        expect(suggestions, hasLength(1));
        expect(suggestions[0].type, MentionType.user);
        expect(suggestions[0].id, 'user-123');
        expect(suggestions[0].displayName, 'John Doe');
        expect(suggestions[0].mediaId, 'media-456');
      });

      test('converts user without mediaId', () {
        final user = User()
          ..id = 'user-123'
          ..name = 'Jane Doe';

        final result = SearchResultItem()
          ..itemType = SearchItemType.SEARCH_ITEM_TYPE_USER
          ..user = user;

        final suggestions =
            MentionSuggestionConverter.fromSearchResults([result]);

        expect(suggestions, hasLength(1));
        expect(suggestions[0].mediaId, isNull);
      });

      test('converts gear with LOAN availability to loan suggestion', () {
        final gear = Gear()
          ..id = 'gear-123'
          ..name = 'Camping Tent'
          ..availability = Availability.AVAILABILITY_FOR_LOAN
          ..mediaIds.add('media-789');

        final result = SearchResultItem()
          ..itemType = SearchItemType.SEARCH_ITEM_TYPE_GEAR
          ..gear = gear;

        final suggestions =
            MentionSuggestionConverter.fromSearchResults([result]);

        expect(suggestions, hasLength(1));
        expect(suggestions[0].type, MentionType.loan);
        expect(suggestions[0].id, 'gear-123');
        expect(suggestions[0].displayName, 'Camping Tent');
        expect(suggestions[0].mediaId, 'media-789');
      });

      test('converts gear with GIVEAWAY availability to giveaway suggestion',
          () {
        final gear = Gear()
          ..id = 'gear-456'
          ..name = 'Old Bike'
          ..availability = Availability.AVAILABILITY_FOR_GIVEAWAY;

        final result = SearchResultItem()
          ..itemType = SearchItemType.SEARCH_ITEM_TYPE_GEAR
          ..gear = gear;

        final suggestions =
            MentionSuggestionConverter.fromSearchResults([result]);

        expect(suggestions, hasLength(1));
        expect(suggestions[0].type, MentionType.giveaway);
        expect(suggestions[0].id, 'gear-456');
        expect(suggestions[0].displayName, 'Old Bike');
      });

      test('uses transferId from activeLoan when available', () {
        final activeLoan = ActiveLoan()
          ..transferId = 'transfer-999'
          ..status = 'On loan';

        final gear = Gear()
          ..id = 'gear-123'
          ..name = 'Camping Tent'
          ..availability = Availability.AVAILABILITY_FOR_LOAN
          ..activeLoan = activeLoan;

        final result = SearchResultItem()
          ..itemType = SearchItemType.SEARCH_ITEM_TYPE_GEAR
          ..gear = gear;

        final suggestions =
            MentionSuggestionConverter.fromSearchResults([result]);

        expect(suggestions, hasLength(1));
        expect(suggestions[0].id, 'transfer-999');
        expect(suggestions[0].subtitle, 'On loan');
      });

      test('converts request result to MentionSuggestion', () {
        final request = Request()
          ..id = 'request-123'
          ..title = 'Need a ladder'
          ..mediaIds.add('media-req-456');

        final result = SearchResultItem()
          ..itemType = SearchItemType.SEARCH_ITEM_TYPE_REQUEST
          ..request = request;

        final suggestions =
            MentionSuggestionConverter.fromSearchResults([result]);

        expect(suggestions, hasLength(1));
        expect(suggestions[0].type, MentionType.request);
        expect(suggestions[0].id, 'request-123');
        expect(suggestions[0].displayName, 'Need a ladder');
        expect(suggestions[0].mediaId, 'media-req-456');
      });

      test('converts experience result to MentionSuggestion', () {
        final experience = Experience()
          ..id = 'exp-123'
          ..name = 'Weekend Hike'
          ..mediaIds.add('media-exp-789');

        final result = SearchResultItem()
          ..itemType = SearchItemType.SEARCH_ITEM_TYPE_EXPERIENCE
          ..experience = experience;

        final suggestions =
            MentionSuggestionConverter.fromSearchResults([result]);

        expect(suggestions, hasLength(1));
        expect(suggestions[0].type, MentionType.experience);
        expect(suggestions[0].id, 'exp-123');
        expect(suggestions[0].displayName, 'Weekend Hike');
        expect(suggestions[0].mediaId, 'media-exp-789');
      });

      test('ignores unknown item types', () {
        final result = SearchResultItem()
          ..itemType = SearchItemType.SEARCH_ITEM_TYPE_UNSPECIFIED;

        final suggestions =
            MentionSuggestionConverter.fromSearchResults([result]);

        expect(suggestions, isEmpty);
      });

      test('converts multiple results', () {
        final user = User()
          ..id = 'user-1'
          ..name = 'Alice';

        final gear = Gear()
          ..id = 'gear-1'
          ..name = 'Tent'
          ..availability = Availability.AVAILABILITY_FOR_LOAN;

        final request = Request()
          ..id = 'req-1'
          ..title = 'Need help';

        final experience = Experience()
          ..id = 'exp-1'
          ..name = 'Beach Party';

        final results = [
          SearchResultItem()
            ..itemType = SearchItemType.SEARCH_ITEM_TYPE_USER
            ..user = user,
          SearchResultItem()
            ..itemType = SearchItemType.SEARCH_ITEM_TYPE_GEAR
            ..gear = gear,
          SearchResultItem()
            ..itemType = SearchItemType.SEARCH_ITEM_TYPE_REQUEST
            ..request = request,
          SearchResultItem()
            ..itemType = SearchItemType.SEARCH_ITEM_TYPE_EXPERIENCE
            ..experience = experience,
        ];

        final suggestions =
            MentionSuggestionConverter.fromSearchResults(results);

        expect(suggestions, hasLength(4));
        expect(suggestions[0].type, MentionType.user);
        expect(suggestions[1].type, MentionType.loan);
        expect(suggestions[2].type, MentionType.request);
        expect(suggestions[3].type, MentionType.experience);
      });

      test('calls imageProviderFactory for items with mediaId', () {
        final mediaIdsRequested = <String>[];

        final user = User()
          ..id = 'user-1'
          ..name = 'Alice'
          ..mediaId = 'media-1';

        final gear = Gear()
          ..id = 'gear-1'
          ..name = 'Tent'
          ..availability = Availability.AVAILABILITY_FOR_LOAN
          ..mediaIds.add('media-2');

        final results = [
          SearchResultItem()
            ..itemType = SearchItemType.SEARCH_ITEM_TYPE_USER
            ..user = user,
          SearchResultItem()
            ..itemType = SearchItemType.SEARCH_ITEM_TYPE_GEAR
            ..gear = gear,
        ];

        MentionSuggestionConverter.fromSearchResults(
          results,
          imageProviderFactory: (mediaId) {
            mediaIdsRequested.add(mediaId);
            return null;
          },
        );

        expect(mediaIdsRequested, contains('media-1'));
        expect(mediaIdsRequested, contains('media-2'));
      });
    });
  });
}
