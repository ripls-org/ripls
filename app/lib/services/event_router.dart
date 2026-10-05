import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart';
import 'package:ripls/services/providers.dart';

final _log = Logger('EventRouter');

/// Identifies a cache invalidation provider so we can collect a set of
/// distinct providers to notify across a batch of events.
enum _InvalidationTarget {
  transfer,
  portfolio,
  content,
  search,
  feedStatus,
  feedListing,
  impact,
  // The two below clear a repository cache rather than only bumping a
  // notifier. Their view models re-read through the cache on rebuild, so
  // bumping alone would rebuild them onto the same stale list.
  communityList,
  deletedCommunities,
}

/// Routes community events from any source (poll, stream, push) to the
/// correct cache invalidation providers. Deduplicates by event ID so that
/// the same event received via multiple vectors triggers only one
/// invalidation cycle.
class EventRouter {
  final Ref _ref;
  final _processedEvents = <String>{}; // LinkedHashSet for insertion-order LRU
  static const _maxProcessedEvents = 500;

  EventRouter(this._ref);

  /// Route a batch of community events, notifying each affected provider
  /// exactly once regardless of how many events target it.
  ///
  /// Use this for poll results where multiple events arrive together.
  void routeCommunityEvents(List<CommunityEventItem> events) {
    final targets = <_InvalidationTarget>{};

    for (final event in events) {
      final eventTargets = _processEvent(event);
      if (eventTargets != null) {
        targets.addAll(eventTargets);
      }
    }

    _notifyTargets(targets);
  }

  /// Route a community event received via FCM push notification.
  ///
  /// Constructs a lightweight [CommunityEventItem] from the FCM data payload
  /// (which already contains event_id and event_type) and routes it through
  /// the standard dedup + invalidation path.
  void routeFcmEvent(Map<String, dynamic> data) {
    final eventId = data['event_id']?.toString() ?? '';
    final eventTypeStr = data['event_type']?.toString() ?? '';

    if (eventId.isEmpty || eventTypeStr.isEmpty) return;

    // Map the string event type to the enum value.
    final eventType = CommunityEventType.values.firstWhere(
      (t) => t.name == eventTypeStr,
      orElse: () => CommunityEventType.COMMUNITY_EVENT_TYPE_UNSPECIFIED,
    );
    if (eventType == CommunityEventType.COMMUNITY_EVENT_TYPE_UNSPECIFIED) {
      _log.warning('unknown FCM event type: $eventTypeStr');
      return;
    }

    routeCommunityEvent(CommunityEventItem(
      id: eventId,
      communityId: data['community_id']?.toString() ?? '',
      eventType: eventType,
    ));
  }

  /// Route a single community event to the correct invalidation providers.
  ///
  /// Safe to call from any source — deduplicates by event ID.
  void routeCommunityEvent(CommunityEventItem event) {
    final targets = _processEvent(event);
    if (targets != null) {
      _notifyTargets(targets);
    }
  }

  /// Whether a direct share was addressed to the viewer.
  ///
  /// Refreshing the community list is a network round trip, not a notifier
  /// bump, so every member must not pay for one person's share. A push carries
  /// no `object_user` — it is delivered only to the recipient — so an event
  /// without one is for whoever received it.
  bool _sharedWithThisViewer(CommunityEventItem event) {
    if (!event.hasObjectUser()) return true;
    final viewerId = _ref.read(authStateProvider).user?.id;
    if (viewerId == null || viewerId.isEmpty) return true;
    return event.objectUser.id == viewerId;
  }

