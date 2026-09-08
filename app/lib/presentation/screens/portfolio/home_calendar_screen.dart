import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:intl/intl.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/core/theme/gen/overlay_tokens.gen.dart';
import 'package:ripls/core/utils/navigation_helpers.dart';
import 'package:ripls/data/gen/ripls/api/portfolio.pb.dart';
import 'package:ripls/presentation/screens/create/blank_create_dispatcher.dart';
import 'package:ripls/presentation/viewmodels/home_tab_view_model.dart';
import 'package:ripls/presentation/viewmodels/tab_search_scope_provider.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/back_button.dart';
import 'package:ripls/presentation/widgets/destination_header.dart';
import 'package:ripls/presentation/widgets/home/calendar/calendar_backdrop.dart';
import 'package:ripls/presentation/widgets/home/calendar/calendar_day_detail.dart';
import 'package:ripls/presentation/widgets/home/calendar/calendar_month_data.dart';
import 'package:ripls/presentation/widgets/home/calendar/calendar_month_grid.dart';
import 'package:ripls/presentation/widgets/home/calendar/calendar_panes.dart';
import 'package:ripls/presentation/widgets/home/calendar/calendar_scrim.dart';
import 'package:ripls/presentation/widgets/home/calendar/month_grid_metrics.dart';
import 'package:ripls/presentation/widgets/home/calendar/plans_date_sheet.dart';
import 'package:ripls/presentation/widgets/home/home_up_next_copy.dart';
import 'package:ripls/presentation/widgets/navigation/nav_dock.dart';
import 'package:ripls/presentation/widgets/search/search_scope_pill.dart';
import 'package:ripls/services/providers.dart' show communitiesProvider;

/// HomeCalendarScreen is the month × backdrop calendar reached from the Home
/// tab's Calendar button (#2514). The selected day's marquee photo fills the
/// screen behind a floating month grid; the bottom half shows that day's detail
/// — events, or a weather-fitted suggestion on an open day. Weather glyphs sit
/// in every cell. Times and weather are server-assembled; the client never
/// invents them.
///
/// With the converged dock (#2634) the same screen doubles as the Plans
/// tab body: [embedded] hides the back button (there is no route to pop)
/// and reserves scroll clearance for the floating dock.
class HomeCalendarScreen extends ConsumerStatefulWidget {
  /// True when hosted inside the home shell's IndexedStack as the Plans
  /// tab rather than pushed as its own route.
  final bool embedded;

  const HomeCalendarScreen({super.key, this.embedded = false});

  @override
  ConsumerState<HomeCalendarScreen> createState() => _HomeCalendarScreenState();
}

class _HomeCalendarScreenState extends ConsumerState<HomeCalendarScreen> {
  DateTime? _selectedDay;

  /// Community scope set from the date & filter sheet (embedded Plans tab
  /// only). Null = all groups.
  String? _communityFilter;

  DateTime get _today {
    final now = DateTime.now();
    return DateTime(now.year, now.month, now.day);
  }

  /// Today, or — when today has nothing — the soonest day with items, so the
  /// calendar opens on a useful day.
  DateTime _initialDay(List<HomeUpNextEntry> entries) {
    final today = _today;
    bool isDay(HomeUpNextEntry e, DateTime d) {
      final w =
          DateTime.fromMillisecondsSinceEpoch(e.timeUnixSec.toInt() * 1000);
      return DateTime(w.year, w.month, w.day) == d;
    }

    if (entries.any((e) => isDay(e, today)) || entries.isEmpty) return today;
    final days = entries
        .map((e) {
          final w =
              DateTime.fromMillisecondsSinceEpoch(e.timeUnixSec.toInt() * 1000);
          return DateTime(w.year, w.month, w.day);
        })
        .where((d) => !d.isBefore(today))
        .toList()
      ..sort();
    return days.isEmpty ? today : days.first;
  }

  void _shiftMonth(int delta) {
    final cur = _selectedDay ?? _today;
    setState(() => _selectedDay = DateTime(cur.year, cur.month + delta, 1));
  }

  /// Opens the Plans date & filter sheet (the ▾'s destination): typed
  /// M / D / Y entry + quick jumps + group scope. There is no confirm —
  /// every change applies to the calendar behind the sheet immediately.
  Future<void> _openDateSheet(DateTime current) async {
    final groups = ref
        .read(communitiesProvider)
        .communities
        .where((c) => c.name.trim().isNotEmpty)
        .map((c) => PlansSheetGroup(id: c.id, name: c.name))
        .toList(growable: false);
    await showAccessibleModal<void>(
      context,
      backgroundColor: Colors.transparent,
      isScrollControlled: true,
      builder: (_) => PlansDateSheet(
        initialDate: current,
        groups: groups,
        selectedGroupId: _communityFilter,
        onDateChanged: (d) {
          if (mounted) setState(() => _selectedDay = d);
        },
        onGroupChanged: (id) {
          if (mounted) setState(() => _communityFilter = id);
        },
      ),
    );
  }

