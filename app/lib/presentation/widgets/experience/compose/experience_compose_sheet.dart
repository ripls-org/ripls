// Bottom-sheet entry point for the experience compose UX. Mirrors
// [RequestComposeSheet] structurally — same chrome, same compose
// section, same publish bar — but dispatches the publish leg against
// the experience repository (addNeedsBatch + per-need claimNeed) so
// the same chips / paste / pre-claim flow works for an event the
// requester is organising rather than a single help request.
//
// Triggered from ExperienceEventPane's "What's Needed" row when the
// experience has zero needs/contributions and the viewer is the host.
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/data/repositories/experience_repository.dart'
    show RSVPIntention;
import 'package:ripls/presentation/viewmodels/experience_needs_view_model.dart';
import 'package:ripls/presentation/viewmodels/request_compose_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/needs/needs_sheet_chrome.dart';
import 'package:ripls/presentation/widgets/request/compose/request_compose_section.dart';
import 'package:ripls/presentation/widgets/request/compose/request_compose_sheet.dart'
    show composeSuggestionsFor;
import 'package:ripls/services/providers.dart';

final _log = Logger('ExperienceComposeSheet');

class ExperienceComposeSheet extends ConsumerStatefulWidget {
  final String experienceId;
  final String communityId;

  /// True when the host hasn't RSVPed yet — the sheet auto-RSVPs YES
  /// before applying pre-claims so the host appears on the attendee
  /// list before their contributions attach.
  final bool needsRsvp;

  const ExperienceComposeSheet({
    super.key,
    required this.experienceId,
    required this.communityId,
    required this.needsRsvp,
  });

  /// Opens the compose sheet. Returns true when at least one item was
  /// successfully added (full or partial success); false if the user
  /// dismissed without publishing.
  static Future<bool> show(
    BuildContext context, {
    required String experienceId,
    required String communityId,
    required bool needsRsvp,
  }) async {
    final result = await showAccessibleModal<bool>(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (ctx) => ExperienceComposeSheet(
        experienceId: experienceId,
        communityId: communityId,
        needsRsvp: needsRsvp,
      ),
    );
    return result ?? false;
  }

  @override
  ConsumerState<ExperienceComposeSheet> createState() =>
      _ExperienceComposeSheetState();
}

