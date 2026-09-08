import 'dart:async';

import 'package:fixnum/fixnum.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:intl/intl.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/core/utils/calendar_helper.dart';
import 'package:ripls/data/gen/ripls/api/experience.pbenum.dart' as expproto;
import 'package:ripls/data/gen/ripls/api/experience_service.pb.dart'
    show RSVPIntention;
import 'package:ripls/data/gen/ripls/api/portfolio.pb.dart'
    show DayForecast, HomeUpNextEntry, HourForecast;
import 'package:ripls/data/gen/ripls/api/time.pb.dart'
    show TimeProposal, TimeVoteStatus;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/screens/experience/widgets/experience_time_manage_menu.dart';
import 'package:ripls/presentation/screens/experience/widgets/experience_time_panel_helpers.dart';
import 'package:ripls/presentation/screens/experience/widgets/experience_time_set_final_view.dart';
import 'package:ripls/presentation/screens/experience/widgets/experience_when_agenda.dart';
import 'package:ripls/presentation/screens/experience/widgets/experience_when_dock.dart';
import 'package:ripls/presentation/screens/experience/widgets/experience_when_tool_button.dart';
import 'package:ripls/presentation/screens/experience/widgets/experience_when_weather_strip.dart';
import 'package:ripls/presentation/viewmodels/experience_day_weather_provider.dart';
import 'package:ripls/presentation/viewmodels/experience_view_model.dart';
import 'package:ripls/presentation/viewmodels/home_tab_view_model.dart';
import 'package:ripls/presentation/viewmodels/time_modal_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';
import 'package:ripls/presentation/widgets/accessibility/semantic_announcer.dart';
import 'package:ripls/presentation/widgets/content/content_morph_panel.dart';
import 'package:ripls/presentation/widgets/home/calendar/calendar_month_data.dart';
import 'package:ripls/presentation/widgets/home/calendar/calendar_month_grid.dart';
import 'package:ripls/presentation/widgets/home/home_up_next_copy.dart';
import 'package:ripls/presentation/widgets/modal/glass/date_time_picker_modal.dart';
import 'package:ripls/presentation/widgets/poll/poll_option_widgets.dart';
import 'package:ripls/services/providers.dart';

/// What the inline picker should do with the time the user saves.
enum _PickIntent {
  /// Replace / set the single confirmed event time (no poll).
  setSingle,

  /// Add the time to a poll (starting or extending it).
  propose,
}

/// ExperienceTimePanel is the full-screen "When" surface the time card expands
/// into — the time twin of [ExperienceLocationPanel] (when-screen v3, modeled
/// 1:1 on Where). The Home calendar's month grid (CalendarMonthGrid, #2514)
/// replaces the map as the spatial anchor: the proposed/confirmed days show as
/// photo cells with the selected day ringed. It absorbs the
/// time-poll bottom sheets into inline states: poll voting (auto-submit), an
/// inline set-final-time view, a single-time (n=1) accept/decline state, owner
/// controls, and an in-panel manage menu from the top bar.
///
/// Reads [timeModalProvider] for state + mutations and [experienceProvider] for
/// the attendee roster. Shared poll presentation widgets live in
/// `widgets/poll/poll_option_widgets.dart` (used by both Where and When).
class ExperienceTimePanel extends ConsumerStatefulWidget {
  final String experienceId;
  final Color accentColor;

  /// Owner experience-settings actions surfaced in the overflow menu (the same
  /// "Mark completed" / "Close event" flows as the content view's Manage
  /// sheet). Null when the viewer isn't the owner.
  final VoidCallback? onMarkCompleted;
  final VoidCallback? onCloseEvent;

  const ExperienceTimePanel({
    super.key,
    required this.experienceId,
    required this.accentColor,
    this.onMarkCompleted,
    this.onCloseEvent,
  });

  @override
  ConsumerState<ExperienceTimePanel> createState() =>
      _ExperienceTimePanelState();
}

class _ExperienceTimePanelState extends ConsumerState<ExperienceTimePanel> {
  final Set<String> _inFlight = {};
  bool _busy = false;
  bool _settingFinal = false;
  String? _finalTargetId;

