import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/observability/events.dart';

void main() {
  group('AnalyticsEvent', () {
    group('Screen View Events', () {
      test('FeedViewedEvent has correct name and parameters', () {
        final event = FeedViewedEvent(communityId: 'comm123');

        expect(event.name, equals('feed_viewed'));
        expect(event.parameters, equals({'community_id': 'comm123'}));
      });

      test('DiscoverViewedEvent has correct name and empty parameters', () {
        final event = DiscoverViewedEvent();

        expect(event.name, equals('discover_viewed'));
        expect(event.parameters, isEmpty);
      });

      test('InboxViewedEvent has correct name and parameters', () {
        final event = InboxViewedEvent(conversationCount: 5);

        expect(event.name, equals('inbox_viewed'));
        expect(event.parameters, equals({'conversation_count': 5}));
      });

    });

    group('Gear Events', () {
      test('GearCreatedEvent has correct name and parameters', () {
        final event = GearCreatedEvent(hasLocation: true);

        expect(event.name, equals('gear_created'));
        expect(event.parameters, equals({'has_location': true}));
      });

      test('GearEditedEvent has correct name and parameters', () {
        final event = GearEditedEvent(gearId: 'gear123');

        expect(event.name, equals('gear_edited'));
        expect(event.parameters, equals({'gear_id': 'gear123'}));
      });

      test('GearDeletedEvent has correct name and parameters', () {
        final event = GearDeletedEvent(gearId: 'gear456');

        expect(event.name, equals('gear_deleted'));
        expect(event.parameters, equals({'gear_id': 'gear456'}));
      });

      test('GearSharedEvent has correct name and parameters', () {
        final event = GearSharedEvent(
          gearId: 'gear789',
          shareType: 'loan',
          communityId: 'comm123',
        );

        expect(event.name, equals('gear_shared'));
        expect(event.parameters, equals({
          'gear_id': 'gear789',
          'share_type': 'loan',
          'community_id': 'comm123',
        }));
      });

      test('GearDetailViewedEvent has correct name and parameters', () {
        final event = GearDetailViewedEvent(gearId: 'gear123', source: 'feed');

        expect(event.name, equals('gear_detail_viewed'));
        expect(event.parameters, equals({
          'gear_id': 'gear123',
          'source': 'feed',
        }));
      });
    });

    group('Transfer Events', () {
      test('TransferInterestEvent has correct name and parameters', () {
        final event = TransferInterestEvent(gearId: 'gear123', transferType: 'loan');

        expect(event.name, equals('transfer_interest'));
        expect(event.parameters, equals({
          'gear_id': 'gear123',
          'transfer_type': 'loan',
        }));
      });

      test('TransferApprovedEvent has correct name and parameters', () {
        final event = TransferApprovedEvent(transferId: 'trans123', transferType: 'giveaway');

        expect(event.name, equals('transfer_approved'));
        expect(event.parameters, equals({
          'transfer_id': 'trans123',
          'transfer_type': 'giveaway',
        }));
      });

      test('TransferStartedEvent has correct name and parameters', () {
        final event = TransferStartedEvent(transferId: 'trans789');

        expect(event.name, equals('transfer_started'));
        expect(event.parameters, equals({'transfer_id': 'trans789'}));
      });

      test('TransferCompletedEvent has correct name and parameters for loan', () {
        final event = TransferCompletedEvent(
          transferId: 'trans101',
          transferType: 'loan',
          durationDays: 7,
        );

        expect(event.name, equals('transfer_completed'));
        expect(event.parameters, equals({
          'transfer_id': 'trans101',
          'transfer_type': 'loan',
          'duration_days': 7,
        }));
      });

      test('TransferCompletedEvent omits durationDays when null', () {
        final event = TransferCompletedEvent(
          transferId: 'trans102',
          transferType: 'giveaway',
        );

        expect(event.name, equals('transfer_completed'));
        expect(event.parameters, equals({
          'transfer_id': 'trans102',
          'transfer_type': 'giveaway',
        }));
        expect(event.parameters.containsKey('duration_days'), isFalse);
      });

      test('TransferCancelledEvent has correct name and parameters', () {
        final event = TransferCancelledEvent(
          transferId: 'trans103',
          transferType: 'loan',
          cancelledBy: 'owner',
        );

        expect(event.name, equals('transfer_cancelled'));
        expect(event.parameters, equals({
          'transfer_id': 'trans103',
          'transfer_type': 'loan',
          'cancelled_by': 'owner',
        }));
      });
    });

    group('Request Events', () {
      test('RequestCreatedEvent has correct name and parameters', () {
        final event = RequestCreatedEvent(communityId: 'comm123', hasLocation: true);

        expect(event.name, equals('request_created'));
        expect(event.parameters, equals({
          'community_id': 'comm123',
          'has_location': true,
        }));
      });

      test('RequestEditedEvent has correct name and parameters', () {
        final event = RequestEditedEvent(requestId: 'req123');

        expect(event.name, equals('request_edited'));
        expect(event.parameters, equals({'request_id': 'req123'}));
      });

      test('RequestDetailViewedEvent has correct name and parameters', () {
        final event = RequestDetailViewedEvent(requestId: 'req456', source: 'search');

        expect(event.name, equals('request_detail_viewed'));
        expect(event.parameters, equals({
          'request_id': 'req456',
          'source': 'search',
        }));
      });

      test('RequestOfferSentEvent has correct name and parameters', () {
        final event = RequestOfferSentEvent(requestId: 'req789');

        expect(event.name, equals('request_offer_sent'));
        expect(event.parameters, equals({'request_id': 'req789'}));
      });

      test('RequestFulfilledEvent has correct name and parameters', () {
        final event = RequestFulfilledEvent(requestId: 'req101');

        expect(event.name, equals('request_fulfilled'));
        expect(event.parameters, equals({'request_id': 'req101'}));
      });

      test('RequestCancelledEvent has correct name and parameters', () {
        final event = RequestCancelledEvent(requestId: 'req102');

        expect(event.name, equals('request_cancelled'));
        expect(event.parameters, equals({'request_id': 'req102'}));
      });
    });

    group('Experience Events', () {
      test('ExperienceCreatedEvent has correct name and parameters', () {
        final event = ExperienceCreatedEvent(communityId: 'comm123');

        expect(event.name, equals('experience_created'));
        expect(event.parameters, equals({'community_id': 'comm123'}));
      });

      test('ExperienceEditedEvent has correct name and parameters', () {
        final event = ExperienceEditedEvent(experienceId: 'exp123');

        expect(event.name, equals('experience_edited'));
        expect(event.parameters, equals({'experience_id': 'exp123'}));
      });

      test('ExperienceDetailViewedEvent has correct name and parameters', () {
        final event = ExperienceDetailViewedEvent(experienceId: 'exp456', source: 'feed');

        expect(event.name, equals('experience_detail_viewed'));
        expect(event.parameters, equals({
          'experience_id': 'exp456',
          'source': 'feed',
        }));
      });

      test('ExperienceRsvpEvent has correct name and parameters', () {
        final event = ExperienceRsvpEvent(experienceId: 'exp789', rsvpStatus: 'going');

        expect(event.name, equals('experience_rsvp'));
        expect(event.parameters, equals({
          'experience_id': 'exp789',
          'rsvp_status': 'going',
        }));
      });
    });

    group('Conversation/Inbox Events', () {
      test('ConversationOpenedEvent has correct name and parameters', () {
        final event = ConversationOpenedEvent(contentType: 'transfer');

        expect(event.name, equals('conversation_opened'));
        expect(event.parameters, equals({'content_type': 'transfer'}));
      });

      test('MessageSentEvent has correct name and parameters', () {
        final event = MessageSentEvent(contentType: 'request');

        expect(event.name, equals('message_sent'));
        expect(event.parameters, equals({'content_type': 'request'}));
      });
    });

    group('Search/Discover Events', () {
      test('SearchPerformedEvent has correct name and parameters', () {
        final event = SearchPerformedEvent(
          queryLength: 10,
          resultCount: 5,
          searchType: 'gear',
        );

        expect(event.name, equals('search_performed'));
        expect(event.parameters, equals({
          'query_length': 10,
          'result_count': 5,
          'search_type': 'gear',
        }));
      });

      test('SearchResultTappedEvent has correct name and parameters', () {
        final event = SearchResultTappedEvent(resultType: 'community', position: 2);

        expect(event.name, equals('search_result_tapped'));
        expect(event.parameters, equals({
          'result_type': 'community',
          'position': 2,
        }));
      });
    });

    group('Community Events', () {
      test('CommunityCreatedEvent has correct name and empty parameters', () {
        final event = CommunityCreatedEvent();

        expect(event.name, equals('community_created'));
        expect(event.parameters, isEmpty);
      });

      test('CommunityJoinedEvent has correct name and parameters', () {
        final event = CommunityJoinedEvent(communityId: 'comm123', joinMethod: 'invite_link');

        expect(event.name, equals('community_joined'));
        expect(event.parameters, equals({
          'community_id': 'comm123',
          'join_method': 'invite_link',
        }));
      });

      test('CommunityLeftEvent has correct name and parameters', () {
        final event = CommunityLeftEvent(communityId: 'comm456');

        expect(event.name, equals('community_left'));
        expect(event.parameters, equals({'community_id': 'comm456'}));
      });

      test('CommunityInviteSharedEvent has correct name and parameters', () {
        final event = CommunityInviteSharedEvent(communityId: 'comm789');

        expect(event.name, equals('community_invite_shared'));
        expect(event.parameters, equals({'community_id': 'comm789'}));
      });
    });

    group('Authentication Events', () {
      test('LoginSuccessEvent has correct name and parameters', () {
        final event = LoginSuccessEvent(method: 'apple');

        expect(event.name, equals('login_success'));
        expect(event.parameters, equals({'method': 'apple'}));
      });

      test('LoginFailedEvent has correct name and parameters', () {
        final event = LoginFailedEvent(method: 'google', errorCode: 'invalid_token');

        expect(event.name, equals('login_failed'));
        expect(event.parameters, equals({
          'method': 'google',
          'error_code': 'invalid_token',
        }));
      });

      test('LogoutEvent has correct name and empty parameters', () {
        final event = LogoutEvent();

        expect(event.name, equals('logout'));
        expect(event.parameters, isEmpty);
      });

      test('SignUpSuccessEvent has correct name and parameters', () {
        final event = SignUpSuccessEvent(method: 'email');

        expect(event.name, equals('sign_up_success'));
        expect(event.parameters, equals({'method': 'email'}));
      });
    });

    group('Notification deep-link diagnostics (#2636)', () {
      test('route phase serializes the decision fields', () {
        final event = NotificationDeepLinkEvent(
          phase: 'route',
          source: 'initial',
          entity: 'experience',
          route: '/experience/exp-1',
          dataKeys: 'community_id,experience_id',
          noEntityId: false,
          onNavigateNull: false,
        );

        expect(event.name, equals('notification_deeplink'));
        expect(event.parameters, equals({
          'phase': 'route',
          'source': 'initial',
          'entity': 'experience',
          'route': '/experience/exp-1',
          'data_keys': 'community_id,experience_id',
          'no_entity_id': false,
          'on_navigate_null': false,
        }));
      });

      test('nav_result phase reports where navigation actually landed', () {
        final event = NotificationDeepLinkEvent(
          phase: 'nav_result',
          source: 'nav',
          route: '/experience/exp-1',
          actualLocation: '/',
          landed: false,
        );

        expect(event.parameters, equals({
          'phase': 'nav_result',
          'source': 'nav',
          'route': '/experience/exp-1',
          'actual_location': '/',
          'landed': false,
        }));
      });

      test('omits null fields so each phase carries only its own keys', () {
        final event = NotificationDeepLinkEvent(phase: 'route', source: 'local');

        expect(event.parameters, equals({'phase': 'route', 'source': 'local'}));
      });

      test('nav_result carries the drain trigger', () {
        final event = NotificationDeepLinkEvent(
          phase: 'nav_result',
          source: 'nav',
          route: '/experience/exp-1',
          actualLocation: '/experience/exp-1',
          landed: true,
          trigger: 'ready',
        );

        expect(event.parameters, equals({
          'phase': 'nav_result',
          'source': 'nav',
          'route': '/experience/exp-1',
          'actual_location': '/experience/exp-1',
          'landed': true,
          'trigger': 'ready',
        }));
      });

      test('a recovered replay flags recovered:true', () {
        final event = NotificationDeepLinkEvent(
          phase: 'nav_result',
          source: 'nav',
          route: '/experience/exp-1',
          actualLocation: '/experience/exp-1',
          landed: true,
          trigger: 'replay',
          recovered: true,
        );

        expect(event.parameters['trigger'], 'replay');
        expect(event.parameters['recovered'], true);
      });

      test('a permanent drop flags recovered:false (the give-up signal)', () {
        final event = NotificationDeepLinkEvent(
          phase: 'nav_result',
          source: 'nav',
          route: '/experience/exp-1',
          actualLocation: '/',
          landed: false,
          trigger: 'replay',
          recovered: false,
        );

        expect(event.parameters['landed'], false);
        expect(event.parameters['recovered'], false);
      });
    });
  });
}