class _ExperienceComposeSheetState
    extends ConsumerState<ExperienceComposeSheet> {
  bool _publishing = false;

  /// Discard guard for the × button — non-empty drafts prompt for
  /// confirmation; empty drafts close immediately.
  Future<void> _handleClose() async {
    if (_publishing) return;
    final compose = ref.read(requestComposeProvider);
    if (compose.pieces.isEmpty) {
      Navigator.of(context).pop(false);
      return;
    }
    final confirmed = await _DiscardConfirmSheet.show(
      context,
      piecesCount: compose.pieces.length,
      youCount: compose.preclaimedCount,
    );
    if (!mounted) return;
    if (confirmed != true) return;

    ref.read(requestComposeProvider.notifier).reset();
    Navigator.of(context).pop(false);

    if (ScaffoldMessenger.maybeOf(context) != null) {
      ToastHelper.showSuccess(context, context.l10n.composeDiscardToast);
    }
  }

  Future<void> _onPublish() async {
    final compose = ref.read(requestComposeProvider);
    if (compose.pieces.isEmpty) {
      Navigator.of(context).pop(false);
      return;
    }

    setState(() => _publishing = true);

    final pieces = compose.pieces;
    final repo = ref.read(experienceRepositoryProvider);

    // ── Step 1: batch-add Needs against the experience repo ────────
    final items = [
      for (final p in pieces) BatchNeedItem(name: p.label, slots: 1),
    ];
    List<String> addedNeedIds = const [];
    try {
      final added = await repo.addNeedsBatch(
        experienceId: widget.experienceId,
        items: items,
      );
      addedNeedIds = [for (final n in added) n.id];
    } catch (e, st) {
      _log.warning('experience compose publish: batch add failed', e, st);
      if (!mounted) return;
      setState(() => _publishing = false);
      ToastHelper.showError(
        context,
        context.l10n.composePartialAddError(pieces.length),
      );
      return;
    }
    if (!mounted) return;

    // ── Step 1.5: ensure the host is RSVPed before claims attach ──
    if (widget.needsRsvp) {
      try {
        await repo.rsvp(
          experienceId: widget.experienceId,
          communityId: widget.communityId,
          intention: RSVPIntention.RSVP_INTENTION_YES,
        );
      } catch (e, st) {
        _log.warning(
            'experience compose publish: RSVP set failed; continuing', e, st);
        // Soft-fail: surface as a partial-claim issue if pre-claims
        // were requested, since the claim step will likely 4xx.
      }
      if (!mounted) return;
    }

    // ── Step 2: pre-claim the "I've got this" pieces one by one ───
    // The experience repo doesn't expose a batch-claim RPC; loop the
    // single-claim call. Failures are aggregated so the requester still
    // sees a single toast at the end.
    final preclaimedNeedIds = <String>[];
    final failedClaims = <String>[];
    for (var i = 0; i < pieces.length; i++) {
      if (!pieces[i].preclaimed) continue;
      if (i >= addedNeedIds.length) continue;
      final needId = addedNeedIds[i];
      if (needId.isEmpty) continue;
      try {
        await repo.claimNeed(
          experienceId: widget.experienceId,
          needId: needId,
        );
        preclaimedNeedIds.add(needId);
      } catch (e, st) {
        _log.warning('experience compose publish: claim failed', e, st);
        failedClaims.add(needId);
      }
      if (!mounted) return;
    }

    // Refresh the experience needs cache so the row updates.
    await ref
        .read(experienceNeedsProvider(widget.experienceId).notifier)
        .refresh();
    if (!mounted) return;

    final hadAnySuccess = addedNeedIds.isNotEmpty;
    final hadAnyFailure = failedClaims.isNotEmpty;
    if (failedClaims.isNotEmpty) {
      ToastHelper.showError(
        context,
        context.l10n.composePartialClaimError(failedClaims.length),
      );
    }

    final publishedPieces = addedNeedIds.length;
    final publishedYou = preclaimedNeedIds.length;

    ref.read(requestComposeProvider.notifier).reset();
    Navigator.of(context).pop(hadAnySuccess);

    if (hadAnySuccess && !hadAnyFailure) {
      ToastHelper.showSuccess(
        context,
        context.l10n.composePublishSuccessToast(publishedPieces, publishedYou),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    final needsState =
        ref.watch(experienceNeedsProvider(widget.experienceId));
    final compose = ref.watch(requestComposeProvider);

    // Experience chips ride on a single `suggestions` list (no
    // breakdownPieces / additionalAsks split — that's request-only).
    // composeSuggestionsFor() dedupes case-insensitively and caps at
    // six, mirroring the Request side.
    final suggestions = composeSuggestionsFor(
      breakdownPieces: const [],
      additionalAsks: needsState.suggestions,
      pieces: compose.pieces,
    );

    return NeedsSheetChrome(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          _Header(onClose: _publishing ? null : _handleClose),
          Flexible(
            child: SingleChildScrollView(
              padding: const EdgeInsets.only(top: 4, bottom: 12),
              child: RequestComposeSection(
                suggestions: suggestions,
                onGlass: true,
                forEvent: true,
              ),
            ),
          ),
          _PublishBar(
            compose: compose,
            isPublishing: _publishing,
            onPublish: _onPublish,
            label: l10n.composeSheetSaveCta(compose.pieces.length),
          ),
        ],
      ),
    );
  }
}

class _Header extends StatelessWidget {
  final VoidCallback? onClose;
  const _Header({required this.onClose});

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 8),
      child: Row(
        children: [
          Expanded(
            child: Text(
              context.l10n.composeSheetTitle,
              style: const TextStyle(
                fontFamily: AppTheme.headingFont,
                fontSize: 26,
                height: 1.05,
                fontWeight: FontWeight.w600,
                letterSpacing: -0.4,
                color: AppColors.modalTextPrimary,
              ),
            ),
          ),
          IconAction(
            icon: Icons.close,
            semanticsLabel: context.l10n.a11yClose,
            color: AppColors.modalTextPrimary,
            onPressed: onClose,
          ),
        ],
      ),
    );
  }
}

