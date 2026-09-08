import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:intl/intl.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/core/utils/navigation_helpers.dart';
import 'package:ripls/data/gen/ripls/api/portfolio.pb.dart'
    show DailyItemType, HomeUpNextEntry;
import 'package:ripls/presentation/screens/create/blank_create_dispatcher.dart';
import 'package:ripls/presentation/screens/workshop/workshop_library_common.dart';
import 'package:ripls/presentation/viewmodels/home_tab_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/home/calendar/calendar_backdrop.dart';
import 'package:ripls/presentation/widgets/home/calendar/calendar_day_detail.dart';
import 'package:ripls/presentation/widgets/home/calendar/calendar_month_data.dart';
import 'package:ripls/presentation/widgets/home/calendar/calendar_month_grid.dart';
import 'package:ripls/presentation/widgets/home/calendar/calendar_panes.dart';
import 'package:ripls/presentation/widgets/home/calendar/calendar_scrim.dart';
import 'package:ripls/presentation/widgets/home/calendar/month_grid_metrics.dart';

/// A standalone month × backdrop calendar scoped to a set of communities —
/// the community/person profile "Calendar" door (issue #2568).
///
/// It reuses the exact same shared `Calendar*` widgets, the cached
/// `GetHomeView`-backed [workshopCommunityCalendarProvider] (filtered to
/// [communityIds]), and the open-day suggestion → create flow as the inbox
/// and former Workshop calendars (Decision 6 — reuse the existing calendar
/// suggestion system rather than the Workshop postcards). Unlike the former
/// Workshop "When" panel it is a plain pushed screen, not a morph panel.
class SharedCalendarScreen extends ConsumerStatefulWidget {
  /// The communities whose timeline this calendar shows. A community
  /// profile passes its single id; a person profile passes the
  /// viewer↔target shared community ids.
  final List<String> communityIds;

  const SharedCalendarScreen({super.key, required this.communityIds});

  @override
  ConsumerState<SharedCalendarScreen> createState() =>
      _SharedCalendarScreenState();
}

class _SharedCalendarScreenState extends ConsumerState<SharedCalendarScreen> {
  late final DateTime _today;

  /// Explicit user selection (day tap, month arrow, date picker). Null
  /// until the viewer picks a day — the calendar instead shows the
  /// soonest planned day (see [_initialDay]), so a community/person with
  /// something coming up opens straight to it instead of an empty
  /// "today".
  DateTime? _selected;

  String get _key => workshopLibraryItemsKey(widget.communityIds);

  @override
  void initState() {
    super.initState();
    final now = DateTime.now();
    _today = DateTime(now.year, now.month, now.day);
  }

  /// Today, or — when today has nothing — the soonest day with items, so
  /// the calendar opens on a useful day. Mirrors
  /// `HomeCalendarScreen._initialDay`.
  DateTime _initialDay(List<HomeUpNextEntry> entries) {
    bool isDay(HomeUpNextEntry e, DateTime d) {
      final w =
          DateTime.fromMillisecondsSinceEpoch(e.timeUnixSec.toInt() * 1000);
      return DateTime(w.year, w.month, w.day) == d;
    }

    if (entries.any((e) => isDay(e, _today)) || entries.isEmpty) return _today;
    final days = entries
        .map((e) {
          final w =
              DateTime.fromMillisecondsSinceEpoch(e.timeUnixSec.toInt() * 1000);
          return DateTime(w.year, w.month, w.day);
        })
        .where((d) => !d.isBefore(_today))
        .toList()
      ..sort();
    return days.isEmpty ? _today : days.first;
  }

  void _shiftMonth(DateTime current, int delta) {
    setState(() {
      _selected = DateTime(current.year, current.month + delta, 1);
    });
  }

  Future<void> _pickDate(DateTime current) async {
    final now = DateTime.now();
    final picked = await showDatePicker(
      context: context,
      initialDate: current,
      firstDate: DateTime(now.year - 1),
      lastDate: DateTime(now.year + 2),
    );
    if (picked == null || !mounted) return;
    setState(() => _selected = DateTime(picked.year, picked.month, picked.day));
  }