  /// Process a single event: dedup, determine targets. Returns null if
  /// the event was already processed or has no ID.
  Set<_InvalidationTarget>? _processEvent(CommunityEventItem event) {
    if (event.id.isEmpty) return null;
    if (_processedEvents.contains(event.id)) return null;
    _trackProcessed(event.id);

    _log.fine('routing event ${event.eventType.name} (${event.id})');

    return switch (event.eventType) {
      // Transfer completion — also refreshes impact metrics.
      CommunityEventType.COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED => {
        _InvalidationTarget.transfer,
        _InvalidationTarget.portfolio,
        _InvalidationTarget.impact,
      },

      // Transfer state changes visible in portfolio (state transitions).
      CommunityEventType.COMMUNITY_EVENT_TYPE_TRANSFER_RECIPIENT_SELECTED ||
      CommunityEventType.COMMUNITY_EVENT_TYPE_TRANSFER_CANCELLED ||
      CommunityEventType.COMMUNITY_EVENT_TYPE_TRANSFER_ACTIVE => {
        _InvalidationTarget.transfer,
        _InvalidationTarget.portfolio,
      },

      // Transfer detail-level events (don't change portfolio listings).
      CommunityEventType.COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_EXPRESSED ||
      CommunityEventType.COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_WITHDRAWN ||
      CommunityEventType.COMMUNITY_EVENT_TYPE_TRANSFER_PICKUP_PROPOSED => {
        _InvalidationTarget.transfer,
      },

      // Request fulfillment — also refreshes impact metrics.
      CommunityEventType.COMMUNITY_EVENT_TYPE_REQUEST_FULFILLED => {
        _InvalidationTarget.content,
        _InvalidationTarget.search,
        _InvalidationTarget.feedStatus,
        _InvalidationTarget.feedListing,
        _InvalidationTarget.impact,
      },

      // Content events that change feed/search listings (items added/removed).
      CommunityEventType.COMMUNITY_EVENT_TYPE_GEAR_SHARED ||
      CommunityEventType.COMMUNITY_EVENT_TYPE_GEAR_UNSHARED ||
      CommunityEventType.COMMUNITY_EVENT_TYPE_REQUEST_CREATED ||
      CommunityEventType.COMMUNITY_EVENT_TYPE_REQUEST_CANCELLED ||
      CommunityEventType.COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED => {
        _InvalidationTarget.content,
        _InvalidationTarget.search,
        _InvalidationTarget.feedStatus,
        _InvalidationTarget.feedListing,
      },

      // Experience lifecycle transitions that change experience state.
      // STARTED: state changes to IN_PROCESS; CANCELLED: experience removed from feed.
      CommunityEventType.COMMUNITY_EVENT_TYPE_EXPERIENCE_STARTED => {
        _InvalidationTarget.content,
      },
      CommunityEventType.COMMUNITY_EVENT_TYPE_EXPERIENCE_CANCELLED => {
        _InvalidationTarget.content,
        _InvalidationTarget.feedStatus,
        _InvalidationTarget.feedListing,
      },

      // Request-offer detail events (don't change feed/search/portfolio listings).
      CommunityEventType.COMMUNITY_EVENT_TYPE_REQUEST_OFFER_MADE ||
      CommunityEventType.COMMUNITY_EVENT_TYPE_REQUEST_OFFER_SELECTED ||
      CommunityEventType.COMMUNITY_EVENT_TYPE_REQUEST_OFFER_WITHDRAWN => {
        _InvalidationTarget.content,
      },

      // Planning collaborative-list changes (needs/contributions on an
      // experience or request). Detail-level: they refresh the open content
      // view — the needs strip and per-person claim pills on a roster that is
      // already on screen (#2724) — without touching feed/search listings.
      CommunityEventType.COMMUNITY_EVENT_TYPE_PLANNING_NEED_ADDED ||
      CommunityEventType.COMMUNITY_EVENT_TYPE_PLANNING_NEED_REMOVED ||
      CommunityEventType.COMMUNITY_EVENT_TYPE_PLANNING_NEED_CLAIMED ||
      CommunityEventType.COMMUNITY_EVENT_TYPE_PLANNING_NEED_UPDATED ||
      CommunityEventType.COMMUNITY_EVENT_TYPE_PLANNING_CONTRIBUTION_ADDED ||
      CommunityEventType.COMMUNITY_EVENT_TYPE_PLANNING_CONTRIBUTION_REMOVED => {
        _InvalidationTarget.content,
      },

      // RSVP changes + host-driven roster changes (reset-to-invited, member
      // removed, sibling-community fan-out): refresh the open event detail AND
      // Home "Up next" — the attendee's calendar subtitle reflects their
      // going / maybe / invited state. (#2492)
      CommunityEventType.COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_YES ||
      CommunityEventType.COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_MAYBE ||
      CommunityEventType.COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_NO ||
      CommunityEventType.COMMUNITY_EVENT_TYPE_EXPERIENCE_ROSTER_CHANGED => {
        _InvalidationTarget.content,
        _InvalidationTarget.portfolio,
      },

      // Community membership events.
      CommunityEventType.COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED ||
      CommunityEventType.COMMUNITY_EVENT_TYPE_MEMBER_LEFT ||
      CommunityEventType.COMMUNITY_EVENT_TYPE_COMMUNITY_CREATED => {
        _InvalidationTarget.portfolio,
      },

      // Someone handed you an item directly (#3106). The share adds you to the
      // item's community, so both the item and that community are new to every
      // list that shows them — this arrives as you are added, not as something
      // changing in a community you were already browsing. For anyone else it
      // is nothing at all, and the per-user stream delivers it to every member.
      CommunityEventType.COMMUNITY_EVENT_TYPE_ITEM_SHARED_WITH_USER =>
        _sharedWithThisViewer(event)
            ? {
                _InvalidationTarget.communityList,
                _InvalidationTarget.portfolio,
                _InvalidationTarget.content,
                _InvalidationTarget.feedListing,
              }
            : <_InvalidationTarget>{},

      // A member rejoining within the 30-day window re-enters the community's
      // roster, so the viewer's community list changes. The rejoiner's own
      // rejoinable list is already invalidated by their rejoinCommunity call;
      // for everyone else it is about a list they cannot see.
      CommunityEventType.COMMUNITY_EVENT_TYPE_MEMBER_REJOINED_WITHIN_WINDOW => {
        _InvalidationTarget.communityList,
        _InvalidationTarget.portfolio,
      },

      // Community lifecycle. Deleting or restoring a community adds or removes
      // it — and everything shared into it — from every surface at once, which
      // is why these fan out wider than any other event.
      //
      // This is also what replaces the per-community stream terminating on
      // COMMUNITY_DELETED (#1658, removed in #2869). That stream ended so the
      // client would route away; a per-user stream cannot end without taking
      // every other community's realtime with it, so the signal has to be
      // handled rather than inferred from the disconnect.
      CommunityEventType.COMMUNITY_EVENT_TYPE_COMMUNITY_DELETED ||
      CommunityEventType.COMMUNITY_EVENT_TYPE_COMMUNITY_RESTORED => {
        _InvalidationTarget.communityList,
        _InvalidationTarget.deletedCommunities,
        _InvalidationTarget.portfolio,
        _InvalidationTarget.content,
        _InvalidationTarget.search,
        _InvalidationTarget.feedStatus,
        _InvalidationTarget.feedListing,
      },

      // Promote-by-naming: an ad-hoc community gaining a name becomes visible
      // in the directory and switcher, and its name appears on cards that
      // previously had none.
      CommunityEventType.COMMUNITY_EVENT_TYPE_COMMUNITY_NAMED => {
        _InvalidationTarget.communityList,
        _InvalidationTarget.portfolio,
        _InvalidationTarget.content,
        _InvalidationTarget.feedListing,
      },

      // Ownership handoff changes the owner shown on the community and who the
      // viewer sees owner-only affordances for.
      CommunityEventType.COMMUNITY_EVENT_TYPE_OWNERSHIP_TRANSFERRED => {
        _InvalidationTarget.communityList,
        _InvalidationTarget.portfolio,
        _InvalidationTarget.content,
      },

      _ => () {
        _log.warning('unhandled event type: ${event.eventType}');
        return <_InvalidationTarget>{};
      }(),
    };
  }

