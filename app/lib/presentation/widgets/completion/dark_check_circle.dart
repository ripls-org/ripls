import 'package:flutter/material.dart';
import 'package:ripls/core/theme/gen/completion_tokens.gen.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/core/theme/gen/overlay_tokens.gen.dart';
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';

// Semantic color constants shared across completion/fulfillment modals.
// These are aliases onto the token layer so the sheets follow a theme change;
// the brand hues live in the `completion` material and everything structural
// (surfaces, text, borders) comes from `glass`, because these sheets render on
// glass (#2492).
//
// The sheet base and glass border used to be aliased here too. Nothing reads
// them now that CompletionColors resolves those roles from the tokens directly,
// so they are gone — an alias with no callers is just a second name for a value.
const kCompletionGreen = CompletionTokens.green;
const kCompletionGreenLight = CompletionTokens.greenLight;
const kCompletionAccentLight = CompletionTokens.accentLight;

/// CompletionColors provides colors for the completion, fulfillment, and
/// people-picker sheets.
///
/// These surfaces render on an always-dark glass background regardless of the
/// app theme — glass is the standard for these sheets (#2492) — so every role
/// below resolves to a `glass` token and none of them branch on brightness.
///
/// The light-mode branches this class used to carry were dead: `_isDark` was
/// pinned to `true`, so half the values here could never be reached. They are
/// removed rather than left as a maintenance trap. If these sheets ever need a
/// light treatment it belongs in the token layer, per theme, not in `? :`
/// branches that no caller can select.
///
/// The `context` parameters are retained so the ~10 call sites and their tests
/// do not have to change; they are unused.
class CompletionColors {
  CompletionColors._();

  // ── Surfaces ────────────────────────────────────────────────────

  /// Sheet background.
  static Color background(BuildContext context) => CompletionTokens.sheet;

  /// Bottom bar / submit button container background — matches sheet.
  static Color bottomBar(BuildContext context) => background(context);

  /// Glass card border (search results container, impact bar).
  static Color glassBorder(BuildContext context) => GlassTokens.hairline;

  /// Search results / impact container fill.
  static Color containerFill(BuildContext context) => OverlayTokens.fieldFill;

  // ── Drag handle ─────────────────────────────────────────────────

  static Color handle(BuildContext context) => GlassTokens.dragHandle;

  // ── Text ────────────────────────────────────────────────────────

  /// Primary text (names, headings, date values).
  static Color textPrimary(BuildContext context) => GlassTokens.textPrimary;

  /// Secondary text (savings label, search placeholder faded).
  ///
  /// The four de-emphasised roles below (secondary, dim, section label,
  /// eyebrow) were four different weights between 25% and 40% white — a
  /// hierarchy fine enough that no one could see it, and low enough that the
  /// bottom of it could not pass 4.5:1 on this sheet. They now share the one
  /// faint-text step the glass ramp defines.
  static Color textSecondary(BuildContext context) => GlassTokens.textFaint;

  /// Dimmed text (subtitles, "tap to change", "Not yet on Ripls").
  static Color textDim(BuildContext context) => GlassTokens.textFaint;

  /// Uppercase section labels ("TAP TO CONFIRM", "WHO HELPED?", etc.).
  static Color sectionLabel(BuildContext context) => GlassTokens.textFaint;

  /// Eyebrow label above the title ("FULFILLED", "PAST LOAN", etc.).
  static Color eyebrow(BuildContext context) => GlassTokens.textFaint;

  /// Faded text used for the search placeholder and cancel button.
  static Color searchPlaceholder(BuildContext context) =>
      GlassTokens.textFaint;

  // ── Person tiles ────────────────────────────────────────────────

  /// Tile background — included vs. not.
  static Color tileBackground(BuildContext context, {required bool included}) =>
      included ? GlassTokens.scrimTint : Colors.transparent;

  /// Tile border — included vs. not.
  static Color tileBorder(BuildContext context, {required bool included}) =>
      included ? GlassTokens.hairline : GlassTokens.fillFaint;

  /// Tile name text — included vs. not.
  static Color tileText(BuildContext context, {required bool included}) =>
      included ? GlassTokens.textPrimary : GlassTokens.textFaint;

  // ── Search field ─────────────────────────────────────────────────

  /// Search field container border.
  static Color searchFieldBorder(BuildContext context) => GlassTokens.hairline;

  /// Search icon / ⌕ glyph color.
  static Color searchIcon(BuildContext context) => GlassTokens.textFaint;

  // ── Community grid ───────────────────────────────────────────────

  /// Grid item avatar badge background (the "+" add circle).
  static Color gridBadgeBackground(BuildContext context) =>
      CompletionTokens.green;

  /// Grid item avatar badge border — matches sheet background.
  static Color gridBadgeBorder(BuildContext context) => background(context);

  // ── Check circle ─────────────────────────────────────────────────

  /// Unchecked circle border.
  static Color checkBorder(BuildContext context) => GlassTokens.borderSoft;

  // ── Decorative glow ──────────────────────────────────────────────

  static Color glowColor(BuildContext context) =>
      CompletionTokens.green.withValues(alpha: 0.22);
}

/// DarkCheckCircle renders an animated check circle for completion modals.
///
/// Shows a filled green circle with a check icon when [checked], and an empty
/// bordered circle otherwise. Adapts to light and dark themes via
/// [CompletionColors].
class DarkCheckCircle extends StatelessWidget {
  const DarkCheckCircle({super.key, required this.checked});

  final bool checked;

  @override
  Widget build(BuildContext context) {
    return AnimatedContainer(
      duration: accessibleDuration(context, const Duration(milliseconds: 200)),
      width: 24,
      height: 24,
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        color: checked ? kCompletionGreen : Colors.transparent,
        border: checked
            ? null
            : Border.all(
                color: CompletionColors.checkBorder(context),
                width: 1.5,
              ),
      ),
      child: checked
          ? const Icon(Icons.check, size: 14, color: Colors.white)
          : null,
    );
  }
}
