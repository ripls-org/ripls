import 'dart:async';
import 'dart:ui' as ui;

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/observability/events.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/core/theme/gen/overlay_tokens.gen.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/presentation/viewmodels/gen_community_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/content/content_error_banner.dart';
import 'package:ripls/presentation/widgets/create/unified_primary_button.dart';
import 'package:ripls/presentation/widgets/media/background_media_image.dart';
import 'package:ripls/presentation/widgets/media/media_picker_dialog.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass_surface.dart';
import 'package:ripls/services/providers.dart';

/// Single community-creation modal rendered on the unified-create glass
/// surface: a full-bleed hero media background (or backdrop blur when no
/// media), a glass card with a required **name** field and an optional
/// **description** field, and a primary Create button.
///
/// The user owns the name and description; the server only finds a
/// relevant background image, streamed in via
/// [GenCommunityNotifier.fetchBackground] keyed on the typed name (and
/// description). The image is replaceable via the Replace Background
/// affordance.
///
/// Follows the keyboard up via `AnimatedPadding(bottom: viewInsets)` and
/// falls back to a `BackdropFilter` blur of the underlying UI when no
/// community image has streamed in yet.
///
/// Usage:
/// ```dart
/// final result = await CommunityCreationModal.show(context);
/// if (result != null) {
///   // Community was created - result is the community ID
/// }
/// ```
class CommunityCreationModal extends ConsumerStatefulWidget {
  const CommunityCreationModal({super.key, this.promoteCommunityId});

  /// When set, the modal runs in **promote** mode: the same two-field form
  /// (name + optional description + background image) names an existing
  /// nameless (per-item / ad-hoc) community in place via UpdateCommunity,
  /// turning it into a real, persistent community (#2492). Null in normal
  /// create mode.
  final String? promoteCommunityId;

  /// Opens the modal. Pass [promoteCommunityId] to run in promote mode
  /// against an existing community; omit it to create a new one.
  static Future<String?> show(BuildContext context, {String? promoteCommunityId}) {
    // Capture outer MediaQuery so SafeArea reads for status-bar inset
    // still work inside the modal after showModalBottomSheet wraps the
    // builder in `removePadding(removeTop: true)` (#1933). `viewInsets`
    // (keyboard inset) flows through live from the inner MediaQuery.
    final outerMediaQuery = MediaQuery.of(context);
    return showAccessibleModal<String>(
      context,
      isScrollControlled: true,
      useSafeArea: false,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      isDismissible: true,
      enableDrag: true,
      builder: (innerCtx) => MediaQuery(
        data: outerMediaQuery.copyWith(
            viewInsets: MediaQuery.of(innerCtx).viewInsets),
        child: CommunityCreationModal(promoteCommunityId: promoteCommunityId),
      ),
    );
  }

  @override
  ConsumerState<CommunityCreationModal> createState() =>
      _CommunityCreationModalState();
}

