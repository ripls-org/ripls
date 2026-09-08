// Standard analytics events for the Ripls application.
//
// Events are organized around the app's core concepts:
// - Transfers: Loans and giveaways of gear between users
// - Requests: Help requests posted by users seeking assistance
// - Experiences: Events and activities organized by community members
// - Gear: Equipment items that can be shared
// - Key views: Feed, Discover, Inbox

/// Base class for all analytics events.
abstract class AnalyticsEvent {
  /// The event name in snake_case format.
  String get name;

  /// The event parameters as a map.
  Map<String, dynamic> get parameters;
}

// =============================================================================
// Screen View Events
// =============================================================================

/// User viewed the community feed.
class FeedViewedEvent implements AnalyticsEvent {
  final String communityId;

  FeedViewedEvent({required this.communityId});

  @override
  String get name => 'feed_viewed';

  @override
  Map<String, dynamic> get parameters => {'community_id': communityId};
}

/// User viewed the discover/search screen.
class DiscoverViewedEvent implements AnalyticsEvent {
  @override
  String get name => 'discover_viewed';

  @override
  Map<String, dynamic> get parameters => {};
}

/// User viewed their inbox.
class InboxViewedEvent implements AnalyticsEvent {
  final int conversationCount;

  InboxViewedEvent({required this.conversationCount});

  @override
  String get name => 'inbox_viewed';

  @override
  Map<String, dynamic> get parameters => {
        'conversation_count': conversationCount,
      };
}

// =============================================================================
// Gear Events
// =============================================================================

/// User created a new gear item (via camera + AI).
class GearCreatedEvent implements AnalyticsEvent {
  final bool hasLocation;

  GearCreatedEvent({required this.hasLocation});

  @override
  String get name => 'gear_created';

  @override
  Map<String, dynamic> get parameters => {'has_location': hasLocation};
}

/// User edited an existing gear item.
class GearEditedEvent implements AnalyticsEvent {
  final String gearId;

  GearEditedEvent({required this.gearId});

  @override
  String get name => 'gear_edited';

  @override
  Map<String, dynamic> get parameters => {'gear_id': gearId};
}

/// User deleted a gear item.
class GearDeletedEvent implements AnalyticsEvent {
  final String gearId;

  GearDeletedEvent({required this.gearId});

  @override
  String get name => 'gear_deleted';

  @override
  Map<String, dynamic> get parameters => {'gear_id': gearId};
}

/// User shared gear to community (made available for loan/giveaway).
class GearSharedEvent implements AnalyticsEvent {
  final String gearId;
  final String shareType; // 'loan' or 'giveaway'
  final String communityId;

  GearSharedEvent({
    required this.gearId,
    required this.shareType,
    required this.communityId,
  });

  @override
  String get name => 'gear_shared';

  @override
  Map<String, dynamic> get parameters => {
        'gear_id': gearId,
        'share_type': shareType,
        'community_id': communityId,
      };
}

/// User viewed gear detail screen.
class GearDetailViewedEvent implements AnalyticsEvent {
  final String gearId;
  final String source; // 'feed', 'search', 'profile', etc.

  GearDetailViewedEvent({required this.gearId, required this.source});

  @override
  String get name => 'gear_detail_viewed';

  @override
  Map<String, dynamic> get parameters => {
        'gear_id': gearId,
        'source': source,
      };
}

// =============================================================================
// Transfer Events (Loans & Giveaways)
// =============================================================================

/// User expressed interest in a transfer (wants to borrow/receive).
class TransferInterestEvent implements AnalyticsEvent {
  final String gearId;
  final String transferType; // 'loan' or 'giveaway'

  TransferInterestEvent({required this.gearId, required this.transferType});

  @override
  String get name => 'transfer_interest';

  @override
  Map<String, dynamic> get parameters => {
        'gear_id': gearId,
        'transfer_type': transferType,
      };
}

/// Owner approved a recipient for transfer.
class TransferApprovedEvent implements AnalyticsEvent {
  final String transferId;
  final String transferType;

  TransferApprovedEvent({required this.transferId, required this.transferType});

  @override
  String get name => 'transfer_approved';

  @override
  Map<String, dynamic> get parameters => {
        'transfer_id': transferId,
        'transfer_type': transferType,
      };
}

/// Loan started (item handed over to borrower).
class TransferStartedEvent implements AnalyticsEvent {
  final String transferId;

  TransferStartedEvent({required this.transferId});

  @override
  String get name => 'transfer_started';

