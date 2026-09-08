import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:intl/intl.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/core/utils/image_cache_keys.dart';
import 'package:ripls/data/gen/ripls/api/portfolio.pb.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/chat/cached_media_image.dart';
import 'package:ripls/presentation/widgets/home/calendar/calendar_weather.dart';
import 'package:ripls/presentation/widgets/home/home_up_next_copy.dart';
import 'package:ripls/services/providers.dart' show mediaObjectProvider;

/// CalendarDayCell is one square of the month grid floating over the backdrop:
/// a rounded thumbnail of the day's marquee event (or a translucent empty cell),
/// the day number, a multi-event count badge, and the day's weather glyph. The
/// selected day carries an amber ring; today's number is amber with a dot.
///
/// [isChosen] is the event "When" screen's committed event date: a solid amber
/// fill (overriding the photo/glyph) so the chosen day stays unmistakable even
/// as the viewer taps other days to explore — those get the amber ring instead.
///
/// The whole grid sits over a dark scrim, so content is white and empty cells
/// are a faint white fill — the same on-media treatment used elsewhere.
class CalendarDayCell extends ConsumerWidget {
  final DateTime day;
  final List<HomeUpNextEntry> events;
  final DayForecast? forecast;
  final String photoMediaId;
  final bool isToday;
  final bool isSelected;

  /// The committed event date on the "When" screen — a solid amber fill that
  /// always wins over photo, glyph, and the selection ring. Defaults to false so
  /// the Home calendar is unaffected.
  final bool isChosen;
  final VoidCallback onTap;

