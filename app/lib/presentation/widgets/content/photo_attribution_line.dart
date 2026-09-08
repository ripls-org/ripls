import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/utils/attribution_utils.dart';
import 'package:ripls/data/gen/ripls/api/media_service.pb.dart' show Attribution;
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:url_launcher/url_launcher.dart';

/// PhotoAttributionLine renders the small, right-aligned "Photo by {artist}
/// on {source}" credit that sits just above the content view tab bar.
///
/// Replaces the credit display that lived in [ContentInfoModal]. Both
/// hyperlinks open the corresponding provider URL when tapped.
class PhotoAttributionLine extends StatelessWidget {
  final Attribution attribution;

  /// When true, the credit is wrapped in a semi-transparent black pill so it
  /// stays legible over busy imagery (e.g. the experience read shell top bar).
  final bool boxed;

  const PhotoAttributionLine({
    super.key,
    required this.attribution,
    this.boxed = false,
  });

  @override
  Widget build(BuildContext context) {
    final artistName = attribution.creatorName.isNotEmpty
        ? attribution.creatorName
        : AttributionUtils.getProviderName(attribution);
    final sourceName = AttributionUtils.getProviderName(attribution);
    final artistUrl = AttributionUtils.getCreatorProfileUrl(attribution);
    final sourceUrl = AttributionUtils.getPlatformUrl(attribution);

    final baseStyle = TextStyle(
      fontSize: 10.5,
      fontStyle: FontStyle.italic,
      height: 1,
      color: Colors.white.withValues(alpha: 0.8),
      shadows: const [
        Shadow(
          blurRadius: 3,
          color: Color(0x8C000000),
          offset: Offset(0, 1),
        ),
      ],
    );
    final linkStyle = baseStyle.copyWith(
      color: Colors.white,
      decoration: TextDecoration.underline,
      decorationColor: Colors.white.withValues(alpha: 0.45),
    );

    Widget line = Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Text('${context.l10n.photoAttributionPrefix} ', style: baseStyle),
        Tappable(
          semanticsLabel: context.l10n.photoAttributionArtistSemantics,
          onTap: () => _open(artistUrl),
          isLink: true,
          child: Text(artistName, style: linkStyle),
        ),
        Text(' ${context.l10n.photoAttributionConjunction} ',
            style: baseStyle),
        Tappable(
          semanticsLabel: context.l10n.photoAttributionSourceSemantics,
          onTap: () => _open(sourceUrl),
          isLink: true,
          child: Text(sourceName, style: linkStyle),
        ),
      ],
    );

    if (boxed) {
      line = DecoratedBox(
        decoration: BoxDecoration(
          color: Colors.black.withValues(alpha: 0.4),
          borderRadius: BorderRadius.circular(999),
        ),
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 5),
          child: line,
        ),
      );
    }

    return Align(
      alignment: Alignment.centerRight,
      child: Padding(
        padding: const EdgeInsets.only(right: 4, bottom: 4),
        child: line,
      ),
    );
  }

  Future<void> _open(String url) async {
    if (url.isEmpty) return;
    final uri = Uri.tryParse(url);
    if (uri == null) return;
    await launchUrl(uri, mode: LaunchMode.externalApplication);
  }
}