  @override
  Map<String, dynamic> get parameters => {'transfer_id': transferId};
}

/// Transfer completed (loan returned or giveaway finalized).
class TransferCompletedEvent implements AnalyticsEvent {
  final String transferId;
  final String transferType;
  final int? durationDays; // For loans only

  TransferCompletedEvent({
    required this.transferId,
    required this.transferType,
    this.durationDays,
  });

  @override
  String get name => 'transfer_completed';

  @override
  Map<String, dynamic> get parameters {
    final params = <String, dynamic>{
      'transfer_id': transferId,
      'transfer_type': transferType,
    };
    if (durationDays != null) {
      params['duration_days'] = durationDays;
    }
    return params;
  }
}

/// Transfer was cancelled.
class TransferCancelledEvent implements AnalyticsEvent {
  final String transferId;
  final String transferType;
  final String cancelledBy; // 'owner' or 'recipient'

  TransferCancelledEvent({
    required this.transferId,
    required this.transferType,
    required this.cancelledBy,
  });

  @override
  String get name => 'transfer_cancelled';

  @override
  Map<String, dynamic> get parameters => {
        'transfer_id': transferId,
        'transfer_type': transferType,
        'cancelled_by': cancelledBy,
      };
}

// =============================================================================
// Request Events (Help Requests)
// =============================================================================

/// User created a help request.
class RequestCreatedEvent implements AnalyticsEvent {
  final String communityId;
  final bool hasLocation;

  RequestCreatedEvent({required this.communityId, required this.hasLocation});

  @override
  String get name => 'request_created';

  @override
  Map<String, dynamic> get parameters => {
        'community_id': communityId,
        'has_location': hasLocation,
      };
}

/// User edited a request.
class RequestEditedEvent implements AnalyticsEvent {
  final String requestId;

  RequestEditedEvent({required this.requestId});

  @override
  String get name => 'request_edited';

  @override
  Map<String, dynamic> get parameters => {'request_id': requestId};
}

/// User viewed a request detail screen.
class RequestDetailViewedEvent implements AnalyticsEvent {
  final String requestId;
  final String source;

  RequestDetailViewedEvent({required this.requestId, required this.source});

  @override
  String get name => 'request_detail_viewed';

  @override
  Map<String, dynamic> get parameters => {
        'request_id': requestId,
        'source': source,
      };
}

/// Someone offered to help with a request.
class RequestOfferSentEvent implements AnalyticsEvent {
  final String requestId;

  RequestOfferSentEvent({required this.requestId});

  @override
  String get name => 'request_offer_sent';

  @override
  Map<String, dynamic> get parameters => {'request_id': requestId};
}

/// Request was marked as fulfilled by the requester.
class RequestFulfilledEvent implements AnalyticsEvent {
  final String requestId;

  RequestFulfilledEvent({required this.requestId});

  @override
  String get name => 'request_fulfilled';

  @override
  Map<String, dynamic> get parameters => {'request_id': requestId};
}

/// Request was cancelled.
class RequestCancelledEvent implements AnalyticsEvent {
  final String requestId;

  RequestCancelledEvent({required this.requestId});

  @override
  String get name => 'request_cancelled';

  @override
  Map<String, dynamic> get parameters => {'request_id': requestId};
}

/// Request was deleted.
class RequestDeletedEvent implements AnalyticsEvent {
  final String requestId;

  RequestDeletedEvent({required this.requestId});

  @override
  String get name => 'request_deleted';

  @override
  Map<String, dynamic> get parameters => {'request_id': requestId};
}

// =============================================================================
// Experience Events
// =============================================================================

/// User created an experience/event.
class ExperienceCreatedEvent implements AnalyticsEvent {
  final String communityId;

  ExperienceCreatedEvent({required this.communityId});

  @override
  String get name => 'experience_created';

  @override
  Map<String, dynamic> get parameters => {'community_id': communityId};
}

/// User edited an experience.
class ExperienceEditedEvent implements AnalyticsEvent {
  final String experienceId;

  ExperienceEditedEvent({required this.experienceId});

  @override
  String get name => 'experience_edited';

  @override
  Map<String, dynamic> get parameters => {'experience_id': experienceId};
}

/// User viewed an experience detail screen.
class ExperienceDetailViewedEvent implements AnalyticsEvent {
  final String experienceId;
  final String source;

  ExperienceDetailViewedEvent({required this.experienceId, required this.source});

  @override
  String get name => 'experience_detail_viewed';

  @override
  Map<String, dynamic> get parameters => {
        'experience_id': experienceId,
        'source': source,
      };
}

