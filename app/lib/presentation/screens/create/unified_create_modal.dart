import 'dart:async';
import 'dart:ui' as ui;

import 'package:cached_network_image/cached_network_image.dart';
import 'package:fixnum/fixnum.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_timezone/flutter_timezone.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/core/theme/gen/overlay_tokens.gen.dart';
import 'package:ripls/data/gen/ripls/api/time.pb.dart';
import 'package:ripls/data/gen/ripls/api/unified_create_service.pb.dart'
    show DetectedContentType;
import 'package:ripls/presentation/screens/create/unified_create_input_drawer.dart';
import 'package:ripls/presentation/viewmodels/unified_create_save_actions.dart';
import 'package:ripls/presentation/viewmodels/unified_create_state.dart';
import 'package:ripls/presentation/viewmodels/unified_create_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/create/unified_preview_card.dart';
import 'package:ripls/presentation/widgets/creation/unified_create_camera_layer.dart';
import 'package:ripls/presentation/widgets/media/background_media_image.dart';
import 'package:ripls/presentation/widgets/media/media_picker_dialog.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass_surface.dart';
import 'package:ripls/services/providers.dart' show mediaRepositoryProvider;
import 'package:video_player/video_player.dart';

final _log = Logger('UnifiedCreateModal');

/// Approximate height in dp of the input drawer when Image mode is
/// selected and the drawer collapses to just the tab strip (drag
/// handle + pill tabs + bottom padding). Used by
/// [UnifiedCreateCameraLayer] to anchor the shutter row just above
/// the drawer without overlapping it.
const double _imageModeDrawerHeight = 84;

/// Entry-point modal for the unified-create flow. Uses
/// `showAccessibleModal` (showModalBottomSheet under the hood) so the
/// underlying app stays visible (dimmed by the barrier) behind the glass
/// drawer/preview overlay — that's what the design mock's "blur of
/// underlying UI" effect comes from.
class UnifiedCreateModal extends ConsumerWidget {
  const UnifiedCreateModal({super.key});

  /// Returns the [UnifiedSaveResult] when the user successfully saved an item
  /// (caller should run the post-creation flow and open the Share sheet), or
  /// null if the modal was cancelled.
  ///
  /// When [initialPrompt] is provided, the modal opens pre-filled and
  /// immediately starts server generation, so the user lands on the streamed
  /// preview to accept or tweak rather than an empty composer — used by the
  /// calendar's open-day suggestions. [initialStartUnixSec], when set, seeds
  /// the event date/time **structurally** so the draft lands on exactly that
  /// day/time regardless of how the prompt text is parsed.
  ///
  /// [targetType] declares the type up front, for entry points where the user
  /// already said what they are making ("Plan an event", "Ask for help",
  /// "Offer something"). It picks the opening input tab, skips the classifier
  /// via `force_type`, and swaps the drawer's mixed example tour for one
  /// type-specific hint. Omit it for the generic `+` create, where the server
  /// infers the type from whatever the user types (#2936).
  static Future<UnifiedSaveResult?> show(
    BuildContext context,
    WidgetRef ref, {
    String? initialPrompt,
    int? initialStartUnixSec,
    DetectedContentType? targetType,
  }) {
    // Reset state before each open since the provider isn't autoDispose.
    final notifier = ref.read(unifiedCreateViewModelProvider.notifier);
    notifier.reset();
    if (targetType != null) {
      notifier.seedTargetType(targetType);
    }
    final seed = initialPrompt?.trim() ?? '';
    if (seed.isNotEmpty) {
      notifier.setInputMode(CreateInputMode.text);
      notifier.setPrompt(seed);
      // Kick off generation now so the modal builds straight into the
      // streamed preview stage.
      unawaited(notifier.start());
      // Seed the date/time structurally. start() runs its reset synchronously
      // before returning, so this lands on the post-reset state and is marked
      // user-edited (setEventTime), so the stream won't overwrite it.
      if (initialStartUnixSec != null && initialStartUnixSec > 0) {
        unawaited(_seedEventTime(notifier, initialStartUnixSec));
      }
    }
    // Capture the outer MediaQuery before pushing the bottom-sheet
    // route. showModalBottomSheet wraps its content in
    // MediaQuery.removePadding(removeTop: true) when useSafeArea is
    // false (we want false so the hero paints full-bleed under the
    // status bar). That removal zeroes BOTH padding.top AND
    // viewPadding.top inside the sheet, breaking SafeArea / status-bar
    // inset reads everywhere in the modal subtree. We restate the
    // outer MediaQueryData inside the builder to restore those top
    // insets — but only `padding`/`viewPadding`. `viewInsets` (the
    // keyboard inset) MUST flow through live from the inner
    // MediaQuery, otherwise the AnimatedPadding below cannot track
    // the keyboard rising/falling and the drawer renders underneath
    // it (issue #1933). See `mergeForBottomSheet` for the merge
    // semantics.
    final outerMediaQuery = MediaQuery.of(context);
    return showAccessibleModal<UnifiedSaveResult>(
      context,
      isScrollControlled: true,
      useSafeArea: false,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      // Match the dismiss semantics of every other modal in the app
      // (#1980): swipe-down and tap-off close the sheet. The defaults
      // are already true in showAccessibleModal, but stating them here
      // makes the intent explicit alongside the in-modal Tappable
      // dismiss zone below.
      isDismissible: true,
      enableDrag: true,
      builder: (innerCtx) => MediaQuery(
        data: mergeForBottomSheet(outerMediaQuery, MediaQuery.of(innerCtx)),
        child: const UnifiedCreateModal(),
      ),
    );
  }

