import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/nudge/nudge_content_view.dart';
import 'package:ripls/presentation/widgets/nudge/nudge_presentation.dart';
import 'package:ripls/services/feed_service.dart' show NudgePayload;

/// HomeZeroStateCard is the Home tab's zero state — shown under the warm
/// greeting when Up next, Needs you, and Yours are all empty.
///
/// It always offers the same three things the user can do: **Plan an event**,
/// **Ask for help**, **Offer something**. They are unconditional and need no
/// server round trip, so the zero state can never actually be empty (#2936).
///
/// A **host prompt** may lead above them. Those come from the momentum engine
/// and ride a real signal — an event worth repeating, a gap in the calendar, an
/// offer nobody has taken up — so they say something specific about what the
/// viewer already did. That is a different thing from the generic pooled nudges
/// that used to fill this slot: those were LLM-written prompts to do anything at
/// all, and #2936 removed them.
///
/// A host prompt is never nested inside one of the three prompts (#2796):
/// [NudgeContentView] is a full-bleed composition carrying its own headline,
/// imagery, and filled CTA, so nesting it produces two heroes and two primary
/// buttons in one box.
class HomeZeroStateCard extends StatelessWidget {
  /// Opens event creation.
  final VoidCallback onPlan;

  /// Opens request creation.
  final VoidCallback onAsk;

  /// Opens the create flow for sharing something.
  final VoidCallback onOffer;

  /// An optional host prompt from the momentum engine, rendered above the three
  /// prompts. Null is the common case.
  final NudgePayload? nudge;

  /// Fired when the rendered host prompt's CTA is tapped, so the screen can
  /// consume (dismiss) it. Null when no prompt is shown.
  final VoidCallback? onNudgeConsumed;

  const HomeZeroStateCard({
    super.key,
    required this.onPlan,
    required this.onAsk,
    required this.onOffer,
    this.nudge,
    this.onNudgeConsumed,
  });

  /// Height of the nudge when it leads the zero state. [NudgeContentView]'s
  /// variants are `Stack(fit: StackFit.expand)` compositions, so they cannot
  /// size themselves to their content and the host must bound them. Matches
  /// what the nested card used before #2796 un-nested it, now at full card
  /// width rather than inset by the request hero's padding. The card fits its
  /// copy to whatever height it is given (`NudgePresentation.embedded`, #2801),
  /// so this number is a design choice rather than a constraint the copy has
  /// to respect.
  static const double _nudgeHeroHeight = 320;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(20, 8, 20, 10),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          // A host prompt leads when the momentum engine has one. The three
          // prompts below it are unconditional either way.
          if (nudge != null) ...[
            _buildNudgeHero(nudge!),
            const SizedBox(height: 18),
          ],
          _buildPlanPrompt(context),
          const SizedBox(height: 18),
          _buildAskPrompt(context),
          const SizedBox(height: 18),
          _buildOfferPrompt(context),
        ],
      ),
    );
  }

  /// The host prompt as the hero — full card width, its own frame, its own CTA.
  Widget _buildNudgeHero(NudgePayload nudge) => ClipRRect(
        borderRadius: BorderRadius.circular(20),
        child: SizedBox(
          height: _nudgeHeroHeight,
          child: NudgeContentView(
            nudge: nudge,
            onCtaTap: onNudgeConsumed,
            presentation: NudgePresentation.embedded,
          ),
        ),
      );

  // The three prompts are deliberately identical in shape — same heading
  // scale, same subtext, same pill. They are peer choices, not a primary and
  // two alternatives, so nothing here distinguishes one from the others.
  //
  // Ask used to be a hero: a bordered sage box carrying a rotating canned
  // example and a filled pill reading "Ask the crew ›". That made sense when
  // it was the only invitation and the surface existed to revive asking. Now
  // that all three ship together it just read as one of them being broken.

  Widget _buildPlanPrompt(BuildContext context) => _ZeroPrompt(
        title: context.l10n.homeZeroPlanTitle,
        subtext: context.l10n.homeZeroPlanSubtext,
        ctaLabel: context.l10n.homeZeroPlanCta,
        onTap: onPlan,
      );

  Widget _buildAskPrompt(BuildContext context) => _ZeroPrompt(
        title: context.l10n.homeZeroAskTitle,
        subtext: context.l10n.homeZeroAskSubtext,
        ctaLabel: context.l10n.homeZeroAskCta,
        onTap: onAsk,
      );

  Widget _buildOfferPrompt(BuildContext context) => _ZeroPrompt(
        title: context.l10n.homeZeroOfferTitle,
        subtext: context.l10n.homeZeroOfferSubtext,
        ctaLabel: context.l10n.homeZeroOfferCta,
        onTap: onOffer,
      );
}

