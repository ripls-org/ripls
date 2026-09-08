import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/image_cache_keys.dart';
import 'package:ripls/presentation/widgets/chat/cached_media_image.dart';
import 'package:ripls/services/providers/media_providers.dart';

/// One face in a [ProfileFaceStack]: an avatar media id (resolved via
/// the media repository) with an initial to fall back on while the
/// avatar loads or when the person has none.
class FaceStackEntry {
  const FaceStackEntry({this.mediaId, this.initial});

  /// Avatar media id. Null or empty renders the tinted-initial fallback.
  final String? mediaId;

  /// Single character shown in the fallback circle (e.g. "B").
  final String? initial;
}

/// A row of small overlapping circular avatars with an optional "+N"
/// overflow tail — the mini social-pull stack on open-item rows and the
/// profile ask sheet (issue #2568, `profile-final-hybrid-v2.html`).
///
/// Purely visual by default (the surrounding row carries the semantic
/// label); pass [semanticsLabel] when the stack is the only place the
/// information appears.
class ProfileFaceStack extends StatelessWidget {
  const ProfileFaceStack({
    super.key,
    required this.faces,
    this.overflowCount = 0,
    this.size = 20,
    this.semanticsLabel,
  });

  final List<FaceStackEntry> faces;

  /// Rendered as a "+N" text tail after the circles when > 0.
  final int overflowCount;

  /// Diameter of each circle.
  final double size;

  /// Spoken label for the whole stack; null excludes it from semantics.
  final String? semanticsLabel;

  @override
  Widget build(BuildContext context) {
    if (faces.isEmpty && overflowCount <= 0) return const SizedBox.shrink();
    final circles = <Widget>[];
    for (var i = 0; i < faces.length; i++) {
      circles.add(Padding(
        padding: EdgeInsets.only(left: i == 0 ? 0 : size * 0.7),
        child: _Face(entry: faces[i], size: size),
      ));
    }
    final row = Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        if (circles.isNotEmpty) Stack(children: circles),
        if (overflowCount > 0) ...[
          const SizedBox(width: 5),
          Text(
            '+$overflowCount',
            style: TextStyle(
              fontSize: size * 0.55,
              fontWeight: FontWeight.w600,
              color: Colors.white.withValues(alpha: 0.82),
            ),
          ),
        ],
      ],
    );
    if (semanticsLabel == null) return ExcludeSemantics(child: row);
    return Semantics(label: semanticsLabel, child: ExcludeSemantics(child: row));
  }
}

class _Face extends ConsumerWidget {
  const _Face({required this.entry, required this.size});

  final FaceStackEntry entry;
  final double size;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final fallback = Container(
      width: size,
      height: size,
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        color: AppColors.primary(context).withValues(alpha: 0.55),
        border: Border.all(
          color: Colors.white.withValues(alpha: 0.35),
          width: 1.2,
        ),
      ),
      alignment: Alignment.center,
      child: entry.initial == null || entry.initial!.isEmpty
          ? null
          : Text(
              entry.initial!,
              style: TextStyle(
                fontSize: size * 0.48,
                fontWeight: FontWeight.w700,
                color: Colors.white,
              ),
            ),
    );
    final mediaId = entry.mediaId;
    if (mediaId == null || mediaId.isEmpty) return fallback;
    final asyncMedia = ref.watch(mediaObjectProvider(mediaId));
    return asyncMedia.when(
      data: (media) {
        if (media.url.isEmpty) return fallback;
        return Container(
          width: size,
          height: size,
          decoration: BoxDecoration(
            shape: BoxShape.circle,
            border: Border.all(
              color: Colors.white.withValues(alpha: 0.35),
              width: 1.2,
            ),
          ),
          child: ClipOval(
            child: CachedMediaImage(
              imageUrl: media.url,
              cacheKey: ImageCacheKeys.thumbnail(mediaId),
              fit: BoxFit.cover,
              errorWidget: fallback,
              semanticsLabel: null,
            ),
          ),
        );
      },
      loading: () => fallback,
      error: (_, _) => fallback,
    );
  }
}
