import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:intl/intl.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/presentation/viewmodels/fulfill_modal_view_model.dart';
import 'package:ripls/presentation/viewmodels/impact_draft_notifier.dart';
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/completion/completion_widgets.dart';
import 'package:ripls/presentation/widgets/content/content_view_helpers.dart';
import 'package:ripls/presentation/widgets/impact/completion/co2_detail_modal.dart';
import 'package:ripls/presentation/widgets/impact/completion/completion_impact_bar.dart';
import 'package:ripls/presentation/widgets/impact/completion/quality_time_detail_modal.dart';
import 'package:ripls/presentation/widgets/impact/completion/value_detail_modal.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass_pickers.dart';
import 'package:ripls/presentation/widgets/provisional_user_avatar.dart';
import 'package:ripls/presentation/widgets/provisional_user_profile_sheet.dart';
import 'package:ripls/presentation/widgets/user_avatar.dart';
import 'package:ripls/services/post_creation_service.dart';
import 'package:ripls/services/providers.dart';

/// MarkFulfilledModal — dark glassmorphic bottom sheet for confirming a
/// request has been fulfilled.
///
/// Pre-populates the list of helpers from [offerers] and [contributors], lets
/// the owner toggle each one and add additional people, then calls
/// [markRequestFulfilled] on submit.
class MarkFulfilledModal extends ConsumerStatefulWidget {
  const MarkFulfilledModal({
    super.key,
    required this.requestId,
    required this.communityId,
    required this.requestTitle,
    required this.ownerId,
    this.offerers = const [],
    this.contributors = const [],
    this.sharedCommunityIds = const {},
  });

  final String requestId;
  final String communityId;
  final String requestTitle;

  /// The requester's user ID — excluded from community grid and search results.
  final String ownerId;
  final List<User> offerers;

  /// Free-form contributors from AddRequestContribution — merged with offerers
  /// in the helpers list so the requester can confirm them without searching.
  final List<User> contributors;

  /// All community IDs the request is shared with; passed to [DarkPersonSearch]
  /// so the quick-add grid and search fan out across every shared community.
  final Set<String> sharedCommunityIds;

  /// Shows the modal as a bottom sheet and refreshes the feed on success.
  static Future<void> show(
    BuildContext context, {
    required String requestId,
    required String communityId,
    required String requestTitle,
    required String ownerId,
    List<User> offerers = const [],
    List<User> contributors = const [],
    Set<String> sharedCommunityIds = const {},
  }) async {
    await showAccessibleModal<void>(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      builder: (_) => FractionallySizedBox(
        heightFactor: 0.92,
        child: MarkFulfilledModal(
          requestId: requestId,
          communityId: communityId,
          requestTitle: requestTitle,
          ownerId: ownerId,
          offerers: offerers,
          contributors: contributors,
          sharedCommunityIds: sharedCommunityIds,
        ),
      ),
    );
  }

  @override
  ConsumerState<MarkFulfilledModal> createState() => _MarkFulfilledModalState();
}

class _MarkFulfilledModalState extends ConsumerState<MarkFulfilledModal> {
  @override
  void initState() {
    super.initState();
    Future.microtask(() {
      if (!mounted) return;
      ref
          .read(fulfillModalProvider(widget.requestId).notifier)
          .initialize(
            widget.offerers,
            contributors: widget.contributors,
          );
      unawaited(
        ref.read(impactDraftProvider(widget.requestId).notifier).draftRequest(),
      );
    });
  }