class _CommunityCreationModalState
    extends ConsumerState<CommunityCreationModal> {
  bool _isCreating = false;
  String? _errorMessage;

  @override
  void initState() {
    super.initState();
    // Clear any prior generation state so the modal opens with empty
    // fields. Deferred to a post-frame callback because notifier mutation
    // during the first build is disallowed.
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!mounted) return;
      final notifier = ref.read(genCommunityProvider.notifier);
      notifier.reset();
      // Promote mode: re-seed the existing-community id *after* reset so the
      // final submit updates that community in place instead of creating a
      // new one. Null in normal create mode (#2492).
      notifier.setExistingCommunityId(widget.promoteCommunityId);
    });
  }

  void _showMediaPickerDialog() {
    final notifier = ref.read(genCommunityProvider.notifier);
    final genState = ref.read(genCommunityProvider);
    final uploadedMediaIds = genState.mediaIds;
    final hasMedia = uploadedMediaIds != null && uploadedMediaIds.isNotEmpty;
    MediaPickerDialog.show(
      context: context,
      hasMedia: hasMedia,
      onVideoTap: () => _handleMediaPick(notifier.pickAndUploadVideoFromGallery),
      onPhotoTap: () =>
          _handleMediaPick(notifier.pickAndUploadMultipleImagesFromGallery),
      onCameraTap: () =>
          _handleMediaPick(notifier.pickAndUploadImageFromCamera),
      candidates: genState.mediaCandidates,
      candidateImportingIndex: genState.candidateImportingIndex,
      onCandidateTap: notifier.useCandidate,
    );
  }

  Future<void> _handleMediaPick(Future<void> Function() pickFunction) async {
    try {
      await pickFunction();
      if (!mounted) return;
      final failedCount =
          ref.read(genCommunityProvider).batchUploadFailedCount;
      if (failedCount != null && failedCount > 0) {
        ToastHelper.showError(
          context,
          context.l10n.mediaBatchUploadPartialFailure(failedCount),
        );
      }
    } catch (e) {
      if (mounted) {
        ToastHelper.showError(
          context,
          context.l10n.communityPreviewMediaUploadError(e.toString()),
        );
      }
    }
  }

  Future<void> _handleCreate() async {
    final state = ref.read(genCommunityProvider);
    final name = (state.name ?? '').trim();

    if (name.isEmpty) {
      setState(() {
        _errorMessage = context.l10n.communityPreviewNameRequired;
      });
      return;
    }

    setState(() {
      _isCreating = true;
      _errorMessage = null;
    });

    final communityId =
        await ref.read(genCommunityProvider.notifier).createCommunity();
    if (!mounted) return;

    if (communityId != null) {
      unawaited(ref
          .read(observabilityServiceProvider)
          .logAnalyticsEvent(CommunityCreatedEvent()));
      Navigator.of(context).pop(communityId);
    } else {
      final viewModelState = ref.read(genCommunityProvider);
      setState(() {
        _errorMessage = viewModelState.error == null
            ? context.l10n.communityPreviewCreateGenericError
            : RpcErrorHandler.localize(viewModelState.error!, context.l10n);
        _isCreating = false;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(genCommunityProvider);
    final media = MediaQuery.of(context);
    final viewInsets = media.viewInsets.bottom;
    final effectiveMediaId = state.mediaIds?.firstOrNull;
    final hasMedia = effectiveMediaId != null && effectiveMediaId.isNotEmpty;

    return SizedBox(
      height: media.size.height,
      child: Stack(
        children: [
          if (hasMedia) ...[
            BackgroundMediaImage(
              key: ValueKey('community-create-hero-$effectiveMediaId'),
              mediaId: effectiveMediaId,
              getMediaUrl: () => ref
                  .read(mediaRepositoryProvider)
                  .getHeroMediaUrl(effectiveMediaId),
            ),
            // Slight dim so the white glass card text stays legible on
            // any hero image's contrast.
            Positioned.fill(
              child: Container(color: GlassTokens.scrimTint),
            ),
          ] else
            Positioned.fill(
              child: BackdropFilter(
                filter: ui.ImageFilter.blur(
                  sigmaX: AppColors.modalBackdropBlurSigma,
                  sigmaY: AppColors.modalBackdropBlurSigma,
                ),
                child: const SizedBox.shrink(),
              ),
            ),
          // Tap-off-to-dismiss zone everywhere the card / button doesn't
          // cover. Skipped while creating so the user can't accidentally
          // dismiss mid-RPC.
          if (!_isCreating)
            Positioned.fill(
              child: Tappable(
                semanticsLabel: context.l10n.a11yDismissModal,
                onTap: () => Navigator.of(context).pop(),
                child: const SizedBox.expand(),
              ),
            ),
          if (!_isCreating)
            Positioned(
              // Anchor from the view-level inset (read straight from the
              // FlutterView) so the X always clears the OS status bar —
              // showModalBottomSheet's `removePadding(removeTop: true)`
              // zeros the MediaQuery-level inset for descendants of the
              // route, which breaks both SafeArea and `viewPadding`
              // reads inside the modal subtree.
              top: _viewLevelTopInset(context) + 8,
              right: 12,
              child: DecoratedBox(
                decoration: BoxDecoration(
                  shape: BoxShape.circle,
                  color: Colors.black.withValues(alpha: 0.5),
                ),
                child: IconAction(
                  icon: Icons.close,
                  semanticsLabel: context.l10n.a11yClose,
                  color: OverlayTokens.textPrimary,
                  onPressed: () => Navigator.of(context).pop(),
                ),
              ),
            ),
          Align(
            alignment: Alignment.bottomCenter,
            child: AnimatedPadding(
              duration: accessibleDuration(
                  context, const Duration(milliseconds: 180)),
              curve: Curves.easeOut,
              padding: EdgeInsets.only(bottom: viewInsets),
              child: SingleChildScrollView(
                // Reverse so when the keyboard opens, the currently-
                // focused field stays visible at the bottom of the view
                // and the top of the card scrolls off-screen instead of
                // the card getting clipped (matches UnifiedCreateModal).
                reverse: true,
                padding: const EdgeInsets.fromLTRB(16, 24, 16, 24),
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    _ReplaceBackgroundButton(
                      onTap: _isCreating ? null : _showMediaPickerDialog,
                    ),
                    const SizedBox(height: 12),
                    // "Building…" pill aligned right above the card,
                    // matching `UnifiedPreviewCard`'s placement of
                    // [UnifiedBuildingPill]. Visible only while
                    // `genCommunityProvider.isStreaming` is true.
                    Align(
                      alignment: Alignment.centerRight,
                      child: Padding(
                        padding: const EdgeInsets.only(bottom: 8, right: 4),
                        child: Semantics(
                          liveRegion: true,
                          child: const _CommunityBuildingPill(),
                        ),
                      ),
                    ),
                    GlassSurface(
                      borderRadius: BorderRadius.circular(24),
                      padding: const EdgeInsets.all(20),
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.stretch,
                        mainAxisSize: MainAxisSize.min,
                        children: [
                          _SyncedTextField(
                            value: state.name ?? '',
                            hintText: context.l10n.communityPreviewNameHint,
                            enabled: !_isCreating,
                            style: TextStyle(
                              fontSize: 26,
                              fontWeight: FontWeight.w700,
                              color: AppColors.modalTextPrimary,
                              height: 1.1,
                            ),
                            textCapitalization: TextCapitalization.sentences,
                            textInputAction: TextInputAction.next,
                            // Auto-load a background as the user types the name
                            // (debounced) — no submit gesture required.
                            onChanged: (value) {
                              final notifier =
                                  ref.read(genCommunityProvider.notifier);
                              notifier.setName(value);
                              notifier.scheduleBackgroundFetch();
                            },
                          ),
                          const SizedBox(height: 6),
                          _SyncedTextField(
                            value: state.description ?? '',
                            hintText:
                                context.l10n.communityPreviewDescriptionHint,
                            enabled: !_isCreating,
                            style: TextStyle(
                              fontSize: 14,
                              color: AppColors.modalTextPrimary
                                  .withValues(alpha: 0.85),
                              height: 1.35,
                            ),
                            maxLines: null,
                            textCapitalization: TextCapitalization.sentences,
                            // Description does NOT trigger a background fetch —
                            // only the name does, and only once (see
                            // GenCommunityNotifier.scheduleBackgroundFetch).
                            onChanged: (value) => ref
                                .read(genCommunityProvider.notifier)
                                .setDescription(value),
                          ),
                        ],
                      ),
                    ),
                    Semantics(
                      liveRegion: true,
                      child: Column(
                        mainAxisSize: MainAxisSize.min,
                        children: [
                          if (state.error != null) ...[
                            const SizedBox(height: 12),
                            ContentErrorBanner(
                              errorMessage: RpcErrorHandler.localize(
                                  state.error!, context.l10n),
                            ),
                          ],
                          if (_errorMessage != null) ...[
                            const SizedBox(height: 12),
                            ContentErrorBanner(errorMessage: _errorMessage!),
                          ],
                        ],
                      ),
                    ),
                    const SizedBox(height: 16),
                    UnifiedPrimaryButton(
                      label: state.existingCommunityId != null
                          ? context.l10n.communityPreviewNameGroup
                          : context.l10n.communityPreviewCreate,
                      // Stable id so e2e can target this confirm even when its
                      // promote-mode label ("Name this group") matches the
                      // roster link that opened the modal.
                      semanticsIdentifier: 'community-preview-confirm',
                      enabled: !_isCreating,
                      loading: _isCreating,
                      onTap: _handleCreate,
                    ),
                  ],
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }
}

/// "Building…" pill shown while the background-image stream is in flight.
/// Mirrors [UnifiedBuildingPill] but watches the community gen provider
/// instead of the unified-create one.
class _CommunityBuildingPill extends ConsumerWidget {
  const _CommunityBuildingPill();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final streaming = ref.watch(
      genCommunityProvider.select((s) => s.isStreaming),
    );
    if (!streaming) return const SizedBox.shrink();
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
      decoration: BoxDecoration(
        color: AppColors.modalChipBackground,
        borderRadius: BorderRadius.circular(999),
        border: Border.all(color: AppColors.modalChipBorder),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          SizedBox(
            width: 12,
            height: 12,
            child: CircularProgressIndicator(
              strokeWidth: 1.8,
              valueColor:
                  AlwaysStoppedAnimation<Color>(AppColors.modalTextPrimary),
            ),
          ),
          const SizedBox(width: 7),
          Text(
            context.l10n.unifiedCreateBuilding,
            style: TextStyle(
              fontSize: 12,
              fontWeight: FontWeight.w600,
              color: AppColors.modalTextPrimary,
              letterSpacing: 0.3,
            ),
          ),
        ],
      ),
    );
  }
}

/// Returns the OS-level top safe-area inset in logical pixels, read
/// directly from the [FlutterView] rather than via [MediaQuery]. The
/// bottom-sheet route's `removePadding(removeTop: true)` zeros the
/// MediaQuery-level inset for descendant subtrees, so any X-button or
/// header offset that needs to clear the status bar should use this
/// function instead of `MediaQuery.viewPadding.top`.
double _viewLevelTopInset(BuildContext context) {
  final view = View.of(context);
  return view.padding.top / view.devicePixelRatio;
}

/// Glass-circle pencil button with "Replace background" label, mirroring
/// the unified-create preview card's affordance.
class _ReplaceBackgroundButton extends StatelessWidget {
  const _ReplaceBackgroundButton({required this.onTap});
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: context.l10n.unifiedCreateReplaceBackground,
      onTap: onTap,
      excludeChildSemantics: true,
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          GlassSurface(
            borderRadius: BorderRadius.circular(28),
            padding: const EdgeInsets.all(14),
            child: Icon(
              Icons.edit_outlined,
              size: 22,
              color: AppColors.modalTextPrimary,
            ),
          ),
          const SizedBox(height: 6),
          Text(
            context.l10n.unifiedCreateReplaceBackground,
            style: TextStyle(
              fontSize: 12.5,
              fontWeight: FontWeight.w500,
              color: AppColors.modalTextPrimary,
              shadows: const [
                Shadow(
                  color: Color(0x73000000),
                  blurRadius: 8,
                  offset: Offset(0, 1),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

/// Text field whose controller stays in sync with an external [value].
/// Mirrors `_SyncedTextField` in `unified_preview_card.dart` so a streamed
/// background can land without disrupting the user's typing, and so the
/// fields can be seeded with empty values on open.
class _SyncedTextField extends StatefulWidget {
  const _SyncedTextField({
    required this.value,
    required this.onChanged,
    this.style,
    this.maxLines,
    this.hintText,
    this.enabled = true,
    this.textCapitalization = TextCapitalization.none,
    this.textInputAction,
  });
  final String value;
  final ValueChanged<String> onChanged;
  final TextStyle? style;
  final int? maxLines;
  final String? hintText;
  final bool enabled;
  final TextCapitalization textCapitalization;
  final TextInputAction? textInputAction;

  @override
  State<_SyncedTextField> createState() => _SyncedTextFieldState();
}

class _SyncedTextFieldState extends State<_SyncedTextField> {
  late final TextEditingController _controller =
      TextEditingController(text: widget.value);

  @override
  void didUpdateWidget(covariant _SyncedTextField old) {
    super.didUpdateWidget(old);
    if (widget.value != _controller.text) {
      _controller.value = TextEditingValue(
        text: widget.value,
        selection: TextSelection.collapsed(offset: widget.value.length),
      );
    }
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final field = TextField(
      controller: _controller,
      onChanged: widget.onChanged,
      enabled: widget.enabled,
      style: widget.style,
      maxLines: widget.maxLines,
      minLines: 1,
      textCapitalization: widget.textCapitalization,
      textInputAction: widget.textInputAction,
      cursorColor: AppColors.modalTextPrimary,
      decoration: InputDecoration(
        isCollapsed: true,
        filled: false,
        fillColor: Colors.transparent,
        border: InputBorder.none,
        enabledBorder: InputBorder.none,
        focusedBorder: InputBorder.none,
        contentPadding: EdgeInsets.zero,
        hintText: widget.hintText,
        hintStyle: widget.style?.copyWith(
          color: AppColors.modalTextPrimary.withValues(alpha: 0.45),
        ),
      ),
    );
    // No wrapping Semantics(label:) — a nested label over a TextField
    // serializes as a *second* a11y node on Flutter Web (the hint is the
    // first), which makes the field ambiguous to role-based locators (e2e)
    // and screen readers. The hint text is the field's accessible name.
    return field;
  }
}
