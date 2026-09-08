import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/data/gen/ripls/api/experience_service.pbenum.dart';
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart';
import 'package:ripls/presentation/viewmodels/community_impact_view_model.dart';
import 'package:ripls/presentation/viewmodels/event_modal_state.dart';
import 'package:ripls/presentation/viewmodels/event_modal_view_model.dart';
import 'package:ripls/presentation/viewmodels/impact_draft_notifier.dart';
import 'package:ripls/presentation/viewmodels/search_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/completion/completion_widgets.dart';
import 'package:ripls/presentation/widgets/content/content_view_helpers.dart';
import 'package:ripls/presentation/widgets/impact/completion/co2_detail_modal.dart';
import 'package:ripls/presentation/widgets/impact/completion/completion_impact_bar.dart';
import 'package:ripls/presentation/widgets/impact/completion/quality_time_detail_modal.dart';
import 'package:ripls/presentation/widgets/impact/completion/value_detail_modal.dart';
import 'package:ripls/presentation/widgets/provisional_user_avatar.dart';
import 'package:ripls/presentation/widgets/provisional_user_profile_sheet.dart';
import 'package:ripls/presentation/widgets/user_avatar.dart';
import 'package:ripls/services/post_creation_service.dart';
import 'package:ripls/services/providers.dart';

/// MarkCompletedModal — unified completion screen for an experience.
///
/// Shows a dark-themed modal with a live impact bar, attendance confirmation
/// list, a quick-add community grid, and an inline search for adding anyone.
/// Replaces the previous two-phase wrap-up → complete flow.
class MarkCompletedModal extends ConsumerStatefulWidget {
  const MarkCompletedModal({
    super.key,
    required this.experienceId,
    required this.communityId,
    this.preTaggedUsers = const [],
    this.preProvisionalUsers = const [],
    this.sharedCommunityIds = const {},
  });

  final String experienceId;
  final String communityId;

  /// Registered member User objects pre-resolved from AI-mentioned names.
  final List<User> preTaggedUsers;

  /// Provisional users pre-created from AI-mentioned names with no member match.
  final List<ProvisionalUser> preProvisionalUsers;

  /// All community IDs the experience is shared with; passed to [DarkPersonSearch]
  /// so the quick-add grid and search fan out across every shared community.
  final Set<String> sharedCommunityIds;

  /// Shows the modal as a bottom sheet.
  static Future<void> show(
    BuildContext context,
    String experienceId,
    String communityId, {
    List<User> preTaggedUsers = const [],
    List<ProvisionalUser> preProvisionalUsers = const [],
    Set<String> sharedCommunityIds = const {},
  }) async {
    await showAccessibleModal<void>(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      builder: (_) => FractionallySizedBox(
        heightFactor: 0.92,
        child: MarkCompletedModal(
          experienceId: experienceId,
          communityId: communityId,
          preTaggedUsers: preTaggedUsers,
          preProvisionalUsers: preProvisionalUsers,
          sharedCommunityIds: sharedCommunityIds,
        ),
      ),
    );
  }

  @override
  ConsumerState<MarkCompletedModal> createState() => _MarkCompletedModalState();
}

class _MarkCompletedModalState extends ConsumerState<MarkCompletedModal> {
  @override
  void initState() {
    super.initState();
    Future.microtask(() async {
      if (!mounted) return;
      await ref
          .read(eventModalProvider(widget.experienceId).notifier)
          .initialize(
            communityId: widget.communityId,
            preTaggedUsers: widget.preTaggedUsers,
            preProvisionalUsers: widget.preProvisionalUsers,
          );
      if (!mounted) return;
      unawaited(
        ref
            .read(impactDraftProvider(widget.experienceId).notifier)
            .draftExperience(),
      );
    });
  }