  Future<void> _handleSubmit() async {
    final eventId = await ref
        .read(fulfillModalProvider(widget.requestId).notifier)
        .submit();
    if (!mounted || eventId == null) return;
    Navigator.of(context).pop();
    await ref.read(postCreationServiceProvider).handlePostCreation();
    if (!mounted) return;
    if (eventId.isNotEmpty) {
      ToastHelper.showServerUndo(
        context: context,
        message: 'Request marked fulfilled',
        undoLabel: 'Undo',
        onUndo: () => ref
            .read(requestRepositoryProvider)
            .undoMarkRequestFulfilled(communityEventId: eventId),
        onUndoSucceeded: () {
          ref.invalidate(requestRepositoryProvider);
        },
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(fulfillModalProvider(widget.requestId));

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
                    CompletionColors.glowColor(context),
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
                child: state.isLoading && state.offerers.isEmpty
                    ? Center(
                        child: CircularProgressIndicator(
                          color: CompletionColors.textSecondary(context),
                        ),
                      )
                    : _buildBody(state),
              ),
              _buildSubmitButton(state),
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

  Widget _buildBody(FulfillModalState state) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        _buildHeader(),
        _buildImpactBar(state.impactEstimate),
        _buildDateField(state),
        if (state.errorMessage != null) _buildErrorBanner(state.errorMessage!),
        Expanded(
          child: ListView(
            padding: const EdgeInsets.only(bottom: 8),
            children: [
              _buildHelperSection(state),
              DarkPersonSearch(
                communityId: widget.communityId,
                searchCommunityIds: widget.sharedCommunityIds.isNotEmpty
                    ? widget.sharedCommunityIds
                    : null,
                excludedMemberIds: {
                  widget.ownerId,
                  ...state.offerers.map((u) => u.id),
                },
                excludedProvisionalIds: state.provisionalHelpers
                    .map((s) => s.id)
                    .toSet(),
                onMemberSelected: (user) => ref
                    .read(fulfillModalProvider(widget.requestId).notifier)
                    .addMemberHelper(user),
                onProvisionalSelected: (prov) => ref
                    .read(fulfillModalProvider(widget.requestId).notifier)
                    .addProvisionalHelper(prov),
                onNewProvisionalRequested: (name) => ref
                    .read(fulfillModalProvider(widget.requestId).notifier)
                    .createAndAddProvisionalHelper(
                      communityId: widget.communityId,
                      name: name,
                    ),
                searchHint: 'Add another helper\u2026',
              ),
            ],
          ),
        ),
      ],
    );
  }

  Widget _buildHeader() {
    return Padding(
      padding: const EdgeInsets.fromLTRB(24, 20, 24, 16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // Intent framing: the request isn't fulfilled until the CTA is
          // tapped, so the eyebrow must not claim it already is (#2724).
          Text(
            context.l10n.markFulfilledEyebrow.toUpperCase(),
            style: TextStyle(
              fontSize: 11,
              fontWeight: FontWeight.w600,
              letterSpacing: 1.5,
              color: CompletionColors.eyebrow(context),
            ),
          ),
          const SizedBox(height: 6),
          Text(
            widget.requestTitle,
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

  Widget _buildErrorBanner(String message) {
    return Container(
      margin: const EdgeInsets.fromLTRB(16, 0, 16, 8),
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

  Widget _buildHelperSection(FulfillModalState state) {
    final hasHelpers =
        state.offerers.isNotEmpty || state.provisionalHelpers.isNotEmpty;

    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 20, 16, 0),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // The list arrives fully pre-checked, so the instruction names
          // the action a tap actually performs — removing someone (#2724).
          Text(
            hasHelpers
                ? context.l10n.markFulfilledConfirmHeader.toUpperCase()
                : 'WHO HELPED?',
            style: TextStyle(
              fontSize: 10,
              fontWeight: FontWeight.w600,
              letterSpacing: 1.2,
              color: CompletionColors.sectionLabel(context),
            ),
          ),
          const SizedBox(height: 10),
          ...state.offerers.map((user) {
            final confirmed = state.confirmedIds.contains(user.id);
            return DarkPersonTile(
              leading: UserAvatar(user: user, radius: 16),
              name: user.name,
              included: confirmed,
              onTap: () => ref
                  .read(fulfillModalProvider(widget.requestId).notifier)
                  .toggleHelper(user.id),
              onAvatarTap: () =>
                  ContentViewHelpers.openUserScreen(context, user.id),
            );
          }),
          ...state.provisionalHelpers.map((prov) {
            final confirmed = state.confirmedIds.contains(prov.id);
            return DarkPersonTile(
              leading: ProvisionalUserAvatar(name: prov.name, radius: 16),
              name: prov.name,
              subtitle: 'Not yet on Ripls',
              included: confirmed,
              onTap: () => ref
                  .read(fulfillModalProvider(widget.requestId).notifier)
                  .toggleHelper(prov.id),
              onAvatarTap: () => ProvisionalUserProfileSheet.show(
                context,
                provisionalUser: prov,
                communityId: widget.communityId,
              ),
            );
          }),
        ],
      ),
    );
  }

  Widget _buildDateField(FulfillModalState state) {
    final date = state.fulfilledAt;
    final now = DateTime.now();
    final today = DateTime(now.year, now.month, now.day);
    final isToday =
        date.year == today.year &&
        date.month == today.month &&
        date.day == today.day;

    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 0, 16, 8),
      child: Tappable(
        semanticsLabel: context.l10n.a11yReqChangeFulfillmentDate,
        onTap: () async {
          final picked = await showGlassDatePicker(
            context: context,
            initialDate: date,
            firstDate: today.subtract(const Duration(days: 365 * 5)),
            lastDate: today,
          );
          if (picked != null && mounted) {
            ref
                .read(fulfillModalProvider(widget.requestId).notifier)
                .setFulfilledAt(picked);
          }
        },
        child: AnimatedContainer(
          duration: accessibleDuration(
            context,
            const Duration(milliseconds: 200),
          ),
          padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
          decoration: BoxDecoration(
            color: CompletionColors.tileBackground(context, included: !isToday),
            borderRadius: BorderRadius.circular(12),
            border: Border.all(
              color: CompletionColors.tileBorder(context, included: !isToday),
            ),
          ),
          child: Row(
            children: [
              Icon(
                Icons.calendar_today_outlined,
                size: 15,
                color: isToday
                    ? CompletionColors.textDim(context)
                    : kCompletionAccentLight,
              ),
              const SizedBox(width: 10),
              Expanded(
                child: Text(
                  isToday
                      ? 'Fulfilled today'
                      : DateFormat('EEEE, MMMM d, yyyy').format(date),
                  style: TextStyle(
                    fontSize: 14,
                    color: isToday
                        ? CompletionColors.textSecondary(context)
                        : CompletionColors.textPrimary(context),
                  ),
                ),
              ),
              Text(
                'Change',
                style: TextStyle(
                  fontSize: 11,
                  color: CompletionColors.textDim(context),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }

  Widget _buildImpactBar(ImpactEstimate? fallbackImpact) {
    final draftState = ref.watch(impactDraftProvider(widget.requestId));
    return CompletionImpactBar(
      impact: draftState.draft ?? fallbackImpact,
      isLoading: draftState.isLoading || draftState.isRedrafting,
      onMoneyTap: () => ValueDetailModal.show(
        context,
        widget.requestId,
        isExperience: false,
      ),
      onQualityTimeTap: () => QualityTimeDetailModal.show(
        context,
        widget.requestId,
        isExperience: false,
      ),
      onCo2Tap: () => Co2DetailModal.show(
        context,
        widget.requestId,
        isExperience: false,
      ),
      moneySemanticsLabel: context.l10n.a11yReqViewMoneySaved,
      qualityTimeSemanticsLabel: context.l10n.a11yReqViewQualityTime,
      co2SemanticsLabel: context.l10n.a11yReqViewCo2,
    );
  }

  Widget _buildSubmitButton(FulfillModalState state) {
    final isSubmitting = state.isLoading;
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
          onPressed: isSubmitting ? null : _handleSubmit,
          style: ElevatedButton.styleFrom(
            padding: const EdgeInsets.symmetric(vertical: 16),
            // See past_transfer_modal: kCompletionGreen cannot carry a label at
            // 4.5:1 in either polarity (white 3.93, near-black 4.12).
            backgroundColor: GlassTokens.primary,
            disabledBackgroundColor: GlassTokens.primary.withValues(alpha: 0.4),
            foregroundColor: GlassTokens.onPrimary,
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
                      'Saving\u2026',
                      style: TextStyle(
                        fontSize: 15,
                        fontWeight: FontWeight.w600,
                      ),
                    ),
                  ],
                )
              : const Text(
                  'Mark Fulfilled',
                  style: TextStyle(fontSize: 16, fontWeight: FontWeight.w600),
                ),
        ),
      ),
    );
  }
}
