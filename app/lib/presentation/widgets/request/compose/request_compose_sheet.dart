// Bottom-sheet entry point for the post-publish compose UX. Wraps the
// shared [NeedsSheetChrome] (frosted glass + drag handle) around
// [RequestComposeSection] with a Save CTA that runs
// [RequestComposeNotifier.publish] against the already-published
// request.
//
// Triggered from RequestRequestPane's "What's Needed" row when the
// request has zero needs and the viewer is the owner.
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/presentation/viewmodels/request_compose_view_model.dart';
import 'package:ripls/presentation/viewmodels/request_needs_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/needs/needs_sheet_chrome.dart';
import 'package:ripls/presentation/widgets/request/compose/request_compose_section.dart';

class RequestComposeSheet extends ConsumerStatefulWidget {
  final String requestId;
  final String communityId;

  const RequestComposeSheet({
    super.key,
    required this.requestId,
    required this.communityId,
  });

  /// Opens the compose sheet for a published [requestId]. Returns true
  /// when at least one piece was successfully added (full or partial
  /// success); false if the user dismissed without publishing.
  static Future<bool> show(
    BuildContext context, {
    required String requestId,
    required String communityId,
  }) async {
    final result = await showAccessibleModal<bool>(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (ctx) => RequestComposeSheet(
        requestId: requestId,
        communityId: communityId,
      ),
    );
    return result ?? false;
  }

  @override
  ConsumerState<RequestComposeSheet> createState() =>
      _RequestComposeSheetState();
}

class _RequestComposeSheetState extends ConsumerState<RequestComposeSheet> {
  bool _publishing = false;

  /// Guard the × button: a non-empty draft must not be silently
  /// discarded. Empty drafts close immediately.
  Future<void> _handleClose() async {
    if (_publishing) return;
    final compose = ref.read(requestComposeProvider);
    if (compose.pieces.isEmpty) {
      Navigator.of(context).pop(false);
      return;
    }
    final confirmed = await _ComposeDiscardConfirmSheet.show(
      context,
      piecesCount: compose.pieces.length,
      youCount: compose.preclaimedCount,
    );
    if (!mounted) return;
    if (confirmed != true) return;

    ref.read(requestComposeProvider.notifier).reset();
    Navigator.of(context).pop(false);

    // Surface the discard confirmation on the published-request
    // screen — the sheet has already popped, so the messenger is the
    // underlying scaffold's.
    final messenger = ScaffoldMessenger.maybeOf(context);
    if (messenger != null) {
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
    final result = await ref.read(requestComposeProvider.notifier).publish(
          requestId: widget.requestId,
          communityId: widget.communityId,
        );
    if (!mounted) return;

    // Refresh the post-publish needs cache so the row updates.
    await ref
        .read(requestNeedsProvider(widget.requestId).notifier)
        .refresh();
    if (!mounted) return;

    final hadAnySuccess = result.addedNeedIds.isNotEmpty;
    final hadAnyFailure =
        result.failedAdds.isNotEmpty || result.failedClaims.isNotEmpty;
    if (result.failedAdds.isNotEmpty) {
      ToastHelper.showError(
        context,
        context.l10n.composePartialAddError(result.failedAdds.length),
      );
    } else if (result.failedClaims.isNotEmpty) {
      ToastHelper.showError(
        context,
        context.l10n.composePartialClaimError(result.failedClaims.length),
      );
    }

    final composeError = ref.read(requestComposeProvider).error;
    if (composeError != null && !hadAnySuccess) {
      setState(() => _publishing = false);
      ToastHelper.showError(
        context,
        RpcErrorHandler.localize(composeError, context.l10n),
      );
      return;
    }

    // Capture the post-publish toast inputs before reset() blows
    // away the count.
    final publishedPieces = result.addedNeedIds.length;
    final publishedYou = result.claimedNeedIds.length;

    ref.read(requestComposeProvider.notifier).reset();
    Navigator.of(context).pop(hadAnySuccess);

    // Clean-publish success — surface on the published-request
    // scaffold messenger. Partial-success already has its own toast
    // above; skip the success toast in that case to avoid stacking.
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
    final needsState = ref.watch(requestNeedsProvider(widget.requestId));
    final compose = ref.watch(requestComposeProvider);

    final suggestions = composeSuggestionsFor(
      breakdownPieces: needsState.breakdownPieces,
      additionalAsks: needsState.additionalAsks,
      pieces: compose.pieces,
    );

    return NeedsSheetChrome(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          _Header(
            onClose: _publishing ? null : _handleClose,
          ),
          Flexible(
            child: SingleChildScrollView(
              padding: const EdgeInsets.only(top: 4, bottom: 12),
              child: RequestComposeSection(
                suggestions: suggestions,
                onGlass: true,
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

/// Modal-over-modal confirmation surfaced when the requester taps × on a
/// non-empty compose draft. Defensive guard against silent data loss
/// — the prototype's "Discard draft?" confirm sheet.
class _ComposeDiscardConfirmSheet extends StatelessWidget {
  final int piecesCount;
  final int youCount;
  const _ComposeDiscardConfirmSheet({
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
      builder: (_) => _ComposeDiscardConfirmSheet(
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
          // Destructive primary first (sage rail, prominent) so it
          // anchors the decision; Keep-editing is the secondary on the
          // bottom.
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

/// Builds the chip strip shown at the top of [RequestComposeSheet].
///
/// Concatenates the server's `breakdownPieces` and `additionalAsks`
/// (the two chip lists that ride on `GenRequestResponse` and end up
/// on [RequestNeedsState]), trims and case-insensitive-dedupes them,
/// drops any whose label already matches a committed piece on the
/// draft (so the strip doesn't keep showing chips the requester has
/// already grabbed), and caps the result at [limit] so the strip
/// stays scannable. Pure function — kept top-level so it's directly
/// unit-testable from `request_compose_sheet_test.dart`.
List<String> composeSuggestionsFor({
  required List<String> breakdownPieces,
  required List<String> additionalAsks,
  required List<ComposePiece> pieces,
  int limit = 6,
}) {
  final taken = <String>{
    for (final p in pieces) p.label.trim().toLowerCase(),
  };
  final seen = <String>{};
  final out = <String>[];
  for (final raw in <String>[...breakdownPieces, ...additionalAsks]) {
    final trimmed = raw.trim();
    if (trimmed.isEmpty) continue;
    final norm = trimmed.toLowerCase();
    if (taken.contains(norm)) continue;
    if (!seen.add(norm)) continue;
    out.add(trimmed);
    if (out.length >= limit) break;
  }
  return out;
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
    // canPublish gates the entire UX: pre-publish (no pieces yet) the
    // button has to look unambiguously disabled — same accent-tinted
    // shape as the propose sheet's _PrimaryButton so the two CTAs
    // read as one design system.
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
