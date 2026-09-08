import 'dart:io';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/data/gen/ripls/api/media_service.pb.dart';
import 'package:ripls/data/repositories/media_url.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/chat/cached_media_image.dart';
import 'package:ripls/presentation/widgets/media/attribution_widget.dart';
import 'package:ripls/presentation/widgets/media/carousel/carousel_media_item.dart';
import 'package:ripls/services/providers/media_providers.dart';

/// CarouselImageViewer renders a single image in the carousel with
/// pinch-to-zoom and a tap handler that opens the fullscreen viewer.
///
/// When [item.mediaId] is set, the viewer resolves a fresh full-resolution
/// presigned URL through [fullMediaUrlProvider]. Presigned URLs expire 35
/// minutes after generation; capturing a URL into a tap closure at chat-row
/// build time and reusing it minutes or hours later is the failure mode
/// behind #1833 — by re-resolving at viewer-mount time we get a URL that
/// is fresh relative to the moment the image is actually displayed, and we
/// can recover from a transient expiry with a single invalidate-and-retry.
///
/// When [item.mediaId] is null (locally-staged uploads, etc.), the viewer
/// falls back to [item.url] verbatim.
class CarouselImageViewer extends StatelessWidget {
  final MediaItem item;
  final VoidCallback onTap;

  const CarouselImageViewer({
    super.key,
    required this.item,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: context.l10n.a11yMediaViewFullscreen,
      onTap: onTap,
      excludeChildSemantics: false,
      child: InteractiveViewer(
        minScale: 1,
        maxScale: 4,
        child: Center(child: _buildImage(context)),
      ),
    );
  }

  Widget _buildImage(BuildContext context) {
    final mediaId = item.mediaId;
    if (mediaId != null && mediaId.isNotEmpty) {
      return _CarouselNetworkImage(mediaId: mediaId, fallbackUrl: item.url);
    }
    if (item.url.startsWith('http')) {
      return _CarouselNetworkImage(mediaId: null, fallbackUrl: item.url);
    }
    return Image.file(
      File(item.url),
      fit: BoxFit.contain,
      errorBuilder: (context, error, stackTrace) =>
          _CarouselImageError(semanticsLabel: context.l10n.a11yMediaLoadError),
    );
  }
}

/// FullscreenImageViewer shows an image fullscreen with pinch-to-zoom and a
/// close button.
class FullscreenImageViewer extends StatelessWidget {
  final String? mediaId;
  final String? imageUrl;
  final Attribution? attribution;

