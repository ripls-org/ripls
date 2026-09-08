import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/responsive.dart';
import 'package:ripls/presentation/screens/experience/experience_content_view.dart';
import 'package:ripls/presentation/viewmodels/event_invite_join_view_model.dart';
import 'package:ripls/presentation/viewmodels/experience_view_model.dart';
import 'package:ripls/presentation/viewmodels/web_event_rsvp_handoff_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/services/providers/auth_providers.dart';

/// Web-only entry point for an event detail surface, mounted at
/// `/event/{experience_id}` (with optional `?rsvp=yes|maybe&code=…`
/// hand-off params from the SSR landing's Yes/Maybe CTA buttons).
///
/// Composes the shared `ExperienceContentView` widget mobile already
/// uses, wrapped in a thin web chrome (no `BottomNav`, no
/// `SwipeToCloseMixin`, no FABs).
///
/// **Auth state handling.** Unauthenticated visitors see a CTA that
/// routes them through the existing `RegisterScreen` flow (email +
/// password, phone OTP via Firebase, Google OIDC) with the
/// `experience_id`, `token`, and `rsvp` URL params threaded through.
/// `RegisterScreen` redirects back to `/event/{id}?rsvp=…&code=…`
/// on successful registration so this screen's
/// `WebEventRsvpHandoffNotifier` can pick up the intention and fire
/// `RSVPToExperience` automatically. The user lands on the event view
/// with the RSVP already recorded; failed RSVPs surface a
/// `LiveRegion` retry banner above the content.
class WebExperienceScreen extends ConsumerStatefulWidget {
  final String experienceId;
  final String? rsvpIntention;
  final String? shortCode;

  /// Event name + hero image threaded from the SSR landing's RSVP links, passed
  /// straight through to the phone-first RSVP screen (#2492) for context — they
  /// aren't fetchable while the guest is unauthenticated.
  final String? eventName;
  final String? eventImageUrl;

  const WebExperienceScreen({
    super.key,
    required this.experienceId,
    this.rsvpIntention,
    this.shortCode,
    this.eventName,
    this.eventImageUrl,
  });

  @override
  ConsumerState<WebExperienceScreen> createState() =>
      _WebExperienceScreenState();
}

class _WebExperienceScreenState extends ConsumerState<WebExperienceScreen> {
  bool _attemptedAutoRsvp = false;
  bool _attemptedJoin = false;
  bool _routedToPhoneAuth = false;

