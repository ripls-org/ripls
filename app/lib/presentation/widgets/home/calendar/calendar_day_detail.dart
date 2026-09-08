import 'package:flutter/material.dart';
import 'package:intl/intl.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/data/gen/ripls/api/portfolio.pb.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/home/calendar/calendar_suggestion_panel.dart';
import 'package:ripls/presentation/widgets/home/face_stack.dart';
import 'package:ripls/presentation/widgets/home/home_up_next_copy.dart';
import 'package:ripls/presentation/widgets/weather/weather_chip.dart';

/// CalendarDayDetail is the bottom half of the calendar — the selected day's
/// detail over the same backdrop. It opens with a weather pill and a date line,
/// then adapts: a single event's full detail, a multi-event list (earliest-first,
/// the first emphasized), or — on an open day — a weather×history suggestion or
/// a static "plan an event" prompt.
class CalendarDayDetail extends StatelessWidget {
  final DateTime day;

  /// Today (day-only), so a past open day shifts from "plan" to "record".
  final DateTime today;
  final List<HomeUpNextEntry> events;
  final DayForecast? forecast;
  final OpenDaySuggestion? suggestion;
  final void Function(HomeUpNextEntry) onEntryTap;

  /// Opens the blank create flow — from the static "nothing planned" prompt.
  final VoidCallback onSuggest;

  /// Opens the create flow pre-filled with a server-generated draft, from an
  /// open-day suggestion CTA/chip. Receives a generation prompt and the
  /// recommended start moment (Unix seconds) to seed the draft's date/time.
  final void Function(String prompt, int startUnixSec) onGeneratePlan;

  const CalendarDayDetail({
    super.key,
    required this.day,
    required this.today,
    required this.events,
    required this.forecast,
    required this.suggestion,
    required this.onEntryTap,
    required this.onSuggest,
    required this.onGeneratePlan,
  });