  /// Seeds a specific start moment onto the draft as the event time, resolving
  /// the device's IANA timezone (UTC fallback). The absolute moment is exact;
  /// the timezone only affects how it's displayed.
  static Future<void> _seedEventTime(
    UnifiedCreateViewModel notifier,
    int startUnixSec,
  ) async {
    String tz;
    try {
      tz = (await FlutterTimezone.getLocalTimezone()).identifier;
    } catch (_) {
      tz = 'UTC';
    }
    notifier.setEventTime(
      ExperienceTime(
        specific: SpecificTime(
          unixTimestampSec: Int64(startUnixSec),
          timezone: tz,
          durationMinutes: 60,
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(unifiedCreateViewModelProvider);
    final media = MediaQuery.of(context);
    final viewInsets = media.viewInsets.bottom;
    final inCameraMode = state.stage == CreateStage.input &&
        state.inputMode == CreateInputMode.image;
    return SizedBox(
      height: media.size.height,
      child: Stack(
        children: [
          // In preview stage, render the AI-fetched / user-replaced
          // hero image full-bleed beneath the glass card; falls back
          // to a backdrop blur of the underlying app when no media
          // has streamed in yet.
          //
          // `_HeroMediaBackground` provides its own `Positioned.fill`
          // (matching `BackgroundMediaImage`'s internal wrapping) — don't
          // add another here or StackParentData gets written twice.
          if (state.stage == CreateStage.preview &&
              state.mediaIds.isNotEmpty) ...[
            _HeroMediaBackground(mediaId: state.mediaIds.first),
            // Slight dim so white text on the glass card stays
            // legible regardless of the hero image's contrast.
            Positioned.fill(
              child: Container(color: GlassTokens.scrimTint),
            ),
          ] else ...[
            Positioned.fill(
              child: BackdropFilter(
                filter: ui.ImageFilter.blur(
                  sigmaX: AppColors.modalBackdropBlurSigma,
                  sigmaY: AppColors.modalBackdropBlurSigma,
                ),
                child: const SizedBox.shrink(),
              ),
            ),
          ],
          // Tap-off-to-dismiss zone (#1980). The modal is full-bleed,
          // so Flutter's standard barrier isn't exposed — this Tappable
          // fakes the barrier on the empty area not covered by the
          // drawer / preview card / X button (which sit later in the
          // stack and catch their own taps first). Skipped in camera
          // mode: there the live-camera layer owns the screen and the
          // X in the corner is the only tap affordance (swipe-down
          // still works via enableDrag).
          if (!inCameraMode)
            Positioned.fill(
              child: Tappable(
                semanticsLabel: context.l10n.a11yDismissModal,
                onTap: () => Navigator.of(context).pop(),
                child: const SizedBox.expand(),
              ),
            ),
          // Image-mode live-camera layer. Sits behind the drawer (which
          // collapses to just the tab strip when Image is selected) and
          // above the backdrop blur. Mounts only while the user is on
          // the Image tab so the CameraController is created lazily and
          // released as soon as they switch to Text/URL.
          if (inCameraMode)
            Positioned.fill(
              bottom: viewInsets,
              child: const UnifiedCreateCameraLayer(
                bottomDrawerHeight: _imageModeDrawerHeight,
              ),
            ),
          // Close (X) button. Only rendered in camera mode — every
          // other state dismisses via swipe-down or the Tappable
          // barrier above, matching the rest of the app's modals
          // (#1980). The camera viewport fills the screen, so a
          // tap-off zone is unreliable there and the X stays as the
          // explicit affordance.
          //
          // SafeArea works correctly here because show() restates the
          // outer MediaQuery in the modal subtree (see show() comment),
          // restoring the OS status-bar inset that
          // showModalBottomSheet's removePadding(removeTop: true)
          // otherwise zeros out. 8 dp gap below the status bar is
          // visual breathing room.
          if (inCameraMode)
            Positioned(
              top: 8,
              right: 12,
              child: SafeArea(
                child: _GlassIconButton(
                  icon: Icons.close,
                  onTap: () => Navigator.of(context).pop(),
                  semanticsLabel: context.l10n.a11yClose,
                ),
              ),
            ),
          Align(
            alignment: Alignment.bottomCenter,
            child: AnimatedPadding(
              duration:
                  accessibleDuration(context, const Duration(milliseconds: 180)),
              curve: Curves.easeOut,
              padding: EdgeInsets.only(bottom: viewInsets),
              child: state.stage == CreateStage.input
                  ? const UnifiedCreateInputDrawer()
                  : SingleChildScrollView(
                      // Reverse so when the keyboard opens, the
                      // currently-focused field stays visible at the
                      // bottom; the top of the card scrolls off-screen
                      // rather than the card getting clipped.
                      reverse: true,
                      padding: const EdgeInsets.fromLTRB(16, 24, 16, 24),
                      child: Column(
                        mainAxisSize: MainAxisSize.min,
                        children: [
                          _ReplaceBackgroundButton(
                            hasMedia: state.mediaIds.isNotEmpty,
                            onTap: () {
                              final vm = ref
                                  .read(unifiedCreateViewModelProvider.notifier);
                              MediaPickerDialog.show(
                                context: context,
                                hasMedia: state.mediaIds.isNotEmpty,
                                onVideoTap: vm.replaceBackgroundFromVideo,
                                onPhotoTap: vm.replaceBackgroundFromPhotos,
                                onCameraTap: vm.replaceBackgroundFromCamera,
                                candidates: state.mediaCandidates,
                                candidateImportingIndex:
                                    state.candidateImportingIndex,
                                onCandidateTap: vm.useCandidate,
                              );
                            },
                          ),
                          const SizedBox(height: 16),
                          UnifiedPreviewCard(
                            onShare: () async {
                              if (state.saving) return;
                              final vm = ref
                                  .read(unifiedCreateViewModelProvider.notifier);

                              // Creation is just Save — no audience picker. The
                              // per-item community is provisioned server-side and
                              // the caller opens the Share sheet after the modal
                              // pops.
                              final fresh =
                                  ref.read(unifiedCreateViewModelProvider);
                              _log.info('save confirmed: '
                                  'type=${fresh.type?.name}');
                              vm.setSaving(true);
                              try {
                                final result = await ref
                                    .read(unifiedCreateSaveActionsProvider)
                                    .save(fresh);
                                _log.info(
                                    'saved ${result.type.name} id=${result.entityId}');
                                // Pop with the result so the caller can run the
                                // post-creation flow AND open the Share sheet
                                // AFTER the modal is fully gone from the route
                                // stack. Running refreshInPlace + navigateToTab
                                // while this modal is still mounted leaves the
                                // per-item provider stuck at isLoading=true
                                // (black bg + spinner) because the new
                                // ContentView mounts underneath the closing
                                // modal and its post-frame `initialize()`
                                // callback races with the modal teardown —
                                // `mounted` ends up false before loadXDetails
                                // fires. See the request flow in
                                // home_screen.dart for the working pattern this
                                // mirrors.
                                if (context.mounted) {
                                  Navigator.of(context).pop(result);
                                }
                              } catch (e, s) {
                                _log.warning('save failed', e, s);
                                vm.setSaving(false);
                                if (context.mounted) {
                                  ScaffoldMessenger.of(context).showSnackBar(
                                    SnackBar(content: Text('Save failed: $e')),
                                  );
                                }
                              }
                            },
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

/// Merges the captured outer [MediaQueryData] (taken before the
/// bottom sheet pushes its `removePadding(removeTop: true)` wrapper)
/// with the live inner [MediaQueryData] (the one that tracks
/// keyboard insets). Keeps `outer.padding`/`outer.viewPadding` so
/// SafeArea reads for the status-bar inset still work inside the
/// sheet, but pulls `viewInsets` from `inner` so keyboard
/// appearance/dismissal propagates to every descendant. Exposed at
/// library scope for unit testing.
@visibleForTesting
MediaQueryData mergeForBottomSheet(
  MediaQueryData outer,
  MediaQueryData inner,
) {
  return outer.copyWith(viewInsets: inner.viewInsets);
}

/// Glass-circle pencil button with a state-aware label below: "Replace
/// background" once media exists, "Add a photo" before any photo has
/// streamed in or been picked — "replace" implies a background that isn't
/// there yet (#2724). Mirrors the design mock — shown above the preview
/// card; tapping picks a new gallery image and writes it into
/// state.mediaIds.
class _ReplaceBackgroundButton extends StatelessWidget {
  const _ReplaceBackgroundButton({
    required this.hasMedia,
    required this.onTap,
  });
  final bool hasMedia;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final label = hasMedia
        ? context.l10n.unifiedCreateReplaceBackground
        : context.l10n.unifiedCreateAddPhoto;
    return Tappable(
      semanticsLabel: label,
      onTap: onTap,
      excludeChildSemantics: true,
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          GlassSurface(
            borderRadius: BorderRadius.circular(28),
            padding: const EdgeInsets.all(14),
            child: Icon(
              hasMedia ? Icons.edit_outlined : Icons.add_a_photo_outlined,
              size: 22,
              color: AppColors.modalTextPrimary,
            ),
          ),
          const SizedBox(height: 6),
          Builder(
            builder: (ctx) => Text(
              label,
              style: TextStyle(
                fontSize: 12.5,
                fontWeight: FontWeight.w500,
                color: AppColors.modalTextPrimary,
                shadows: const [
                  Shadow(
                      color: Color(0x73000000),
                      blurRadius: 8,
                      offset: Offset(0, 1)),
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }
}

/// Full-screen hero rendered behind the glass preview card in preview
/// stage. Mirrors `experience_preview_modal._buildBackground`: if the
/// view model has finished loading a [VideoPlayerController] for the
/// streamed media, render [VideoPlayer]; otherwise show the still via
/// [BackgroundMediaImage] (which uses `mediaRepository.getMediaUrl` so
/// the path is image-decode safe for video thumbs too).
///
/// Video controller lifecycle (load + dispose) is owned by
/// [UnifiedCreateViewModel._loadPreviewVideoIfNeeded].
class _HeroMediaBackground extends ConsumerWidget {
  const _HeroMediaBackground({required this.mediaId});
  final String mediaId;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final swap = ref.watch(
      unifiedCreateViewModelProvider.select(
        (s) => (
          posterUrl: s.candidatePreviewPosterUrl,
          loading: s.mediaSwapInProgress,
          controller: s.previewVideoController,
        ),
      ),
    );
    // Transient poster: while a candidate import is in flight, render
    // the candidate's poster URL directly so the user sees the new
    // image right away. Once the import + (optional) video controller
    // settle, the poster clears and the regular hero path takes over.
    final Widget media;
    if (swap.posterUrl != null) {
      media = Positioned.fill(
        child: Container(
          color: Colors.black,
          child: CachedNetworkImage(
            imageUrl: swap.posterUrl!,
            fit: BoxFit.cover,
            placeholder: (_, _) => Container(color: Colors.black),
            errorWidget: (_, _, _) => Container(color: Colors.black),
          ),
        ),
      );
    } else if (swap.controller != null &&
        swap.controller!.value.isInitialized) {
      final c = swap.controller!;
      media = Positioned.fill(
        child: Container(
          color: Colors.black,
          child: SizedBox.expand(
            child: FittedBox(
              fit: BoxFit.cover,
              child: SizedBox(
                width: c.value.size.width,
                height: c.value.size.height,
                child: VideoPlayer(c),
              ),
            ),
          ),
        ),
      );
    } else {
      // getHeroMediaUrl picks the right URL for the hero shape: full
      // resolution for images (sharp full-bleed render), poster
      // thumbnail for videos (still while the MP4 downloads — the
      // full video URL can't be decoded by CachedNetworkImage).
      media = BackgroundMediaImage(
        key: ValueKey('unified-create-hero-$mediaId'),
        mediaId: mediaId,
        getMediaUrl: () =>
            ref.read(mediaRepositoryProvider).getHeroMediaUrl(mediaId),
      );
    }

    if (!swap.loading) return media;
    return Stack(
      children: [
        media,
        const Positioned.fill(
          child: ColoredBox(color: OverlayTokens.fieldFill),
        ),
        const Positioned.fill(
          child: Center(
            child: SizedBox(
              width: 36,
              height: 36,
              child: CircularProgressIndicator(
                strokeWidth: 3,
                color: Colors.white,
              ),
            ),
          ),
        ),
      ],
    );
  }
}

class _GlassIconButton extends StatelessWidget {
  const _GlassIconButton({
    required this.icon,
    required this.onTap,
    required this.semanticsLabel,
  });
  final IconData icon;
  final VoidCallback onTap;
  final String semanticsLabel;

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: semanticsLabel,
      onTap: onTap,
      inkBorderRadius: BorderRadius.circular(20),
      child: GlassSurface(
        borderRadius: BorderRadius.circular(20),
        padding: const EdgeInsets.all(8),
        child: Icon(icon, size: 20, color: AppColors.modalTextPrimary),
      ),
    );
  }
}
