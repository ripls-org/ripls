import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/observability/events.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/screens/communities/community_public_screen.dart';
import 'package:ripls/presentation/viewmodels/event_invite_join_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/services/providers.dart';
import 'package:ripls/services/providers/auth_providers.dart';

/// Entry point for a community reached by link, mounted at
/// `/group/{communityId}` (with `?intent=join&code=…` hand-off params from the
/// SSR landing's CTA). The community analog of [WebItemActionScreen] (#2875).
///
/// **Not web-only, despite the name and the directory.** It began as the web
/// half of the phone-first community invite, but a member-joined push tap now
/// routes here too (#2876) — on mobile, with no share code, it skips the join
/// gate and renders [CommunityPublicScreen] directly, which is the same
/// community profile the rest of the app pushes. Keeping one entry point means
/// the `tab=discuss` destination is honored identically however the reader
/// arrived. (A rename to something platform-neutral is warranted; it's deferred
/// only because the `web-community-screen` semantics identifier is a published
/// e2e locator.)
///
/// **Auth state handling.** A guest who tapped the SSR CTA arrives here
/// unauthenticated with a join intent. They're routed straight to the
/// phone-first verify screen (`/verify-phone`); after they verify, the
/// auth-aware redirect (app_router.dart) returns them here authenticated and
/// the screen joins the community (idempotent `AcceptInvitationLink`) before
/// showing it. An already-authenticated visitor skips the detour and joins
/// directly — which is the case the install-only landing this replaced could
/// never serve at all.
///
/// Unlike the gear/request screens there is no separate action to auto-fire:
/// on a community invite the join *is* the action, so the join notifier is
/// both the gate and the payload.
class WebCommunityScreen extends ConsumerStatefulWidget {
  /// The community being joined.
  final String communityId;

  /// The action carrier from the SSR CTA (`join`). Its presence, with a share
  /// code, is what triggers the phone-first routing; absent, the screen just
  /// shows the community to an authenticated visitor.
  final String? intent;
  final String? shortCode;

  /// Group name + hero image threaded from the SSR landing's CTA, passed
  /// through to the phone-first screen for context (neither is fetchable
  /// pre-auth).
  final String? communityName;
  final String? communityImageUrl;

  /// Which surface of the community to land on. [tabDiscuss] opens the
  /// conversation on arrival — set by the SSR landing when the link carried
  /// the `to=discuss` destination hint, i.e. it came from a "say hi"
  /// notification whose copy promised the discussion (#2876).
  final String? tab;

  /// The one recognized [tab] value. Must match the server's
  /// `destinationDiscuss` (community_page.go) and
  /// `notifications.DestinationDiscuss`.
  static const tabDiscuss = 'discuss';

  const WebCommunityScreen({
    super.key,
    required this.communityId,
    this.intent,
    this.shortCode,
    this.communityName,
    this.communityImageUrl,
    this.tab,
  });

  @override
  ConsumerState<WebCommunityScreen> createState() => _WebCommunityScreenState();
}

class _WebCommunityScreenState extends ConsumerState<WebCommunityScreen> {
  bool _attemptedJoin = false;
  bool _routedToPhoneAuth = false;
  bool _loggedJoin = false;

  bool get _hasCode => (widget.shortCode ?? '').isNotEmpty;
  bool get _wantsJoin => (widget.intent ?? '').isNotEmpty && _hasCode;