/// _ZeroPrompt is one invitation in the zero state: a heading, a line of
/// subtext, and a single pill CTA. All three prompts render through it with no
/// per-prompt options, which is what keeps them identical.
class _ZeroPrompt extends StatelessWidget {
  final String title;
  final String subtext;
  final String ctaLabel;
  final VoidCallback onTap;

  const _ZeroPrompt({
    required this.title,
    required this.subtext,
    required this.ctaLabel,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 2),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            title,
            style: TextStyle(
              fontFamily: AppTheme.headingFont,
              fontSize: 16,
              fontWeight: FontWeight.w600,
              color: AppColors.textPrimary(context),
            ),
          ),
          const SizedBox(height: 4),
          Text(
            subtext,
            style: TextStyle(
              fontSize: 12.5,
              height: 1.45,
              color: AppColors.textSecondary(context),
            ),
          ),
          const SizedBox(height: 12),
          _ZeroPromptButton(label: ctaLabel, onTap: onTap),
        ],
      ),
    );
  }
}

/// _ZeroPromptButton is the pill CTA the zero state's three prompts share.
///
/// It takes no styling options: every prompt renders the same bordered pill,
/// and the label doubles as the spoken label because it no longer carries a
/// decorative chevron to strip.
class _ZeroPromptButton extends StatelessWidget {
  final String label;
  final VoidCallback onTap;

  const _ZeroPromptButton({
    required this.label,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    final radius = BorderRadius.circular(24);

    return Tappable(
      semanticsLabel: label,
      onTap: onTap,
      inkBorderRadius: radius,
      child: ConstrainedBox(
        // The pill's own padding leaves it a few points under the 48dp touch
        // floor at these type sizes; growing the pill itself keeps the target
        // and the visible affordance the same shape, which a transparent hit
        // box would not. A minimum (rather than a fixed height) so the pill
        // still grows with the platform text scale instead of clipping.
        constraints: const BoxConstraints(minHeight: kMinInteractiveDimension),
        child: Container(
          // Horizontal padding keeps the label inside the pill's rounded
          // bounds — with none, a long label runs into the corner radius and
          // reads as overflowing (#2724).
          padding: const EdgeInsets.symmetric(horizontal: 20),
          decoration: BoxDecoration(
            color: AppColors.cardBackground(context),
            border: Border.all(color: AppColors.border(context), width: 1.5),
            borderRadius: radius,
          ),
          // widthFactor pins the pill's width to the label — without it the
          // Align fills the row and the pill stretches edge to edge — while
          // the unconstrained height lets it centre the label in the 48dp box.
          child: Align(
            widthFactor: 1,
            child: Text(
              label,
              textAlign: TextAlign.center,
              style: TextStyle(
                fontSize: 13,
                fontWeight: FontWeight.w600,
                color: AppColors.textSecondary(context),
              ),
            ),
          ),
        ),
      ),
    );
  }
}

/// HomeCalmCard replaces the Needs-you header and queue when the user has
/// zero open decisions but other commitments present — the earned calm
/// state (achievement, not void).
class HomeCalmCard extends StatelessWidget {
  const HomeCalmCard({super.key});

  @override
  Widget build(BuildContext context) {
    final sage = AppColors.primary(context);
    return Container(
      margin: const EdgeInsets.fromLTRB(18, 8, 18, 0),
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
      decoration: BoxDecoration(
        color: sage.withAlpha(31),
        border: Border.all(color: sage.withAlpha(77)),
        borderRadius: BorderRadius.circular(15),
      ),
      child: Row(
        children: [
          Container(
            width: 34,
            height: 34,
            decoration: BoxDecoration(color: sage, shape: BoxShape.circle),
            child: Icon(Icons.check,
                size: 18, color: AppColors.onPrimary(context)),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  context.l10n.homeAllCaughtUpTitle,
                  style: TextStyle(
                    fontFamily: AppTheme.headingFont,
                    fontSize: 15.5,
                    fontWeight: FontWeight.w600,
                    color: sage,
                  ),
                ),
                Text(
                  context.l10n.homeAllCaughtUpBody,
                  style: TextStyle(
                    fontSize: 11.5,
                    color: AppColors.textSecondary(context),
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}
