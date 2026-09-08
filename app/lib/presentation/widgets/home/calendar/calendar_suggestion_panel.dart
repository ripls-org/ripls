import 'package:flutter/material.dart';
import 'package:intl/intl.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/data/gen/ripls/api/portfolio.pb.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/home/face_stack.dart';

/// CalendarSuggestionPanel renders an open day's suggestion in the editorial
/// dock style: a quiet kicker (Suggested · date), a serif headline, an
/// italic "why", the crew to go with, a single gold CTA, and the
/// alternatives as understated inline links — plus a gold "something else" link
/// to plan from scratch. Restrained on purpose so it clears the calendar.
/// The day's weather is not rendered separately: the server weaves it into
/// the suggestion's reason line so it reads as one thought.
class CalendarSuggestionPanel extends StatelessWidget {
  final OpenDaySuggestion suggestion;

  /// The day this suggestion is for, for the kicker date.
  final DateTime day;

  /// Starts planning. Receives a ready-made **generation prompt** (the chosen
  /// activity + crew) and the recommended start moment (Unix seconds), so the
  /// create flow can generate a pre-filled draft seeded to the right day/time.
  final void Function(String prompt, int startUnixSec) onPlan;

  /// Opens a blank create flow — the "something else" / plan-from-scratch link.
  final VoidCallback onSomethingElse;

