import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/observability/events.dart';
import 'package:ripls/services/fcm_service.dart';

/// Tests for notification payload routing logic.
///
/// Verifies that notifications route to the correct item detail screen based on
/// entity IDs, with ?tab=chat appended for chat message notifications, and
/// that community_id is passed through for community context switching.
void main() {
  late List<String> navigateRoutes;
  late List<String?> navigateCommunityIds;

  setUp(() {
    navigateRoutes = [];
    navigateCommunityIds = [];
  });

  void route(Map<String, dynamic> data) {
    routeNotificationPayload(
      data: data,
      onNavigate: (route, {String? communityId}) {
        navigateRoutes.add(route);
        navigateCommunityIds.add(communityId);
      },
    );
  }

  group('routeNotificationPayload', () {
    test('gear with conversation_id appends tab=chat', () {
      route({
        'type': 'chat_message',
        'gear_id': 'gear-123',
        'conversation_id': 'conv-456',
      });

      expect(navigateRoutes, ['/gear/gear-123?tab=chat']);
    });

    test('gear without conversation_id has no tab param', () {
      route({
        'type': 'community_event',
        'gear_id': 'gear-123',
      });

      expect(navigateRoutes, ['/gear/gear-123']);
    });

    test('experience with conversation_id appends tab=chat', () {
      route({
        'experience_id': 'exp-1',
        'conversation_id': 'conv-1',
      });

      expect(navigateRoutes, ['/experience/exp-1?tab=chat']);
    });

    test('experience without conversation_id has no tab param', () {
      route({
        'type': 'community_event',
        'experience_id': 'exp-123',
      });

      expect(navigateRoutes, ['/experience/exp-123']);
    });

    test('request_id with conversation_id appends tab=chat', () {
      route({
        'type': 'chat_message',
        'request_id': 'req-123',
        'conversation_id': 'conv-456',
      });

      expect(navigateRoutes, ['/request/req-123?tab=chat']);
    });

    test('request_id without conversation_id has no tab param', () {
      route({
        'type': 'community_event',
        'request_id': 'req-123',
      });

      expect(navigateRoutes, ['/request/req-123']);
    });

    test('empty data does not call callback', () {
      route({});

      expect(navigateRoutes, isEmpty);
    });

    test('conversation_id + community_id with no entity lands on the community',
        () {
      // No gear/experience/request → the community itself, not home. Home
      // makes the reader hunt for the thing they were just told about (#2876).
      route({
        'conversation_id': 'conv-999',
        'community_id': 'comm-111',
      });

      expect(navigateRoutes, ['/group/comm-111']);
      expect(navigateCommunityIds, ['comm-111']);
    });

    test('member joined lands in the community discussion', () {
      // The notification says "say hi"; a tap that lands anywhere but the
      // conversation breaks that promise (#2876).
      route({
        'type': 'community_event',
        'event_type': 'COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED',
        'community_id': 'comm-111',
      });

      expect(navigateRoutes, ['/group/comm-111?tab=discuss']);
      expect(navigateCommunityIds, ['comm-111']);
    });

    test('other community-scoped events land on the community, not the chat',
        () {
      // A deletion notice does not belong in a chat pane.
      for (final eventType in [
        'COMMUNITY_EVENT_TYPE_COMMUNITY_DELETED',
        'COMMUNITY_EVENT_TYPE_COMMUNITY_RESTORED',
        'COMMUNITY_EVENT_TYPE_OWNERSHIP_TRANSFERRED',
      ]) {
        navigateRoutes.clear();
        route({
          'type': 'community_event',
          'event_type': eventType,
          'community_id': 'comm-111',
        });
        expect(navigateRoutes, ['/group/comm-111'], reason: eventType);
      }
    });

    test('the discussion tab needs the community, not just the event type', () {
      // An INVITATION_LINK_USED payload that somehow lost its community_id has
      // nowhere to land; it must not synthesize a route.
      route({
        'type': 'community_event',
        'event_type': 'COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED',
      });

      expect(navigateRoutes, isEmpty);
    });

    test('gear_id takes priority over experience_id and request_id', () {
      route({
        'gear_id': 'gear-1',
        'experience_id': 'exp-1',
        'request_id': 'req-1',
        'conversation_id': 'conv-1',
      });

      expect(navigateRoutes, ['/gear/gear-1?tab=chat']);
    });

    test('experience_id takes priority over request_id', () {
      route({
        'experience_id': 'exp-1',
        'request_id': 'req-1',
      });

      expect(navigateRoutes, ['/experience/exp-1']);
    });

    test('scheduled EXPERIENCE_REMINDER deep-links to the event detail screen', () {
      // Server-side payload from
      // server/jobs/scheduled_notifications/experience_reminders.go.
      // The router branches on entity IDs, not event_type, so the
      // existing experience route handles scheduled reminders
      // without client changes.
      route({
        'type': 'community_event',
        'event_type': 'EXPERIENCE_REMINDER',
        'experience_id': 'exp-1',
        'community_id': 'comm-1',
      });

      expect(navigateRoutes, ['/experience/exp-1']);
      expect(navigateCommunityIds, ['comm-1']);
    });

    test('scheduled EXPERIENCE_CLOSE_PROMPT deep-links to the event detail screen', () {
      // Close-prompt rows are user-scoped (no community_id by
      // design); the router falls through to a null communityId.
      route({
        'type': 'community_event',
        'event_type': 'EXPERIENCE_CLOSE_PROMPT',
        'experience_id': 'exp-2',
      });

      expect(navigateRoutes, ['/experience/exp-2']);
      expect(navigateCommunityIds, [null]);
    });

    test('scheduled LOAN_RETURN_REMINDER deep-links to the gear detail screen', () {
      // Server-side payload from
      // server/jobs/scheduled_notifications/loan_return_reminders.go.
      // gear_id routes to the gear detail screen, where the
      // borrower can tap "Mark returned".
      route({
        'type': 'community_event',
        'event_type': 'LOAN_RETURN_REMINDER',
        'gear_id': 'gear-99',
        'community_id': 'comm-1',
      });

      expect(navigateRoutes, ['/gear/gear-99']);
      expect(navigateCommunityIds, ['comm-1']);
    });
  });

  group('community_id passthrough', () {
    test('passes community_id when present', () {
      route({
        'gear_id': 'gear-1',
        'community_id': 'comm-abc',
      });

      expect(navigateRoutes, ['/gear/gear-1']);
      expect(navigateCommunityIds, ['comm-abc']);
    });

    test('passes null communityId when not present', () {
      route({
        'gear_id': 'gear-1',
      });

      expect(navigateRoutes, ['/gear/gear-1']);
      expect(navigateCommunityIds, [null]);
    });

    test('passes null communityId when empty string', () {
      route({
        'gear_id': 'gear-1',
        'community_id': '',
      });

      expect(navigateRoutes, ['/gear/gear-1']);
      expect(navigateCommunityIds, [null]);
    });

    test('passes community_id with chat tab for experience', () {
      route({
        'experience_id': 'exp-1',
        'conversation_id': 'conv-1',
        'community_id': 'comm-xyz',
      });

      expect(navigateRoutes, ['/experience/exp-1?tab=chat']);
      expect(navigateCommunityIds, ['comm-xyz']);
    });

    test('passes community_id for request', () {
      route({
        'request_id': 'req-1',
        'community_id': 'comm-123',
      });

      expect(navigateRoutes, ['/request/req-1']);
      expect(navigateCommunityIds, ['comm-123']);
    });

    test('routes community-only payload to that community', () {
      // A payload carrying only community_id lands on the community itself,
      // with it still passed through for context switching (#2876).
      route({
        'community_id': 'comm-orphan',
      });

      expect(navigateRoutes, ['/group/comm-orphan']);
      expect(navigateCommunityIds, ['comm-orphan']);
    });
  });

  // Deep-link diagnostics (#2636): every route decision emits one queryable
  // NotificationDeepLinkEvent so a dropped tap is visible in prod.
  group('deep-link telemetry', () {
    late List<AnalyticsEvent> events;

    setUp(() => events = []);

    /// Routes with a telemetry sink + source; onNavigate optional so we can
    /// exercise the "callback unwired" drop path.
    void routeT(
      Map<String, dynamic> data, {
      String source = 'opened',
      bool withNavigate = true,
    }) {
      routeNotificationPayload(
        data: data,
        source: source,
        onNavigate: withNavigate ? (r, {String? communityId}) {} : null,
        logAnalyticsEvent: events.add,
      );
    }

    Map<String, dynamic> only(AnalyticsEvent e) {
      expect(e, isA<NotificationDeepLinkEvent>());
      expect(e.name, 'notification_deeplink');
      return e.parameters;
    }

    test('experience route emits a resolved route event', () {
      routeT({'experience_id': 'exp-1', 'community_id': 'c1'}, source: 'initial');

      expect(events, hasLength(1));
      final p = only(events.single);
      expect(p['phase'], 'route');
      expect(p['source'], 'initial');
      expect(p['entity'], 'experience');
      expect(p['route'], '/experience/exp-1');
      expect(p['no_entity_id'], false);
      expect(p['on_navigate_null'], false);
    });

    test('data_keys are keys-only and sorted (no values / PII)', () {
      routeT({
        'experience_id': 'exp-1',
        'community_id': 'c1',
        'event_type': 'EXPERIENCE_RSVP_YES',
      });

      final p = only(events.single);
      expect(p['data_keys'], 'community_id,event_type,experience_id');
    });

    test('no entity id flags no_entity_id (the primary drop cause)', () {
      routeT({'event_type': 'SOMETHING', 'unrelated': 'x'});

      final p = only(events.single);
      expect(p['entity'], 'none');
      expect(p['no_entity_id'], true);
      expect(p.containsKey('route'), isFalse);
    });

    test('resolved route but null callback flags on_navigate_null', () {
      routeT({'experience_id': 'exp-1'}, withNavigate: false);

      final p = only(events.single);
      expect(p['entity'], 'experience');
      expect(p['route'], '/experience/exp-1');
      expect(p['on_navigate_null'], true);
    });

    test('empty data still emits a drop event with the source', () {
      routeT({}, source: 'missed');

      final p = only(events.single);
      expect(p['source'], 'missed');
      expect(p['no_entity_id'], true);
      expect(p['data_keys'], '');
    });
  });
}
