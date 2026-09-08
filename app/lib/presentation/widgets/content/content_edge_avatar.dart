import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/chat/cached_media_image.dart';
import 'package:ripls/presentation/widgets/content/content_edge_data.dart';
import 'package:ripls/services/providers.dart';

/// ContentEdgeAvatar is one participant's face on a content view's "who's in"
/// card: their photo (initials when they have none) with a small status badge
/// at the bottom-right.
///
/// The status rides as a badge instead of replacing the face. A roster of
/// identical check circles says how many people are in but not *who* — and the
/// people are the point of the card (#2724).
///
/// Takes an [EdgeViewData] rather than a `User` because the card's rows are
/// proto-free view data; that also means the media id and initials are already
/// resolved by the view-model.
class ContentEdgeAvatar extends ConsumerWidget {
  const ContentEdgeAvatar({
    super.key,
    required this.edge,
    required this.accentColor,
    this.size = 26,
  });

  /// The participant to render.
  final EdgeViewData edge;

  /// The content view's accent, used for the fallback face and the
  /// confirmed-participation badge.
  final Color accentColor;

  /// Avatar diameter. The badge scales with it.
  final double size;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return SizedBox(
      width: size,
      height: size,
      child: Stack(
        clipBehavior: Clip.none,
        children: [
          ClipOval(
            child: SizedBox(width: size, height: size, child: _face(ref)),
          ),
          Positioned(right: -2, bottom: -2, child: _badge()),
        ],
      ),
    );
  }

  /// The photo, or the initials fallback while it loads / when there is none.
  Widget _face(WidgetRef ref) {
    final mediaId = edge.mediaId;
    if (mediaId == null || mediaId.isEmpty) return _initials();
    return FutureBuilder<String?>(
      future: ref
          .read(mediaRepositoryProvider)
          .getMediaUrl(mediaId)
          .then((mediaUrl) => mediaUrl.url),
      builder: (context, snapshot) {
        final url = snapshot.data;
        if (url == null || url.isEmpty) return _initials();
        return CachedMediaImage(
          // Decorative: the row's name text carries the identity.
          semanticsLabel: null,
          imageUrl: url,
          cacheKey: mediaId,
          width: size,
          height: size,
          fit: BoxFit.cover,
          errorWidget: _initials(),
          placeholder: _initials(),
        );
      },
    );
  }

  Widget _initials() {
    return DecoratedBox(
      decoration: BoxDecoration(
        gradient: LinearGradient(
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
          colors: [accentColor, accentColor.withValues(alpha: 0.7)],
        ),
      ),
      child: Center(
        child: Text(
          edge.initials,
          style: TextStyle(
            color: AppColors.darkBackground,
            fontSize: size * 0.36,
            fontWeight: FontWeight.w800,
          ),
        ),
      ),
    );
  }

  /// The status dot: a check for confirmed, an amber "?" for maybe, and a
  /// hollow ring for an invitee who hasn't replied. Ringed in the card's own
  /// background so it reads as cut out of the face.
  Widget _badge() {
    final diameter = size * 0.46;
    late final Color fill;
    late final Widget? glyph;
    switch (edge.status) {
      case EdgeStatus.going:
      case EdgeStatus.host:
      case EdgeStatus.wrapped:
        fill = accentColor;
        glyph = Icon(
          Icons.check_rounded,
          size: diameter * 0.78,
          color: AppColors.darkBackground,
        );
      case EdgeStatus.maybe:
        fill = AppColors.statusWarningOnDark;
        glyph = Text(
          '?',
          style: TextStyle(
            color: AppColors.darkBackground,
            fontSize: diameter * 0.72,
            fontWeight: FontWeight.w800,
            height: 1,
          ),
        );
      case EdgeStatus.invited:
        // No reply yet — an empty ring, the same "waiting" shape the roster
        // has always used for invitees.
        fill = AppColors.darkTextTertiary;
        glyph = null;
    }
    return Container(
      width: diameter,
      height: diameter,
      alignment: Alignment.center,
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        color: fill,
        border: Border.all(color: AppColors.darkBackground, width: 1.5),
      ),
      child: glyph,
    );
  }
}
