import 'dart:convert';
import 'dart:io';

import 'package:connectrpc/connect.dart' as connect;
import 'package:firebase_app_installations/firebase_app_installations.dart';
import 'package:firebase_messaging/firebase_messaging.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter_local_notifications/flutter_local_notifications.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/observability/events.dart';
import 'package:ripls/core/observability/logging/logger.dart';
import 'package:ripls/core/utils/rpc_utils.dart';
import 'package:ripls/data/gen/ripls/api/device_service.connect.client.dart';
import 'package:ripls/data/gen/ripls/api/device_service.pb.dart';
import 'package:ripls/services/chat_notification_manager.dart';
import 'package:ripls/services/notification_route_replayer.dart';

export 'package:ripls/core/errors/rpc_error_handler.dart' show ServiceException;

final _log = ObservableLogger.named('FCMService');

/// FcmDeviceRegistrar abstracts device token registration calls for testability.
abstract interface class FcmDeviceRegistrar {
  /// Registers a device token with the server.
  Future<RegisterDeviceTokenResponse> registerDeviceToken(
    RegisterDeviceTokenRequest request, {
    connect.Headers? headers,
  });

  /// Unregisters a device token from the server.
  Future<UnregisterDeviceTokenResponse> unregisterDeviceToken(
    UnregisterDeviceTokenRequest request, {
    connect.Headers? headers,
  });
}

class _DefaultFcmDeviceRegistrar implements FcmDeviceRegistrar {
  final DeviceServiceClient _client;

  _DefaultFcmDeviceRegistrar(connect.Transport transport)
      : _client = DeviceServiceClient(transport);

  @override
  Future<RegisterDeviceTokenResponse> registerDeviceToken(
    RegisterDeviceTokenRequest request, {
    connect.Headers? headers,
  }) =>
      _client.registerDeviceToken(request, headers: headers);

  @override
  Future<UnregisterDeviceTokenResponse> unregisterDeviceToken(
    UnregisterDeviceTokenRequest request, {
    connect.Headers? headers,
  }) =>
      _client.unregisterDeviceToken(request, headers: headers);
}

/// FCMService handles Firebase Cloud Messaging setup and notification handling
class FCMService {
  final String? Function() _getAccessToken;
  final FirebaseMessaging _messaging;
  final FlutterLocalNotificationsPlugin _localNotifications;
  final RpcErrorHandler _errorHandler;
  final Future<void> Function()? _onUnauthenticated;
  final FcmDeviceRegistrar _deviceRegistrar;

  /// Callback for when a notification is received in the foreground
  Function(RemoteMessage)? onForegroundMessage;

  /// Callback for when a notification is tapped (app opened from notification)
  Function(RemoteMessage)? onNotificationTapped;

  /// Callback for navigation - receives the route path and optional community ID.
  void Function(String route, {String? communityId})? onNavigate;

  /// Callback for community events received via push notification.
  /// Passes the FCM data payload so the caller can route through EventRouter.
  void Function(Map<String, dynamic> data)? onCommunityEvent;

  /// Sink for deep-link diagnostic telemetry (#2636). Wired from the
  /// observability service so a dropped notification tap is queryable in prod
  /// rather than lost in INFO logs. Null in tests that don't assert telemetry.
  final void Function(AnalyticsEvent event)? _logAnalyticsEvent;

  /// Last handled notification message ID for deduplication.
  /// Prevents double-handling when both onMessageOpenedApp and
  /// getInitialMessage() fire for the same notification tap (Android).
  String? _lastHandledNotificationMessageId;

  /// The startup `getInitialMessage()` consume, tracked so the resume
  /// [checkForMissedNotification] can serialize behind it. Without this, an app
  /// that resumes during cold start could read and route the same terminated-
  /// launch message before the startup path recorded its ID, double-handling it
  /// (#2636). Null until [initialize] wires the handlers.
  Future<void>? _initialMessageInFlight;

  /// Seeds [_initialMessageInFlight] so the resume-vs-startup ordering can be
  /// exercised without a running Firebase environment.
  @visibleForTesting
  void setInitialMessageInFlightForTest(Future<void> future) {
    _initialMessageInFlight = future;
  }