  const CalendarSuggestionPanel({
    super.key,
    required this.suggestion,
    required this.day,
    required this.onPlan,
    required this.onSomethingElse,
  });

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: [
        Text(
          _kicker(context).toUpperCase(),
          style: TextStyle(
            fontSize: 11,
            fontWeight: FontWeight.w700,
            letterSpacing: 0.9,
            color: Colors.white.withAlpha(153),
          ),
        ),
        const SizedBox(height: 10),
        Text(
          suggestion.title,
          style: const TextStyle(
            fontFamily: AppTheme.headingFont,
            fontSize: 34,
            fontWeight: FontWeight.w600,
            height: 1.04,
            color: Colors.white,
            shadows: [Shadow(color: Color(0x8C000000), blurRadius: 14)],
          ),
        ),
        if (suggestion.reason.isNotEmpty) ...[
          const SizedBox(height: 9),
          Text(
            suggestion.reason,
            style: TextStyle(
              fontFamily: AppTheme.headingFont,
              fontStyle: FontStyle.italic,
              fontSize: 16,
              height: 1.4,
              color: Colors.white.withAlpha(230),
              shadows: const [Shadow(color: Color(0x73000000), blurRadius: 8)],
            ),
          ),
        ],
        if (suggestion.people.isNotEmpty) _peopleRow(context),
        const SizedBox(height: 18),
        _planCta(context),
        _alternatives(context),
      ],
    );
  }

  String _kicker(BuildContext context) {
    return [
      context.l10n.homeCalSuggested,
      DateFormat('EEE d').format(day),
    ].join(' · ');
  }

  Widget _peopleRow(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(top: 16),
      child: Row(
        children: [
          FaceStack(people: suggestion.people, size: 30),
          const SizedBox(width: 10),
          Flexible(
            child: Text(
              context.l10n.homeCalWith(_names()),
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: TextStyle(
                fontSize: 13,
                color: Colors.white.withAlpha(204),
                shadows: const [Shadow(color: Color(0x73000000), blurRadius: 8)],
              ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _planCta(BuildContext context) {
    return Tappable(
      semanticsLabel: context.l10n.homeCalPlanItWithThem,
      onTap: () => onPlan(_planPrompt(_leadActivity()), _startUnixSec()),
      inkBorderRadius: BorderRadius.circular(15),
      child: Container(
        width: double.infinity,
        padding: const EdgeInsets.symmetric(vertical: 15),
        alignment: Alignment.center,
        decoration: BoxDecoration(
          color: AppColors.statusWarningOnDark,
          borderRadius: BorderRadius.circular(15),
        ),
        child: Text(
          context.l10n.homeCalPlanItWithThem,
          style: const TextStyle(
            fontSize: 15,
            fontWeight: FontWeight.w800,
            color: Color(0xFF3A2C08),
          ),
        ),
      ),
    );
  }

  /// "or hiking · climbing · something else" — quiet inline links. The
  /// alternatives generate a pre-filled draft; "something else" opens a blank
  /// create.
  Widget _alternatives(BuildContext context) {
    final dotStyle = TextStyle(
      fontFamily: AppTheme.headingFont,
      fontSize: 15,
      color: Colors.white.withAlpha(110),
    );
    final children = <Widget>[
      Text(
        '${context.l10n.homeCalOr} ',
        style: TextStyle(
          fontFamily: AppTheme.headingFont,
          fontSize: 15,
          color: Colors.white.withAlpha(158),
        ),
      ),
    ];
    for (final alt in suggestion.alternatives) {
      if (children.length > 1) children.add(Text(' · ', style: dotStyle));
      children.add(_link(
        context,
        alt.label,
        () => onPlan(_planPrompt(alt.label), _startUnixSec()),
        gold: false,
      ));
    }
    if (suggestion.alternatives.isNotEmpty) {
      children.add(Text(' · ', style: dotStyle));
    }
    children.add(_link(
      context,
      context.l10n.homeCalSomethingElse,
      onSomethingElse,
      gold: true,
    ));

    return Padding(
      padding: const EdgeInsets.only(top: 15),
      child: Wrap(
        crossAxisAlignment: WrapCrossAlignment.center,
        children: children,
      ),
    );
  }

  Widget _link(
    BuildContext context,
    String label,
    VoidCallback onTap, {
    required bool gold,
  }) {
    final color = gold ? AppColors.statusWarningOnDark : Colors.white;
    return Tappable(
      semanticsLabel: label,
      onTap: onTap,
      inkBorderRadius: BorderRadius.circular(4),
      child: Text(
        label,
        style: TextStyle(
          fontFamily: AppTheme.headingFont,
          fontSize: 15,
          color: color,
          decoration: TextDecoration.underline,
          decorationColor: color.withAlpha(120),
        ),
      ),
    );
  }

  /// The lead activity text — the title minus its trailing "?".
  String _leadActivity() {
    var t = suggestion.title.trim();
    if (t.endsWith('?')) t = t.substring(0, t.length - 1).trim();
    return t;
  }

  /// The recommended start moment (Unix seconds) to seed onto the draft, built
  /// from the **local day this panel is showing** plus the server's recommended
  /// time of day. Building it from the local day (rather than an absolute
  /// server timestamp) keeps it on exactly the day the user sees — no timezone
  /// skew.
  int _startUnixSec() {
    final mins = suggestion.recommendedStartMinutes;
    final m = mins > 0 ? mins : 9 * 60; // default 9:00 AM
    final start = DateTime(day.year, day.month, day.day, m ~/ 60, m % 60);
    return start.millisecondsSinceEpoch ~/ 1000;
  }

  /// The generation prompt — the activity and crew only, e.g. "Plan a flatirons
  /// hike with Mia, Ben & Sam". The day/time are seeded structurally.
  String _planPrompt(String activity) {
    final a = activity.trim();
    final lead = a.isEmpty ? a : '${a[0].toLowerCase()}${a.substring(1)}';
    final names = _names();
    final base = 'Plan $lead';
    return names.isEmpty ? base : '$base with $names';
  }

  String _names() {
    final names = suggestion.people
        .map((p) => p.displayName.split(' ').first)
        .where((n) => n.isNotEmpty)
        .toList();
    if (names.isEmpty) return '';
    if (names.length == 1) return names.first;
    if (names.length == 2) return '${names[0]} & ${names[1]}';
    return '${names.sublist(0, names.length - 1).join(', ')} & ${names.last}';
  }
}
