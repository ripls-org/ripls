import 'package:cached_network_image/cached_network_image.dart';
import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/overlay_tokens.gen.dart';

/// CachedMediaImage displays a cached network image with proper handling of
/// presigned URLs that may expire. Uses the media ID as a stable cache key
/// to avoid issues with URL expiration.
///
/// When [imageUrl] is empty (e.g., a video without a thumbnail), a static
/// placeholder is shown immediately without attempting a network load.
///
/// When [contentType] indicates video and [imageUrl] is non-empty, a play
/// icon overlay is rendered on top of the thumbnail image so users can
/// distinguish video items in list views.
class CachedMediaImage extends StatelessWidget {
  final String imageUrl;
  final String? cacheKey;
  final BoxFit? fit;
  final double? width;
  final double? height;
  final Widget? placeholder;
  final Widget? errorWidget;
  final BorderRadius? borderRadius;

  /// MIME content type of the media (e.g. "video/mp4", "image/jpeg").
  ///
  /// When set to a video type, a play icon overlay is rendered over the
  /// thumbnail to signal that the item is a video.
  final String? contentType;

  /// Semantic label describing the image for assistive technologies. When
  /// null, the image is treated as decorative and excluded from the semantics
  /// tree. Callers should pass a localized, content-specific description
  /// (e.g. "Photo of ${gear.name}") whenever the image conveys meaning.
  final String? semanticsLabel;

  /// Optional kebab-case identifier exposed on the Semantics node, used by
  /// the e2e Playwright harness to find this image. See
  /// docs/client/testing/semantics_identifiers.md. When set, the image is
  /// included in the semantics tree even if [semanticsLabel] is null — but
  /// no spoken label is added, so it stays silent to screen readers.
  final String? semanticsIdentifier;

  const CachedMediaImage({
    super.key,
    required this.imageUrl,
    this.cacheKey,
    this.fit,
    this.width,
    this.height,
    this.placeholder,
    this.errorWidget,
    this.borderRadius,
    this.contentType,
    this.semanticsLabel,
    this.semanticsIdentifier,
  });

  @override
  Widget build(BuildContext context) {
    final visual = _buildVisual(context);
    if (semanticsLabel == null && semanticsIdentifier == null) {
      return ExcludeSemantics(child: visual);
    }
    return Semantics(
      identifier: semanticsIdentifier,
      label: semanticsLabel,
      image: semanticsLabel != null,
      child: ExcludeSemantics(child: visual),
    );
  }

  Widget _buildVisual(BuildContext context) {
    // Empty URL means no displayable image (e.g., video without thumbnail).
    // Show a static placeholder immediately to avoid CachedNetworkImage
    // attempting to decode video bytes, which causes "Invalid image data" errors.
    if (imageUrl.isEmpty) {
      final empty = _buildEmptyPlaceholder(context);
      if (borderRadius != null) {
        return ClipRRect(borderRadius: borderRadius!, child: empty);
      }
      return empty;
    }

    Widget image = CachedNetworkImage(
      imageUrl: imageUrl,
      cacheKey: cacheKey,
      fit: fit ?? BoxFit.cover,
      width: width,
      height: height,
      placeholder: (context, url) =>
          placeholder ?? _buildDefaultPlaceholder(context),
      errorWidget: (context, url, error) =>
          errorWidget ?? _buildDefaultErrorWidget(context),
      // Fade the loaded bytes in over the placeholder instead of snapping
      // (#2724). Memory-cache hits resolve synchronously and skip the fade
      // entirely (OctoImage renders sync-available images without animating),
      // so cached images still appear instantly.
      fadeInDuration: const Duration(milliseconds: 220),
      fadeOutDuration: const Duration(milliseconds: 160),
      placeholderFadeInDuration: Duration.zero,
    );

    // Overlay a play icon when the content is a video.
    final isVideo = contentType?.toLowerCase().startsWith('video/') ?? false;
    if (isVideo) {
      image = Stack(
        alignment: Alignment.center,
        children: [
          image,
          _buildPlayOverlay(context),
        ],
      );
    }

    if (borderRadius != null) {
      return ClipRRect(borderRadius: borderRadius!, child: image);
    }

    return image;
  }

  /// Builds a static placeholder for empty URLs (no spinner — nothing is loading).
  Widget _buildEmptyPlaceholder(BuildContext context) {
    return Container(
      width: width,
      height: height,
      color: AppColors.surface(context),
    );
  }

  /// Default loading placeholder: the theme surface with a subtle shimmer
  /// sweep instead of a spinner (#2724). Spinners multiply badly in media
  /// grids, and a bright block is the worst-case treatment on the dark
  /// theme — the shimmer stays inside the theme's surface ramp.
  Widget _buildDefaultPlaceholder(BuildContext context) {
    return _ShimmerPlaceholder(width: width, height: height);
  }

  Widget _buildDefaultErrorWidget(BuildContext context) {
    return Container(
      width: width,
      height: height,
      color: AppColors.surface(context),
      child: Center(
        child: Icon(
          Icons.construction,
          size: 40,
          color: AppColors.textTertiary(context),
        ),
      ),
    );
  }

  Widget _buildPlayOverlay(BuildContext context) {
    return Container(
      decoration: BoxDecoration(
        color: AppColors.contentOverlay(context),
        shape: BoxShape.circle,
      ),
      padding: const EdgeInsets.all(6),
      child: const Icon(
        Icons.play_arrow,
        color: OverlayTokens.textPrimary,
        size: 22,
      ),
    );
  }
}

/// Theme-surface loading placeholder with a slow, low-contrast shimmer sweep.
/// Self-contained (no shimmer package): a repeating [AnimationController]
/// slides a soft highlight band across the surface color. Honors the OS
/// reduce-motion setting by rendering the static surface instead.
class _ShimmerPlaceholder extends StatefulWidget {
  const _ShimmerPlaceholder({this.width, this.height});

  final double? width;
  final double? height;

  @override
  State<_ShimmerPlaceholder> createState() => _ShimmerPlaceholderState();
}

class _ShimmerPlaceholderState extends State<_ShimmerPlaceholder>
    with SingleTickerProviderStateMixin {
  late final AnimationController _controller = AnimationController(
    vsync: this,
    duration: const Duration(milliseconds: 1400),
  )..repeat();

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final surface = AppColors.surface(context);
    if (MediaQuery.of(context).disableAnimations) {
      return Container(
        width: widget.width,
        height: widget.height,
        color: surface,
      );
    }
    // The highlight is the surface nudged toward the theme's text color, so
    // it reads as a sheen on both the light and dark surface. alphaBlend
    // keeps the band fully opaque — a translucent gradient stop would let
    // the content behind the card bleed through instead of shimmering.
    final highlight = Color.alphaBlend(
      AppColors.textPrimary(context).withValues(alpha: 0.06),
      surface,
    );
    return AnimatedBuilder(
      animation: _controller,
      builder: (context, _) {
        // Slide the gradient band from off-screen left to off-screen right.
        final dx = _controller.value * 3 - 1.5;
        return Container(
          width: widget.width,
          height: widget.height,
          decoration: BoxDecoration(
            // No separate fill: the gradient's edge stops are the surface
            // color, so the band always sits on the plain surface.
            gradient: LinearGradient(
              begin: Alignment(dx - 1, -0.3),
              end: Alignment(dx + 1, 0.3),
              colors: [surface, highlight, surface],
              stops: const [0.35, 0.5, 0.65],
            ),
          ),
        );
      },
    );
  }
}