  FCMService({
    required connect.Transport transport,
    required String? Function() getAccessToken,
    RpcErrorHandler? errorHandler,
    Future<void> Function()? onUnauthenticated,
    FirebaseMessaging? messaging,
    FlutterLocalNotificationsPlugin? localNotifications,
    ChatNotificationManager? chatNotificationManager,
    void Function(AnalyticsEvent event)? logAnalyticsEvent,
    // Injects a pre-built registrar in tests; production code omits this.
    @visibleForTesting FcmDeviceRegistrar? deviceRegistrar,
  }) : _getAccessToken = getAccessToken,
       _errorHandler = errorHandler ?? RpcErrorHandler(),
       _logAnalyticsEvent = logAnalyticsEvent,
       _onUnauthenticated = onUnauthenticated,
       _messaging = messaging ?? FirebaseMessaging.instance,
       _localNotifications =
           localNotifications ?? FlutterLocalNotificationsPlugin(),
       _deviceRegistrar =
           deviceRegistrar ?? _DefaultFcmDeviceRegistrar(transport) {
    _chatNotificationManager = chatNotificationManager ??
        ChatNotificationManager(localNotifications: _localNotifications);
  }

  /// Shared chat-notification renderer. Exposed so `ChatRepository` can
  /// reach it via a Riverpod provider for cancel-on-read and so the
  /// background-isolate handler can reuse the same instance.
  ChatNotificationManager get chatNotificationManager =>
      _chatNotificationManager;

  late final ChatNotificationManager _chatNotificationManager;

  connect.Headers _buildHeaders() {
    return RpcUtils.buildHeaders(
      _getAccessToken,
      onUnauthenticated: _onUnauthenticated,
    );
  }

  /// Initialize Firebase Messaging and register device token.
  Future<void> initialize() async {
    try {
      _log.info('Initializing FCM service...');

      // Set up message handlers FIRST — critical for notification deep linking.
      // getInitialMessage() and onMessageOpenedApp.listen() must run even if
      // device token registration fails (e.g., stale network after inactivity).
      _setupMessageHandlers();

      // Request permission for notifications
      final permissionGranted = await _requestPermission();
      if (!permissionGranted) {
        _log.warning('Notification permission denied');
        return;
      }

      // Initialize local notifications for foreground messages (Android)
      await _initializeLocalNotifications();

      // Device token registration makes a network call that can fail.
      // Wrap independently so message handlers remain functional.
      try {
        await _registerDeviceToken();
        _messaging.onTokenRefresh.listen(_onTokenRefresh);
      } catch (e) {
        _log.error(
          'Device token registration failed after retry '
          '(message handlers still active): $e',
        );
      }

      _log.info('FCM service initialized successfully');
    } catch (e) {
      _log.error('Failed to initialize FCM service: $e');
      rethrow;
    }
  }

  /// Request notification permission from the user
  Future<bool> _requestPermission() async {
    final settings = await _messaging.requestPermission(
      alert: true,
      announcement: false,
      badge: true,
      carPlay: false,
      criticalAlert: false,
      provisional: false,
      sound: true,
    );

    _log.info('Permission status: ${settings.authorizationStatus}');

    return settings.authorizationStatus == AuthorizationStatus.authorized ||
        settings.authorizationStatus == AuthorizationStatus.provisional;
  }

  /// Initialize local notifications for displaying foreground messages
  Future<void> _initializeLocalNotifications() async {
    const androidSettings = AndroidInitializationSettings(
      '@drawable/ic_launcher_monochrome',
    );
    const iosSettings = DarwinInitializationSettings(
      requestAlertPermission: false,
      requestBadgePermission: false,
      requestSoundPermission: false,
    );

    const initSettings = InitializationSettings(
      android: androidSettings,
      iOS: iosSettings,
    );

    await _localNotifications.initialize(
      settings: initSettings,
      onDidReceiveNotificationResponse: _onLocalNotificationTapped,
    );

    // Create notification channels for Android. The 'chats' channel is
    // registered alongside 'community_events' so users can mute one without
    // muting the other (system Settings → Apps → Ripls → Notifications).
    if (Platform.isAndroid) {
      const communityEventsChannel = AndroidNotificationChannel(
        'community_events',
        'Community Events',
        description: 'Notifications for community activity',
        importance: Importance.high,
      );

      // Channel name + description fall back to English at the moment of
      // first channel creation. `ChatNotificationManager.persistLocalizedStrings`
      // (called once from main.dart with the running locale) seeds
      // SharedPreferences so subsequent reads — including channel reads on
      // future app starts — see the user's locale. Android does not allow
      // renaming a channel after creation, so the first-install name sticks
      // until app data is cleared; acceptable tradeoff documented in #1892.
      final localizedChannel =
          await ChatNotificationManager.readChannelStrings();
      final chatsChannel = AndroidNotificationChannel(
        ChatNotificationManager.channelId,
        localizedChannel.name,
        description: localizedChannel.description,
        importance: Importance.high,
      );

      final androidPlugin = _localNotifications
          .resolvePlatformSpecificImplementation<
            AndroidFlutterLocalNotificationsPlugin
          >();
      await androidPlugin?.createNotificationChannel(communityEventsChannel);
      await androidPlugin?.createNotificationChannel(chatsChannel);
    }
  }