  Future<void> _handleComplete() async {
    final eventId = await ref
        .read(eventModalProvider(widget.experienceId).notifier)
        .wrapUp();
    if (!mounted) return;
    final state = ref.read(eventModalProvider(widget.experienceId));
    if (state.error != null) return;

    // Refresh community impact metrics to reflect the new completion.
    unawaited(
      ref.read(communityImpactProvider(widget.communityId).notifier).refresh(),
    );

    // Clear cached discover search results so the completed experience is
    // no longer shown in the Upcoming section.
    ref.invalidate(searchProvider);

    // Refresh feed, navigate to tab 0, and scroll to top — same flow used
    // by all content creation (gear, request, experience).
    // Story generation is now synchronous on the server (part of RecordAttendance),
    // so by the time handlePostCreation() refreshes the feed the story already exists.
    await ref.read(postCreationServiceProvider).handlePostCreation();

    if (!mounted) return;
    Navigator.of(context).pop();
    if (eventId != null && eventId.isNotEmpty && mounted) {
      ToastHelper.showServerUndo(
        context: context,
        message: context.l10n.eventWrappedUpToast,
        undoLabel: 'Undo',
        onUndo: () => ref
            .read(experienceRepositoryProvider)
            .undoCompleteExperience(communityEventId: eventId),
        onUndoSucceeded: () {
          ref.invalidate(eventModalProvider(widget.experienceId));
        },
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(eventModalProvider(widget.experienceId));

    return Container(
      decoration: BoxDecoration(
        color: CompletionColors.background(context),
        borderRadius: const BorderRadius.vertical(top: Radius.circular(20)),
      ),
      clipBehavior: Clip.hardEdge,
      child: Stack(
        children: [
          // Decorative glow
          Positioned(
            top: -40,
            right: -40,
            child: Container(
              width: 180,
              height: 180,
              decoration: BoxDecoration(
                shape: BoxShape.circle,
                gradient: RadialGradient(
                  colors: [
                    AppColors.transferCoral.withValues(alpha: 0.18),
                    Colors.transparent,
                  ],
                ),
              ),
            ),
          ),
          Column(
            children: [
              _buildHandle(),
              Expanded(
                child: state.isLoading
                    ? Center(
                        child: CircularProgressIndicator(
                          color: CompletionColors.textSecondary(context),
                        ),
                      )
                    : _buildBody(state),
              ),
              _buildCompleteButton(state),
            ],
          ),
        ],
      ),
    );
  }

  Widget _buildHandle() {
    return Padding(
      padding: const EdgeInsets.only(top: 12, bottom: 4),
      child: Center(
        child: Container(
          width: 36,
          height: 4,
          decoration: BoxDecoration(
            color: CompletionColors.handle(context),
            borderRadius: BorderRadius.circular(2),
          ),
        ),
      ),
    );
  }

  Widget _buildBody(EventModalState state) {
    final experience = state.experience;
    if (experience == null) return const SizedBox.shrink();

    // Already-added user IDs for filtering community suggestions.
    final addedIds = {
      ...state.rsvps.map((r) => r.user.id),
      ...state.extraMemberAttendees.map((u) => u.id),
    };
    final addedProvisionalIds = state.provisionalAttendees
        .map((s) => s.id)
        .toSet();

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        _buildHeader(experience.name),
        _buildImpactBar(state.impactEstimate),
        if (state.error != null)
          _buildErrorBanner(
            RpcErrorHandler.localize(state.error!, context.l10n),
          ),
        Expanded(
          child: ListView(
            padding: const EdgeInsets.only(bottom: 8),
            children: [
              _buildAttendeeSection(state),
              DarkPersonSearch(
                communityId: widget.communityId,
                searchCommunityIds: widget.sharedCommunityIds.isNotEmpty
                    ? widget.sharedCommunityIds
                    : null,
                excludedMemberIds: addedIds,
                excludedProvisionalIds: addedProvisionalIds,
                onMemberSelected: (user) => ref
                    .read(eventModalProvider(widget.experienceId).notifier)
                    .addMemberAttendee(user),
                onProvisionalSelected: (prov) => ref
                    .read(eventModalProvider(widget.experienceId).notifier)
                    .addExistingProvisionalAttendee(prov),
                onNewProvisionalRequested: (name) => ref
                    .read(eventModalProvider(widget.experienceId).notifier)
                    .addProvisionalAttendee(
                      communityId: widget.communityId,
                      name: name,
                    ),
                searchHint: context.l10n.markCompletedAddPersonSearch,
              ),
            ],
          ),
        ),
      ],
    );
  }

