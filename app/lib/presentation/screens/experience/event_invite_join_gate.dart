import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/screens/experience/experience_screen.dart';
import 'package:ripls/presentation/viewmodels/event_invite_join_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// Joins the event's community (idempotent AcceptInvitationLink) before showing
/// the event, for an authenticated visitor who followed an event share link to
/// a community they're not yet in (#2237). Without the join, ExperienceScreen's
/// GetExperience is rejected by the community-scoped access gate (#2136) for a
/// non-member.
///
/// Used by the `/invite` event branch, which both Android/iOS and the web SPA
/// reach (mobile never visits the web-only `/event/{id}` route). New users join
/// during registration via the short_code (2050 §D1) and never hit this gate.
class EventInviteJoinGate extends ConsumerStatefulWidget {
  final String shortCode;
  final String experienceId;
  final String? communityId;

  const EventInviteJoinGate({
    super.key,
    required this.shortCode,
    required this.experienceId,
    this.communityId,
  });

  @override
  ConsumerState<EventInviteJoinGate> createState() =>
      _EventInviteJoinGateState();
}

class _EventInviteJoinGateState extends ConsumerState<EventInviteJoinGate> {
  bool _started = false;

  @override
  Widget build(BuildContext context) {
    final joinState = ref.watch(eventInviteJoinProvider(widget.shortCode));

    // Kick off the join once, after the first frame so the build completes
    // before any notifier state write.
    if (!_started) {
      _started = true;
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (!mounted) return;
        ref.read(eventInviteJoinProvider(widget.shortCode).notifier).join();
      });
    }

    if (joinState.hasSucceeded) {
      return ExperienceScreen(
        experienceId: widget.experienceId,
        initialCommunityId: widget.communityId,
      );
    }
    if (joinState.hasFailed) {
      return _buildError(context);
    }
    return _buildJoining(context);
  }

  Widget _buildJoining(BuildContext context) {
    return Scaffold(
      body: SafeArea(
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
      ),
    );
  }

  Widget _buildError(BuildContext context) {
    return Scaffold(
      body: SafeArea(
        child: Center(
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: 24, vertical: 32),
            child: Semantics(
              liveRegion: true,
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
                  const SizedBox(height: 24),
                  Tappable(
                    semanticsLabel: context.l10n.a11yWebEventJoinRetry,
                    onTap: () => ref
                        .read(eventInviteJoinProvider(widget.shortCode).notifier)
                        .retry(),
                    child: Container(
                      padding: const EdgeInsets.symmetric(
                        horizontal: 32,
                        vertical: 14,
                      ),
                      decoration: BoxDecoration(
                        color: AppColors.primary(context),
                        borderRadius: BorderRadius.circular(28),
                      ),
                      child: Icon(
                        Icons.refresh,
                        color: AppColors.onPrimary(context),
                        size: 24,
                      ),
                    ),
                  ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}