  /// Get device token and register with server via the centralized retry path.
  ///
  /// Uses [RpcUtils.executeRpc] so a stale-but-refreshable token at cold start
  /// triggers a token refresh and retry rather than a SEVERE log. SEVERE is
  /// only emitted by the caller ([initialize]) after the retry also fails.
  Future<void> _registerDeviceToken() async {
    final token = await _messaging.getToken();
    if (token == null) {
      _log.warning('Failed to get device token');
      return;
    }

    _log.info('Got device token: ${token.substring(0, 20)}...');

    // Determine platform
    final platform = Platform.isIOS
        ? DevicePlatform.DEVICE_PLATFORM_IOS
        : DevicePlatform.DEVICE_PLATFORM_ANDROID;

    final request = RegisterDeviceTokenRequest(
      deviceToken: token,
      platform: platform,
      installationId: await _installationId(),
    );

    final response = await RpcUtils.executeRpc(
      () async => _deviceRegistrar.registerDeviceToken(
        request,
        headers: _buildHeaders(),
      ),
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'RegisterDeviceToken',
    );
    _log.info('Device registered with ID: ${response.deviceId}');
  }

  /// The Firebase installation ID for this app instance, or an empty string
  /// when it cannot be read.
  ///
  /// FCM deprecated addressing a push by registration token in favour of the
  /// installation ID, but the two are different identifiers — the server sends
  /// to whichever it has and falls back to the token (#3043). So a failure
  /// here is not fatal: it costs nothing except staying on the deprecated
  /// path, which is exactly where every already-registered device is.
  Future<String> _installationId() async {
    try {
      return await FirebaseInstallations.instance.getId();
    } on Object catch (e) {
      _log.warning('Failed to read Firebase installation ID: $e');
      return '';
    }
  }

  /// Registers the device token with the server.
  ///
  /// Exposed for unit tests that need to exercise the retry behavior without a
  /// full Firebase environment. Production code reaches this path via
  /// [initialize] and [_onTokenRefresh].
  @visibleForTesting
  Future<void> registerDeviceTokenForTest() => _registerDeviceToken();

  /// Handle token refresh
  Future<void> _onTokenRefresh(String token) async {
    _log.info('Token refreshed: ${token.substring(0, 20)}...');
    await _registerDeviceToken();
  }

  /// Set up message handlers for different app states
  void _setupMessageHandlers() {
    // Foreground messages
    FirebaseMessaging.onMessage.listen(_onForegroundMessageReceived);

    // Background/terminated - message tapped
    FirebaseMessaging.onMessageOpenedApp
        .listen((message) => _onMessageOpenedApp(message, source: 'opened'));

    // Check if app was opened from terminated state by tapping notification.
    // Track the consume so checkForMissedNotification() can serialize behind it
    // and the cold-start message is handled exactly once (#2636).
    _initialMessageInFlight = _messaging.getInitialMessage().then((message) {
      if (message != null) {
        _log.info(
          'App opened from terminated state by notification: '
          'title=${message.notification?.title}, '
          'dataKeys=${message.data.keys.toList()}, '
          'data=${message.data}',
        );
        _onMessageOpenedApp(message, source: 'initial');
      } else {
        _log.info('getInitialMessage: no initial message (normal launch)');
      }
    });
  }

