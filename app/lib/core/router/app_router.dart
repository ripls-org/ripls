// Every route declared here must have a `name:` field (enforced by
// scripts/check_route_names.js). Every `RpcUtils.executeRpc` call in
// app/lib/services/ must pass `operationName:` (enforced by the Dart
// compiler — see #1581, #1809). NavigationHelpers.pushWithSlide and
// pushScreen accept an optional `routeName:` that flows through to
// RouteSettings for analytics attribution.
import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:ripls/core/config/environment.dart';
import 'package:ripls/core/observability/analytics_route_observer.dart';
import 'package:ripls/core/observability/events.dart';
import 'package:ripls/core/observability/navigation_observer.dart';
import 'package:ripls/core/observability/performance_route_observer.dart';
import 'package:ripls/core/router/video_background_route_observer.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/post_registration_destination.dart';
import 'package:ripls/data/repositories/share_link_repository.dart';
import 'package:ripls/presentation/screens/auth/forgot_password_screen.dart';
import 'package:ripls/presentation/screens/auth/login_screen.dart';
import 'package:ripls/presentation/screens/auth/phone_auth_screen.dart';
import 'package:ripls/presentation/screens/auth/register_screen.dart';
import 'package:ripls/presentation/screens/auth/reset_password_screen.dart';
import 'package:ripls/presentation/screens/experience/event_invite_join_gate.dart';
import 'package:ripls/presentation/screens/experience/experience_screen.dart';
import 'package:ripls/presentation/screens/experience/web_experience_screen.dart';
import 'package:ripls/presentation/screens/gear/gear_screen.dart';
import 'package:ripls/presentation/screens/home/home_screen.dart';
import 'package:ripls/presentation/screens/request/request_screen.dart';
import 'package:ripls/presentation/screens/settings/restore_community_screen.dart';
import 'package:ripls/presentation/screens/web/web_community_screen.dart';
import 'package:ripls/presentation/screens/web/web_item_action_screen.dart';
import 'package:ripls/services/deferred_deep_link_service.dart';
import 'package:ripls/services/providers.dart';
import 'package:ripls/services/providers/auth_providers.dart';

/// Global key for the root navigator, used for pushing screens outside of the
/// widget tree (e.g. from FCM notification handlers).
final rootNavigatorKey = GlobalKey<NavigatorState>();

