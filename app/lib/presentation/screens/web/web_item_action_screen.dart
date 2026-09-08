import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/presentation/screens/gear/gear_content_view.dart';
import 'package:ripls/presentation/screens/request/request_content_view.dart';
import 'package:ripls/presentation/viewmodels/event_invite_join_view_model.dart';
import 'package:ripls/presentation/viewmodels/request_view_model.dart';
import 'package:ripls/presentation/viewmodels/web_action_handoff_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/services/providers/auth_providers.dart';

/// Which non-RSVP item a [WebItemActionScreen] is for, and therefore which
/// content view + auto-fire action it composes.
enum WebItemKind { gear, request }

/// Web-only entry point for a shared gear or request, mounted at
/// `/item/{gearId}` or `/need/{requestId}` (with `?intent=interest|offer&code=…`
/// hand-off params from the SSR landing's CTA). The non-RSVP analog of
/// [WebExperienceScreen] (#2492, WEB-3/WEB-4).
///
/// **Auth state handling.** A guest who tapped the SSR CTA arrives here
/// unauthenticated with an action intent. They're routed straight to the
/// phone-first verify screen (`/verify-phone`); after they verify, the
/// auth-aware redirect (app_router.dart) returns them here authenticated, the
/// screen joins the item's ad-hoc community (idempotent
/// `AcceptInvitationLink`), and the matching action auto-fires —
/// `ExpressInterest` for gear, `OfferToFulfill` for a request. A failed action
/// surfaces a `LiveRegion` retry banner above the content.
class WebItemActionScreen extends ConsumerStatefulWidget {
  final WebItemKind kind;

  /// The gear_id (gear) or request_id (request).
  final String itemId;

  /// The action carrier from the SSR CTA — `interest` (gear) / `offer`
  /// (request). Its presence (with a share code) is what triggers the
  /// phone-first routing + the post-verify auto-fire; absent, the screen just
  /// shows the item to an authenticated visitor.
  final String? intent;
  final String? shortCode;

  /// Item name + hero image threaded from the SSR landing's CTA, passed through
  /// to the phone-first screen for context (neither is fetchable pre-auth).
  final String? itemName;
  final String? itemImageUrl;

  const WebItemActionScreen({
    super.key,
    required this.kind,
    required this.itemId,
    this.intent,
    this.shortCode,
    this.itemName,
    this.itemImageUrl,
  });

  @override
  ConsumerState<WebItemActionScreen> createState() =>
      _WebItemActionScreenState();
}

class _WebItemActionScreenState extends ConsumerState<WebItemActionScreen> {
  bool _attemptedAction = false;
  bool _attemptedJoin = false;
  bool _routedToPhoneAuth = false;

  bool get _hasCode => (widget.shortCode ?? '').isNotEmpty;
  bool get _wantsAction => (widget.intent ?? '').isNotEmpty && _hasCode;
  bool get _isGear => widget.kind == WebItemKind.gear;

  WebActionHandoffState _watchHandoff() {
    return _isGear
        ? ref.watch(webGearInterestHandoffProvider(widget.itemId))
        : ref.watch(webRequestOfferHandoffProvider(widget.itemId));
  }

  Future<void> _attemptAction() async {
    final shortCode = widget.shortCode!;
    if (_isGear) {
      await ref
          .read(webGearInterestHandoffProvider(widget.itemId).notifier)
          .attempt(shortCode: shortCode);
      return;
    }
    await ref
        .read(webRequestOfferHandoffProvider(widget.itemId).notifier)
        .attempt(shortCode: shortCode);
    if (!mounted) return;

    // Confirm the offer from server truth rather than the handoff notifier's
    // state — the autoDispose notifier can be torn down mid-flight, which
    // silently drops its succeeded transition (#2701). Reloading the request
    // makes the guest's own view show the OFFERING TO HELP row ("You"), and
    // the toast tells them the tap did something beyond creating an account.
    final requestNotifier = ref.read(requestProvider(widget.itemId).notifier);
    await requestNotifier.loadRequestDetails();
    if (!mounted) return;
    final me = ref.read(authStateProvider).user?.id;
    final offered = ref
            .read(requestProvider(widget.itemId))
            .requestDetails
            ?.offerers
            .any((u) => u.id == me) ??
        false;
    if (offered) {
      ToastHelper.showSuccess(context, context.l10n.webRequestOfferSentToast);
    }
  }