  /// Handle foreground message - show local notification
  Future<void> _onForegroundMessageReceived(RemoteMessage message) async {
    _log.info(
      'Foreground message: title=${message.notification?.title}, '
      'dataKeys=${message.data.keys.toList()}',
    );

    final notification = message.notification;
    if (message.data['type'] == 'chat_message') {
      // Render via the chat-specific MessagingStyle path. Replaces the
      // prior community_events-style render for chat to give a conversation-
      // stable notification id and a multi-message transcript.
      await _chatNotificationManager.renderForChatMessage(message.data);
    } else if (notification != null) {
      // Non-chat notifications continue through the legacy single-message
      // render on the community_events channel.
      await _showLocalNotification(notification, message.data);
    }

    // Route community events through the EventRouter for unified invalidation.
    if (message.data['type'] == 'community_event') {
      onCommunityEvent?.call(message.data);
    }

    // Call callback if set
    onForegroundMessage?.call(message);
  }

  /// Handle notification tap with message ID deduplication.
  ///
  /// [source] identifies which vector delivered the tap (`opened` /
  /// `initial` / `missed`), carried into the deep-link diagnostic (#2636).
  void _onMessageOpenedApp(RemoteMessage message, {required String source}) {
    final messageId = message.messageId;
    if (messageId != null && messageId == _lastHandledNotificationMessageId) {
      _log.info('Skipping duplicate notification: messageId=$messageId');
      return;
    }
    if (messageId != null) {
      _lastHandledNotificationMessageId = messageId;
    }

    _log.info(
      'Notification tapped: title=${message.notification?.title}, '
      'source=$source, messageId=$messageId, '
      'dataKeys=${message.data.keys.toList()}, '
      'data=${message.data}',
    );
    onNotificationTapped?.call(message);

    // Handle navigation based on notification payload. Route even when the
    // data map is empty so the diagnostic records the drop (source + empty
    // keys) instead of returning silently.
    _routeNotificationData(message.data, source: source);
  }

  /// Handle local notification tap - parse payload and navigate.
  void _onLocalNotificationTapped(NotificationResponse response) {
    _log.info('Local notification tapped: ${response.payload}');

    final payload = response.payload;
    if (payload == null || payload.isEmpty) {
      _log.warning('No payload in local notification');
      return;
    }

    try {
      final data = jsonDecode(payload) as Map<String, dynamic>;
      _routeNotificationData(data, source: 'local');
    } catch (e) {
      _log.warning('Failed to parse local notification payload: $e');
    }
  }

  /// Routes a notification data map to the appropriate navigation callback,
  /// forwarding the deep-link diagnostic sink and tap [source].
  void _routeNotificationData(Map<String, dynamic> data,
      {required String source}) {
    _log.info(
      'routeNotificationData: source=$source, '
      'onNavigate=${onNavigate != null ? "set" : "NULL"}',
    );
    routeNotificationPayload(
      data: data,
      onNavigate: onNavigate,
      // Capture the resolved target in the process-wide holder the instant it's
      // known, so it survives an unwired callback or a not-yet-mounted router
      // and can be replayed later (#2636). This write does not depend on
      // [onNavigate], which is the whole point.
      onRouteResolved: (route) =>
          PendingNotificationRouteHolder.value = route,
      source: source,
      logAnalyticsEvent: _logAnalyticsEvent,
    );
  }

  /// Show a local notification for foreground messages
  Future<void> _showLocalNotification(
    RemoteNotification notification,
    Map<String, dynamic> data,
  ) async {
    const androidDetails = AndroidNotificationDetails(
      'community_events',
      'Community Events',
      channelDescription: 'Notifications for community activity',
      importance: Importance.high,
      priority: Priority.high,
    );

    const iosDetails = DarwinNotificationDetails(
      presentAlert: true,
      presentBadge: true,
      presentSound: true,
    );

    const details = NotificationDetails(
      android: androidDetails,
      iOS: iosDetails,
    );

    // Encode data as JSON payload for navigation when tapped
    final payload = data.isNotEmpty ? jsonEncode(data) : null;

    await _localNotifications.show(
      id: notification.hashCode,
      title: notification.title,
      body: notification.body,
      notificationDetails: details,
      payload: payload,
    );
  }