  @override
  Widget build(BuildContext context) {
    // The open-day suggestion weaves the weather into its server-built
    // reason line, so the separate pill is suppressed there to keep it one
    // thought; events and prompt-only open days still show it.
    final suggesting = events.isEmpty && suggestion != null;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: [
        if (forecast != null && !suggesting) ...[
          _weatherPill(context),
          const SizedBox(height: 10),
        ],
        _body(context),
      ],
    );
  }

  Widget _body(BuildContext context) {
    if (events.isEmpty) return _openDay(context);
    if (events.length == 1) return _singleEvent(context, events.first);
    return _multiEvent(context);
  }

  // ── Weather pill ──────────────────────────────────────────────────────────

  Widget _weatherPill(BuildContext context) => WeatherChip(forecast: forecast!);

  Widget _dateLine(BuildContext context, {String? suffix}) {
    final base = DateFormat('EEEE · MMM d').format(day).toUpperCase();
    return Text(
      suffix == null ? base : '$base · $suffix',
      style: TextStyle(
        fontSize: 10.5,
        fontWeight: FontWeight.w800,
        letterSpacing: 1,
        color: Colors.white.withAlpha(235),
      ),
    );
  }

  // ── Single event ──────────────────────────────────────────────────────────

  Widget _singleEvent(BuildContext context, HomeUpNextEntry e) {
    final title = homeUpNextTitle(context.l10n, e);
    final statusLabel = homeUpNextStatusLabel(context.l10n, e);
    final timeLabel = homeUpNextTimeLabel(context.l10n, e);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: [
        _dateLine(context),
        if (timeLabel.isNotEmpty) ...[
          const SizedBox(height: 9),
          Text(
            timeLabel.toUpperCase(),
            style: TextStyle(
              fontFamily: AppTheme.bodyFont,
              fontSize: 13,
              fontWeight: FontWeight.w800,
              letterSpacing: 0.5,
              color: AppColors.statusWarningOnDark,
            ),
          ),
        ],
        const SizedBox(height: 3),
        Tappable(
          semanticsLabel: title,
          onTap: () => onEntryTap(e),
          inkBorderRadius: BorderRadius.circular(8),
          child: Text(
            title,
            style: const TextStyle(
              fontFamily: AppTheme.headingFont,
              fontSize: 28,
              fontWeight: FontWeight.w600,
              height: 1.05,
              color: Colors.white,
              shadows: [Shadow(color: Color(0x8C000000), blurRadius: 14)],
            ),
          ),
        ),
        if (statusLabel.isNotEmpty) ...[
          const SizedBox(height: 5),
          Text(
            statusLabel,
            style: TextStyle(
              fontFamily: AppTheme.headingFont,
              fontStyle: FontStyle.italic,
              fontSize: 15,
              color: Colors.white.withAlpha(235),
              shadows: const [Shadow(color: Color(0x80000000), blurRadius: 8)],
            ),
          ),
        ],
        if (e.helpers.isNotEmpty) ...[
          const SizedBox(height: 14),
          FaceStack(people: e.helpers, size: 30),
        ],
      ],
    );
  }

  // ── Multi event ───────────────────────────────────────────────────────────

  Widget _multiEvent(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: [
        _dateLine(context, suffix: context.l10n.homeCalEventsCount(events.length)),
        const SizedBox(height: 4),
        for (var i = 0; i < events.length; i++) _eventRow(context, events[i], i == 0),
      ],
    );
  }

  Widget _eventRow(BuildContext context, HomeUpNextEntry e, bool hero) {
    final title = homeUpNextTitle(context.l10n, e);
    return Tappable(
      semanticsLabel: title,
      onTap: () => onEntryTap(e),
      inkBorderRadius: BorderRadius.zero,
      child: Container(
        padding: const EdgeInsets.symmetric(vertical: 11),
        decoration: BoxDecoration(
          border: Border(top: BorderSide(color: Colors.white.withAlpha(46))),
        ),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            SizedBox(
              width: 60,
              child: Text(
                homeUpNextTimeLabel(context.l10n, e),
                style: TextStyle(
                  fontSize: 12,
                  fontWeight: FontWeight.w800,
                  color: hero
                      ? AppColors.statusWarningOnDark
                      : Colors.white.withAlpha(235),
                ),
              ),
            ),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    title,
                    maxLines: 2,
                    overflow: TextOverflow.ellipsis,
                    style: TextStyle(
                      fontFamily: AppTheme.headingFont,
                      fontSize: hero ? 21 : 18,
                      fontWeight: FontWeight.w600,
                      height: 1.1,
                      color: Colors.white,
                      shadows: const [Shadow(color: Color(0x80000000), blurRadius: 10)],
                    ),
                  ),
                  if (e.helpers.isNotEmpty) ...[
                    const SizedBox(height: 6),
                    FaceStack(people: e.helpers, size: 22),
                  ],
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }

  // ── Open day ──────────────────────────────────────────────────────────────

  Widget _openDay(BuildContext context) {
    // A past empty day can't be planned — shift from planning to recording.
    if (day.isBefore(today)) return _pastDayPrompt(context);
    final s = suggestion;
    if (s != null) {
      return CalendarSuggestionPanel(
        suggestion: s,
        day: day,
        onPlan: onGeneratePlan,
        onSomethingElse: onSuggest,
      );
    }
    // An open day with no weather×history suggestion falls through to the
    // static "plan an event" prompt. A generic nudge used to fill this slot;
    // #2936 removed those — an open day is already the whole message, and
    // one deterministic affordance says more than a card inventing a reason
    // to use it.
    return _staticPrompt(context);
  }

  /// A past empty day: nothing to plan, so invite the viewer to *record* what
  /// happened. One of four warm copy variants, chosen deterministically per day
  /// (stable across rebuilds, varied across days), over an outlined "record"
  /// button that opens the create flow.
  Widget _pastDayPrompt(BuildContext context) {
    final l10n = context.l10n;
    final variants = <({String head, String sub, String cta})>[
      (
        head: l10n.homeCalPastQuietHead,
        sub: l10n.homeCalPastQuietSub,
        cta: l10n.homeCalPastQuietCta,
      ),
      (
        head: l10n.homeCalPastUnloggedHead,
        sub: l10n.homeCalPastUnloggedSub,
        cta: l10n.homeCalPastUnloggedCta,
      ),
      (
        head: l10n.homeCalPastSlippedHead,
        sub: l10n.homeCalPastSlippedSub,
        cta: l10n.homeCalPastSlippedCta,
      ),
      (
        head: l10n.homeCalPastMemoryHead,
        sub: l10n.homeCalPastMemorySub,
        cta: l10n.homeCalPastMemoryCta,
      ),
    ];
    final v = variants[(day.month * 31 + day.day) % variants.length];

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: [
        _dateLine(context),
        const SizedBox(height: 12),
        Text(
          v.head,
          style: const TextStyle(
            fontFamily: AppTheme.headingFont,
            fontSize: 34,
            fontWeight: FontWeight.w600,
            height: 1.02,
            color: Colors.white,
            shadows: [Shadow(color: Color(0x8C000000), blurRadius: 14)],
          ),
        ),
        const SizedBox(height: 9),
        Text(
          v.sub,
          style: TextStyle(
            fontFamily: AppTheme.headingFont,
            fontStyle: FontStyle.italic,
            fontSize: 15.5,
            color: Colors.white.withAlpha(184),
            shadows: const [Shadow(color: Color(0x73000000), blurRadius: 8)],
          ),
        ),
        const SizedBox(height: 20),
        Tappable(
          semanticsLabel: v.cta,
          onTap: onSuggest,
          inkBorderRadius: BorderRadius.circular(14),
          child: Container(
            padding: const EdgeInsets.symmetric(horizontal: 20, vertical: 13),
            decoration: BoxDecoration(
              color: Colors.white.withAlpha(16),
              borderRadius: BorderRadius.circular(14),
              border: Border.all(color: Colors.white.withAlpha(102), width: 1.5),
            ),
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(Icons.add, size: 18, color: Colors.white.withAlpha(230)),
                const SizedBox(width: 9),
                Text(
                  v.cta,
                  style: const TextStyle(
                    fontSize: 14.5,
                    fontWeight: FontWeight.w700,
                    color: Colors.white,
                  ),
                ),
                const SizedBox(width: 6),
                Icon(Icons.chevron_right,
                    size: 18, color: Colors.white.withAlpha(178)),
              ],
            ),
          ),
        ),
      ],
    );
  }

  Widget _staticPrompt(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: [
        _dateLine(context),
        const SizedBox(height: 9),
        Text(
          context.l10n.homeCalNothingPlanned,
          style: const TextStyle(
            fontFamily: AppTheme.headingFont,
            fontSize: 28,
            fontWeight: FontWeight.w600,
            color: Colors.white,
            shadows: [Shadow(color: Color(0x8C000000), blurRadius: 14)],
          ),
        ),
        const SizedBox(height: 10),
        Tappable(
          semanticsLabel: context.l10n.homeCalSuggestPlan,
          onTap: onSuggest,
          inkBorderRadius: BorderRadius.circular(16),
          child: Container(
            padding: const EdgeInsets.symmetric(horizontal: 18, vertical: 12),
            decoration: BoxDecoration(
              color: Colors.white.withAlpha(36),
              borderRadius: BorderRadius.circular(16),
              border: Border.all(color: Colors.white.withAlpha(128)),
            ),
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Text(
                  context.l10n.homeCalSuggestPlan,
                  style: const TextStyle(
                    fontSize: 14,
                    fontWeight: FontWeight.w700,
                    color: Colors.white,
                  ),
                ),
                const Icon(Icons.chevron_right, size: 18, color: Colors.white),
              ],
            ),
          ),
        ),
      ],
    );
  }
}