  @override
  Widget build(BuildContext context) {
    final isAuthenticated = ref.watch(authStateProvider).isAuthenticated;
    final hasCode = widget.shortCode != null && widget.shortCode!.isNotEmpty;
    final wantsRsvp = widget.rsvpIntention != null && hasCode;

    // Phone-first onboarding (#2492): a guest who tapped "I'm in"/"Maybe" on the
    // SSR landing arrives here unauthenticated with an RSVP intention. Skip the
    // "Sign in to RSVP" gate AND the register screen — send them straight to the
    // phone-entry screen ("Confirm your phone to RSVP") at /verify-phone. After
    // they verify, the auth-aware redirect (app_router.dart) returns them to
    // /event/{id}?rsvp=…&code=… (now authenticated) to join + auto-RSVP.
    if (!isAuthenticated && wantsRsvp && !_routedToPhoneAuth) {
      _routedToPhoneAuth = true;
      final params = <String>[
        'token=${Uri.encodeComponent(widget.shortCode!)}',
        'experience_id=${Uri.encodeComponent(widget.experienceId)}',
        'rsvp=${Uri.encodeComponent(widget.rsvpIntention!)}',
        if ((widget.eventName ?? '').isNotEmpty)
          'n=${Uri.encodeComponent(widget.eventName!)}',
        if ((widget.eventImageUrl ?? '').isNotEmpty)
          'img=${Uri.encodeComponent(widget.eventImageUrl!)}',
      ].join('&');
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (!mounted) return;
        context.go('/verify-phone?$params');
      });
    }

    // Anchor the autoDispose lifecycles of the handoff (and, when a share
    // code is present, the community-join) providers to this screen so their
    // in-flight state survives rebuilds even when their surfaces aren't
    // rendering. (See PR #2122 review §S1.)
    ref.watch(webEventRsvpHandoffProvider(widget.experienceId));
    if (hasCode) {
      ref.watch(eventInviteJoinProvider(widget.shortCode!));
    }

    // When the auto-RSVP succeeds, trigger a silent refresh on the
    // experience view-model so the on-screen RSVP toggle reflects the
    // just-written intention (the view-model loaded its snapshot before the
    // write). Visible confirmation comes from the toggle flipping to "Yes".
    ref.listen<WebRsvpHandoffState>(
      webEventRsvpHandoffProvider(widget.experienceId),
      (prev, next) {
        final justSucceeded = (prev?.hasSucceeded != true) && next.hasSucceeded;
        if (!justSucceeded) return;
        ref
            .read(experienceProvider(widget.experienceId).notifier)
            .refreshExperienceDetails();
      },
    );

    // Fire the auto-RSVP only once the community join has succeeded, so the
    // write never races ahead of membership (#2237). New users join during
    // registration; this listener covers the already-authenticated visitor.
    if (wantsRsvp) {
      ref.listen<EventInviteJoinState>(
        eventInviteJoinProvider(widget.shortCode!),
        (prev, next) {
          final justJoined = (prev?.hasSucceeded != true) && next.hasSucceeded;
          if (!justJoined || _attemptedAutoRsvp) return;
          _attemptedAutoRsvp = true;
          WidgetsBinding.instance.addPostFrameCallback((_) {
            if (!mounted) return;
            ref
                .read(webEventRsvpHandoffProvider(widget.experienceId).notifier)
                .attempt(
                  rsvpIntention: widget.rsvpIntention!,
                  shortCode: widget.shortCode!,
                );
          });
        },
      );
    }

    // First authenticated build with a share code: join the event's community
    // before the event view loads, so GetExperience / RSVPToExperience are
    // authorized by the community-scoped access gate (#2136/#2237). The server
    // is idempotent, so an existing member falls straight through.
    if (isAuthenticated && hasCode && !_attemptedJoin) {
      _attemptedJoin = true;
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (!mounted) return;
        ref.read(eventInviteJoinProvider(widget.shortCode!).notifier).join();
      });
    }

    return Scaffold(
      backgroundColor: AppColors.background(context),
      body: Semantics(
        // Anchor identifier for the e2e Playwright harness (#2162).
        // explicitChildNodes: true forces the engine to keep this
        // node distinct from its descendants instead of merging the
        // identifier into the child tree (where it gets dropped from
        // DOM serialization). container: true alone wasn't enough.
        explicitChildNodes: true,
        container: true,
        identifier: 'web-event-screen',
        child: Stack(
          children: [
            Positioned.fill(
              child: isAuthenticated
                  ? _buildAuthenticatedBody(context)
                  : (wantsRsvp
                        // Routing to the phone-first screen — show a spinner
                        // rather than flashing the sign-in gate.
                        ? const Center(child: CircularProgressIndicator())
                        : _buildAuthPendingPlaceholder(context)),
            ),
            // Persistent home affordance. Web guests arrive here from
            // the SSR /go/{code} page and have no in-app history to
            // navigate back through, so this overlay is their always-
            // available escape hatch into the rest of the app. Mirrors
            // mobile's upper-left back chevron in styling, but routes
            // to `/` instead of popping a Navigator.
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

  Widget _buildAuthenticatedBody(BuildContext context) {
    // Gate the event content behind the community join when the visitor
    // arrived via a share link: joining first satisfies the community-scoped
    // access gate (#2136/#2237) so ExperienceContentView's GetExperience and
    // the RSVP handoff are authorized.
    if (widget.shortCode != null && widget.shortCode!.isNotEmpty) {
      final joinState = ref.watch(eventInviteJoinProvider(widget.shortCode!));
      if (joinState.hasFailed) {
        return _buildJoinFailed(context);
      }
      if (!joinState.hasSucceeded) {
        return _buildJoining(context);
      }
    }

    // The top-level `ref.watch` in `build()` already anchored the
    // handoff provider's lifecycle; here we just inspect the state to
    // decide whether to render the retry banner. The banner only
    // surfaces when an rsvp intention was passed in via URL AND the
    // auto-RSVP exhausted its retries.
    final hasRsvpContext =
        widget.rsvpIntention != null &&
        widget.shortCode != null &&
        widget.shortCode!.isNotEmpty;
    final handoffState = hasRsvpContext
        ? ref.watch(webEventRsvpHandoffProvider(widget.experienceId))
        : null;

    return Column(
      children: [
        if (handoffState != null && handoffState.hasFailed)
          _buildRsvpRetryBanner(context),
        Expanded(
          child: ExperienceContentView(
            experienceId: widget.experienceId,
            // Guests don't own the event — hide owner-only edit
            // affordances and the floating action buttons that
            // overlay the content body.
            showEditControls: false,
            showFloatingActions: false,
            showOwnerInfo: true,
          ),
        ),
      ],
    );
  }

  Widget _buildRsvpRetryBanner(BuildContext context) {
    // `LiveRegion` reannounces this banner whenever its child rebuilds
    // — useful because the banner is the failure surface for the auto-
    // RSVP write, and a screen reader should hear about it on first
    // appearance.
    return Semantics(
      liveRegion: true,
      child: Material(
        color: AppColors.statusError(context).withValues(alpha: 0.1),
        child: Tappable(
          semanticsLabel: context.l10n.a11yWebEventRsvpRetry,
          onTap: () => ref
              .read(webEventRsvpHandoffProvider(widget.experienceId).notifier)
              .retry(
                rsvpIntention: widget.rsvpIntention!,
                shortCode: widget.shortCode!,
              ),
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: 20, vertical: 14),
            child: Row(
              children: [
                Icon(
                  Icons.refresh,
                  color: AppColors.statusError(context),
                  size: 20,
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: Text(
                    context.l10n.webEventRsvpRetryBanner,
                    style: TextStyle(
                      fontSize: 14,
                      color: AppColors.textPrimary(context),
                    ),
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }

  /// Shown while the authenticated visitor is being added to the event's
  /// community (idempotent AcceptInvitationLink) before the event loads.
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

  /// Shown when the community join failed; the whole card retries the join
  /// (mirrors `_buildRsvpRetryBanner`).
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

  Widget _buildAuthPendingPlaceholder(BuildContext context) {
    return SafeArea(
      child: Center(
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 24, vertical: 32),
          // Cap the copy at the reading measure on desktop-wide windows
          // (#2912); no-op at phone widths.
          child: ConstrainedBox(
            constraints: const BoxConstraints(
              maxWidth: Responsive.contentMaxWidth,
            ),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.center,
              children: [
                Icon(
                  Icons.event_outlined,
                  size: 56,
                  color: AppColors.textSecondary(context),
                ),
                const SizedBox(height: 20),
                Text(
                  context.l10n.webEventAuthPlaceholderTitle,
                  style: TextStyle(
                    fontSize: 20,
                    fontWeight: FontWeight.w600,
                    color: AppColors.textPrimary(context),
                  ),
                  textAlign: TextAlign.center,
                ),
                const SizedBox(height: 12),
                Text(
                  context.l10n.webEventAuthPlaceholderBody,
                  style: TextStyle(
                    fontSize: 15,
                    color: AppColors.textSecondary(context),
                    height: 1.5,
                  ),
                  textAlign: TextAlign.center,
                ),
                const SizedBox(height: 24),
                Tappable(
                  semanticsLabel: context.l10n.a11yWebEventSignInToRsvp,
                  onTap: () => _navigateToLogin(context),
                  child: Container(
                    padding: const EdgeInsets.symmetric(
                      horizontal: 32,
                      vertical: 14,
                    ),
                    decoration: BoxDecoration(
                      color: AppColors.primary(context),
                      borderRadius: BorderRadius.circular(28),
                    ),
                    child: Text(
                      context.l10n.webEventSignInToRsvp,
                      style: const TextStyle(
                        fontSize: 16,
                        fontWeight: FontWeight.w600,
                        color: Colors.white,
                      ),
                    ),
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }

  /// Sends the guest to sign in, preserving the current event URL so login
  /// returns them here to join + RSVP. This is the no-RSVP-intention fallback
  /// gate; the primary path (an "I'm in"/"Maybe" tap) goes phone-first to
  /// /verify-phone above. Matches the gear/request web guests
  /// (`WebItemActionScreen`): the item/event context lives on the route, and
  /// sign-in routes to `/login` rather than a separate register screen (the
  /// event context resolves from the share-link row via CheckInvitation).
  void _navigateToLogin(BuildContext context) {
    final from = Uri.encodeComponent(GoRouterState.of(context).uri.toString());
    context.go('/login?from=$from');
  }
}
