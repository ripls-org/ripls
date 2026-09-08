import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/content/content_action_chip.dart';

/// A call-to-action chip rendered at the foot of a [ContentFactData] card
/// (event V6b): "Vote ›" while a poll is running, or "Set time ›" / "Set
/// location ›" for the owner when the fact is still unset.
class ContentFactCta {
  /// Visible chip text (caller-resolved from `context.l10n`).
  final String label;

  /// Screen-reader label for the chip (caller-resolved).
  final String semanticsLabel;

  /// Tap handler. Receives the parent fact card's on-screen footprint
  /// ([Rect], global coords) so a caller can morph/grow a full-screen panel
  /// from the card (docs/client/modals.md — Morph-reveal content panels).
  /// Callers that don't need the rect ignore it: `(_) => doThing()`.
  final ValueChanged<Rect> onTap;

  const ContentFactCta({
    required this.label,
    required this.semanticsLabel,
    required this.onTap,
  });
}

/// Color treatment for a [ContentFactData] card.
enum ContentFactTone {
  /// Default, low-emphasis fact.
  neutral,

  /// Confirmed / locked-in fact.
  confirmed,

  /// Imminent / time-sensitive fact.
  imminent,

  /// Problem / blocking fact.
  warn,

  /// Completed / wrapped fact.
  done,
}

/// A single fact card (label + value + optional detail) in a [ContentFactsRow].
class ContentFactData {
  /// Uppercase kicker (e.g. "WHEN", "WHERE").
  final String label;

  /// The primary value (e.g. "Thu Jun 4").
  final String value;

  /// Optional secondary line shown beneath the value in a dimmer tone (e.g.
  /// "3 mi away", "3d 2h from now", "2 options").
  final String? detail;

  final ContentFactTone tone;

  /// Optional tap handler (e.g. open the time/location panel). Receives the
  /// card's on-screen footprint ([Rect], global coords) so a caller can
  /// morph/grow a full-screen panel from the card. Callers that don't need the
  /// rect ignore it: `(_) => doThing()`. When set, [semanticsLabel] must also
  /// be provided.
  final ValueChanged<Rect>? onTap;

  /// Required when [onTap] is set — the screen-reader label (caller-resolved
  /// from `context.l10n`).
  final String? semanticsLabel;

  /// Optional call-to-action chip rendered at the foot of the card (event V6b):
  /// "Vote ›" / "Set time ›" / "Set location ›". Null when there is nothing the
  /// viewer needs to do for this fact.
  final ContentFactCta? cta;

  /// Optional compact weather accessory shown beneath the detail line: a
  /// decorative condition glyph plus a short temperature (e.g. "☀️ 72°"). Set on
  /// the WHEN card when the event day's forecast is known. The reader-facing
  /// description rides on [semanticsLabel]; the inline glyph/temp are decorative.
  final String? weatherGlyph;
  final String? weatherTemp;

  const ContentFactData({
    required this.label,
    required this.value,
    this.detail,
    this.tone = ContentFactTone.neutral,
    this.onTap,
    this.semanticsLabel,
    this.cta,
    this.weatherGlyph,
    this.weatherTemp,
  }) : assert(
         onTap == null || semanticsLabel != null,
         'semanticsLabel is required when onTap is set',
       );
}

/// ContentFactsRow lays out one or two [ContentFactData] cards side by side
/// below the headline (see docs/issues/2278-experience-content-redesign.md).
/// Always-on-dark and role-agnostic.
class ContentFactsRow extends StatelessWidget {
  final List<ContentFactData> facts;

  /// Action accent for any [ContentFactData.cta] chips (heritage sage on
  /// experiences). Defaults to the experience sage so existing callers need no
  /// change.
  final Color ctaAccentColor;

  const ContentFactsRow({
    super.key,
    required this.facts,
    this.ctaAccentColor = AppColors.experienceSageGreen,
  });

  Color _accent(ContentFactTone tone) => switch (tone) {
    ContentFactTone.neutral => AppColors.darkTextTertiary,
    ContentFactTone.confirmed => AppColors.statusInfoOnDark,
    ContentFactTone.imminent => AppColors.statusWarningOnDark,
    ContentFactTone.warn => AppColors.statusErrorOnDark,
    ContentFactTone.done => AppColors.statusSuccessOnDark,
  };

