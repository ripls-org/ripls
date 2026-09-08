import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/request.pb.dart' show Request;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/models/discover_item.dart';

void main() {
  group('GearDiscoverItem', () {
    test('id returns gear id', () {
      final gear = CommunityGearItem(id: 'gear-123');
      final item = GearDiscoverItem(gear);

      expect(item.id, 'gear-123');
    });

    test('primaryMediaId returns first media id when available', () {
      final gear = CommunityGearItem(
        id: 'gear-1',
        mediaIds: ['media-1', 'media-2'],
      );
      final item = GearDiscoverItem(gear);

      expect(item.primaryMediaId, 'media-1');
    });

    test('primaryMediaId returns empty string when no media', () {
      final gear = CommunityGearItem(id: 'gear-1');
      final item = GearDiscoverItem(gear);

      expect(item.primaryMediaId, '');
    });

    test('thumbnailMediaId returns same as primaryMediaId', () {
      final gear = CommunityGearItem(
        id: 'gear-1',
        mediaIds: ['media-1', 'media-2'],
      );
      final item = GearDiscoverItem(gear);

      expect(item.thumbnailMediaId, item.primaryMediaId);
      expect(item.thumbnailMediaId, 'media-1');
    });

    test('name returns gear name', () {
      final gear = CommunityGearItem(
        id: 'gear-1',
        name: 'Test Gear Name',
      );
      final item = GearDiscoverItem(gear);

      expect(item.name, 'Test Gear Name');
    });

    test('user returns gear owner', () {
      final owner = User(id: 'owner-1', name: 'Owner Name');
      final gear = CommunityGearItem(
        id: 'gear-1',
        owner: owner,
      );
      final item = GearDiscoverItem(gear);

      expect(item.user.id, 'owner-1');
      expect(item.user.name, 'Owner Name');
    });

    test('ownerName returns gear owner name', () {
      final owner = User(id: 'owner-1', name: 'Owner Name');
      final gear = CommunityGearItem(
        id: 'gear-1',
        owner: owner,
      );
      final item = GearDiscoverItem(gear);

      expect(item.ownerName, 'Owner Name');
    });

    test('when calls gear callback', () {
      final gear = CommunityGearItem(id: 'gear-1', name: 'Test');
      final item = GearDiscoverItem(gear);

      final result = item.when(
        gear: (g) => 'gear: ${g.name}',
        request: (r) => 'request: ${r.title}',
        experience: (e) => 'experience: ${e.name}',
      );

      expect(result, 'gear: Test');
    });

    test('equality is based on gear id', () {
      final gear1 = CommunityGearItem(id: 'gear-1', name: 'Name 1');
      final gear2 = CommunityGearItem(id: 'gear-1', name: 'Name 2');
      final gear3 = CommunityGearItem(id: 'gear-2', name: 'Name 1');

      expect(GearDiscoverItem(gear1), equals(GearDiscoverItem(gear2)));
      expect(GearDiscoverItem(gear1), isNot(equals(GearDiscoverItem(gear3))));
    });

    test('hashCode is based on gear id', () {
      final gear1 = CommunityGearItem(id: 'gear-1');
      final gear2 = CommunityGearItem(id: 'gear-1');

      expect(GearDiscoverItem(gear1).hashCode, GearDiscoverItem(gear2).hashCode);
    });
  });

  group('RequestDiscoverItem', () {
    test('id returns request id', () {
      final request = Request(id: 'req-123');
      final item = RequestDiscoverItem(request);

      expect(item.id, 'req-123');
    });

    test('primaryMediaId returns request mediaId', () {
      final request = Request(
        id: 'req-1',
        mediaIds: ['media-123'],
      );
      final item = RequestDiscoverItem(request);

      expect(item.primaryMediaId, 'media-123');
    });

    test('primaryMediaId returns empty string when no media', () {
      final request = Request(id: 'req-1');
      final item = RequestDiscoverItem(request);

      expect(item.primaryMediaId, '');
    });

    test('thumbnailMediaId returns same as primaryMediaId', () {
      final request = Request(
        id: 'req-1',
        mediaIds: ['media-123'],
      );
      final item = RequestDiscoverItem(request);

      expect(item.thumbnailMediaId, item.primaryMediaId);
      expect(item.thumbnailMediaId, 'media-123');
    });

    test('name returns request title', () {
      final request = Request(
        id: 'req-1',
        title: 'Test Request Title',
      );
      final item = RequestDiscoverItem(request);

      expect(item.name, 'Test Request Title');
    });

    test('user returns request requester', () {
      final requester = User(id: 'requester-1', name: 'Requester Name');
      final request = Request(
        id: 'req-1',
        requester: requester,
      );
      final item = RequestDiscoverItem(request);

      expect(item.user.id, 'requester-1');
      expect(item.user.name, 'Requester Name');
    });

    test('ownerName returns request requester name', () {
      final requester = User(id: 'requester-1', name: 'Requester Name');
      final request = Request(
        id: 'req-1',
        requester: requester,
      );
      final item = RequestDiscoverItem(request);

      expect(item.ownerName, 'Requester Name');
    });

    test('when calls request callback', () {
      final request = Request(id: 'req-1', title: 'Test Request');
      final item = RequestDiscoverItem(request);

      final result = item.when(
        gear: (g) => 'gear: ${g.name}',
        request: (r) => 'request: ${r.title}',
        experience: (e) => 'experience: ${e.name}',
      );

      expect(result, 'request: Test Request');
    });

    test('equality is based on request id', () {
      final req1 = Request(id: 'req-1', title: 'Title 1');
      final req2 = Request(id: 'req-1', title: 'Title 2');
      final req3 = Request(id: 'req-2', title: 'Title 1');

      expect(RequestDiscoverItem(req1), equals(RequestDiscoverItem(req2)));
      expect(RequestDiscoverItem(req1), isNot(equals(RequestDiscoverItem(req3))));
    });

    test('hashCode is based on request id', () {
      final req1 = Request(id: 'req-1');
      final req2 = Request(id: 'req-1');

      expect(RequestDiscoverItem(req1).hashCode, RequestDiscoverItem(req2).hashCode);
    });
  });

  group('DiscoverItem polymorphism', () {
    test('GearDiscoverItem is a DiscoverItem', () {
      final gear = CommunityGearItem(id: 'gear-1');
      final item = GearDiscoverItem(gear);

      expect(item, isA<DiscoverItem>());
    });

    test('RequestDiscoverItem is a DiscoverItem', () {
      final request = Request(id: 'req-1');
      final item = RequestDiscoverItem(request);

      expect(item, isA<DiscoverItem>());
    });

    test('can use common getters on DiscoverItem type', () {
      final gear = CommunityGearItem(
        id: 'gear-1',
        name: 'Gear Name',
        owner: User(id: 'owner-1', name: 'Owner'),
        mediaIds: ['media-1'],
      );
      final request = Request(
        id: 'req-1',
        title: 'Request Title',
        requester: User(id: 'req-1', name: 'Requester'),
        mediaIds: ['media-2'],
      );

      final List<DiscoverItem> items = [
        GearDiscoverItem(gear),
        RequestDiscoverItem(request),
      ];

      // All items can be accessed uniformly
      expect(items[0].id, 'gear-1');
      expect(items[0].name, 'Gear Name');
      expect(items[0].ownerName, 'Owner');
      expect(items[0].thumbnailMediaId, 'media-1');

      expect(items[1].id, 'req-1');
      expect(items[1].name, 'Request Title');
      expect(items[1].ownerName, 'Requester');
      expect(items[1].thumbnailMediaId, 'media-2');
    });
  });
}