/// User RSVPed to an experience.
class ExperienceRsvpEvent implements AnalyticsEvent {
  final String experienceId;
  final String rsvpStatus; // 'going', 'interested', 'not_going'

  ExperienceRsvpEvent({required this.experienceId, required this.rsvpStatus});

  @override
  String get name => 'experience_rsvp';

  @override
  Map<String, dynamic> get parameters => {
        'experience_id': experienceId,
        'rsvp_status': rsvpStatus,
      };
}

// =============================================================================
// Conversation/Inbox Events
// =============================================================================

/// User opened a conversation.
class ConversationOpenedEvent implements AnalyticsEvent {
  final String contentType; // 'transfer' or 'request'

  ConversationOpenedEvent({required this.contentType});

  @override
  String get name => 'conversation_opened';

  @override
  Map<String, dynamic> get parameters => {'content_type': contentType};
}

/// User sent a message in a conversation.
class MessageSentEvent implements AnalyticsEvent {
  final String contentType;

  MessageSentEvent({required this.contentType});

  @override
  String get name => 'message_sent';

  @override
  Map<String, dynamic> get parameters => {'content_type': contentType};
}

// =============================================================================
// Search/Discover Events
// =============================================================================

/// User performed a search.
class SearchPerformedEvent implements AnalyticsEvent {
  final int queryLength;
  final int resultCount;
  final String searchType; // 'gear', 'community', 'all'

  SearchPerformedEvent({
    required this.queryLength,
    required this.resultCount,
    required this.searchType,
  });

  @override
  String get name => 'search_performed';

  @override
  Map<String, dynamic> get parameters => {
        'query_length': queryLength,
        'result_count': resultCount,
        'search_type': searchType,
      };
}

/// User tapped a search result.
class SearchResultTappedEvent implements AnalyticsEvent {
  final String resultType; // 'gear', 'community', 'user'
  final int position;

  SearchResultTappedEvent({required this.resultType, required this.position});

  @override
  String get name => 'search_result_tapped';

  @override
  Map<String, dynamic> get parameters => {
        'result_type': resultType,
        'position': position,
      };
}

// =============================================================================
// Community Events
// =============================================================================

/// User created a new community.
class CommunityCreatedEvent implements AnalyticsEvent {
  @override
  String get name => 'community_created';

  @override
  Map<String, dynamic> get parameters => {};
}

/// User joined a community via invite link.
class CommunityJoinedEvent implements AnalyticsEvent {
  final String communityId;
  final String joinMethod; // 'invite_link', 'request'

  CommunityJoinedEvent({required this.communityId, required this.joinMethod});

  @override
  String get name => 'community_joined';

  @override
  Map<String, dynamic> get parameters => {
        'community_id': communityId,
        'join_method': joinMethod,
      };
}

/// User left a community.
class CommunityLeftEvent implements AnalyticsEvent {
  final String communityId;

  CommunityLeftEvent({required this.communityId});

  @override
  String get name => 'community_left';

  @override
  Map<String, dynamic> get parameters => {'community_id': communityId};
}

/// User shared a community invite link.
class CommunityInviteSharedEvent implements AnalyticsEvent {
  final String communityId;

  CommunityInviteSharedEvent({required this.communityId});

  @override
  String get name => 'community_invite_shared';

  @override
  Map<String, dynamic> get parameters => {'community_id': communityId};
}

/// User copied a community invite link to clipboard.
class CommunityInviteCopiedEvent implements AnalyticsEvent {
  final String communityId;

  CommunityInviteCopiedEvent({required this.communityId});

  @override
  String get name => 'community_invite_copied';

  @override
  Map<String, dynamic> get parameters => {'community_id': communityId};
}

/// User opened the unified community selection sheet.
///
/// `source` identifies the entry point so we can verify the unification is
/// actually being used and compare engagement across flows. Values:
/// `invite`, `gear_create`, `gear_access`, `experience_create`,
/// `experience_access`, `request_create`, `request_access`.
class CommunitySelectionOpenedEvent implements AnalyticsEvent {
  final String source;

  CommunitySelectionOpenedEvent({required this.source});

  @override
  String get name => 'community_selection_opened';

  @override
  Map<String, dynamic> get parameters => {'source': source};
}

/// User launched the community-creation flow directly (skipping the
/// invite/selection chain). `source` identifies the entry point so we
/// can compare engagement vs. the picker-footer path. Values:
/// `workshop_carousel` (the trailing "New Community" tile on the
/// Workshop community carousel).
class CommunityCreationLaunchedEvent implements AnalyticsEvent {
  final String source;