  /// Checks for a notification that opened the app but wasn't handled.
  ///
  /// Call on app resume to catch notifications missed by onMessageOpenedApp
  /// (e.g., when Android destroys and recreates the Activity while the process
  /// is alive). Deduplicates by message ID to prevent double-handling.
  Future<void> checkForMissedNotification() async {
    try {
      // Wait for the startup consume to finish first, so the terminated-launch
      // message is recorded (and deduped) before this resume path reads it —
      // otherwise the two can race and double-handle the same tap (#2636).
      final inFlight = _initialMessageInFlight;
      if (inFlight != null) {
        await inFlight;
      }

      final message = await _messaging.getInitialMessage();
      if (message == null) {
        return;
      }

      final messageId = message.messageId;
      _log.info(
        'Resume check: getInitialMessage returned '
        'messageId=$messageId, data=${message.data}',
      );

      if (messageId != null && messageId == _lastHandledNotificationMessageId) {
        _log.info('Resume check: skipping duplicate messageId=$messageId');
        return;
      }

      _onMessageOpenedApp(message, source: 'missed');
    } catch (e) {
      _log.warning('Resume check for missed notification failed: $e');
    }
  }
  }

/// Top-level entry point for FCM background message delivery.
///
/// Runs in a separate Dart isolate spawned by `firebase_messaging` when the
/// app is in the background or terminated. The isolate has no access to
/// Riverpod, widget context, or main-isolate singletons — it must construct
/// its own plugin instances and persistent state lives only in
/// SharedPreferences and the platform NotificationManager.
///
/// For `chat_message` payloads the server omits the top-level `notification`
/// block on Android so this handler is the only path that posts the system
/// notification; the body is built from the data fields by
/// `ChatNotificationManager.renderForChatMessage`.
@pragma('vm:entry-point')
Future<void> firebaseMessagingBackgroundHandler(RemoteMessage message) async {
  _log.info(
    'Background message: type=${message.data['type']} '
    'dataKeys=${message.data.keys.toList()}',
  );

  if (message.data['type'] == 'chat_message') {
    try {
      await _renderBackgroundChatNotification(message.data);
    } catch (e) {
      _log.warning('Failed to render background chat notification: $e');
    }
    return;
  }

  // Non-chat payloads: nothing to do here. Android still displays its own
  // notification because the server keeps the `notification` block for those.
}

/// Builds the local-notifications plugin and renders the chat notification
/// from the background isolate. Plugin initialization is required per isolate
/// — the main-isolate `_initializeLocalNotifications` doesn't carry over.
Future<void> _renderBackgroundChatNotification(
    Map<String, dynamic> data) async {
  final plugin = FlutterLocalNotificationsPlugin();
  const androidSettings = AndroidInitializationSettings(
    '@drawable/ic_launcher_monochrome',
  );
  const iosSettings = DarwinInitializationSettings(
    requestAlertPermission: false,
    requestBadgePermission: false,
    requestSoundPermission: false,
  );
  await plugin.initialize(
    settings: const InitializationSettings(
      android: androidSettings,
      iOS: iosSettings,
    ),
  );

  final manager = ChatNotificationManager(localNotifications: plugin);
  await manager.renderForChatMessage(data);
}

