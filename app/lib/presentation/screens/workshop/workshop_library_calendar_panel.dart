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
import 'package:ripls/presentation/widgets/workshop/workshop_morph.dart';
import 'package:ripls/presentation/widgets/workshop/workshop_overview_theme.dart';

/// The Community Library's calendar — the "When" destination (#2447, #2514):
/// the same **month × backdrop** design as the inbox calendar
/// ([home_calendar_screen.dart]), scoped to this community. The selected day's
/// photo fills the panel behind a floating month grid (weather glyph per cell),
/// and the bottom dock shows that day's events, an open-day suggestion, or the
/// past-day record prompt.
///
/// It shows the **same calendar timeline as the inbox calendar**
/// ([workshopCommunityCalendarProvider] reuses `GetHomeView`) — filtered to this
/// community — plus the viewer's per-day weather and suggestions. See
/// `docs/client/calendar.md`.
class WorkshopLibraryCalendarPanel extends ConsumerStatefulWidget {
  final List<String> communityIds;

  const WorkshopLibraryCalendarPanel({super.key, required this.communityIds});

  @override
  ConsumerState<WorkshopLibraryCalendarPanel> createState() =>
      _WorkshopLibraryCalendarPanelState();
}

class _WorkshopLibraryCalendarPanelState
    extends ConsumerState<WorkshopLibraryCalendarPanel> {
  late final DateTime _today;
  late DateTime _selected;
  // Once the calendar data loads, jump to the soonest planned day so a community
  // whose only event is weeks out doesn't open on an empty "today" (#2675).
  // Only until the user navigates: a manual month/day/date change locks it.
  bool _didAutoSelect = false;

  String get _key => workshopLibraryItemsKey(widget.communityIds);

  @override
  void initState() {
    super.initState();
    final now = DateTime.now();
    _today = DateTime(now.year, now.month, now.day);
    _selected = _today;
  }

  /// After first data load, move the selection to the soonest upcoming event
  /// day (if any), once, before the user interacts.
  void _maybeAutoSelectFirstEventDay(CalendarMonthData data) {
    if (_didAutoSelect) return;
    _didAutoSelect = true;
    final soonest = data.firstEventDayOnOrAfter(_today);
    if (soonest == null || soonest == _selected) return;
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) setState(() => _selected = soonest);
    });
  }

  void _shiftMonth(int delta) {
    setState(() {
      _didAutoSelect = true; // manual nav wins over auto-select
      _selected = DateTime(_selected.year, _selected.month + delta, 1);
    });
  }

  Future<void> _pickDate() async {
    final now = DateTime.now();
    // Widget async exception: date picker is inherently UI-level.
    final picked = await showDatePicker(
      context: context,
      initialDate: _selected,
      firstDate: DateTime(now.year - 1),
      lastDate: DateTime(now.year + 2),
    );
    if (picked == null || !mounted) return;
    setState(() {
      _didAutoSelect = true; // manual nav wins over auto-select
      _selected = DateTime(picked.year, picked.month, picked.day);
    });
  }

  @override
  Widget build(BuildContext context) {
    final async = ref.watch(workshopCommunityCalendarProvider(_key));
    return WorkshopMorphPanel(
      child: async.when(
        loading: () => _framed(WorkshopLibraryStates.loading()),
        error: (e, _) => _framed(WorkshopLibraryStates.error(e)),
        data: (d) {
          final data = CalendarMonthData.from(
            entries: d.entries,
            forecast: d.forecast,
            suggestions: d.suggestions,
          );
          _maybeAutoSelectFirstEventDay(data);
          return Stack(
            fit: StackFit.expand,
            children: [
              CalendarBackdrop(mediaId: data.marqueePhotoOn(_selected)),
              const CalendarScrim(),
              LayoutBuilder(
                builder: (context, outer) => Column(
                children: [
                  _header(context),
                  // Stacked on phones (bit-identical), grid beside a
                  // day-detail rail at expanded widths (#2912).
                  Expanded(
                    child: CalendarPanes(
                      // Budget the month so the day detail keeps room on wide
                      // windows — see monthGridCellSide (#2908).
                      stackedGridMaxHeight:
                          outer.maxHeight * monthGridHeightShare,
                      gridRows: monthGridRows(_selected),
                      gridBuilder: (context, maxGridHeight) =>
                          CalendarMonthGrid(
                        month: _selected,
                        data: data,
                        selectedDay: _selected,
                        today: _today,
                        maxHeight: maxGridHeight,
                        onDaySelected: (day) => setState(() {
                          // manual nav wins over auto-select
                          _didAutoSelect = true;
                          _selected = day;
                        }),
                      ),
                      detail: Align(
                        alignment: Alignment.bottomLeft,
                        child: SingleChildScrollView(
                          reverse: true,
                          padding: const EdgeInsets.fromLTRB(22, 24, 22, 24),
                          child: CalendarDayDetail(
                            day: _selected,
                            today: _today,
                            events: data.eventsOn(_selected),
                            forecast: data.forecastOn(_selected),
                            suggestion: data.suggestionOn(_selected),
                            onEntryTap: _openEntry,
                            onSuggest: () => unawaited(
                                openBlankCreateExperience(context, ref)),
                            onGeneratePlan: (prompt, startUnixSec) => unawaited(
                                _planFromSuggestion(prompt, startUnixSec)),
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
    );
  }

  /// Loading / error states keep the month header so the panel doesn't jump.
  Widget _framed(Widget body) => Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [_header(context), Expanded(child: body)],
      );

  Widget _header(BuildContext context) {
    return Padding(
      // Right padding clears the morph panel's close (X) button.
      padding: const EdgeInsets.fromLTRB(20, 6, 52, 4),
      child: Row(
        children: [
          Flexible(
            child: Tappable(
              semanticsLabel: context.l10n.homeCalendarJumpToDate,
              onTap: _pickDate,
              inkBorderRadius: BorderRadius.circular(8),
              child: Text(
                DateFormat('MMMM yyyy').format(_selected),
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: const TextStyle(
                  fontFamily: AppTheme.headingFont,
                  fontWeight: FontWeight.w600,
                  fontSize: 22,
                  color: WorkshopOverviewPalette.onPhoto,
                ),
              ),
            ),
          ),
          const SizedBox(width: 6),
          IconAction(
            icon: Icons.chevron_left,
            semanticsLabel: context.l10n.homeCalendarPrevMonth,
            iconSize: 20,
            color: WorkshopOverviewPalette.onPhoto,
            onPressed: () => _shiftMonth(-1),
          ),
          IconAction(
            icon: Icons.chevron_right,
            semanticsLabel: context.l10n.homeCalendarNextMonth,
            iconSize: 20,
            color: WorkshopOverviewPalette.onPhoto,
            onPressed: () => _shiftMonth(1),
          ),
        ],
      ),
    );
  }

  /// After saving a suggestion-seeded plan, refresh the shared home view and
  /// re-read this community's calendar so the new item appears.
  Future<void> _planFromSuggestion(String prompt, int startUnixSec) async {
    final shared = await openSuggestedCreate(context, ref, prompt, startUnixSec);
    if (!shared || !mounted) return;
    await ref.read(homeTabProvider.notifier).refresh();
    ref.invalidate(workshopCommunityCalendarProvider(_key));
  }

  /// Opens a calendar entry's underlying gear / request / event — the same
  /// routing the inbox calendar uses.
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