  const CalendarDayCell({
    super.key,
    required this.day,
    required this.events,
    required this.forecast,
    required this.photoMediaId,
    required this.isToday,
    required this.isSelected,
    this.isChosen = false,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final hasPhoto = events.isNotEmpty && photoMediaId.isNotEmpty && !isChosen;

    return Tappable(
      semanticsLabel: _semanticLabel(context),
      onTap: onTap,
      inkBorderRadius: BorderRadius.circular(11),
      child: AspectRatio(
        aspectRatio: 1,
        child: Container(
          decoration: BoxDecoration(
            color: isChosen
                ? AppColors.statusWarningOnDark
                : hasPhoto
                    ? null
                    : Colors.white.withAlpha(12),
            borderRadius: BorderRadius.circular(11),
            boxShadow: isChosen
                ? [
                    BoxShadow(
                      color:
                          AppColors.statusWarningOnDark.withValues(alpha: 0.4),
                      blurRadius: 16,
                      offset: const Offset(0, 6),
                    ),
                  ]
                : null,
          ),
          // Paint the border in the foreground so the selection ring shows over
          // a day's thumbnail, not behind it. The chosen (gold) cell carries no
          // ring — the fill itself is the highlight.
          foregroundDecoration: BoxDecoration(
            borderRadius: BorderRadius.circular(11),
            border: isChosen
                ? null
                : isSelected
                    ? Border.all(
                        color: AppColors.statusWarningOnDark, width: 2.5)
                    : isToday
                        ? Border.all(color: Colors.white, width: 2)
                        : Border.all(color: Colors.white.withAlpha(20)),
          ),
          child: ClipRRect(
            borderRadius: BorderRadius.circular(11),
            child: Stack(
              fit: StackFit.expand,
              children: [
                if (hasPhoto) _thumbnail(context, ref),
                _dayNumber(),
                if (!isChosen && events.length > 1) _countBadge(),
                if (!isChosen && forecast != null) _weatherGlyph(),
                if (isChosen) _chosenBar(),
                if (isToday && !isChosen) _todayDot(),
              ],
            ),
          ),
        ),
      ),
    );
  }

  Widget _thumbnail(BuildContext context, WidgetRef ref) {
    final mediaAsync = ref.watch(mediaObjectProvider(photoMediaId));
    final image = mediaAsync.maybeWhen(
      data: (media) => media.url.isEmpty
          ? const SizedBox.shrink()
          : CachedMediaImage(
              semanticsLabel: null,
              imageUrl: media.url,
              cacheKey: ImageCacheKeys.thumbnail(photoMediaId),
              width: double.infinity,
              height: double.infinity,
              fit: BoxFit.cover,
            ),
      orElse: () => const SizedBox.shrink(),
    );
    return Stack(
      fit: StackFit.expand,
      children: [
        image,
        // Darken the bottom so the day number and glyph stay legible.
        const DecoratedBox(
          decoration: BoxDecoration(
            gradient: LinearGradient(
              begin: Alignment.topCenter,
              end: Alignment.bottomCenter,
              colors: [Color(0x1A000000), Color(0x94000000)],
            ),
          ),
        ),
      ],
    );
  }

  Widget _dayNumber() => Positioned(
        top: 3,
        left: 5,
        child: Text(
          DateFormat('d').format(day),
          style: TextStyle(
            fontFamily: AppTheme.headingFont,
            fontSize: 11,
            // The chosen (gold) cell carries a dark, bold number with no shadow;
            // today is amber; every other day is white.
            fontWeight: (isToday || isChosen) ? FontWeight.w800 : FontWeight.w600,
            color: isChosen
                ? const Color(0xFF3A2C08)
                : isToday
                    ? AppColors.statusWarningOnDark
                    : Colors.white,
            shadows: isChosen
                ? null
                : const [Shadow(color: Color(0xBF000000), blurRadius: 3)],
          ),
        ),
      );

  // The short dark bar under the chosen day's number — the design's committed
  // marker that reads even on the solid gold fill.
  Widget _chosenBar() => Positioned(
        bottom: 5,
        left: 0,
        right: 0,
        child: Center(
          child: Container(
            width: 14,
            height: 3,
            decoration: BoxDecoration(
              color: const Color(0x993A2C08),
              borderRadius: BorderRadius.circular(2),
            ),
          ),
        ),
      );

  Widget _countBadge() => Positioned(
        top: 3,
        right: 3,
        child: Container(
          constraints: const BoxConstraints(minWidth: 14),
          height: 14,
          padding: const EdgeInsets.symmetric(horizontal: 3),
          alignment: Alignment.center,
          decoration: BoxDecoration(
            color: AppColors.statusWarningOnDark,
            borderRadius: BorderRadius.circular(999),
          ),
          child: Text(
            '${events.length}',
            style: const TextStyle(
              fontSize: 8,
              fontWeight: FontWeight.w800,
              color: Color(0xFF3A2C08),
            ),
          ),
        ),
      );

  // The raw emoji is decorative; the cell's semantic label carries the weather.
  Widget _weatherGlyph() => Positioned(
        bottom: 2,
        left: 0,
        right: 0,
        child: ExcludeSemantics(
          child: Text(
            calendarConditionGlyph(forecast!.condition),
            textAlign: TextAlign.center,
            style: const TextStyle(
              fontSize: 20,
              shadows: [Shadow(color: Color(0xB3000000), blurRadius: 3)],
            ),
          ),
        ),
      );

  Widget _todayDot() => Positioned(
        top: 5,
        left: 0,
        right: 0,
        child: Center(
          child: Container(
            width: 4,
            height: 4,
            decoration: const BoxDecoration(
              color: AppColors.statusWarningOnDark,
              shape: BoxShape.circle,
            ),
          ),
        ),
      );

  String _semanticLabel(BuildContext context) {
    final l10n = context.l10n;
    final date = DateFormat('EEEE, MMMM d').format(day);
    final parts = <String>[];
    if (events.isEmpty) {
      parts.add(l10n.homeCalNothingPlanned);
    } else if (events.length == 1) {
      parts.add(homeUpNextTitle(l10n, events.first));
    } else {
      parts.add(l10n.homeCalEventsCount(events.length));
    }
    final weather = calendarWeatherSemantic(context, forecast);
    if (weather.isNotEmpty) parts.add(weather);
    return l10n.homeCalDayCellSemantic(date, parts.join('. '));
  }
}