/// Routes a notification data payload to the appropriate navigation callback.
///
/// Priority: gear_id > experience_id > request_id.
/// When conversation_id is also present (chat message notifications), appends
/// ?tab=chat to open the item detail screen on the Chat tab.
/// Passes community_id through so the caller can switch community context.
///
/// [source] tags where the tap entered from (`opened` / `initial` / `missed` /
/// `local`) and [logAnalyticsEvent], when supplied, receives one
/// [NotificationDeepLinkEvent] (`phase: 'route'`) describing the decision — so a
/// dropped deep link is queryable in prod instead of vanishing into INFO logs
/// (#2636). This is the single choke point for the route decision; keep it that
/// way so telemetry and the drop-cause WARNINGs stay in one place.
///
/// [onRouteResolved], when supplied, is invoked with the resolved
/// [PendingNotificationRoute] before navigation is attempted — the "catch" that
/// captures the target so a [NotificationRouteReplayer] can recover it if the
/// immediate [onNavigate] can't run.
void routeNotificationPayload({
  required Map<String, dynamic> data,
  void Function(String route, {String? communityId})? onNavigate,
  void Function(PendingNotificationRoute route)? onRouteResolved,
  String source = 'unknown',
  void Function(AnalyticsEvent event)? logAnalyticsEvent,
}) {
  // Keys only — never values — so the diagnostic carries no titles/names/bodies.
  final dataKeys = (data.keys.map((k) => k.toString()).toList()..sort()).join(',');

  if (data.isEmpty) {
    _log.warning(
        'routeNotificationPayload: data is empty, cannot deep link (source=$source)');
    logAnalyticsEvent?.call(NotificationDeepLinkEvent(
      phase: 'route',
      source: source,
      entity: 'none',
      dataKeys: dataKeys,
      noEntityId: true,
      onNavigateNull: onNavigate == null,
    ));
    return;
  }

  final eventType = data['event_type']?.toString() ?? '';
  final gearId = data['gear_id']?.toString() ?? '';
  final experienceId = data['experience_id']?.toString() ?? '';
  final requestId = data['request_id']?.toString() ?? '';
  final conversationId = data['conversation_id']?.toString() ?? '';
  final communityId = data['community_id']?.toString() ?? '';

  // When conversation_id is also present, append ?tab=chat to open the
  // item detail screen with the Chat tab selected.
  final chatTab = conversationId.isNotEmpty ? '?tab=chat' : '';
  final effectiveCommunityId = communityId.isNotEmpty ? communityId : null;

  // Resolve the target route + entity kind up front, so telemetry and
  // navigation share one decision. Priority mirrors the historical order.
  String? route;
  String entity;
  if (eventType == 'REQUEST_FOLLOWUP_PROMPT' && requestId.isNotEmpty) {
    // Deep-link to the request detail screen where the Mark Fulfilled /
    // Cancel action bar is already present. The requester resolves the
    // request from inside the app via the existing fulfillment UI.
    route = '/request/$requestId';
    entity = 'request';
  } else if (gearId.isNotEmpty) {
    route = '/gear/$gearId$chatTab';
    entity = 'gear';
  } else if (experienceId.isNotEmpty) {
    route = '/experience/$experienceId$chatTab';
    entity = 'experience';
  } else if (requestId.isNotEmpty) {
    route = '/request/$requestId$chatTab';
    entity = 'request';
  } else if (effectiveCommunityId != null) {
    // Community-scoped notification with no inner entity. Land on the
    // community itself rather than home — home makes the reader hunt for the
    // thing they were just told about (#2876).
    //
    // Member-joined goes one step further, to the discussion: its notification
    // says "say hi", and a tap that lands anywhere else breaks that promise.
    // The other community-scoped events (deleted, restored, ownership
    // transferred) stay on the community — a deletion notice does not belong
    // in a chat pane.
    final discussTab = eventType == 'COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED'
        ? '?tab=discuss'
        : '';
    route = '/group/$effectiveCommunityId$discussTab';
    entity = 'community';
  } else {
    route = null;
    entity = 'none';
  }

  final noEntityId = route == null;
  final onNavigateNull = onNavigate == null;

  // One queryable event per route decision — the two drop causes below
  // (no_entity_id, on_navigate_null) are the fields a prod query keys on.
  logAnalyticsEvent?.call(NotificationDeepLinkEvent(
    phase: 'route',
    source: source,
    entity: entity,
    route: route,
    dataKeys: dataKeys,
    noEntityId: noEntityId,
    onNavigateNull: onNavigateNull,
  ));

  if (route == null) {
    _log.warning(
      'routeNotificationPayload: no entity ID found, cannot deep link '
      '(source=$source, dataKeys=$dataKeys)',
    );
    return;
  }

  // Catch: hand the resolved target to the holder before attempting navigation,
  // so it is recoverable even if [onNavigate] is unwired or the router isn't
  // ready. Independent of [onNavigate] by design (#2636).
  onRouteResolved?.call(PendingNotificationRoute(
    route: route,
    communityId: effectiveCommunityId,
    source: source,
  ));

  if (onNavigate == null) {
    // No immediate executor (e.g. a rebuilt FCM service whose callback wasn't
    // re-wired). Not a drop anymore: the holder retains the route above and a
    // later drain (ready/resume) replays it. WARNING so the anomaly is still a
    // breadcrumb; on_navigate_null in the event above is the queryable signal.
    _log.warning(
      'routeNotificationPayload: resolved route $route but onNavigate is null '
      '— captured in holder for replay (source=$source)',
    );
    return;
  }
  _log.info('routeNotificationPayload: navigating to $route (source=$source)');
  onNavigate.call(route, communityId: effectiveCommunityId);
}