  /// The day highlighted in the month calendar. Null until the user taps a day;
  /// defaults to the proposed/confirmed anchor day.
  DateTime? _calSelected;

  /// When set the panel shows the inline date/time picker (absorbed from
  /// `DateTimePickerModal`); the value is what to do with the picked time.
  _PickIntent? _picking;
  DateTime? _pickInitial;

  String get _experienceId => widget.experienceId;

  @override
  void initState() {
    super.initState();
    ref.listenManual(contentCacheInvalidationProvider, (previous, next) {
      if (previous == next) return;
      ref.invalidate(timeModalProvider(_experienceId));
    });
  }

  @override
  Widget build(BuildContext context) {
    final dataAsync = ref.watch(timeModalProvider(_experienceId));
    return ContentMorphPanel(
      child: SafeArea(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            _header(context, dataAsync.asData?.value),
            Expanded(
              child: _picking != null
                  ? _pickerBody()
                  : dataAsync.when(
                      data: (data) => _content(context, data),
                      loading: () =>
                          const Center(child: CircularProgressIndicator()),
                      error: (_, _) => Center(
                        child: Text(
                          context.l10n.commonError,
                          style:
                              const TextStyle(color: AppColors.onContentImage),
                        ),
                      ),
                    ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _header(BuildContext context, TimeModalData? data) {
    final l10n = context.l10n;
    final picking = _picking != null;
    // The overflow is always available to the owner: it carries the
    // experience-settings actions (mark completed / close event) plus the
    // poll-management actions while a poll is running.
    final showManage =
        !_settingFinal && !picking && data != null && data.isOrganizer;
    return Padding(
      padding: const EdgeInsets.fromLTRB(20, 8, 12, 8),
      child: Row(
        children: [
          Expanded(
            child: Semantics(
              header: true,
              child: Text(
                l10n.contentFactWhen,
                style: const TextStyle(
                  fontFamily: AppTheme.headingFont,
                  fontSize: 24,
                  fontWeight: FontWeight.w600,
                  color: AppColors.onContentImage,
                ),
              ),
            ),
          ),
          if (showManage)
            IconAction(
              icon: Icons.more_horiz,
              semanticsLabel: l10n.a11yLocationPanelManage,
              color: AppColors.onContentImage,
              onPressed: _busy ? null : () => _openManageMenu(data),
            ),
          if (picking)
            IconAction(
              icon: Icons.arrow_back,
              semanticsLabel: l10n.commonCancel,
              color: AppColors.onContentImage,
              onPressed: () => setState(() {
                _picking = null;
                _pickInitial = null;
              }),
            )
          else
            IconAction(
              icon: Icons.close_rounded,
              semanticsLabel: l10n.a11yClose,
              color: AppColors.onContentImage,
              onPressed: () => Navigator.of(context).pop(),
            ),
        ],
      ),
    );
  }

  Widget _content(BuildContext context, TimeModalData data) {
    // The month calendar anchors the confirmed/TBD states. During a poll (and
    // the set-final flow) the options list needs the room, so hide it.
    final showCalendar = !data.timePollActive && !_settingFinal;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        if (showCalendar) _calendarSection(context, data),
        Expanded(
          child: ListView(
            padding: const EdgeInsets.fromLTRB(20, 16, 20, 28),
            children: _settingFinal
                ? [
                    ExperienceTimeSetFinalView(
                      experienceId: _experienceId,
                      finalTargetId: _finalTargetId,
                      accentColor: widget.accentColor,
                      busy: _busy,
                      onRetarget: (id) => setState(() => _finalTargetId = id),
                      onCancel: () => setState(() => _settingFinal = false),
                      onLock: _lockInFinal,
                    ),
                  ]
                : _body(context, data),
          ),
        ),
      ],
    );
  }

  // ── Calendar (the "map") ─────────────────────────────────────────────────

  Widget _calendarSection(BuildContext context, TimeModalData data) {
    // The event's photo (so proposed/confirmed days render like the Home
    // calendar) and its day's forecast (so the cell shows a weather glyph).
    final details = ref.watch(experienceProvider(_experienceId)).experienceDetails;
    final exp = details?.experience;
    final mediaId = (exp != null && exp.mediaIds.isNotEmpty) ? exp.mediaIds.first : '';
    final title = exp?.name ?? '';

    // The viewer's other commitments + per-day weather, straight from the Home
    // calendar feed, so the grid shows the same entries the Home calendar does
    // and the user can spot conflicts. This experience is excluded by content id
    // so its confirmed day isn't double-counted — its own days come from the
    // proposed/confirmed options below (which also cover unconfirmed poll days
    // the Home feed doesn't carry).
    final homeView = ref.watch(homeTabProvider).view;
    final otherEntries = [
      for (final e in homeView?.calendar ?? const <HomeUpNextEntry>[])
        if (e.contentId != _experienceId) e,
    ];
    // Weather is for the EXPERIENCE's location (not the viewer's home), plus the
    // event-day forecast as an immediate fallback while the window loads.
    final calWeather =
        ref.watch(experienceCalendarWeatherProvider(_experienceId));
    final forecast = <DayForecast>[
      ...?calWeather.asData?.value,
      if (details != null && details.hasForecast()) details.forecast,
    ];

    // Collect the proposed times (or the confirmed time) as calendar entries.
    final options = <TimeOptionDisplay>[];
    DateTime? anchor;
    if (data.timePollActive) {
      for (final p in data.currentPollProposals) {
        final opt = resolveTimeOption(p);
        if (opt.dateTime == null) continue;
        options.add(opt);
        anchor ??= opt.dateTime;
      }
    } else {
      final confirmed = _confirmedOption(data);
      if (confirmed?.dateTime != null) {
        options.add(confirmed!);
        anchor = confirmed.dateTime;
      }
    }

    final entries = [
      ...otherEntries,
      for (final opt in options)
        HomeUpNextEntry()
          ..contentId = _experienceId
          ..timeUnixSec = Int64(opt.dateTime!.millisecondsSinceEpoch ~/ 1000)
          ..title = title
          ..thumbnailMediaId = mediaId,
    ];
    final monthData = CalendarMonthData.from(
      entries: entries,
      forecast: forecast,
      suggestions: const [],
    );

    final now = DateTime.now();
    final today = DateTime(now.year, now.month, now.day);
    // The committed event day(s) stay gold; the viewed (selected) day is where
    // the user has tapped, defaulting to the event's first day. Multi-day events
    // highlight every day in [chosenDay, chosenEndDay].
    final chosenDay = confirmedEventDay(data);
    final chosenEndDay = confirmedEventEndDay(data);
    final selected = _calSelected != null
        ? timeDayKey(_calSelected!)
        : chosenDay ??
            (anchor != null
                ? DateTime(anchor.year, anchor.month, anchor.day)
                : today);

    return Semantics(
      label: context.l10n.a11yTimePanelCalendar,
      child: Padding(
        padding: const EdgeInsets.fromLTRB(18, 8, 18, 8),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            _monthNav(context, selected),
            const SizedBox(height: 6),
            CalendarMonthGrid(
              month: selected,
              data: monthData,
              selectedDay: selected,
              today: today,
              chosenDay: chosenDay,
              chosenEndDay: chosenEndDay,
              onDaySelected: (d) => setState(() => _calSelected = d),
            ),
          ],
        ),
      ),
    );
  }

  /// Month label + prev/next chevrons over the calendar, matching the Home
  /// calendar's month nav.
  Widget _monthNav(BuildContext context, DateTime selected) {
    void shift(int delta) => setState(
        () => _calSelected = DateTime(selected.year, selected.month + delta, 1));
    return Row(
      children: [
        Text(
          DateFormat('MMMM yyyy').format(selected),
          style: const TextStyle(
            fontFamily: AppTheme.headingFont,
            fontSize: 18,
            fontWeight: FontWeight.w600,
            color: AppColors.onContentImage,
          ),
        ),
        const Spacer(),
        IconAction(
          icon: Icons.chevron_left,
          semanticsLabel: context.l10n.homeCalendarPrevMonth,
          iconSize: 20,
          color: AppColors.onContentImage,
          onPressed: () => shift(-1),
        ),
        IconAction(
          icon: Icons.chevron_right,
          semanticsLabel: context.l10n.homeCalendarNextMonth,
          iconSize: 20,
          color: AppColors.onContentImage,
          onPressed: () => shift(1),
        ),
      ],
    );
  }

  // ── Body (state-dependent) ───────────────────────────────────────────────

  List<Widget> _body(BuildContext context, TimeModalData data) {
    if (data.timePollActive) {
      final visible = data.currentPollProposals;
      final viewerIsProposer = visible.length == 1 &&
          visible.first.proposedBy.id == data.currentUserId;
      if (visible.length == 1 && !data.isOrganizer && !viewerIsProposer) {
        return _singleBody(context, data, visible.first);
      }
      return _pollBody(context, data, visible);
    }
    if (_confirmedOption(data) != null) return [_confirmedDock(context, data)];
    return _tbdBody(context, data);
  }

  List<Widget> _pollBody(
    BuildContext context,
    TimeModalData data,
    List<TimeProposal> visible,
  ) {
    final l10n = context.l10n;
    final submitted = data.repliedUserIds.contains(data.currentUserId);
    final leadingId = data.leadingProposalId;
    final tiedTop = pollTiedTopCount(
      visible.map((p) => p.id).toList(),
      data.effectiveVoteCount,
    );

    return [
      Text(
        submitted
            ? l10n.locationPollVoteSubmittedTitle
            : '${l10n.timePollVoteTitleLine1} ${l10n.timePollVoteTitleLine2}',
        style: const TextStyle(
          color: AppColors.onContentImage,
          fontSize: 22,
          fontWeight: FontWeight.w800,
          height: 1.1,
        ),
      ),
      const SizedBox(height: 12),
      ..._unrepliedSection(context, data),
      const SizedBox(height: 14),
      for (var i = 0; i < visible.length; i++)
        _optionRow(context, data, visible[i], i, leadingId, tiedTop),
      ...whenPollActions(
        context: context,
        locked: data.proposalsLocked,
        isOrganizer: data.isOrganizer,
        busy: _busy,
        hasOptions: visible.isNotEmpty,
        onAddAnother: () => _pickTime(setSingle: false),
        onSetFinal: () => _enterSetFinal(data),
      ),
    ];
  }

  Widget _optionRow(
    BuildContext context,
    TimeModalData data,
    TimeProposal p,
    int index,
    String? leadingId,
    int tiedTop,
  ) {
    final opt = resolveTimeOption(p);
    return PollOptionRow(
      letter: pollOptionLetter(index),
      name: opt.name,
      subtitle: opt.subtitle,
      addedByName: p.proposedBy.id != data.currentUserId
          ? p.proposedBy.name
          : null,
      voters: timeYesVoters(p.votes),
      voted: data.getUserVote(p.id) == TimeVoteStatus.TIME_VOTE_STATUS_YES,
      leading: p.id == leadingId,
      tied: tiedTop > 1 && data.effectiveVoteCount(p.id) == tiedTop,
      accentColor: widget.accentColor,
      enabled: !_inFlight.contains(p.id),
      onTap: () => _toggleVote(p.id),
    );
  }

  /// Single-time (n=1) state for an attendee who didn't propose it.
  List<Widget> _singleBody(
    BuildContext context,
    TimeModalData data,
    TimeProposal p,
  ) {
    final l10n = context.l10n;
    final opt = resolveTimeOption(p);
    final voted = data.getUserVote(p.id) == TimeVoteStatus.TIME_VOTE_STATUS_YES;
    final proposer = p.proposedBy.name;

    return [
      if (proposer.isNotEmpty)
        Text(
          l10n.timePanelSingleKicker(proposer).toUpperCase(),
          style: const TextStyle(
            color: AppColors.darkTextSecondary,
            fontSize: 11.5,
            fontWeight: FontWeight.w800,
            letterSpacing: 1.2,
          ),
        ),
      const SizedBox(height: 6),
      Text(
        opt.name,
        style: const TextStyle(
          color: AppColors.onContentImage,
          fontSize: 24,
          fontWeight: FontWeight.w800,
          height: 1.15,
        ),
      ),
      if (opt.subtitle.isNotEmpty) ...[
        const SizedBox(height: 6),
        Text(
          opt.subtitle,
          style: const TextStyle(
            color: AppColors.darkTextSecondary,
            fontSize: 15,
          ),
        ),
      ],
      const SizedBox(height: 12),
      Text(
        l10n.timePanelSingleSubtitle,
        style: const TextStyle(
          color: AppColors.darkTextSecondary,
          fontSize: 14.5,
          height: 1.45,
        ),
      ),
      const SizedBox(height: 16),
      PollPanelButton(
        label: l10n.locationPanelWorksForMe,
        icon: Icons.check,
        accentColor: widget.accentColor,
        primary: true,
        onTap: (voted || _busy) ? null : () => _setWorks(p.id),
      ),
      const SizedBox(height: 10),
      Row(
        children: [
          Expanded(
            child: PollPanelButton(
              label: l10n.locationPanelDoesntWork,
              accentColor: widget.accentColor,
              onTap: _busy ? null : () => _pickTime(setSingle: false),
            ),
          ),
          const SizedBox(width: 10),
          Expanded(
            child: PollPanelButton(
              label: l10n.timePollFinalizedAddToCalendar,
              icon: Icons.event_available_outlined,
              accentColor: widget.accentColor,
              onTap: _addToCalendar,
            ),
          ),
        ],
      ),
    ];
  }

  /// Confirmed single time, redesigned as the agenda dock (event-when-agenda):
  /// a hero (event date/time + export icon, or "Exploring … Back to your event"),
  /// the viewed day's weather strip + agenda of other commitments, and the host
  /// control bar (Mark done · Change time · Ask the group).
  Widget _confirmedDock(BuildContext context, TimeModalData data) {
    final l10n = context.l10n;
    final confirmed = _confirmedOption(data)!;
    final ct = confirmedExperienceTime(data);
    final startSec = timeStartSec(ct);
    final endSec = timeEndSec(ct);

    final eventDay = confirmedEventDay(data) ?? timeDayKey(DateTime.now());
    final eventEndDay = confirmedEventEndDay(data) ?? eventDay;
    final viewedDay = _calSelected != null ? timeDayKey(_calSelected!) : eventDay;
    // Any day within a multi-day event reads as "the event", not exploring.
    final exploring =
        viewedDay.isBefore(eventDay) || viewedDay.isAfter(eventEndDay);
    final viewedMidnight = viewedDay.millisecondsSinceEpoch ~/ 1000;
    final focusHour = startSec != null
        ? DateTime.fromMillisecondsSinceEpoch(startSec * 1000).hour
        : 12;

    // Hour-by-hour weather for the viewed day (lazy + cached per day), with a
    // coarse daily fallback for days beyond the hourly horizon.
    final weatherAsync = ref.watch(experienceDayWeatherProvider(
        (experienceId: _experienceId, dayUnixSec: viewedMidnight)));
    final weather = weatherAsync.asData?.value;
    final hours = weather?.hours ?? const <HourForecast>[];

    final completed =
        ref.watch(experienceProvider(_experienceId)).experienceDetails?.experience.state ==
            expproto.ExperienceState.EXPERIENCE_STATE_COMPLETED;

    return ExperienceWhenDock(
      exploring: exploring,
      eventDateText: confirmed.name,
      eventTimeText: confirmed.subtitle,
      completed: completed,
      onExport: _addToCalendar,
      exploringDateText: DateFormat('EEE, MMM d').format(viewedDay),
      onBackToEvent: () => setState(() => _calSelected = null),
      weatherStrip: ExperienceWhenWeatherStrip(
        label: exploring
            ? l10n.timePanelWeatherHourByHour(
                DateFormat('EEE, MMM d').format(viewedDay))
            : l10n.timePanelWeatherWindowLabel(confirmed.subtitle),
        hours: windowHours(hours, focusHour),
        // Highlight the event window only on the event day.
        windowStartUnixSec: exploring ? null : startSec,
        windowEndUnixSec: exploring ? null : endSec,
        loading: weatherAsync.isLoading,
        fallback: weather?.dayForecast,
        unavailableText: l10n.timePanelWeatherUnavailable,
      ),
      agenda: ExperienceWhenAgenda(
        dayLabel: l10n.timePanelAgendaOnDay(DateFormat('MMM d').format(viewedDay)),
        rows: _agendaRows(context, viewedDay, startSec, endSec, confirmed.subtitle),
        emptyText: l10n.timePanelAgendaClear,
      ),
      showControls: data.isOrganizer,
      // Mark done runs the content view's completion flow; disabled once done.
      onMarkDone: completed ? null : widget.onMarkCompleted,
      onChangeTime: () => _pickTime(setSingle: true),
      onAskGroup: () => _pickTime(setSingle: false),
    );
  }

  /// The viewer's other commitments on [viewedDay], from the Home feed, with a
  /// conflict tag on any that overlaps the event window.
  List<WhenAgendaRow> _agendaRows(
    BuildContext context,
    DateTime viewedDay,
    int? startSec,
    int? endSec,
    String windowRange,
  ) {
    final homeView = ref.watch(homeTabProvider).view;
    final l10n = context.l10n;
    final rows = <(int, WhenAgendaRow)>[];
    for (final e in homeView?.calendar ?? const <HomeUpNextEntry>[]) {
      if (e.contentId == _experienceId) continue;
      final sec = e.timeUnixSec.toInt();
      if (sec == 0) continue;
      final d = timeDayKey(DateTime.fromMillisecondsSinceEpoch(sec * 1000));
      if (d != viewedDay) continue;
      final conflict =
          startSec != null && endSec != null && sec >= startSec && sec < endSec;
      final timeText =
          DateFormat('h:mm a').format(DateTime.fromMillisecondsSinceEpoch(sec * 1000));
      final agendaTitle = homeUpNextTitle(l10n, e);
      rows.add((
        sec,
        WhenAgendaRow(
          name: agendaTitle,
          time: conflict
              ? '$timeText · ${l10n.timePanelAgendaOverlaps(windowRange)}'
              : timeText,
          conflict: conflict,
          // Tapping a commitment opens its item screen (slides in from the right).
          semanticsLabel: canOpenAgendaItem(e) ? agendaTitle : null,
          onTap: canOpenAgendaItem(e) ? () => openAgendaItem(context, e) : null,
        ),
      ));
    }
    rows.sort((a, b) => a.$1.compareTo(b.$1));
    return [for (final r in rows) r.$2];
  }

  /// No time and no poll: owner can pick a time or start a poll; everyone else
  /// sees a short "not set yet" note.
  List<Widget> _tbdBody(BuildContext context, TimeModalData data) {
    final l10n = context.l10n;
    return [
      Text(
        l10n.timePollProposeTitle,
        style: const TextStyle(
          color: AppColors.onContentImage,
          fontSize: 22,
          fontWeight: FontWeight.w800,
          height: 1.15,
        ),
      ),
      const SizedBox(height: 6),
      Text(
        data.isOrganizer ? l10n.timePollProposeSubtitle : l10n.timePanelTbdHostBody,
        style: const TextStyle(
          color: AppColors.darkTextSecondary,
          fontSize: 14,
          height: 1.4,
        ),
      ),
      if (data.isOrganizer) ...[
        const SizedBox(height: 16),
        PollPanelButton(
          label: l10n.timePollProposeSetTime,
          icon: Icons.schedule_outlined,
          accentColor: widget.accentColor,
          primary: true,
          onTap: _busy ? null : () => _pickTime(setSingle: true),
        ),
        const SizedBox(height: 10),
        PollPanelButton(
          label: l10n.timePollProposeAskGroup,
          icon: Icons.how_to_vote_outlined,
          accentColor: widget.accentColor,
          onTap: _busy ? null : () => _pickTime(setSingle: false),
        ),
      ],
    ];
  }

  // ── Unreplied strip ───────────────────────────────────────────────────────

  List<Widget> _unrepliedSection(BuildContext context, TimeModalData data) {
    final attendees = _attendees();
    if (attendees.isEmpty) return const [];
    final unreplied =
        attendees.where((u) => !data.repliedUserIds.contains(u.id)).toList();
    final urgent = unreplied.isNotEmpty && _deadlineNear(data);
    final names = unreplied.map((u) => u.name).where((n) => n.isNotEmpty);
    final l10n = context.l10n;
    final message = unreplied.isEmpty
        ? l10n.locationPanelEveryoneReplied
        : urgent
            ? l10n.locationPanelClosesSoonNames(names.join(', '))
            : l10n.locationPanelUnrepliedNames(names.join(', '));
    return [
      PollUnrepliedStrip(
        unreplied: unreplied,
        message: message,
        urgent: urgent,
        accentColor: widget.accentColor,
        onNudge: data.isOrganizer ? _nudge : null,
      ),
    ];
  }

  List<User> _attendees() {
    final exp = ref.watch(experienceProvider(_experienceId));
    final rsvps = exp.experienceDetails?.rsvps ?? const [];
    return [
      for (final r in rsvps)
        if (r.intention == RSVPIntention.RSVP_INTENTION_YES ||
            r.intention == RSVPIntention.RSVP_INTENTION_MAYBE)
          r.user,
    ];
  }

  bool _deadlineNear(TimeModalData data) {
    final deadline = data.pollDeadlineUnixSec;
    if (deadline == null) return false;
    final secsLeft = deadline - DateTime.now().millisecondsSinceEpoch ~/ 1000;
    return secsLeft > 0 && secsLeft < 24 * 3600;
  }

  // ── Helpers ───────────────────────────────────────────────────────────────

  /// The confirmed event time as a display option — the locked proposal if any,
  /// else the experience's set event time. Null when nothing is confirmed.
  TimeOptionDisplay? _confirmedOption(TimeModalData data) {
    final locked = data.lockedProposal;
    if (locked != null) return resolveTimeOption(locked);
    final t = data.eventTime;
    if (t != null && (t.hasSpecific() || t.hasRange())) {
      return resolveExperienceTime(t);
    }
    return null;
  }

  // ── Actions ──────────────────────────────────────────────────────────────

  Future<void> _toggleVote(String proposalId) async {
    if (_inFlight.contains(proposalId)) return;
    setState(() => _inFlight.add(proposalId));
    try {
      await ref
          .read(timeModalProvider(_experienceId).notifier)
          .voteOnTime(proposalId);
      if (!mounted) return;
      unawaited(
        SemanticAnnouncer.announce(context, context.l10n.timePollVoteRecorded),
      );
    } catch (_) {
      _showError();
    } finally {
      if (mounted) setState(() => _inFlight.remove(proposalId));
    }
  }

  Future<void> _setWorks(String proposalId) async {
    await _guard(() => ref
        .read(timeModalProvider(_experienceId).notifier)
        .voteOnTime(proposalId));
    if (mounted) {
      unawaited(
        SemanticAnnouncer.announce(context, context.l10n.timePollVoteRecorded),
      );
    }
  }

  /// Opens the date/time picker, then either sets the single event time or adds
  /// the time to the poll (starting or extending it).
  /// Opens the inline date/time picker (on the panel itself, not a modal),
  /// seeded with the confirmed time, to set / propose the picked time on save.
  void _pickTime({required bool setSingle}) {
    final data = ref.read(timeModalProvider(_experienceId)).asData?.value;
    setState(() {
      _pickInitial = _confirmedOption(data ?? const TimeModalData())?.dateTime ??
          DateTime.now().add(const Duration(hours: 1));
      _picking = setSingle ? _PickIntent.setSingle : _PickIntent.propose;
    });
  }

  void _onPickerSaved(DateTime dt) {
    final intent = _picking;
    setState(() {
      _picking = null;
      _pickInitial = null;
    });
    if (intent == null) return;
    final notifier = ref.read(timeModalProvider(_experienceId).notifier);
    final pollActive =
        ref.read(timeModalProvider(_experienceId)).asData?.value.timePollActive ??
            false;
    _guard(() {
      if (intent == _PickIntent.setSingle) return notifier.setSingleTime(dt);
      final options = [(dt, TimeOfDay.fromDateTime(dt))];
      return pollActive
          ? notifier.addTimesToActivePoll(options)
          : notifier.createTimePoll(options);
    });
  }

  /// The inline date/time picker (absorbed `DateTimePickerModal`) rendered on
  /// the panel surface — no separate modal.
  Widget _pickerBody() {
    final now = DateTime.now();
    return Padding(
      padding: const EdgeInsets.fromLTRB(20, 8, 20, 16),
      child: DateTimePickerView(
        initialDateTime: _pickInitial ?? now.add(const Duration(hours: 1)),
        firstDate: now.subtract(const Duration(days: 1)),
        lastDate: now.add(const Duration(days: 365)),
        // Adding a poll candidate reads as "Add"; setting the single confirmed
        // time keeps the default "Save" verb.
        primaryLabel: _picking == _PickIntent.propose
            ? context.l10n.commonAdd
            : null,
        onSaved: _onPickerSaved,
        onCancel: () => setState(() {
          _picking = null;
          _pickInitial = null;
        }),
      ),
    );
  }

  Future<void> _addToCalendar() async {
    final exp =
        ref.read(experienceProvider(_experienceId)).experienceDetails?.experience;
    if (exp == null) return;
    await CalendarHelper.addToCalendar(exp);
  }

  void _enterSetFinal(TimeModalData data) {
    setState(() {
      _finalTargetId =
          data.leadingProposalId ?? data.currentPollProposals.firstOrNull?.id;
      _settingFinal = true;
    });
  }

  Future<void> _lockInFinal() async {
    final id = _finalTargetId;
    if (id == null) return;
    await _guard(() =>
        ref.read(timeModalProvider(_experienceId).notifier).lockTime(id));
    if (mounted) setState(() => _settingFinal = false);
  }

  Future<void> _nudge() async {
    await _guard(() async {
      final count = await ref
          .read(timeModalProvider(_experienceId).notifier)
          .nudgeTimePollVoters();
      if (!mounted) return;
      final l10n = context.l10n;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(count == 0
              ? l10n.locationPollNudgeNoneSnack
              : l10n.locationPollNudgeSentSnack(count)),
        ),
      );
    });
  }

  /// Presents the owner overflow menu (see [showTimeManageMenu]) and applies
  /// whichever action came back.
  Future<void> _openManageMenu(TimeModalData data) async {
    final action = await showTimeManageMenu(context, data);
    if (action == null || !mounted) return;
    final notifier = ref.read(timeModalProvider(_experienceId).notifier);
    switch (action) {
      case TimeManageAction.addOption:
        _pickTime(setSingle: false);
      case TimeManageAction.setFinal:
        _enterSetFinal(data);
      case TimeManageAction.toggleLock:
        await _guard(() => notifier.lockTimeProposals(!data.proposalsLocked));
      case TimeManageAction.nudge:
        await _nudge();
      case TimeManageAction.cancelPoll:
        final confirmed = await confirmCancelTimePoll(context);
        if (confirmed ?? false) await _guard(notifier.endTimePoll);
      case TimeManageAction.markCompleted:
        widget.onMarkCompleted?.call();
      case TimeManageAction.closeEvent:
        // Closing the event tears down the content view it's pushed over, so
        // pop this panel first, then run the close flow on the content view.
        Navigator.of(context).pop();
        widget.onCloseEvent?.call();
    }
  }

  Future<void> _guard(Future<void> Function() op) async {
    setState(() => _busy = true);
    try {
      await op();
    } catch (_) {
      _showError();
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  void _showError() {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(content: Text(context.l10n.commonError)),
    );
  }
}