  Future<void> _pickDate(DateTime current) async {
    final now = DateTime.now();
    // Widget async exception: date picker is inherently UI-level.
    final picked = await showDatePicker(
      context: context,
      initialDate: current,
      firstDate: DateTime(now.year - 1),
      lastDate: DateTime(now.year + 2),
    );
    if (picked == null || !mounted) return;
    setState(() =>
        _selectedDay = DateTime(picked.year, picked.month, picked.day));
  }

  @override
  Widget build(BuildContext context) {
    final homeState = ref.watch(homeTabProvider);
    final view = homeState.view;
    final allEntries = view?.calendar ?? const <HomeUpNextEntry>[];
    // Scope to the sheet-selected group when one is active; the shared
    // timeline carries each entry's community so no re-fetch is needed.
    final grouped = _communityFilter == null
        ? allEntries
        : allEntries
            .where((e) => e.communityId == _communityFilter)
            .toList(growable: false);
    // A universal-search escape further scopes the tab to its query
    // (embedded Plans only); the pill under the header cancels it.
    final searchScope =
        widget.embedded ? ref.watch(plansSearchScopeProvider) : null;
    final entries = searchScope == null
        ? grouped
        : grouped.where((e) {
            final q = searchScope.toLowerCase();
            return homeUpNextTitle(context.l10n, e).toLowerCase().contains(q) ||
                homeUpNextStatusLabel(context.l10n, e)
                    .toLowerCase()
                    .contains(q);
          }).toList(growable: false);
    final data = CalendarMonthData.from(
      entries: entries,
      forecast: view?.forecast ?? const <DayForecast>[],
      // While scoped, open days show no suggestion cards — the tab holds
      // only the search's results.
      suggestions: searchScope == null
          ? view?.openDaySuggestions ?? const <OpenDaySuggestion>[]
          : const <OpenDaySuggestion>[],
    );
    // The Plans tab anchors on today — a destination you land on daily.
    // Only the pushed see-all variant keeps the soonest-useful-day seek.
    final selectedDay =
        _selectedDay ?? (widget.embedded ? _today : _initialDay(entries));

    return Scaffold(
      backgroundColor: Colors.black,
      body: Stack(
        fit: StackFit.expand,
        children: [
          CalendarBackdrop(mediaId: data.marqueePhotoOn(selectedDay)),
          // Full-screen scrim: dark at top for the header, dark at bottom for
          // the day detail, lighter through the grid.
          const CalendarScrim(),
          SafeArea(
            child: LayoutBuilder(
              builder: (context, outer) => Column(
              children: [
                _header(context, selectedDay),
                if (searchScope != null)
                  Padding(
                    padding: const EdgeInsets.fromLTRB(20, 0, 20, 4),
                    child: SearchScopePill(
                      query: searchScope,
                      onClear: () => ref
                          .read(plansSearchScopeProvider.notifier)
                          .clear(),
                    ),
                  ),
                // Stacked (grid over detail) in compact windows — the phone
                // layout, bit-identical — or grid beside a day-detail rail at
                // expanded widths (#2912). CalendarPanes owns the switch.
                Expanded(
                  child: CalendarPanes(
                    // Cap the month at just over half the screen so the day
                    // detail always has room in the stacked arm. Without a
                    // budget the grid's square cells size from width alone and
                    // swallow the whole viewport on any landscape window
                    // (#2908). Phones are unaffected — there width is still
                    // the binding constraint.
                    stackedGridMaxHeight: outer.maxHeight * monthGridHeightShare,
                    gridRows: monthGridRows(selectedDay),
                    gridBuilder: (context, maxGridHeight) => CalendarMonthGrid(
                      month: selectedDay,
                      data: data,
                      selectedDay: selectedDay,
                      today: _today,
                      maxHeight: maxGridHeight,
                      onDaySelected: (d) => setState(() => _selectedDay = d),
                    ),
                    detail: LayoutBuilder(
                      builder: (context, constraints) {
                        final bottomPad = widget.embedded
                            ? 24 + NavDock.bottomContentInset
                            : 24.0;
                        return Align(
                          alignment: Alignment.bottomLeft,
                          child: SingleChildScrollView(
                            reverse: true,
                            padding: EdgeInsets.fromLTRB(22, 24, 22, bottomPad),
                            child: CalendarDayDetail(
                              day: selectedDay,
                              today: _today,
                              events: data.eventsOn(selectedDay),
                              forecast: data.forecastOn(selectedDay),
                              suggestion: data.suggestionOn(selectedDay),
                              onEntryTap: (e) => _openEntry(context, e),
                              onSuggest: () => unawaited(
                                  openBlankCreateExperience(context, ref)),
                              onGeneratePlan: (prompt, startUnixSec) =>
                                  unawaited(_planFromSuggestion(
                                      prompt, startUnixSec)),
                            ),
                          ),
                        );
                      },
                    ),
                  ),
                ),
              ],
              ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _header(BuildContext context, DateTime selectedDay) {
    // As the Plans tab (#2634 v2), the header matches the shared
    // destination-header geometry: serif title left, month subtitle
    // (tap = date picker), month arrows, avatar chip — in white over
    // the photo backdrop. The calendar page doesn't scroll as a whole,
    // so the header pins at the shared position.
    if (widget.embedded) {
      // Dynamic title (Rev 14): Plans answers "when" — the month is the
      // headline and a control ("July 2026 ▾" opens the date & filter
      // sheet). The subtitle states the honest scope: the filtered
      // group's name, or "across your groups".
      final filterName = _communityFilter == null
          ? null
          : ref
              .watch(communitiesProvider)
              .communities
              .where((c) => c.id == _communityFilter)
              .firstOrNull
              ?.name;
      return DestinationHeader(
        title: '${DateFormat('MMMM yyyy').format(selectedDay)} ▾',
        onTitleTap: () => _openDateSheet(selectedDay),
        titleSemanticsLabel: context.l10n.plansSheetTitle,
        subtitle: filterName ?? context.l10n.plansSubtitleAllGroups,
        subtitleColor: OverlayTokens.textFaint,
        foreground: OverlayTokens.textPrimary,
        trailing: [
          _monthArrow(context, back: true),
          _monthArrow(context, back: false),
        ],
      );
    }
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 6, 16, 4),
      child: Row(
        children: [
          BackButtonWidget(onPressed: () => Navigator.pop(context)),
          const Spacer(),
          Tappable(
            // The month title taps to a date picker.
            semanticsLabel: context.l10n.homeCalendarJumpToDate,
            onTap: () => _pickDate(selectedDay),
            inkBorderRadius: BorderRadius.circular(8),
            child: Text(
              DateFormat('MMMM yyyy').format(selectedDay),
              style: const TextStyle(
                fontFamily: AppTheme.headingFont,
                fontSize: 23,
                fontWeight: FontWeight.w600,
                color: OverlayTokens.textPrimary,
                shadows: [Shadow(color: Color(0x80000000), blurRadius: 10)],
              ),
            ),
          ),
          const Spacer(),
          IconAction(
            icon: Icons.chevron_left,
            semanticsLabel: context.l10n.homeCalendarPrevMonth,
            color: OverlayTokens.textPrimary,
            onPressed: () => _shiftMonth(-1),
          ),
          IconAction(
            icon: Icons.chevron_right,
            semanticsLabel: context.l10n.homeCalendarNextMonth,
            color: OverlayTokens.textPrimary,
            onPressed: () => _shiftMonth(1),
          ),
        ],
      ),
    );
  }

  /// A month-nav chevron for the embedded header's trailing slot, sized
  /// to the header's 28dp chip line (avatar / search). Deliberately not
  /// [IconAction]: Material's padded tap-target inflates an IconButton's
  /// *layout* box to 48×48 around any tighter visual constraints, which
  /// re-centers the glyph ~10dp below the title/avatar line. A plain
  /// [Tappable] box keeps the glyph on it (the search chip's pattern).
  Widget _monthArrow(BuildContext context, {required bool back}) {
    return Tappable(
      semanticsLabel: back
          ? context.l10n.homeCalendarPrevMonth
          : context.l10n.homeCalendarNextMonth,
      onTap: () => _shiftMonth(back ? -1 : 1),
      inkBorderRadius: BorderRadius.circular(14),
      child: SizedBox(
        width: 36,
        height: 28,
        child: Center(
          child: Icon(
            back ? Icons.chevron_left : Icons.chevron_right,
            size: 22,
            color: OverlayTokens.textPrimary,
          ),
        ),
      ),
    );
  }

  /// Opens the suggestion-seeded create flow and, when the user saves the
  /// generated item, refreshes the home view so the new item shows on the
  /// calendar straight away.
  Future<void> _planFromSuggestion(String prompt, int startUnixSec) async {
    final shared =
        await openSuggestedCreate(context, ref, prompt, startUnixSec);
    if (!shared || !mounted) return;
    await ref.read(homeTabProvider.notifier).refresh();
  }

  void _openEntry(BuildContext context, HomeUpNextEntry entry) {
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