  CommunityCreationLaunchedEvent({required this.source});

  @override
  String get name => 'community_creation_launched';

  @override
  Map<String, dynamic> get parameters => {'source': source};
}

/// User displayed QR code for sharing an item (gear, request, or experience).
class ItemShareQrDisplayedEvent implements AnalyticsEvent {
  final String communityId;
  final String itemType; // 'gear', 'request', 'experience'
  final String itemId;

  ItemShareQrDisplayedEvent({
    required this.communityId,
    required this.itemType,
    required this.itemId,
  });

  @override
  String get name => 'item_share_qr_displayed';

  @override
  Map<String, dynamic> get parameters => {
        'community_id': communityId,
        'item_type': itemType,
        'item_id': itemId,
      };
}

/// User copied an item share invite link to clipboard.
class ItemShareCopiedEvent implements AnalyticsEvent {
  final String communityId;
  final String itemType; // 'gear', 'request', 'experience'
  final String itemId;

  ItemShareCopiedEvent({
    required this.communityId,
    required this.itemType,
    required this.itemId,
  });

  @override
  String get name => 'item_share_copied';

  @override
  Map<String, dynamic> get parameters => {
        'community_id': communityId,
        'item_type': itemType,
        'item_id': itemId,
      };
}

/// User shared an item invite link using the system share sheet.
class ItemShareSharedEvent implements AnalyticsEvent {
  final String communityId;
  final String itemType; // 'gear', 'request', 'experience'
  final String itemId;

  ItemShareSharedEvent({
    required this.communityId,
    required this.itemType,
    required this.itemId,
  });

  @override
  String get name => 'item_share_shared';

  @override
  Map<String, dynamic> get parameters => {
        'community_id': communityId,
        'item_type': itemType,
        'item_id': itemId,
      };
}

// =============================================================================
// Authentication Events
// =============================================================================

/// User completed login.
class LoginSuccessEvent implements AnalyticsEvent {
  final String method; // 'apple', 'google', 'email'

  LoginSuccessEvent({required this.method});

  @override
  String get name => 'login_success';

  @override
  Map<String, dynamic> get parameters => {'method': method};
}

/// User chose a login method on the login screen chooser.
class LoginMethodSelectedEvent implements AnalyticsEvent {
  final String method; // 'phone', 'google', 'email'

  LoginMethodSelectedEvent({required this.method});

  @override
  String get name => 'login_method_selected';

  @override
  Map<String, dynamic> get parameters => {'method': method};
}

/// User chose a registration method on the register screen chooser.
class RegisterMethodSelectedEvent implements AnalyticsEvent {
  final String method; // 'phone', 'google', 'email'

  RegisterMethodSelectedEvent({required this.method});

  @override
  String get name => 'register_method_selected';

  @override
  Map<String, dynamic> get parameters => {'method': method};
}

/// Login attempt failed.
class LoginFailedEvent implements AnalyticsEvent {
  final String method;
  final String errorCode;

  LoginFailedEvent({required this.method, required this.errorCode});

  @override
  String get name => 'login_failed';

  @override
  Map<String, dynamic> get parameters => {
        'method': method,
        'error_code': errorCode,
      };
}

/// User logged out.
class LogoutEvent implements AnalyticsEvent {
  @override
  String get name => 'logout';

  @override
  Map<String, dynamic> get parameters => {};
}

/// User completed sign up.
class SignUpSuccessEvent implements AnalyticsEvent {
  final String method;

  SignUpSuccessEvent({required this.method});

  @override
  String get name => 'sign_up_success';

  @override
  Map<String, dynamic> get parameters => {'method': method};
}

// =============================================================================
// Impact Equivalence Events
// =============================================================================

/// Community crossed a new tier on one of the impact-equivalence ladders
/// (time_together / money_saved). Fired at most once per
/// `(surface, metric, tier_id)` tuple via [TierTrackingRepository].
class TierUnlockedEvent implements AnalyticsEvent {
  final String metric; // 'time_together' | 'money_saved'
  final String tierId; // e.g. 't_40hr', 'm_1025'
  final double value; // raw value (hours or USD)
  final int communitySize; // 1 for solo / profile surface

  TierUnlockedEvent({
    required this.metric,
    required this.tierId,
    required this.value,
    required this.communitySize,
  });

  @override
  String get name => 'tier_unlocked';

  @override
  Map<String, dynamic> get parameters => {
        'metric': metric,
        'tier_id': tierId,
        'value': value,
        'community_size': communitySize,
      };
}