  void _retryAction() {
    final shortCode = widget.shortCode!;
    if (_isGear) {
      ref
          .read(webGearInterestHandoffProvider(widget.itemId).notifier)
          .retry(shortCode: shortCode);
    } else {
      ref
          .read(webRequestOfferHandoffProvider(widget.itemId).notifier)
          .retry(shortCode: shortCode);
    }
  }

  @override
  Widget build(BuildContext context) {
    final isAuthenticated = ref.watch(authStateProvider).isAuthenticated;

    // Phone-first: an unauthenticated guest with an action intent goes straight
    // to /verify-phone, carrying the item id so the post-verify redirect
    // returns here and the action auto-fires.
    if (!isAuthenticated && _wantsAction && !_routedToPhoneAuth) {
      _routedToPhoneAuth = true;
      final idParam = _isGear ? 'gear_id' : 'request_id';
      final params = <String>[
        'token=${Uri.encodeComponent(widget.shortCode!)}',
        '$idParam=${Uri.encodeComponent(widget.itemId)}',
        if ((widget.itemName ?? '').isNotEmpty)
          'n=${Uri.encodeComponent(widget.itemName!)}',
        if ((widget.itemImageUrl ?? '').isNotEmpty)
          'img=${Uri.encodeComponent(widget.itemImageUrl!)}',
      ].join('&');
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (!mounted) return;
        context.go('/verify-phone?$params');
      });
    }

    // Anchor the handoff + community-join providers to this screen so their
    // in-flight state survives rebuilds (mirrors WebExperienceScreen).
    if (_isGear) {
      ref.watch(webGearInterestHandoffProvider(widget.itemId));
    } else {
      ref.watch(webRequestOfferHandoffProvider(widget.itemId));
    }
    if (_hasCode) {
      ref.watch(eventInviteJoinProvider(widget.shortCode!));
    }

    // Fire the action only once the community join has succeeded, so the write
    // never races ahead of membership. New users join during registration;
    // this listener covers the already-authenticated visitor too.
    if (_wantsAction) {
      ref.listen<EventInviteJoinState>(
        eventInviteJoinProvider(widget.shortCode!),
        (prev, next) {
          final justJoined = (prev?.hasSucceeded != true) && next.hasSucceeded;
          if (!justJoined || _attemptedAction) return;
          _attemptedAction = true;
          WidgetsBinding.instance.addPostFrameCallback((_) {
            if (!mounted) return;
            _attemptAction();
          });
        },
      );
    }

    // First authenticated build with a share code: join the item's community
    // before the content loads, so the content RPCs (and the action) are
    // authorized by the community-scoped access gate. Idempotent server-side.
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
        identifier: _isGear ? 'web-gear-screen' : 'web-request-screen',
        child: Stack(
          children: [
            Positioned.fill(
              child: isAuthenticated
                  ? _buildAuthenticatedBody(context)
                  : (_wantsAction
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
    // Gate the content behind the community join when the visitor arrived via a
    // share link: joining first satisfies the community-scoped access gate.
    if (_hasCode) {
      final joinState = ref.watch(eventInviteJoinProvider(widget.shortCode!));
      if (joinState.hasFailed) {
        return _buildJoinFailed(context);
      }
      if (!joinState.hasSucceeded) {
        return _buildJoining(context);
      }
    }

    final handoff = _wantsAction ? _watchHandoff() : null;

    return Column(
      children: [
        if (handoff != null && handoff.hasFailed)
          _buildActionRetryBanner(context),
        Expanded(
          child: _isGear
              ? GearContentView(
                  gearId: widget.itemId,
                  showEditControls: false,
                  showOwnerInfo: true,
                )
              : RequestContentView(
                  requestId: widget.itemId,
                  showEditControls: false,
                  showFloatingActions: false,
                  showOwnerInfo: true,
                ),
        ),
      ],
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

  Widget _buildActionRetryBanner(BuildContext context) {
    return Semantics(
      liveRegion: true,
      child: Material(
        color: AppColors.statusError(context).withValues(alpha: 0.1),
        child: Tappable(
          semanticsLabel: context.l10n.a11yWebItemActionRetry,
          onTap: _retryAction,
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: 20, vertical: 14),
            child: Row(
              children: [
                Icon(Icons.refresh,
                    color: AppColors.statusError(context), size: 20),
                const SizedBox(width: 12),
                Expanded(
                  child: Text(
                    context.l10n.webItemActionRetryBanner,
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

  /// No action intent + unauthenticated (someone hit the bare item URL):
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