  Widget _buildHeader(String title) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(24, 20, 24, 16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // Intent framing: the event isn't completed until the CTA is
          // tapped, so the eyebrow must not claim it already is (#2724).
          Text(
            context.l10n.markCompletedEyebrow.toUpperCase(),
            style: TextStyle(
              fontSize: 11,
              fontWeight: FontWeight.w600,
              letterSpacing: 1.5,
              color: CompletionColors.eyebrow(context),
            ),
          ),
          const SizedBox(height: 6),
          Text(
            title,
            style: TextStyle(
              fontFamily: AppTheme.headingFont,
              fontSize: 24,
              color: CompletionColors.textPrimary(context),
              height: 1.2,
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildImpactBar(ImpactEstimate? fallbackImpact) {
    final draftState = ref.watch(impactDraftProvider(widget.experienceId));
    return CompletionImpactBar(
      impact: draftState.draft ?? fallbackImpact,
      isLoading: draftState.isLoading || draftState.isRedrafting,
      onMoneyTap: () => ValueDetailModal.show(
        context,
        widget.experienceId,
        isExperience: true,
      ),
      onQualityTimeTap: () => QualityTimeDetailModal.show(
        context,
        widget.experienceId,
        isExperience: true,
      ),
      onCo2Tap: () => Co2DetailModal.show(
        context,
        widget.experienceId,
        isExperience: true,
      ),
    );
  }

  Widget _buildErrorBanner(String message) {
    return Container(
      margin: const EdgeInsets.fromLTRB(16, 8, 16, 0),
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: Colors.red.withValues(alpha: 0.15),
        borderRadius: BorderRadius.circular(10),
        border: Border.all(color: Colors.red.withValues(alpha: 0.3)),
      ),
      child: Text(
        message,
        style: const TextStyle(fontSize: 13, color: AppColors.errorBannerText),
      ),
    );
  }

  Widget _buildAttendeeSection(EventModalState state) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 20, 16, 0),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // The list arrives fully pre-checked, so the instruction names
          // the action a tap actually performs — removing someone (#2724).
          Text(
            context.l10n.markCompletedConfirmHeader.toUpperCase(),
            style: TextStyle(
              fontSize: 10,
              fontWeight: FontWeight.w600,
              letterSpacing: 1.2,
              color: CompletionColors.sectionLabel(context),
            ),
          ),
          const SizedBox(height: 10),

          // RSVP yes attendees
          ...state.rsvps
              .where((r) => r.intention == RSVPIntention.RSVP_INTENTION_YES)
              .map((rsvp) {
                final attended = state.attendanceMap[rsvp.user.id] ?? false;
                return DarkPersonTile(
                  leading: UserAvatar(user: rsvp.user, radius: 16),
                  name: rsvp.user.name,
                  included: attended,
                  onTap: () => ref
                      .read(eventModalProvider(widget.experienceId).notifier)
                      .toggleAttendance(rsvp.user.id),
                  onAvatarTap: () =>
                      ContentViewHelpers.openUserScreen(context, rsvp.user.id),
                );
              }),

          // Provisional attendees
          ...state.provisionalAttendees.map((prov) {
            final attended = state.provisionalAttendanceMap[prov.id] ?? false;
            return DarkPersonTile(
              leading: ProvisionalUserAvatar(name: prov.name, radius: 16),
              name: prov.name,
              subtitle: 'Not yet on Ripls',
              included: attended,
              onTap: () => ref
                  .read(eventModalProvider(widget.experienceId).notifier)
                  .toggleProvisionalAttendance(prov.id),
              onAvatarTap: () => ProvisionalUserProfileSheet.show(
                context,
                provisionalUser: prov,
                communityId: widget.communityId,
              ),
            );
          }),

          // Extra member attendees (added via search)
          ...state.extraMemberAttendees.map((user) {
            final attended = state.attendanceMap[user.id] ?? false;
            return DarkPersonTile(
              leading: UserAvatar(user: user, radius: 16),
              name: user.name,
              included: attended,
              onTap: () => ref
                  .read(eventModalProvider(widget.experienceId).notifier)
                  .toggleAttendance(user.id),
              onAvatarTap: () =>
                  ContentViewHelpers.openUserScreen(context, user.id),
            );
          }),
        ],
      ),
    );
  }

  Widget _buildCompleteButton(EventModalState state) {
    final isSubmitting = state.isLoading && state.experience != null;
    return Container(
      padding: EdgeInsets.fromLTRB(
        16,
        12,
        16,
        MediaQuery.of(context).padding.bottom + 16,
      ),
      color: CompletionColors.bottomBar(context),
      child: SizedBox(
        width: double.infinity,
        child: ElevatedButton(
          onPressed: isSubmitting ? null : _handleComplete,
          style: ElevatedButton.styleFrom(
            padding: const EdgeInsets.symmetric(vertical: 16),
            // `onPrimary`, not `textPrimary`. The fill here is the glass
            // primary — a LIGHT sage — and the sheet's white body colour on top
            // of it measured 1.86:1.
            backgroundColor: GlassTokens.primary,
            disabledBackgroundColor: GlassTokens.primary.withValues(
              alpha: 0.4,
            ),
            foregroundColor: GlassTokens.onPrimary,
            // Pill, like every other primary CTA. This one was a 14px squircle.
            shape: const StadiumBorder(),
            elevation: 0,
          ),
          child: isSubmitting
              ? const Row(
                  mainAxisAlignment: MainAxisAlignment.center,
                  children: [
                    SizedBox(
                      width: 16,
                      height: 16,
                      child: CircularProgressIndicator(
                        strokeWidth: 2,
                        color: GlassTokens.onPrimary,
                      ),
                    ),
                    SizedBox(width: 10),
                    Text(
                      'Crafting your story…',
                      style: TextStyle(
                        fontSize: 15,
                        fontWeight: FontWeight.w600,
                      ),
                    ),
                  ],
                )
              : const Text(
                  'Complete',
                  style: TextStyle(fontSize: 16, fontWeight: FontWeight.w600),
                ),
        ),
      ),
    );
  }
}