  @override
  Widget build(BuildContext context) {
    final isAuthenticated = ref.watch(authStateProvider).isAuthenticated;

    // Phone-first: an unauthenticated guest with a join intent goes straight
    // to /verify-phone, carrying the community id so the post-verify redirect
    // returns here and the join completes.
    if (!isAuthenticated && _wantsJoin && !_routedToPhoneAuth) {
      _routedToPhoneAuth = true;
      final params = <String>[
        'token=${Uri.encodeComponent(widget.shortCode!)}',
        'community_id=${Uri.encodeComponent(widget.communityId)}',
        if ((widget.communityName ?? '').isNotEmpty)
          'n=${Uri.encodeComponent(widget.communityName!)}',
        if ((widget.communityImageUrl ?? '').isNotEmpty)
          'img=${Uri.encodeComponent(widget.communityImageUrl!)}',
        // Survives the verify detour so the guest still lands where the link
        // said, not just in the community.
        if ((widget.tab ?? '').isNotEmpty)
          'tab=${Uri.encodeComponent(widget.tab!)}',
      ].join('&');
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (!mounted) return;
        context.go('/verify-phone?$params');
      });
    }

    // Anchor the join provider to this screen so its in-flight state survives
    // rebuilds (mirrors WebItemActionScreen).
    if (_hasCode) {
      ref.watch(eventInviteJoinProvider(widget.shortCode!));

      // Log the join exactly once, from the same place /invite does, so
      // community-join volume doesn't under-report as traffic moves to this
      // flow. Fires on the idle→succeeded transition, which covers both the
      // just-registered guest and the already-authenticated visitor.
      ref.listen<EventInviteJoinState>(
        eventInviteJoinProvider(widget.shortCode!),
        (prev, next) {
          final justJoined = (prev?.hasSucceeded != true) && next.hasSucceeded;
          if (!justJoined || _loggedJoin) return;
          _loggedJoin = true;
          ref
              .read(observabilityServiceProvider)
              .logAnalyticsEvent(
                CommunityJoinedEvent(
                  communityId: widget.communityId,
                  joinMethod: 'invite_link',
                ),
              );
        },
      );
    }

    // First authenticated build with a share code: join before the community
    // content loads, so the content RPCs are authorized by the
    // community-scoped access gate. Idempotent server-side.
    if (isAuthenticated && _hasCode && !_attemptedJoin) {
      _attemptedJoin = true;
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (!mounted) return;
        ref.read(eventInviteJoinProvider(widget.shortCode!).notifier).join();
      });
    }

    return Scaffold(
      backgroundColor: AppColors.background(context),
      body: Semantics(
        explicitChildNodes: true,
        container: true,
        identifier: 'web-community-screen',
        child: Stack(
          children: [
            Positioned.fill(
              child: isAuthenticated
                  ? _buildAuthenticatedBody(context)
                  : (_wantsJoin
                        ? const Center(child: CircularProgressIndicator())
                        : _buildSignInPrompt(context)),
            ),
            Positioned(
              top: 0,
              left: 0,
              right: 0,
              child: SafeArea(
                child: Padding(
                  padding: const EdgeInsets.all(16),
                  child: Align(
                    alignment: Alignment.centerLeft,
                    child: _buildHomeButton(context),
                  ),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildAuthenticatedBody(BuildContext context) {
    // Gate the content behind the join when the visitor arrived via a share
    // link: joining first satisfies the community-scoped access gate.
    if (_hasCode) {
      final joinState = ref.watch(eventInviteJoinProvider(widget.shortCode!));
      if (joinState.hasFailed) {
        return _buildJoinFailed(context);
      }
      if (!joinState.hasSucceeded) {
        return _buildJoining(context);
      }
    }

    return CommunityPublicScreen(
      communityId: widget.communityId,
      openConversationOnLoad: widget.tab == WebCommunityScreen.tabDiscuss,
    );
  }

  Widget _buildHomeButton(BuildContext context) {
    return Tappable(
      semanticsLabel: context.l10n.a11yWebEventHome,
      onTap: () => context.go('/'),
      child: Container(
        padding: const EdgeInsets.all(8),
        decoration: const BoxDecoration(
          color: Color(0x66000000),
          shape: BoxShape.circle,
        ),
        child: const Icon(
          Icons.arrow_back_ios_new,
          color: Colors.white,
          size: 24,
        ),
      ),
    );
  }

  Widget _buildJoining(BuildContext context) {
    return SafeArea(
      child: Center(
        child: Semantics(
          liveRegion: true,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const CircularProgressIndicator(),
              const SizedBox(height: 16),
              Text(
                context.l10n.webEventJoiningCommunity,
                style: TextStyle(
                  fontSize: 15,
                  color: AppColors.textSecondary(context),
                ),
                textAlign: TextAlign.center,
              ),
            ],
          ),
        ),
      ),
    );
  }

  Widget _buildJoinFailed(BuildContext context) {
    return SafeArea(
      child: Center(
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 24, vertical: 32),
          child: Semantics(
            liveRegion: true,
            child: Tappable(
              semanticsLabel: context.l10n.a11yWebEventJoinRetry,
              onTap: () => ref
                  .read(eventInviteJoinProvider(widget.shortCode!).notifier)
                  .retry(),
              child: Column(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Icon(
                    Icons.error_outline,
                    size: 56,
                    color: AppColors.statusError(context),
                  ),
                  const SizedBox(height: 20),
                  Text(
                    context.l10n.webEventJoinFailedBanner,
                    style: TextStyle(
                      fontSize: 15,
                      color: AppColors.textPrimary(context),
                      height: 1.5,
                    ),
                    textAlign: TextAlign.center,
                  ),
                  const SizedBox(height: 16),
                  Icon(
                    Icons.refresh,
                    color: AppColors.primary(context),
                    size: 28,
                  ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }

  /// No join intent + unauthenticated (someone hit the bare community URL):
  /// route to login, preserving the destination.
  Widget _buildSignInPrompt(BuildContext context) {
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!mounted) return;
      final from = Uri.encodeComponent(GoRouterState.of(context).uri.toString());
      context.go('/login?from=$from');
    });
    return const Center(child: CircularProgressIndicator());
  }
}