/// User tapped a candidate thumbnail in the Replace Media modal.
/// `index` is 0-based position in the candidate row. `contentType`
/// distinguishes image-vs-video for the play-overlay UX.
class ReplaceMediaCandidateTappedEvent implements AnalyticsEvent {
  final int index;
  // "pexels" | "unsplash" | "url" | "unspecified" for server-emitted
  // candidates; "ripls-internal" for swap-back slots (the
  // previously-active media surfaced in the row so swaps are
  // reversible).
  final String provider;
  final String contentType;

  ReplaceMediaCandidateTappedEvent({
    required this.index,
    required this.provider,
    required this.contentType,
  });

  @override
  String get name => 'replace_media_candidate_tapped';

  @override
  Map<String, dynamic> get parameters => {
        'index': index,
        'provider': provider,
        'content_type': contentType,
      };
}

// =============================================================================
// Notification deep-link diagnostics (#2636)
// =============================================================================

/// Diagnostic telemetry for the notification tap → navigation pipeline.
///
/// A dropped notification deep link is not a crash, so it previously vanished
/// into INFO-only logs and was invisible in prod — which is why the "notification
/// doesn't route to the item" bug kept recurring (#2636). This event makes each
/// decisive hop queryable in Firebase Analytics / BigQuery.
///
/// **Privacy:** carries entity IDs (in [route]/[actualLocation]) and data-map
/// KEYS only ([dataKeys]) — never titles, names, bodies, or data values.
class NotificationDeepLinkEvent implements AnalyticsEvent {
  /// Which hop this event records: `route` (the payload→route decision) or
  /// `nav_result` (the router's actual location one frame after `go()`).
  final String phase;

  /// Where the tap entered from: `opened` (onMessageOpenedApp), `initial`
  /// (getInitialMessage at startup), `missed` (resume re-check), `local`
  /// (foreground local-notification tap), or `unknown`.
  final String source;

  /// Resolved entity kind: `gear`, `experience`, `request`, `community`, or
  /// `none` when the payload carried no navigable entity id.
  final String? entity;

  /// The route resolved (`route` phase) or navigated to (`nav_result` phase).
  /// Path + entity id only — no PII.
  final String? route;

  /// Sorted, comma-joined data-map keys present in the payload. Keys only —
  /// values are never included.
  final String? dataKeys;

  /// `route` phase: true when the payload had no navigable entity id — a drop
  /// cause.
  final bool? noEntityId;

  /// `route` phase: true when the navigation callback was null at route time,
  /// so the resolved route could not be applied — a drop cause.
  final bool? onNavigateNull;

  /// `nav_result` phase: the router's actual location one frame after `go()`.
  final String? actualLocation;

  /// `nav_result` phase: whether [actualLocation] matched the intended [route]
  /// (false ⇒ the navigation did not stick).
  final bool? landed;

  /// `nav_result` phase: which drain vector fired this navigation attempt —
  /// `tap` (live tap), `ready` (splash→ready drain of a route captured before
  /// the router was mounted), `resume` (app-resume drain), or `replay` (a
  /// bounded retry after a prior attempt did not stick). Names the recovery
  /// path so a query can tell an immediate landing from a recovered one.
  final String? trigger;

  /// `nav_result` phase: set only on a `replay` attempt — true when the retry
  /// finally landed (the deep link was *recovered* rather than lost), false
  /// when even the retry missed (a permanent client-side drop). Null on the
  /// first attempt, so `recovered == false` is the queryable "gave up" signal.
  final bool? recovered;

  NotificationDeepLinkEvent({
    required this.phase,
    required this.source,
    this.entity,
    this.route,
    this.dataKeys,
    this.noEntityId,
    this.onNavigateNull,
    this.actualLocation,
    this.landed,
    this.trigger,
    this.recovered,
  });

  @override
  String get name => 'notification_deeplink';

  @override
  Map<String, dynamic> get parameters => {
        'phase': phase,
        'source': source,
        if (entity != null) 'entity': entity,
        if (route != null) 'route': route,
        if (dataKeys != null) 'data_keys': dataKeys,
        if (noEntityId != null) 'no_entity_id': noEntityId,
        if (onNavigateNull != null) 'on_navigate_null': onNavigateNull,
        if (actualLocation != null) 'actual_location': actualLocation,
        if (landed != null) 'landed': landed,
        if (trigger != null) 'trigger': trigger,
        if (recovered != null) 'recovered': recovered,
      };
}
