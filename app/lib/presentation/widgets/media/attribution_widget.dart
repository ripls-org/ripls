import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/utils/attribution_utils.dart';
import 'package:ripls/data/gen/ripls/api/media_service.pb.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:url_launcher/url_launcher.dart';

/// Widget that displays photo attribution for stock images from Unsplash or Pexels.
///
/// Shows "Photo by [Creator Name] on [Provider]" with clickable links to the
/// creator's profile and the provider's website. Only displayed when attribution data exists.
///
/// Complies with Unsplash and Pexels API Guidelines requiring creator attribution.
class AttributionWidget extends StatelessWidget {
  final Attribution attribution;
  final TextStyle? textStyle;
  final bool compact;

  const AttributionWidget({
    super.key,
    required this.attribution,
    this.textStyle,
    this.compact = false,
  });

  @override
  Widget build(BuildContext context) {
    // Use white text on semi-transparent dark overlay (design system pattern)
    final overlayTextStyle = textStyle ??
        const TextStyle(
          color: Colors.white,
          fontSize: 11,
          shadows: [
            Shadow(
              color: Colors.black,
              blurRadius: 2,
            ),
          ],
        );

    if (compact) {
      return _buildCompactAttribution(context, overlayTextStyle);
    }

    return _buildFullAttribution(context, overlayTextStyle);
  }

  Widget _buildFullAttribution(BuildContext context, TextStyle? style) {
    final providerName = AttributionUtils.getProviderName(attribution);
    return DefaultTextStyle.merge(
      style: style,
      child: Semantics(
        label: context.l10n.a11yMediaAttributionPhotoBy(
          attribution.creatorName,
          providerName,
        ),
        child: Wrap(
          children: [
            Text('Photo by ', style: style),
            _buildCreatorLink(context, style),
            Text(' on ', style: style),
            _buildPlatformLink(context, style),
          ],
        ),
      ),
    );
  }

  Widget _buildCompactAttribution(BuildContext context, TextStyle? style) {
    final providerName = AttributionUtils.getProviderName(attribution);
    return DefaultTextStyle.merge(
      style: style,
      child: Semantics(
        label: context.l10n.a11yMediaAttributionPhotoBy(
          attribution.creatorName,
          providerName,
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(Icons.camera_alt, size: 12, color: Colors.white),
            const SizedBox(width: 4),
            Flexible(
              child: _buildCreatorLink(context, style),
            ),
            Text(' on ', style: style),
            _buildPlatformLink(context, style),
          ],
        ),
      ),
    );
  }

  Widget _buildCreatorLink(BuildContext context, TextStyle? style) {
    final providerName = AttributionUtils.getProviderName(attribution);
    return Tappable(
      semanticsLabel: context.l10n.a11yMediaAttributionOpenCreatorProfile(
        attribution.creatorName,
        providerName,
      ),
      isLink: true,
      onTap: _openCreatorProfile,
      child: Text(
        attribution.creatorName,
        style: style?.copyWith(
          decoration: TextDecoration.underline,
          decorationColor: Colors.white,
          decorationThickness: 1.5,
        ),
        overflow: compact ? TextOverflow.ellipsis : null,
      ),
    );
  }

  Widget _buildPlatformLink(BuildContext context, TextStyle? style) {
    final providerName = AttributionUtils.getProviderName(attribution);
    return Tappable(
      semanticsLabel: context.l10n.a11yMediaAttributionOpenProvider(providerName),
      isLink: true,
      onTap: _openPlatform,
      child: Text(
        providerName,
        style: style?.copyWith(
          decoration: TextDecoration.underline,
          decorationColor: Colors.white,
          decorationThickness: 1.5,
        ),
      ),
    );
  }

  Future<void> _openCreatorProfile() async {
    final urlString = AttributionUtils.getCreatorProfileUrl(attribution);
    if (urlString.isEmpty) return;

    final url = Uri.parse(urlString);
    if (await canLaunchUrl(url)) {
      await launchUrl(url, mode: LaunchMode.externalApplication);
    }
  }

  Future<void> _openPlatform() async {
    final urlString = AttributionUtils.getPlatformUrl(attribution);
    if (urlString.isEmpty) return;

    final url = Uri.parse(urlString);
    if (await canLaunchUrl(url)) {
      await launchUrl(url, mode: LaunchMode.externalApplication);
    }
  }
}