  @override
  Widget build(BuildContext context) {
    final async = ref.watch(workshopCommunityCalendarProvider(_key));
    final entries = async.asData?.value.entries ?? const <HomeUpNextEntry>[];
    final selectedDay = _selected ?? _initialDay(entries);
    return Scaffold(
      backgroundColor: Colors.black,
      body: SafeArea(
        child: async.when(
          loading: () =>
              _framed(selectedDay, const Center(child: CircularProgressIndicator())),
          error: (e, _) => _framed(selectedDay, Center(
            child: Text('$e', style: const TextStyle(color: Colors.white70)),
          )),
          data: (d) {
            final data = CalendarMonthData.from(
              entries: d.entries,
              forecast: d.forecast,
              suggestions: d.suggestions,
            );
            return Stack(
              fit: StackFit.expand,
              children: [
                CalendarBackdrop(mediaId: data.marqueePhotoOn(selectedDay)),
                const CalendarScrim(),
                LayoutBuilder(
                  builder: (context, outer) => Column(
                  children: [
                    _header(context, selectedDay),
                    // Stacked on phones (bit-identical), grid beside a
                    // day-detail rail at expanded widths (#2912).
                    Expanded(
                      child: CalendarPanes(
                        // Budget the month so the day detail keeps room on
                        // wide windows — see monthGridCellSide (#2908).
                        stackedGridMaxHeight:
                            outer.maxHeight * monthGridHeightShare,
                        gridRows: monthGridRows(selectedDay),
                        gridBuilder: (context, maxGridHeight) =>
                            CalendarMonthGrid(
                          month: selectedDay,
                          data: data,
                          selectedDay: selectedDay,
                          today: _today,
                          maxHeight: maxGridHeight,
                          onDaySelected: (day) =>
                              setState(() => _selected = day),
                        ),
                        detail: Align(
                          alignment: Alignment.bottomLeft,
                          child: SingleChildScrollView(
                            reverse: true,
                            padding:
                                const EdgeInsets.fromLTRB(22, 24, 22, 24),
                            child: CalendarDayDetail(
                              day: selectedDay,
                              today: _today,
                              events: data.eventsOn(selectedDay),
                              forecast: data.forecastOn(selectedDay),
                              suggestion: data.suggestionOn(selectedDay),
                              onEntryTap: _openEntry,
                              onSuggest: () => unawaited(
                                  openBlankCreateExperience(context, ref)),
                              onGeneratePlan: (prompt, startUnixSec) =>
                                  unawaited(_planFromSuggestion(
                                      prompt, startUnixSec)),
                            ),
                          ),
                        ),
                      ),
                    ),
                  ],
                  ),
                ),
              ],
            );
          },
        ),
      ),
    );
  }

  Widget _framed(DateTime selectedDay, Widget body) => Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [_header(context, selectedDay), Expanded(child: body)],
      );

  Widget _header(BuildContext context, DateTime selectedDay) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(8, 6, 12, 4),
      child: Row(
        children: [
          IconAction(
            icon: Icons.arrow_back_ios_new,
            semanticsLabel: MaterialLocalizations.of(context).backButtonTooltip,
            iconSize: 18,
            color: Colors.white,
            onPressed: () => Navigator.of(context).maybePop(),
          ),
          const SizedBox(width: 4),
          Flexible(
            child: Tappable(
              semanticsLabel: context.l10n.homeCalendarJumpToDate,
              onTap: () => _pickDate(selectedDay),
              inkBorderRadius: BorderRadius.circular(8),
              child: Text(
                DateFormat('MMMM yyyy').format(selectedDay),
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: const TextStyle(
                  fontFamily: AppTheme.headingFont,
                  fontWeight: FontWeight.w600,
                  fontSize: 22,
                  color: Colors.white,
                ),
              ),
            ),
          ),
          const SizedBox(width: 6),
          IconAction(
            icon: Icons.chevron_left,
            semanticsLabel: context.l10n.homeCalendarPrevMonth,
            iconSize: 20,
            color: Colors.white,
            onPressed: () => _shiftMonth(selectedDay, -1),
          ),
          IconAction(
            icon: Icons.chevron_right,
            semanticsLabel: context.l10n.homeCalendarNextMonth,
            iconSize: 20,
            color: Colors.white,
            onPressed: () => _shiftMonth(selectedDay, 1),
          ),
        ],
      ),
    );
  }

  Future<void> _planFromSuggestion(String prompt, int startUnixSec) async {
    final shared = await openSuggestedCreate(context, ref, prompt, startUnixSec);
    if (!shared || !mounted) return;
    await ref.read(homeTabProvider.notifier).refresh();
    ref.invalidate(workshopCommunityCalendarProvider(_key));
  }

  void _openEntry(HomeUpNextEntry entry) {
    if (entry.contentId.isEmpty) return;
    final routeType = switch (entry.itemType) {
      DailyItemType.DAILY_ITEM_TYPE_TRANSFER ||
      DailyItemType.DAILY_ITEM_TYPE_GIVEAWAY =>
        'gear',
      DailyItemType.DAILY_ITEM_TYPE_EXPERIENCE => 'experience',
      DailyItemType.DAILY_ITEM_TYPE_REQUEST => 'request',
      _ => null,
    };
    if (routeType == null) return;
    unawaited(NavigationHelpers.pushToItemScreen(
      context: context,
      itemId: entry.contentId,
      itemType: routeType,
    ));
  }
}