  void _notifyTargets(Set<_InvalidationTarget> targets) {
    for (final target in targets) {
      switch (target) {
        case _InvalidationTarget.transfer:
          _ref.read(transferCacheInvalidationProvider.notifier).notify();
        case _InvalidationTarget.portfolio:
          _ref.read(portfolioCacheInvalidationProvider.notifier).notify();
        case _InvalidationTarget.content:
          _ref.read(contentCacheInvalidationProvider.notifier).notify();
        case _InvalidationTarget.search:
          _ref.read(searchCacheInvalidationProvider.notifier).notify();
        case _InvalidationTarget.feedStatus:
          _ref.read(feedStatusCacheInvalidationProvider.notifier).notify();
        case _InvalidationTarget.feedListing:
          _ref.read(feedListingCacheInvalidationProvider.notifier).notify();
        case _InvalidationTarget.impact:
          _ref.read(impactCacheInvalidationProvider.notifier).notify();
        case _InvalidationTarget.communityList:
          unawaited(_invalidate(
            'communityList',
            () => _ref.read(communityRepositoryProvider).refreshUserCommunities(),
          ));
        case _InvalidationTarget.deletedCommunities:
          unawaited(_invalidate(
            'deletedCommunities',
            () => _ref.read(communityRepositoryProvider).invalidateDeletedList(),
          ));
      }
    }
  }

  /// Runs a repository cache invalidation off the routing path.
  ///
  /// Routing is synchronous and fire-and-forget from three call sites (stream,
  /// poll, push), so a repository round-trip cannot be awaited here. A failure
  /// is logged rather than thrown: the caches converge on the next poll tick
  /// anyway, and an exception escaping a stream's onData would tear down the
  /// subscription over a stale list.
  Future<void> _invalidate(String label, Future<void> Function() run) async {
    try {
      await run();
    } catch (e) {
      _log.warning('failed to invalidate $label after community event: $e');
    }
  }

  void _trackProcessed(String eventId) {
    _processedEvents.add(eventId);
    while (_processedEvents.length > _maxProcessedEvents) {
      _processedEvents.remove(_processedEvents.first);
    }
  }
}
