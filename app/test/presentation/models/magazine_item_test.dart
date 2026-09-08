import 'package:fixnum/fixnum.dart' show Int64;
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/experience.pb.dart' show Experience;
import 'package:ripls/data/gen/ripls/api/gear.pb.dart' show Availability;
import 'package:ripls/data/gen/ripls/api/request.pb.dart' show Request;
import 'package:ripls/data/gen/ripls/api/time.pb.dart'
    show ExperienceTime, SpecificTime, TimeRange, TimeTBD;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/models/discover_item.dart';
import 'package:ripls/presentation/models/magazine_item.dart';

void main() {
  // ─── MagazineItem.fromDiscoverItem ────────────────────────────────────────

  group('MagazineItem.fromDiscoverItem', () {
    group('gear items', () {
      test('maps loan gear correctly', () {
        final gear = CommunityGearItem(
          id: 'gear-1',
          name: 'Pressure Washer',
          availability: Availability.AVAILABILITY_FOR_LOAN,
          owner: User(id: 'user-1', name: 'Thomas'),
          mediaIds: ['media-g1'],
        );

        final item = MagazineItem.fromDiscoverItem(GearDiscoverItem(gear));

        expect(item.id, 'gear-1');
        expect(item.mediaId, 'media-g1');
        expect(item.title, 'Pressure Washer');
        expect(item.typeLabel, 'Lending');
        expect(item.emoji, '↩');
        expect(item.itemType, 'lending');
        expect(item.ownerName, 'Thomas');
        expect(item.commentSender, isNull);
        expect(item.commentText, isNull);
        expect(item.dateLabel, isNull);
      });

      test('maps giveaway gear correctly', () {
        final gear = CommunityGearItem(
          id: 'gear-2',
          name: 'Old Bike',
          availability: Availability.AVAILABILITY_FOR_GIVEAWAY,
          owner: User(id: 'user-1', name: 'Alice'),
        );

        final item = MagazineItem.fromDiscoverItem(GearDiscoverItem(gear));

        expect(item.typeLabel, 'Giving');
        expect(item.emoji, '🎁');
        expect(item.itemType, 'giving');
      });

      test('uses empty mediaId when no media', () {
        final gear = CommunityGearItem(id: 'gear-3', mediaIds: []);

        final item = MagazineItem.fromDiscoverItem(GearDiscoverItem(gear));

        expect(item.mediaId, '');
      });

      test('ownerName is null when owner name is empty', () {
        final gear = CommunityGearItem(
          id: 'gear-4',
          owner: User(id: 'user-1', name: ''),
        );

        final item = MagazineItem.fromDiscoverItem(GearDiscoverItem(gear));

        expect(item.ownerName, isNull);
      });
    });

    group('request items', () {
      test('maps request correctly', () {
        final request = Request(
          id: 'req-1',
          title: 'Need a ladder',
          requester: User(id: 'user-2', name: 'Bob'),
          mediaIds: ['media-r1'],
        );

        final item = MagazineItem.fromDiscoverItem(RequestDiscoverItem(request));

        expect(item.id, 'req-1');
        expect(item.mediaId, 'media-r1');
        expect(item.title, 'Need a ladder');
        expect(item.typeLabel, 'Help');
        expect(item.emoji, '🤝');
        expect(item.itemType, 'help');
        expect(item.ownerName, 'Bob');
      });

      test('falls back to description when title is empty', () {
        final request = Request(
          id: 'req-2',
          title: '',
          description: 'Looking for a drill',
          requester: User(id: 'user-2', name: 'Bob'),
        );

        final item = MagazineItem.fromDiscoverItem(RequestDiscoverItem(request));

        expect(item.title, 'Looking for a drill');
      });
    });

    group('experience items', () {
      test('maps experience correctly', () {
        final experience = Experience(
          id: 'exp-1',
          name: 'Community Potluck',
          owner: User(id: 'user-3', name: 'Carol'),
          mediaIds: ['media-e1'],
        );

        final item =
            MagazineItem.fromDiscoverItem(ExperienceDiscoverItem(experience));

        expect(item.id, 'exp-1');
        expect(item.mediaId, 'media-e1');
        expect(item.title, 'Community Potluck');
        expect(item.typeLabel, 'Event');
        expect(item.emoji, '📅');
        expect(item.itemType, 'event');
        expect(item.ownerName, 'Carol');
      });

      test('dateLabel includes date and time for specific timed experience', () {
        final experience = Experience(
          id: 'exp-2',
          name: 'Morning Run',
          owner: User(id: 'u1', name: 'Alice'),
          time: ExperienceTime(
            specific: SpecificTime(
              unixTimestampSec: Int64(1709650800), // 2024-03-05 15:00:00 UTC
              isAllDay: false,
            ),
          ),
        );

        final item =
            MagazineItem.fromDiscoverItem(ExperienceDiscoverItem(experience));

        expect(item.dateLabel, isNotNull);
        expect(item.dateLabel, startsWith('Mar'));
        expect(item.dateLabel, anyOf(contains('AM'), contains('PM')));
      });

      test('dateLabel is set for all-day experience', () {
        final experience = Experience(
          id: 'exp-3',
          name: 'All Day Festival',
          owner: User(id: 'u1', name: 'Alice'),
          time: ExperienceTime(
            specific: SpecificTime(
              unixTimestampSec: Int64(1709650800),
              isAllDay: true,
            ),
          ),
        );

        final item =
            MagazineItem.fromDiscoverItem(ExperienceDiscoverItem(experience));

        expect(item.dateLabel, isNotNull);
        expect(item.dateLabel, contains('Mar'));
      });

      test('dateLabel is set for range experience', () {
        final experience = Experience(
          id: 'exp-4',
          name: 'Weekend Camp',
          owner: User(id: 'u1', name: 'Alice'),
          time: ExperienceTime(
            range: TimeRange(
              startUnixSec: Int64(1709650800),
              endUnixSec: Int64(1709823600), // 2024-03-07
            ),
          ),
        );

        final item =
            MagazineItem.fromDiscoverItem(ExperienceDiscoverItem(experience));

        expect(item.dateLabel, isNotNull);
        expect(item.dateLabel, contains('Mar'));
      });

      test('dateLabel is null for TBD experience', () {
        final experience = Experience(
          id: 'exp-5',
          name: 'TBD Event',
          owner: User(id: 'u1', name: 'Alice'),
          time: ExperienceTime(tbd: TimeTBD()),
        );

        final item =
            MagazineItem.fromDiscoverItem(ExperienceDiscoverItem(experience));

        expect(item.dateLabel, isNull);
      });

      test('dateLabel is null when no time is set', () {
        final experience = Experience(
          id: 'exp-6',
          name: 'No Time Event',
          owner: User(id: 'u1', name: 'Alice'),
        );

        final item =
            MagazineItem.fromDiscoverItem(ExperienceDiscoverItem(experience));

        expect(item.dateLabel, isNull);
      });
    });

    group('location name', () {
      test('passes through pre-resolved locationName', () {
        final gear = CommunityGearItem(
          id: 'gear-5',
          owner: User(id: 'u1', name: 'Alice'),
        );

        final item = MagazineItem.fromDiscoverItem(
          GearDiscoverItem(gear),
          locationName: 'Boulder, CO',
        );

        expect(item.locationName, 'Boulder, CO');
      });

      test('locationName is null when not provided', () {
        final gear = CommunityGearItem(id: 'gear-6', owner: User(id: 'u1'));

        final item = MagazineItem.fromDiscoverItem(GearDiscoverItem(gear));

        expect(item.locationName, isNull);
      });
    });
  });

  // ─── bottomLine display logic ─────────────────────────────────────────────

  group('bottomLine', () {
    test('priority 1: shows comment sender and text', () {
      const item = MagazineItem(
        id: 'x',
        mediaId: '',
        title: '',
        typeLabel: '',
        emoji: '',
        itemType: 'lending',
        commentSender: 'Thomas',
        commentText: 'Still available?',
        ownerName: 'Alice',
        locationName: 'Denver, CO',
      );

      expect(item.bottomLine, 'Thomas · Still available?');
    });

    test('priority 1: shows comment text alone when sender is absent', () {
      const item = MagazineItem(
        id: 'x',
        mediaId: '',
        title: '',
        typeLabel: '',
        emoji: '',
        itemType: 'lending',
        commentText: 'Still available?',
      );

      expect(item.bottomLine, 'Still available?');
    });

    test('priority 1: shows sender alone when text is absent', () {
      const item = MagazineItem(
        id: 'x',
        mediaId: '',
        title: '',
        typeLabel: '',
        emoji: '',
        itemType: 'lending',
        commentSender: 'Thomas',
      );

      expect(item.bottomLine, 'Thomas');
    });

    test('priority 2: shows owner · location when no comment', () {
      const item = MagazineItem(
        id: 'x',
        mediaId: '',
        title: '',
        typeLabel: '',
        emoji: '',
        itemType: 'lending',
        ownerName: 'Alice',
        locationName: 'Denver, CO',
      );

      expect(item.bottomLine, 'Alice · Denver, CO');
    });

    test('priority 3: shows owner alone', () {
      const item = MagazineItem(
        id: 'x',
        mediaId: '',
        title: '',
        typeLabel: '',
        emoji: '',
        itemType: 'lending',
        ownerName: 'Alice',
      );

      expect(item.bottomLine, 'Alice');
    });

    test('priority 4: shows location alone', () {
      const item = MagazineItem(
        id: 'x',
        mediaId: '',
        title: '',
        typeLabel: '',
        emoji: '',
        itemType: 'lending',
        locationName: 'Denver, CO',
      );

      expect(item.bottomLine, 'Denver, CO');
    });

    test('priority 5: returns empty string when nothing available', () {
      const item = MagazineItem(
        id: 'x',
        mediaId: '',
        title: '',
        typeLabel: '',
        emoji: '',
        itemType: 'lending',
      );

      expect(item.bottomLine, '');
    });

    test('comment takes priority over owner and location', () {
      const item = MagazineItem(
        id: 'x',
        mediaId: '',
        title: '',
        typeLabel: '',
        emoji: '',
        itemType: 'lending',
        commentSender: 'Thomas',
        commentText: 'Looks great!',
        ownerName: 'Alice',
        locationName: 'Denver, CO',
      );

      // Must show comment, not "Alice · Denver, CO".
      expect(item.bottomLine, 'Thomas · Looks great!');
    });
  });
}