/// Provider for GoRouter instance with authentication-aware routing
final routerProvider = Provider<GoRouter>((ref) {
  final observabilityService = ref.watch(observabilityServiceProvider);

  return GoRouter(
    navigatorKey: rootNavigatorKey,
    initialLocation: '/',
    debugLogDiagnostics: true,
    observers: [
      BreadcrumbNavigatorObserver(),
      PerformanceRouteObserver(
        getPerformanceMonitor: () => observabilityService.performanceMonitor,
      ),
      AnalyticsRouteObserver(
        getAnalyticsProvider: () => observabilityService.analyticsProvider,
      ),
      // VideoBackgroundHost subscribes here so it can pause when another
      // route covers the host (e.g. MediaCarousel) and resume on pop.
      videoBackgroundRouteObserver,
    ],

    // Redirect logic - runs before every route to enforce authentication
    redirect: (context, state) {
      // Ignore Firebase Auth callback URLs (reCAPTCHA redirect for phone auth).
      // On mobile these arrive via the REVERSED_CLIENT_ID custom URL scheme
      // (GoRouter may see the full scheme or just the path, /link from
      // firebaseauth/link); redirecting to /login (valid for unauthenticated
      // users) prevents the home screen from triggering RPC calls without auth
      // tokens. The PhoneAuthScreen is pushed imperatively above GoRouter and
      // survives this redirect as long as the underlying route is an auth screen.
      //
      // On WEB, the reCAPTCHA verifier runs inline (no custom-scheme deep link)
      // and Firebase handles its own callback under /__/auth/*; GoRouter never
      // sees it. The `firebaseauth` substring match below is mobile-specific and
      // could mis-catch legitimate web navigations, so skip it entirely on web.
      if (!kIsWeb) {
        final uriStr = state.uri.toString();
        if (state.uri.scheme.startsWith('com.googleusercontent.apps.') ||
            uriStr.contains('firebaseauth') ||
            state.uri.path == '/link') {
          return '/login';
        }
      }

      // Handle custom scheme deep links (<scheme>://invite?token=xxx).
      // For custom schemes, host is 'invite' and path is '/'.
      // Redirect to /invite which handles authentication logic. The
      // deep-link target is resolved from the share-link row via
      // CheckInvitation, so only the token is forwarded (#2562).
      //
      // The scheme is build-time config (#2953); it must match the
      // AndroidManifest intent-filter and iOS CFBundleURLSchemes entry, which
      // are what actually route the URL to the app.
      final scheme = Environment.deepLinkScheme;
      if (state.uri.scheme == scheme && state.uri.host == 'invite') {
        final token = state.uri.queryParameters['token'];
        if (token != null) {
          return '/invite?token=${Uri.encodeComponent(token)}';
        }
        return '/invite';
      }

      // Handle custom scheme password reset deep links
      // (<scheme>://reset-password?token=xxx)
      if (state.uri.scheme == scheme && state.uri.host == 'reset-password') {
        final token = state.uri.queryParameters['token'];
        if (token != null) {
          return '/reset-password?token=${Uri.encodeComponent(token)}';
        }
        return '/reset-password';
      }

      // Read current auth state directly (not from closure)
      final authState = ref.read(authStateProvider);
      final isAuthenticated = authState.isAuthenticated;
      final isLoading = authState.isLoading;

      // Use path instead of matchedLocation to ignore query parameters
      final path = state.uri.path;
      final isGoingToLogin = path == '/login';
      final isGoingToRegister = path == '/register';
      // Phone-first guest verify entry (#2492) — behaves like /register for
      // routing: unauthenticated guests may reach it, and after auth they're
      // handed off to the item view (see the post-auth redirect below). Shared
      // across event RSVP, gear borrow/claim, and request offer flows.
      final isGoingToVerifyPhone = path == '/verify-phone';
      final isGoingToForgotPassword = path == '/forgot-password';
      final isGoingToResetPassword = path == '/reset-password';
      final isGoingToInvite = path == '/invite' || path.startsWith('/go/');
      // The Flutter Web event landing. The browser URL and GoRouter
      // path are the same (`/event/{id}`) since the bundle is built
      // with `<base href="/">` and mounted at the apex domain.
      final isGoingToWebEvent = path.startsWith('/event/');
      // The Flutter Web gear (/item/{id}), request (/need/{id}), and community
      // (/group/{id}) guest landings — the non-RSVP analogs of /event/{id}
      // (#2492 WEB-3/WEB-4, #2875).
      final isGoingToWebItem =
          path.startsWith('/item/') ||
          path.startsWith('/need/') ||
          path.startsWith('/group/');
      final isGoingToAuthScreen =
          isGoingToLogin ||
          isGoingToRegister ||
          isGoingToVerifyPhone ||
          isGoingToForgotPassword ||
          isGoingToResetPassword ||
          isGoingToInvite ||
          isGoingToWebEvent ||
          isGoingToWebItem;

      // Wait for auth check to complete before making routing decisions.
      // The cold-start path doesn't usually reach this code: the
      // _GearAppState.build splash gate (splashProvider) keeps
      // MaterialApp.router from instantiating until loadAuthState()
      // resolves. This branch covers edge cases (hot reload, post-
      // logout transitions, late RPC-driven auth invalidation). Stay
      // on the current location — the screen above us continues to
      // render whatever it had; bouncing to /login here would
      // produce a login flash on web cold-starts where Firebase
      // Auth occasionally re-asserts isLoading after splash dismiss.
      // Once auth resolves, this redirect re-fires and the
      // !isAuthenticated path below routes accordingly.
      if (isLoading) {
        return null;
      }

      // Redirect to login if not authenticated (except if already going to auth screens)
      if (!isAuthenticated && !isGoingToAuthScreen) {
        // Check for deferred deep link context (e.g., from Play Install Referrer).
        // If the user just installed via a share link, route them to the invite
        // flow instead of the generic login screen. The deep-link target
        // is resolved from the share-link row via CheckInvitation, so only
        // the short code is forwarded (#2562).
        final deferred = DeferredDeepLinkContextHolder.value;
        if (deferred != null && deferred.shortCode != null) {
          // Consume the context so it doesn't re-trigger on subsequent redirects.
          DeferredDeepLinkContextHolder.consume();
          return '/invite?token=${Uri.encodeComponent(deferred.shortCode!)}';
        }

        // On first launch, go straight to registration — new installs need
        // an invite code, and the register screen shows the "Tap your invite
        // link again, or enter the code below" helper.
        if (FirstLaunchFlag.value) {
          FirstLaunchFlag.value = false;
          return '/register';
        }

        // Preserve the intended destination (including query params) in the 'from' parameter
        final intendedLocation = state.uri.toString();
        return '/login?from=${Uri.encodeComponent(intendedLocation)}';
      }

      // Redirect to home if authenticated and trying to access auth screens.
      //
      // Web guest auth-completion exception: a guest who authenticated
      // mid-flow on a phone-first screen (/verify-phone?experience_id=… /
      // ?gear_id=… / ?request_id=… / ?community_id=…) is handed back to the
      // destination view so the action handoff fires, rather than being
      // dropped on `/`. Doing this in the redirect (vs only the auth screen's
      // post-success context.go) protects against a race where setAuthState()
      // -> refreshListenable -> this redirect fires before the post-await
      // context.go.
      if (isAuthenticated &&
          (isGoingToLogin || isGoingToRegister || isGoingToVerifyPhone)) {
        if (kIsWeb) {
          // A web guest who authenticated mid-flow is handed back to the
          // destination view so the handoff fires: /event/{id} (RSVP),
          // /item/{id} (gear ExpressInterest), /need/{id} (request
          // OfferToFulfill), or /group/{id} (community join).
          // postRegistrationDestination owns that mapping.
          final experienceId = state.uri.queryParameters['experience_id'];
          final gearId = state.uri.queryParameters['gear_id'];
          final requestId = state.uri.queryParameters['request_id'];
          final communityId = state.uri.queryParameters['community_id'];
          final hasItem =
              (experienceId != null && experienceId.isNotEmpty) ||
              (gearId != null && gearId.isNotEmpty) ||
              (requestId != null && requestId.isNotEmpty) ||
              (communityId != null && communityId.isNotEmpty);
          if (hasItem) {
            return postRegistrationDestination(
              shortCode: state.uri.queryParameters['token'],
              experienceId: experienceId,
              gearId: gearId,
              requestId: requestId,
              communityId: communityId,
              communityTab: state.uri.queryParameters['tab'],
              rsvpIntention: state.uri.queryParameters['rsvp'],
            );
          }
        }
        return '/';
      }

      return null; // No redirect needed
    },

    // Refresh listenable - triggers redirect when auth or tutorial state changes
    refreshListenable: RouterRefreshNotifier(ref),

    routes: [
      GoRoute(
        name: 'login',
        path: '/login',
        builder: (context, state) {
          final from = state.uri.queryParameters['from'];
          return LoginScreen(redirectTo: from);
        },
      ),
      GoRoute(
        name: 'register',
        path: '/register',
        builder: (context, state) {
          // RegisterScreen is a new-user register surface only. Web guests
          // (event RSVP, gear borrow, request offer) go phone-first to
          // /verify-phone; the deep-link target resolves from the share-link
          // row via CheckInvitation, not the URL.
          final token = state.uri.queryParameters['token'];
          return RegisterScreen(
            shortCode: token,
          );
        },
      ),
      // Phone-first guest verify entry (#2492): the SSR-landing guest lands
      // here straight from the web item screen — "Confirm your phone to …",
      // skipping the old sign-in gate and register screen. Shared across event
      // RSVP, gear borrow/claim, and request offer. On successful registration
      // the auth-aware redirect above hands the guest to /event/{id},
      // /item/{id}, or /need/{id} so the matching action handoff fires.
      GoRoute(
        name: 'verify_phone',
        path: '/verify-phone',
        builder: (context, state) {
          return PhoneAuthScreen(
            mode: PhoneAuthMode.register,
            shortCode: state.uri.queryParameters['token'],
            experienceId: state.uri.queryParameters['experience_id'],
            gearId: state.uri.queryParameters['gear_id'],
            requestId: state.uri.queryParameters['request_id'],
            communityId: state.uri.queryParameters['community_id'],
            communityTab: state.uri.queryParameters['tab'],
            rsvpIntention: state.uri.queryParameters['rsvp'],
            eventName: state.uri.queryParameters['n'],
            eventImageUrl: state.uri.queryParameters['img'],
          );
        },
      ),
      GoRoute(
        name: 'forgot_password',
        path: '/forgot-password',
        builder: (context, state) => const ForgotPasswordScreen(),
      ),
      GoRoute(
        name: 'reset_password',
        path: '/reset-password',
        builder: (context, state) {
          final token = state.uri.queryParameters['token'];
          if (token == null) {
            // No token - redirect to login
            WidgetsBinding.instance.addPostFrameCallback((_) {
              context.go('/login');
            });
            return const Scaffold(
              body: Center(child: CircularProgressIndicator()),
            );
          }
          return ResetPasswordScreen(token: token);
        },
      ),
      // Handle /go/<code> short links - redirect to /invite with token param.
      // The deep-link target lives on the share-link row (read via
      // CheckInvitation), so the redirect only forwards the code. Legacy
      // `?gear_id=`/`?request_id=`/`?experience_id=` params are no longer
      // propagated (#2562).
      //
      // The `to` destination hint IS forwarded (#2876): unlike those legacy
      // params it doesn't name a target — the row still does that — it says
      // which surface of the resolved target the reader was promised. Dropping
      // it here would silently strand an app user on the community profile
      // while the same link takes a browser user to the discussion.
      GoRoute(
        name: 'short_link',
        path: '/go/:code',
        redirect: (context, state) {
          final code = state.pathParameters['code'] ?? '';
          final to = state.uri.queryParameters['to'] ?? '';
          final dest = to.isEmpty ? '' : '&to=${Uri.encodeComponent(to)}';
          return '/invite?token=${Uri.encodeComponent(code)}$dest';
        },
      ),
      // Handle invitation deep links (both ripls://invite and https://ripls.app/invite)
      GoRoute(
        name: 'invite',
        path: '/invite',
        builder: (context, state) {
          // The deep-link target lives on the share-link row (resolved via
          // CheckInvitation by RegisterScreen / the describe call below), so
          // /invite only needs the token. Legacy item-ID query params are no
          // longer read (#2562).
          final token = state.uri.queryParameters['token'];

          // Check if user is already authenticated
          final authState = ref.read(authStateProvider);
          final isAuthenticated = authState.isAuthenticated;

          if (token == null) {
            // No token - redirect to login (can't register without invitation)
            WidgetsBinding.instance.addPostFrameCallback((_) {
              if (isAuthenticated) {
                context.go('/');
              } else {
                context.go('/login');
              }
            });
            return const Scaffold(
              body: Center(child: CircularProgressIndicator()),
            );
          }

          if (!isAuthenticated) {
            // Unauthenticated invitees go straight to the register screen,
            // which now hosts the invitation hero, all auth methods, and an
            // "I already have an account" link in one place.
            return RegisterScreen(
              shortCode: token,
            );
          }

          // User is logged in. Resolve the share link first so we can branch
          // by target_kind: event-shaped links join the community via
          // EventInviteJoinGate (so an authenticated non-member isn't 403'd by
          // the community-scoped access gate, #2136/#2237) and then show the
          // event; everything else falls through to the existing auto-accept
          // path, which already joins.
          return FutureBuilder<InvitationCheckResult>(
            future:
                ref.read(shareLinkRepositoryProvider).describe(token),
            builder: (context, describeSnapshot) {
              if (describeSnapshot.connectionState !=
                  ConnectionState.done) {
                return const Scaffold(
                  body: Center(child: CircularProgressIndicator()),
                );
              }
              final describe = describeSnapshot.data;
              if (describe != null && describe.isEvent) {
                return EventInviteJoinGate(
                  shortCode: token,
                  experienceId: describe.targetId!,
                  communityId: describe.communityId,
                );
              }
              return FutureBuilder(
                future: ref
                    .read(communityServiceProvider)
                    .acceptInvitationLink(shortCode: token),
                builder: (context, snapshot) {
              if (snapshot.connectionState == ConnectionState.waiting) {
                return const Scaffold(
                  body: Center(
                    child: Column(
                      mainAxisAlignment: MainAxisAlignment.center,
                      children: [
                        CircularProgressIndicator(),
                        SizedBox(height: 16),
                        Text('Joining community...'),
                      ],
                    ),
                  ),
                );
              }

              if (snapshot.hasError) {
                // Check error type for specific messaging
                final errorMessage = snapshot.error.toString();
                final isCommunityFull = errorMessage.contains('Too many requests') ||
                    errorMessage.contains('capacity') ||
                    errorMessage.contains('full');
                final isRevoked = errorMessage.contains('revoked');
                final isInvalidToken = errorMessage.contains('invalid') ||
                    errorMessage.contains('Invalid');

                // Determine title and message based on error type
                String title;
                String message;
                IconData icon;
                Color iconColor;

                if (isCommunityFull) {
                  title = 'Community is Full';
                  message = 'This community has reached its maximum of 32 members. Communities are limited in size to maintain a close-knit atmosphere.';
                  icon = Icons.people;
                  iconColor = AppColors.statusWarning(context);
                } else if (isRevoked) {
                  title = 'Link Expired';
                  message = 'This invitation link is no longer active. Please ask the person who shared it to send you a new link.';
                  icon = Icons.link_off;
                  iconColor = AppColors.statusWarning(context);
                } else if (isInvalidToken) {
                  title = 'Invalid Link';
                  message = 'This invitation link is not valid. Please check the link and try again.';
                  icon = Icons.link_off;
                  iconColor = Colors.red;
                } else {
                  title = 'Unable to Join';
                  message = 'Failed to join community: $errorMessage';
                  icon = Icons.error_outline;
                  iconColor = Colors.red;
                }

                return Scaffold(
                  body: Center(
                    child: Padding(
                      padding: const EdgeInsets.all(24),
                      child: Column(
                        mainAxisAlignment: MainAxisAlignment.center,
                        children: [
                          Icon(icon, size: 64, color: iconColor),
                          const SizedBox(height: 16),
                          Text(
                            title,
                            style: const TextStyle(
                              fontSize: 24,
                              fontWeight: FontWeight.bold,
                            ),
                            textAlign: TextAlign.center,
                          ),
                          const SizedBox(height: 12),
                          Text(
                            message,
                            textAlign: TextAlign.center,
                            style: const TextStyle(fontSize: 16),
                          ),
                          const SizedBox(height: 24),
                          ElevatedButton(
                            onPressed: () => context.go('/'),
                            child: const Text('Go Home'),
                          ),
                        ],
                      ),
                    ),
                  ),
                );
              }

              // Success! Get the response with community details
              final response = snapshot.data!;

              // Log analytics event for community joined
              ref.read(observabilityServiceProvider).logAnalyticsEvent(
                    CommunityJoinedEvent(
                      communityId: response.communityId,
                      joinMethod: 'invite_link',
                    ),
                  );

              // Schedule navigation after build completes
              WidgetsBinding.instance.addPostFrameCallback((_) async {
                // Refresh communities
                final communities = await ref
                    .read(communityServiceProvider)
                    .listCommunities();
                await ref
                    .read(communitiesProvider.notifier)
                    .setCommunities(communities);

                // Navigate to home with the new community selected. The
                // deep-link target now lives on the share-link row (resolved
                // via CheckInvitation), and legacy `?gear_id=`-style in-app
                // item navigation went away with the query-param handling
                // (#2562). Typed item links already routed here to home.
                //
                // Exception: a link carrying the `to` destination hint asked
                // for a specific surface of the community — a "say hi"
                // notification promises the discussion — so honor it instead
                // of dropping the reader on home (#2876).
                if (!context.mounted) return;
                final to = state.uri.queryParameters['to'] ?? '';
                if (to.isNotEmpty && response.communityId.isNotEmpty) {
                  context.go('/group/${Uri.encodeComponent(response.communityId)}'
                      '?tab=${Uri.encodeComponent(to)}');
                  return;
                }
                context.go('/');
              });

              return const Scaffold(
                body: Center(child: CircularProgressIndicator()),
              );
            },
          );
            },
          );
        },
      ),
      GoRoute(
        name: 'home',
        path: '/',
        builder: (context, state) {
          final gearService = ref.read(gearServiceProvider);
          return HomeScreen(gearService: gearService);
        },
      ),
      GoRoute(
        name: 'gear_detail',
        path: '/gear/:id',
        builder: (context, state) {
          final gearId = state.pathParameters['id']!;
          final tabParam = state.uri.queryParameters['tab'];
          final initialTab = tabParam == 'chat' ? 2 : 0;
          // Wrap with PopScope to handle back button when accessed via deep link.
          // When there's no navigation stack (e.g., direct deep link), the back
          // button would show a black screen. PopScope intercepts and goes home.
          return PopScope(
            canPop: false,
            onPopInvokedWithResult: (didPop, result) {
              if (!didPop) {
                context.go('/');
              }
            },
            child: GearScreen(gearId: gearId, initialTab: initialTab),
          );
        },
      ),
      GoRoute(
        name: 'request_detail',
        path: '/request/:id',
        builder: (context, state) {
          final requestId = state.pathParameters['id']!;
          final tabParam = state.uri.queryParameters['tab'];
          final initialTab = tabParam == 'chat' ? 2 : 0;
          // Wrap with PopScope to handle back button when accessed via deep link.
          return PopScope(
            canPop: false,
            onPopInvokedWithResult: (didPop, result) {
              if (!didPop) {
                context.go('/');
              }
            },
            child: RequestScreen(requestId: requestId, initialTab: initialTab),
          );
        },
      ),
      GoRoute(
        name: 'experience_detail',
        path: '/experience/:id',
        builder: (context, state) {
          final experienceId = state.pathParameters['id']!;
          final tabParam = state.uri.queryParameters['tab'];
          final initialTab = tabParam == 'chat' ? 2 : 0;
          // Wrap with PopScope to handle back button when accessed via deep link.
          return PopScope(
            canPop: false,
            onPopInvokedWithResult: (didPop, result) {
              if (!didPop) {
                context.go('/');
              }
            },
            child: ExperienceScreen(
              experienceId: experienceId,
              initialTab: initialTab,
            ),
          );
        },
      ),
      // Deep-link target for the day-before-purge push notification
      // (#1716). In-app navigation to the same screen happens via
      // NavigationHelpers.pushWithSlide from the deleted-list, not
      // through this route.
      GoRoute(
        name: 'restore_community',
        path: '/settings/communities/:id/restore',
        builder: (context, state) {
          final communityId = state.pathParameters['id']!;
          return PopScope(
            canPop: false,
            onPopInvokedWithResult: (didPop, _) {
              if (!didPop) context.go('/');
            },
            child: RestoreCommunityScreenById(communityId: communityId),
          );
        },
      ),

      // Flutter Web guest entry point — reached only on web via the
      // browser URL `https://ripls.app/event/{experience_id}?rsvp=…&code=…`.
      // The bundle is served at the apex domain with `<base href="/">`,
      // so the browser URL and the GoRouter path are identical. The
      // mobile app uses ExperienceScreen via NavigationHelpers.pushScreen
      // instead and never visits this URL.
      GoRoute(
        name: 'web_event',
        path: '/event/:experienceId',
        builder: (context, state) {
          final experienceId = state.pathParameters['experienceId']!;
          return WebExperienceScreen(
            experienceId: experienceId,
            rsvpIntention: state.uri.queryParameters['rsvp'],
            shortCode: state.uri.queryParameters['code'],
            // Event name + hero image ride through from the SSR "I'm in"/"Maybe"
            // links so the phone-first RSVP screen can show "…to {event}" over a
            // dark-washed hero (#2492) — neither is fetchable pre-auth.
            eventName: state.uri.queryParameters['n'],
            eventImageUrl: state.uri.queryParameters['img'],
          );
        },
      ),

      // Flutter Web gear guest entry — reached on web via
      // `https://ripls.app/item/{gear_id}?intent=interest&code=…` (the gear
      // analog of /event/{id}, #2492 WEB-4). The mobile app uses GearScreen at
      // /gear/{id} and never visits this URL.
      GoRoute(
        name: 'web_gear',
        path: '/item/:gearId',
        builder: (context, state) {
          return WebItemActionScreen(
            kind: WebItemKind.gear,
            itemId: state.pathParameters['gearId']!,
            intent: state.uri.queryParameters['intent'],
            shortCode: state.uri.queryParameters['code'],
            itemName: state.uri.queryParameters['n'],
            itemImageUrl: state.uri.queryParameters['img'],
          );
        },
      ),

      // Flutter Web request guest entry — reached on web via
      // `https://ripls.app/need/{request_id}?intent=offer&code=…` (#2492
      // WEB-3). The mobile app uses RequestScreen at /request/{id}.
      GoRoute(
        name: 'web_request',
        path: '/need/:requestId',
        builder: (context, state) {
          return WebItemActionScreen(
            kind: WebItemKind.request,
            itemId: state.pathParameters['requestId']!,
            intent: state.uri.queryParameters['intent'],
            shortCode: state.uri.queryParameters['code'],
            itemName: state.uri.queryParameters['n'],
            itemImageUrl: state.uri.queryParameters['img'],
          );
        },
      ),

      // Flutter Web community guest entry — reached on web via
      // `https://ripls.app/group/{community_id}?intent=join&code=…`, the
      // community analog of /event, /item, and /need (#2875). Deliberately
      // NOT in apple-app-site-association: only /go/* is universal-linked,
      // because the point of this route is to stay in the browser.
      GoRoute(
        name: 'web_community',
        path: '/group/:communityId',
        builder: (context, state) {
          return WebCommunityScreen(
            communityId: state.pathParameters['communityId']!,
            intent: state.uri.queryParameters['intent'],
            shortCode: state.uri.queryParameters['code'],
            communityName: state.uri.queryParameters['n'],
            communityImageUrl: state.uri.queryParameters['img'],
            tab: state.uri.queryParameters['tab'],
          );
        },
      ),
    ],

    errorBuilder: (context, state) => Scaffold(
      appBar: AppBar(title: const Text('Page Not Found')),
      body: Center(
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            const Icon(Icons.error_outline, size: 64, color: Colors.grey),
            const SizedBox(height: 16),
            Text(
              'Page not found: ${state.matchedLocation}',
              textAlign: TextAlign.center,
              style: const TextStyle(fontSize: 16),
            ),
            const SizedBox(height: 24),
            ElevatedButton(
              onPressed: () => context.go('/'),
              child: const Text('Go Home'),
            ),
          ],
        ),
      ),
    ),
  );
});

/// RouterRefreshNotifier listens to auth and tutorial state changes and notifies
/// go_router to re-evaluate routes when either status changes.
class RouterRefreshNotifier extends ChangeNotifier {
  RouterRefreshNotifier(this.ref) {
    ref.listen(authStateProvider, (previous, next) {
      notifyListeners();
    });
  }

  final Ref ref;
}