  /// The card's on-screen footprint (global coords) captured from [context] at
  /// tap time, so a caller can morph a full-screen panel out of the card.
  static Rect _rectOf(BuildContext context) {
    final box = context.findRenderObject();
    return box is RenderBox && box.hasSize
        ? box.localToGlobal(Offset.zero) & box.size
        : Rect.zero;
  }

  Widget _card(ContentFactData fact) {
    // Wrapped in a Builder so the tap closures get a card-local context whose
    // render object is this card's box — the footprint the morph grows from.
    return Builder(builder: (cardContext) => _cardBody(cardContext, fact));
  }

  Widget _cardBody(BuildContext cardContext, ContentFactData fact) {
    final accent = _accent(fact.tone);
    final tinted = fact.tone != ContentFactTone.neutral;
    final card = Container(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
      decoration: BoxDecoration(
        color: tinted
            ? accent.withValues(alpha: 0.06)
            : AppColors.darkTextPrimary.withValues(alpha: 0.03),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(
          color: tinted ? accent.withValues(alpha: 0.35) : AppColors.darkBorder,
        ),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(
            fact.label.toUpperCase(),
            style: TextStyle(
              color: tinted ? accent : AppColors.darkTextTertiary,
              fontSize: 9,
              fontWeight: FontWeight.w700,
              letterSpacing: 1.2,
            ),
          ),
          const SizedBox(height: 2),
          Text(
            fact.value,
            maxLines: 2,
            overflow: TextOverflow.ellipsis,
            style: const TextStyle(
              color: AppColors.onContentImage,
              fontSize: 13,
              fontWeight: FontWeight.w600,
              height: 1.25,
            ),
          ),
          if (fact.detail != null && fact.detail!.isNotEmpty) ...[
            const SizedBox(height: 2),
            Text(
              fact.detail!,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: const TextStyle(
                color: AppColors.darkTextTertiary,
                fontSize: 11,
                fontWeight: FontWeight.w500,
              ),
            ),
          ],
          if (fact.weatherGlyph != null && fact.weatherTemp != null) ...[
            const SizedBox(height: 5),
            // Decorative: the day's forecast rides on the card's semanticsLabel.
            ExcludeSemantics(
              child: Row(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Text(fact.weatherGlyph!, style: const TextStyle(fontSize: 20)),
                  const SizedBox(width: 6),
                  Text(
                    fact.weatherTemp!,
                    style: const TextStyle(
                      color: AppColors.darkTextSecondary,
                      fontSize: 14,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                ],
              ),
            ),
          ],
          if (fact.cta != null) ...[
            const SizedBox(height: 8),
            ContentActionChip(
              label: fact.cta!.label,
              semanticsLabel: fact.cta!.semanticsLabel,
              // The chip taps but the morph grows from the whole card.
              onTap: () => fact.cta!.onTap(_rectOf(cardContext)),
              accentColor: ctaAccentColor,
            ),
          ],
        ],
      ),
    );

    // The whole card is the tap target — tapping anywhere (not just the CTA
    // chip) fires [onTap]. When a CTA is present we keep the child semantics so
    // the chip stays a distinct, separately-announced action nested inside the
    // card's button; without a CTA the inner text is redundant with the label.
    if (fact.onTap == null) return card;
    return Tappable(
      semanticsLabel: fact.semanticsLabel!,
      onTap: () => fact.onTap!(_rectOf(cardContext)),
      excludeChildSemantics: fact.cta == null,
      child: card,
    );
  }

  @override
  Widget build(BuildContext context) {
    // IntrinsicHeight bounds the cross-axis so the cards can stretch to equal
    // heights even inside an unbounded-height scroll view (the content sheet).
    return IntrinsicHeight(
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          for (var i = 0; i < facts.length; i++) ...[
            if (i > 0) const SizedBox(width: 10),
            Expanded(child: _card(facts[i])),
          ],
        ],
      ),
    );
  }
}
