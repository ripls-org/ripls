import 'package:flutter/gestures.dart';
import 'package:flutter/material.dart';
import 'package:url_launcher/url_launcher.dart';

/// A regex that matches HTTP/HTTPS URLs in text.
final urlRegex = RegExp(
  r'https?://[^\s]+',
  caseSensitive: false,
);

/// Splits [text] into TextSpan segments, making URLs tappable.
///
/// Non-URL text uses [style]. URLs use [linkStyle] and get a
/// [TapGestureRecognizer] that calls [onUrlTap].
List<InlineSpan> buildLinkifiedSpans({
  required String text,
  TextStyle? style,
  required TextStyle linkStyle,
  required void Function(String url) onUrlTap,
}) {
  final matches = urlRegex.allMatches(text).toList();
  if (matches.isEmpty) {
    return [TextSpan(text: text, style: style)];
  }

  final spans = <InlineSpan>[];
  var lastEnd = 0;

  for (final match in matches) {
    if (match.start > lastEnd) {
      spans.add(TextSpan(
        text: text.substring(lastEnd, match.start),
        style: style,
      ));
    }

    final url = match.group(0)!;
    spans.add(TextSpan(
      text: url,
      style: linkStyle,
      recognizer: TapGestureRecognizer()..onTap = () => onUrlTap(url),
    ));

    lastEnd = match.end;
  }

  if (lastEnd < text.length) {
    spans.add(TextSpan(
      text: text.substring(lastEnd),
      style: style,
    ));
  }

  return spans;
}

/// LinkifiedText renders text with embedded URLs as tappable links.
///
/// Non-URL text is rendered with [style]. URL segments use the same style but
/// with underline decoration and the theme's primary color, and open in the
/// system browser when tapped.
///
/// Supports [maxLines] and [overflow] for truncation.
class LinkifiedText extends StatelessWidget {
  final String text;
  final TextStyle? style;
  final int? maxLines;
  final TextOverflow? overflow;

  const LinkifiedText({
    super.key,
    required this.text,
    this.style,
    this.maxLines,
    this.overflow,
  });

  @override
  Widget build(BuildContext context) {
    final linkStyle = (style ?? const TextStyle()).copyWith(
      decoration: TextDecoration.underline,
      color: Theme.of(context).colorScheme.primary,
    );

    final spans = buildLinkifiedSpans(
      text: text,
      style: style,
      linkStyle: linkStyle,
      onUrlTap: _launchUrl,
    );

    return Text.rich(
      TextSpan(children: spans),
      maxLines: maxLines,
      overflow: overflow ?? TextOverflow.clip,
    );
  }

  Future<void> _launchUrl(String url) async {
    final uri = Uri.tryParse(url);
    if (uri != null) {
      await launchUrl(uri, mode: LaunchMode.externalApplication);
    }
  }
}
