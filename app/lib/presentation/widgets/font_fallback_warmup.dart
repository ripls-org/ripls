import 'package:flutter/foundation.dart';
import 'package:flutter/widgets.dart';

/// Starts the Flutter Web engine's lazy fallback-font downloads before real
/// content paints (#2724).
///
/// The web bundle ships no emoji or symbol glyphs: anything outside the
/// bundled fonts and the engine's default Latin subset — color emoji in chat,
/// the ₂ in "CO₂", the calendar weather glyphs — is covered by Noto fallback
/// fonts that the engine downloads only after it first lays out a paragraph
/// containing such a glyph. Text painted before that download completes shows
/// tofu boxes, and paragraphs already on screen do not reliably re-render when
/// the font lands (chat history keeps tofu while a message arriving seconds
/// later renders color emoji fine). Laying out this offstage text on the
/// splash screen moves the downloads to app boot, so the first real paint of
/// chat history, impact metrics, and weather rows resolves every common glyph.
///
/// The engine covers emoji with a family of "Noto Color Emoji" subset fonts
/// and resolves missing glyphs with a greedy set cover, so [warmupGlyphs]
/// holds one representative glyph per font we warm — each chosen (against the
/// Flutter 3.44 web engine's fallback tables) to be covered by exactly one
/// subset font, which makes the download set deterministic. Regional-indicator
/// flags (🇺🇸 …) are deliberately not warmed: their subset font is ~700 KB and
/// flags are rare in real content; they still lazy-load on first use. All
/// downloads run in parallel during the splash screen's minimum display time
/// and are long-lived in the browser cache, so the cost is first-visit-only.
///
/// On non-web platforms the system fonts cover all of these glyphs natively
/// and this widget renders nothing.
class FontFallbackWarmup extends StatelessWidget {
  const FontFallbackWarmup({super.key});

  /// One glyph per fallback font warmed at boot.
  static const String warmupGlyphs = '₂' // subscript two ("CO₂") → Noto Sans
      '→' // arrow used in drill-down link copy → Noto Sans Symbols
      '🏁' // pennants → Noto Color Emoji subset 1
      '✅' // check / cross marks → subset 2
      '👑' // objects & clothing → subset 3
      '🎁' // celebration → subset 4
      '🎠' // places & scenery → subset 5
      '🌮' // food → subset 6
      '⛅🌊' // nature & weather, incl. all calendar glyphs → subset 7
      '🎅' // people → subset 8
      '👋' // smileys & hands → subset 9
      '🫩'; // newest emoji → subset 11 (subset 10 is fully co-covered)

  @override
  Widget build(BuildContext context) {
    if (!kIsWeb) return const SizedBox.shrink();
    // Offstage still lays out its child, and the web engine detects
    // unresolved code points at paragraph layout — painting is not required
    // to start the font downloads. Offstage also keeps the glyphs out of the
    // semantics tree, so screen readers and the e2e locators never see them.
    return const Offstage(
      child: Text(warmupGlyphs, textDirection: TextDirection.ltr),
    );
  }
}
