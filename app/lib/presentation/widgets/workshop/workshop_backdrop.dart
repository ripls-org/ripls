import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/presentation/widgets/media/video_background_host.dart';
import 'package:ripls/services/providers/media_providers.dart';

/// Full-bleed background for the Workshop overview and its destinations
/// (chooser, library map/calendar) — #2447.
///
/// - **Single community** ([mediaId] non-null): resolves the community's hero
///   media id to a URL via [heroMediaUrlProvider] (the provider only paints
///   from a resolved `mediaPath`, not a bare id) and renders it through
///   [VideoBackgroundHost]. While the URL resolves, the bundled ripples image
///   shows so the screen is never bare black.
/// - **Everything / no community photo** ([mediaId] null): the standard bundled
///   `assets/images/ripples.png` backdrop.
///
/// Always decorative — excluded from the semantics tree.
class WorkshopBackdrop extends ConsumerWidget {
  const WorkshopBackdrop({super.key, this.mediaId});

  final String? mediaId;

  static const String _asset = 'assets/images/ripples.png';

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final id = mediaId;
    if (id == null) return _ripples();

    final mediaUrl = ref.watch(heroMediaUrlProvider(id)).asData?.value;
    if (mediaUrl == null || mediaUrl.url.isEmpty) {
      // Resolving (or unresolvable) — fall back to the standard backdrop
      // rather than flashing black.
      return _ripples();
    }
    return ExcludeSemantics(
      child: VideoBackgroundHost(mediaPath: mediaUrl.url, mediaId: id),
    );
  }

  Widget _ripples() => const ExcludeSemantics(
        child: SizedBox.expand(
          child: Image(
            image: AssetImage(_asset),
            fit: BoxFit.cover,
            alignment: Alignment.topCenter,
          ),
        ),
      );
}
