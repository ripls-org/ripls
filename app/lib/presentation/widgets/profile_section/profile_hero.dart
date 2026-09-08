import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// The identity block for the person and community profile surfaces
/// (issue #2568): an accent eyebrow, the serif display name, and a
/// serif "common ground" sentence. No snapshot avatar — the full-height
/// background photo *is* the identity, matching the content views. The
/// relationship/member details render as their own [ProfileMemberRow]
/// below the hero.
///
/// It renders **over the profile's full-bleed background photo** (the
/// entity's primary media, drawn by the screen behind the bottom text
/// panel like every content view), so all text is light.
class ProfileHero extends StatelessWidget {
  const ProfileHero({
    super.key,
    required this.eyebrow,
    required this.title,
    this.sentence,
    this.titleFontSize = 30,
  });

  /// Accent uppercase eyebrow — "YOU & BETTY" (person) or "ALL MEMBERS
  /// · 3" (community).
  final String eyebrow;

  /// Serif display name.
  final String title;

  /// Optional serif "common ground" sentence rendered below the title.
  final String? sentence;

  final double titleFontSize;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(22, 0, 22, 4),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(
            eyebrow,
            style: TextStyle(
              fontSize: 10,
              fontWeight: FontWeight.w700,
              letterSpacing: 1.5,
              color: AppColors.primary(context),
            ),
          ),
          const SizedBox(height: 6),
          Text(
            title,
            style: TextStyle(
              fontFamily: AppTheme.headingFont,
              fontWeight: FontWeight.w600,
              fontSize: titleFontSize,
              height: 1,
              letterSpacing: -0.4,
              color: Colors.white,
            ),
          ),
          if (sentence != null && sentence!.isNotEmpty) ...[
            const SizedBox(height: 11),
            _ExpandableSentence(text: sentence!),
          ],
        ],
      ),
    );
  }
}

/// The hero's serif description, clamped to three lines with an
/// ellipsis. When the text overflows the clamp, tapping toggles the
/// full text; short descriptions render as plain text with no tap
/// affordance.
class _ExpandableSentence extends StatefulWidget {
  const _ExpandableSentence({required this.text});

  final String text;

  static const int _maxCollapsedLines = 3;
  static const TextStyle _style = TextStyle(
    fontFamily: AppTheme.headingFont,
    fontSize: 15.5,
    height: 1.42,
    color: Colors.white,
  );

  @override
  State<_ExpandableSentence> createState() => _ExpandableSentenceState();
}

class _ExpandableSentenceState extends State<_ExpandableSentence> {
  bool _expanded = false;

  /// Whether the text exceeds the clamp at the last-measured width.
  /// Deliberately **state**, synced via a post-frame callback, never
  /// applied inside the LayoutBuilder pass itself: the builder runs
  /// during the *layout* phase, and letting the measurement flip the
  /// widget structure right there inserts/removes the toggle's
  /// [Tappable] — a semantics mutation mid-layout. On device that
  /// trips the semantics compiler's `!semantics.parentDataDirty`
  /// assertion (object.dart) and wedges the whole route un-laid-out
  /// (see the profile-open crash, and semantics_repro_test.dart /
  /// user_screen_semantics_test.dart for the harness). Structure now
  /// only changes on the *next* frame, in a normal build phase.
  bool _overflows = false;

  void _syncOverflows(bool overflows) {
    if (overflows == _overflows) return;
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted && _overflows != overflows) {
        setState(() => _overflows = overflows);
      }
    });
  }

  @override
  Widget build(BuildContext context) {
    return LayoutBuilder(
      builder: (context, constraints) {
        final painter = TextPainter(
          text: TextSpan(
              text: widget.text, style: _ExpandableSentence._style),
          maxLines: _ExpandableSentence._maxCollapsedLines,
          textDirection: Directionality.of(context),
          textScaler: MediaQuery.textScalerOf(context),
        )..layout(maxWidth: constraints.maxWidth);
        _syncOverflows(painter.didExceedMaxLines);

        final text = Text(
          widget.text,
          maxLines: _expanded || !_overflows
              ? null
              : _ExpandableSentence._maxCollapsedLines,
          overflow: _expanded || !_overflows ? null : TextOverflow.ellipsis,
          style: _ExpandableSentence._style,
        );
        if (!_overflows) return text;
        return Tappable(
          semanticsLabel: _expanded
              ? context.l10n.a11yProfileSentenceCollapse
              : context.l10n.a11yProfileSentenceExpand,
          // Keep the sentence itself in the semantics tree — the label
          // only names the toggle action.
          excludeChildSemantics: false,
          onTap: () => setState(() => _expanded = !_expanded),
          child: text,
        );
      },
    );
  }
}