class _DiscardConfirmSheet extends StatelessWidget {
  final int piecesCount;
  final int youCount;
  const _DiscardConfirmSheet({
    required this.piecesCount,
    required this.youCount,
  });

  static Future<bool?> show(
    BuildContext context, {
    required int piecesCount,
    required int youCount,
  }) {
    return showAccessibleModal<bool>(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (_) => _DiscardConfirmSheet(
        piecesCount: piecesCount,
        youCount: youCount,
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    return NeedsSheetChrome(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          const SizedBox(height: 4),
          Text(
            l10n.composeDiscardTitle,
            style: const TextStyle(
              fontSize: 22,
              fontWeight: FontWeight.w600,
              color: AppColors.modalTextPrimary,
            ),
          ),
          const SizedBox(height: 10),
          Text(
            l10n.composeDiscardBody(piecesCount, youCount),
            style: TextStyle(
              fontSize: 14,
              height: 1.4,
              color: AppColors.modalTextPrimary.withValues(alpha: 0.72),
            ),
          ),
          const SizedBox(height: 20),
          Tappable(
            semanticsLabel: l10n.a11yComposeDiscardConfirm,
            onTap: () => Navigator.of(context).pop(true),
            inkBorderRadius: BorderRadius.circular(16),
            child: Container(
              width: double.infinity,
              height: 52,
              alignment: Alignment.center,
              decoration: BoxDecoration(
                color: AppColors.statusErrorOnDark,
                borderRadius: BorderRadius.circular(16),
              ),
              child: Text(
                l10n.composeDiscardConfirmCta,
                style: const TextStyle(
                  fontSize: 16,
                  fontWeight: FontWeight.w700,
                  letterSpacing: 0.1,
                  color: AppColors.modalTextPrimary,
                ),
              ),
            ),
          ),
          const SizedBox(height: 10),
          Tappable(
            semanticsLabel: l10n.a11yComposeDiscardCancel,
            onTap: () => Navigator.of(context).pop(false),
            inkBorderRadius: BorderRadius.circular(16),
            child: Container(
              width: double.infinity,
              height: 52,
              alignment: Alignment.center,
              decoration: BoxDecoration(
                color: Colors.transparent,
                borderRadius: BorderRadius.circular(16),
                border: Border.all(color: GlassTokens.border),
              ),
              child: Text(
                l10n.composeDiscardKeepCta,
                style: const TextStyle(
                  fontSize: 16,
                  fontWeight: FontWeight.w600,
                  color: AppColors.modalTextPrimary,
                ),
              ),
            ),
          ),
          const SizedBox(height: 4),
        ],
      ),
    );
  }
}

class _PublishBar extends StatelessWidget {
  final RequestComposeState compose;
  final bool isPublishing;
  final VoidCallback onPublish;
  final String label;

  const _PublishBar({
    required this.compose,
    required this.isPublishing,
    required this.onPublish,
    required this.label,
  });

  @override
  Widget build(BuildContext context) {
    final canPublish = compose.canPublish && !isPublishing;
    const accent = AppColors.lightAccent;
    return Padding(
      padding: const EdgeInsets.only(top: 8),
      child: Tappable(
        semanticsLabel: label,
        onTap: canPublish ? onPublish : null,
        inkBorderRadius: BorderRadius.circular(16),
        child: Container(
          width: double.infinity,
          height: 52,
          alignment: Alignment.center,
          decoration: BoxDecoration(
            color: canPublish ? accent : accent.withValues(alpha: 0.30),
            borderRadius: BorderRadius.circular(16),
            boxShadow: canPublish
                ? [
                    BoxShadow(
                      color: accent.withValues(alpha: 0.30),
                      offset: const Offset(0, 8),
                      blurRadius: 20,
                    ),
                  ]
                : null,
          ),
          child: isPublishing
              ? const SizedBox(
                  width: 20,
                  height: 20,
                  child: CircularProgressIndicator(strokeWidth: 2),
                )
              : Text(
                  label,
                  style: TextStyle(
                    fontSize: 16,
                    fontWeight: FontWeight.w700,
                    letterSpacing: 0.1,
                    color: canPublish
                        ? AppColors.cardBackground(context)
                        : AppColors.modalTextMuted,
                  ),
                ),
        ),
      ),
    );
  }
}
