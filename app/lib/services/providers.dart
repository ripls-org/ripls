// Re-export shim — every symbol that was public in the original providers.dart
// is re-exported here so all callers continue to compile without edits.

// App-level (theme, FCM)
export 'package:ripls/services/providers/app_providers.dart'
    show
        fcmServiceProvider,
        ThemeModeNotifier,
        themeModeProvider;
// Auth, transport, observability
export 'package:ripls/services/providers/auth_providers.dart'
    show
        transportProvider,
        authStateProvider,
        observabilitySettingsProvider,
        observabilityServiceProvider,
        rpcErrorHandlerProvider,
        authServiceProvider,
        authRepositoryProvider,
        secureStorageProvider,
        sharedPreferencesAsyncProvider;
// Cache manager and cross-domain invalidation signals
export 'package:ripls/services/providers/cache_providers.dart'
    show
        cacheManagerProvider,
        searchCacheInvalidationProvider,
        SearchCacheInvalidationNotifier,
        feedStatusCacheInvalidationProvider,
        FeedStatusCacheInvalidationNotifier,
        feedListingCacheInvalidationProvider,
        FeedListingCacheInvalidationNotifier,
        portfolioCacheInvalidationProvider,
        PortfolioCacheInvalidationNotifier,
        contentCacheInvalidationProvider,
        ContentCacheInvalidationNotifier,
        transferCacheInvalidationProvider,
        TransferCacheInvalidationNotifier,
        impactCacheInvalidationProvider,
        ImpactCacheInvalidationNotifier,
        deletedCommunitiesCacheInvalidationProvider,
        DeletedCommunitiesCacheInvalidationNotifier,
        rejoinableCommunitiesCacheInvalidationProvider,
        RejoinableCommunitiesCacheInvalidationNotifier;
// Chat
export 'package:ripls/services/providers/chat_providers.dart'
    show
        chatServiceProvider,
        chatRepositoryProvider,
        unreadCountRepositoryProvider,
        unreadCountProvider,
        UnreadCountState,
        UnreadCountNotifier,
        unreadMessageCountTotalProvider,
        unreadMessageCountForCommunityProvider,
        communityConversationProvider,
        resetUnreadForConversation;
// Community (service, repository, events, user-portfolio state)
export 'package:ripls/services/providers/community_providers.dart'
    show
        eventRouterProvider,
        communityEventPollerProvider,
        communityEventStreamProvider,
        communityServiceProvider,
        communityRepositoryProvider,
        provisionalUserRepositoryProvider,
        communityProvider,
        CommunitiesState,
        CommunitiesNotifier,
        communitiesProvider;
// Experience
export 'package:ripls/services/providers/experience_providers.dart'
    show
        experienceServiceProvider,
        experienceRepositoryProvider;
// Feed and portfolio
export 'package:ripls/services/providers/feed_providers.dart'
    show
        feedServiceProvider,
        portfolioServiceProvider,
        feedRepositoryProvider,
        portfolioRepositoryProvider;
// Gear and transfer
export 'package:ripls/services/providers/gear_providers.dart'
    show
        gearServiceProvider,
        transferServiceProvider,
        gearRepositoryProvider,
        transferRepositoryProvider;
// Impact, feedback
export 'package:ripls/services/providers/impact_providers.dart'
    show
        impactMetricsServiceProvider,
        feedbackServiceProvider,
        impactMetricsRepositoryProvider,
        feedbackRepositoryProvider;
// Location (service and repository)
export 'package:ripls/services/providers/location_providers.dart'
    show
        locationServiceProvider,
        locationRepositoryProvider;
// Media
export 'package:ripls/services/providers/media_providers.dart'
    show
        mediaServiceProvider,
        mediaRepositoryProvider,
        mediaUrlProvider,
        mediaObjectProvider,
        mediaAttributionProvider;
// Request
export 'package:ripls/services/providers/request_providers.dart'
    show
        requestServiceProvider,
        requestRepositoryProvider;
// Search
export 'package:ripls/services/providers/search_providers.dart'
    show
        searchServiceProvider,
        searchRepositoryProvider;
// User location provider (separate to avoid circular dependency with user_position_resolver)
export 'package:ripls/services/providers/user_location_provider.dart'
    show userLocationProvider;
// User
export 'package:ripls/services/providers/user_providers.dart'
    show
        userServiceProvider,
        userRepositoryProvider,
        userProfileProvider;