  const FullscreenImageViewer({
    super.key,
    this.mediaId,
    this.imageUrl,
    this.attribution,
  }) : assert(mediaId != null || imageUrl != null,
            'FullscreenImageViewer needs at least a mediaId or an imageUrl');

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.black,
      body: Tappable(
        semanticsLabel: context.l10n.a11yClose,
        onTap: () => Navigator.of(context).pop(),
        excludeChildSemantics: false,
        child: Stack(
          fit: StackFit.expand,
          children: [
            InteractiveViewer(
              minScale: 1,
              maxScale: 5,
              child: Center(
                child: Stack(
                  children: [
                    _buildImage(context),
                    if (attribution != null)
                      Positioned(
                        left: 0,
                        right: 0,
                        bottom: 0,
                        child: Container(
                          padding: const EdgeInsets.symmetric(
                              horizontal: 16, vertical: 12),
                          decoration: BoxDecoration(
                            gradient: LinearGradient(
                              begin: Alignment.bottomCenter,
                              end: Alignment.topCenter,
                              colors: [
                                Colors.black.withValues(alpha: 0.7),
                                Colors.black.withValues(alpha: 0.3),
                                Colors.transparent,
                              ],
                            ),
                          ),
                          child: AttributionWidget(attribution: attribution!),
                        ),
                      ),
                  ],
                ),
              ),
            ),
            Positioned(
              top: 0,
              right: 0,
              child: SafeArea(
                child: Padding(
                  padding: const EdgeInsets.all(16),
                  child: IconAction(
                    icon: Icons.close,
                    iconSize: 32,
                    color: Colors.white,
                    semanticsLabel: context.l10n.a11yClose,
                    onPressed: () => Navigator.of(context).pop(),
                  ),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildImage(BuildContext context) {
    final id = mediaId;
    if (id != null && id.isNotEmpty) {
      return _CarouselNetworkImage(mediaId: id, fallbackUrl: imageUrl ?? '');
    }
    final url = imageUrl ?? '';
    if (url.startsWith('http')) {
      return _CarouselNetworkImage(mediaId: null, fallbackUrl: url);
    }
    return Image.file(
      File(url),
      fit: BoxFit.contain,
      errorBuilder: (context, error, stackTrace) =>
          _CarouselImageError(semanticsLabel: context.l10n.a11yMediaLoadError),
    );
  }
}

/// _CarouselNetworkImage resolves a fresh [MediaUrl] from
/// [fullMediaUrlProvider] (when [mediaId] is non-null) and renders the bytes
/// via [CachedMediaImage]. On error it invalidates the metadata cache and
/// re-invalidates the provider once, recovering from a single expired
/// presigned URL without paying a round-trip on the happy path.
///
/// When [mediaId] is null the widget renders [fallbackUrl] directly with no
/// re-resolution — used for locally-staged uploads that have no server-side
/// metadata yet.
class _CarouselNetworkImage extends ConsumerStatefulWidget {
  final String? mediaId;
  final String fallbackUrl;

  const _CarouselNetworkImage({
    required this.mediaId,
    required this.fallbackUrl,
  });

  @override
  ConsumerState<_CarouselNetworkImage> createState() =>
      _CarouselNetworkImageState();
}

class _CarouselNetworkImageState extends ConsumerState<_CarouselNetworkImage> {
  bool _retried = false;

  @override
  Widget build(BuildContext context) {
    final mediaId = widget.mediaId;
    if (mediaId == null) {
      return CachedMediaImage(
        imageUrl: widget.fallbackUrl,
        fit: BoxFit.contain,
        semanticsLabel: context.l10n.a11yViewMediaContent,
        errorWidget:
            _CarouselImageError(semanticsLabel: context.l10n.a11yMediaLoadError),
      );
    }

    final async = ref.watch(fullMediaUrlProvider(mediaId));
    return async.when(
      loading: () => const Center(
        child: CircularProgressIndicator(color: Colors.white),
      ),
      error: (error, _) {
        _maybeRetry(mediaId);
        return _CarouselImageError(
          semanticsLabel: context.l10n.a11yMediaLoadError,
          retryLabel: context.l10n.a11yMediaRetry,
          onRetry: () => _forceRetry(mediaId),
        );
      },
      data: (mediaUrl) => _CarouselDataImage(
        mediaId: mediaId,
        mediaUrl: mediaUrl,
        onLoadFailed: () => _maybeRetry(mediaId),
        onRetry: () => _forceRetry(mediaId),
      ),
    );
  }

  void _maybeRetry(String mediaId) {
    if (_retried) return;
    _retried = true;
    WidgetsBinding.instance.addPostFrameCallback((_) async {
      if (!mounted) return;
      await ref.read(mediaRepositoryProvider).invalidate(mediaId);
      if (!mounted) return;
      ref.invalidate(fullMediaUrlProvider(mediaId));
    });
  }

  void _forceRetry(String mediaId) async {
    setState(() => _retried = false);
    await ref.read(mediaRepositoryProvider).invalidate(mediaId);
    if (!mounted) return;
    ref.invalidate(fullMediaUrlProvider(mediaId));
  }
}

/// _CarouselDataImage renders the resolved [MediaUrl] using
/// [CachedMediaImage]. The byte fetch can still fail (the metadata cache
/// could have served a slightly-aged URL); on that failure we call
/// [onLoadFailed] so the parent can drive the single invalidate-and-retry.
class _CarouselDataImage extends StatelessWidget {
  final String mediaId;
  final MediaUrl mediaUrl;
  final VoidCallback onLoadFailed;
  final VoidCallback onRetry;

  const _CarouselDataImage({
    required this.mediaId,
    required this.mediaUrl,
    required this.onLoadFailed,
    required this.onRetry,
  });

  @override
  Widget build(BuildContext context) {
    return CachedMediaImage(
      imageUrl: mediaUrl.url,
      cacheKey: mediaUrl.cacheKey,
      fit: BoxFit.contain,
      semanticsLabel: context.l10n.a11yViewMediaContent,
      errorWidget: _CarouselImageError(
        semanticsLabel: context.l10n.a11yMediaLoadError,
        retryLabel: context.l10n.a11yMediaRetry,
        onRetry: () {
          onLoadFailed();
          onRetry();
        },
      ),
    );
  }
}

/// _CarouselImageError renders the in-place error glyph and (when [onRetry]
/// is non-null) an icon button that re-fetches the URL. The whole region is
/// wrapped in a [Semantics] live region so screen readers announce the
/// failure when it appears in place of a loading image.
class _CarouselImageError extends StatelessWidget {
  final String semanticsLabel;
  final String? retryLabel;
  final VoidCallback? onRetry;

  const _CarouselImageError({
    required this.semanticsLabel,
    this.retryLabel,
    this.onRetry,
  });

  @override
  Widget build(BuildContext context) {
    return Semantics(
      label: semanticsLabel,
      liveRegion: true,
      container: true,
      child: Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(Icons.error_outline, color: Colors.white, size: 48),
            if (onRetry != null && retryLabel != null) ...[
              const SizedBox(height: 12),
              IconAction(
                icon: Icons.refresh,
                iconSize: 28,
                color: Colors.white,
                semanticsLabel: retryLabel!,
                onPressed: onRetry,
              ),
            ],
          ],
        ),
      ),
    );
  }
}
