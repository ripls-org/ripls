import 'package:fixnum/fixnum.dart' show Int64;
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:intl/intl.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/calendar_helper.dart';
import 'package:ripls/data/gen/ripls/api/time.pb.dart'
    show ExperienceTime, SpecificTime, TimeProposal, TimeVoteStatus;
import 'package:ripls/presentation/providers/user_timezone_provider.dart';
import 'package:ripls/presentation/screens/experience/time_poll_propose_modal.dart';
import 'package:ripls/presentation/viewmodels/date_time_picker_state.dart';
import 'package:ripls/presentation/viewmodels/experience_view_model.dart';
import 'package:ripls/presentation/viewmodels/time_modal_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/modal/glass/date_time_picker_modal.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass.dart';
import 'package:ripls/presentation/widgets/poll/poll_manage_menu_sheet.dart';
import 'package:ripls/presentation/widgets/web/web_unsupported.dart';

/// Read-only "It's a plan." view shown once a time has been locked in.
///
/// Surfaces the winning time on a sage-tinted card with an "N of M chose this
/// time" footer, a faint pill row of the non-winning proposals for context,
/// and an "Add to calendar" primary CTA. Owners also get a "Change" secondary
/// action that opens a menu to either swap the confirmed time directly or
/// open a fresh poll. Mirrors [LocationPollFinalizedModal].
class TimePollFinalizedModal extends ConsumerWidget {
  const TimePollFinalizedModal({
    super.key,
    required this.experienceId,
    this.embedded = false,
  });

  final String experienceId;

  /// When true, the body is rendered without its own [GlassSheet] chrome so
  /// it can be hosted inside the morphing [TimePollSheet].
  final bool embedded;

  static Future<void> show(BuildContext context, String experienceId) async {
    await showAccessibleModal(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (_) => TimePollFinalizedModal(experienceId: experienceId),
    );
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = context.l10n;
    final dataAsync = ref.watch(timeModalProvider(experienceId));
    final content = SafeArea(
      top: false,
      child: ConstrainedBox(
        constraints: BoxConstraints(
          maxHeight: MediaQuery.of(context).size.height * 0.85,
        ),
        child: dataAsync.when(
          data: (data) => _buildLoaded(context, ref, data),
          loading: () => const Padding(
            padding: EdgeInsets.all(48),
            child: Center(child: CircularProgressIndicator()),
          ),
          error: (e, _) => Padding(
            padding: const EdgeInsets.all(32),
            child: Center(
              child: Text(
                l10n.commonError,
                style: TextStyle(color: AppColors.modalTextPrimary),
              ),
            ),
          ),
        ),
      ),
    );
    if (embedded) return content;
    return GlassSheet(padding: EdgeInsets.zero, child: content);
  }

  Widget _buildLoaded(
      BuildContext context, WidgetRef ref, TimeModalData data) {
    final l10n = context.l10n;

    // Resolve the winning proposal. Prefer the confirmed proposal (lockedId);
    // fall back to whichever proposal matches the experience's event time.
    final winner = _resolveWinner(data);

    final others = _dedupeOthers(data.proposals, winner, data.eventTime);

    final winnerYes = winner == null
        ? 0
        : winner.votes
            .where((v) => v.status == TimeVoteStatus.TIME_VOTE_STATUS_YES)
            .length;
    final totalVoters = data.proposals
        .expand((p) => p.votes
            .where((v) => v.status == TimeVoteStatus.TIME_VOTE_STATUS_YES)
            .map((v) => v.user.id))
        .toSet()
        .length;

    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(20, 16, 20, 0),
          child: Row(
            children: [
              Text(
                l10n.timePollFinalizedKicker,
                style: TextStyle(
                  fontSize: 11,
                  fontWeight: FontWeight.w700,
                  color: AppColors.lightAccent,
                  letterSpacing: 0.8,
                ),
              ),
              const Spacer(),
              Tappable(
                semanticsLabel: l10n.a11yClose,
                onTap: () => Navigator.of(context).pop(),
                child: Icon(
                  Icons.close,
                  size: 22,
                  color: AppColors.modalTextSecondary,
                ),
              ),
            ],
          ),
        ),
        Padding(
          padding: const EdgeInsets.fromLTRB(20, 8, 20, 0),
          child: Text(
            l10n.timePollFinalizedTitle,
            style: const TextStyle(
              fontSize: 22,
              fontWeight: FontWeight.w800,
              color: AppColors.modalTextPrimary,
              height: 1.2,
            ),
          ),
        ),
        Flexible(
          child: SingleChildScrollView(
            padding: const EdgeInsets.fromLTRB(20, 16, 20, 12),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                // Render the sage-tinted card whenever there is a confirmed
                // time, even if the time was set directly by the owner with
                // no underlying proposal. Falling back to [data.eventTime]
                // matches the location flow, which renders its Final Spot
                // card from the experience's `locationId` regardless of
                // whether any proposal exists.
                if (winner != null)
                  _WinnerCard.fromProposal(
                    proposal: winner,
                    votedCount: winnerYes,
                    invitedCount: totalVoters,
                  )
                else if (data.eventTime != null &&
                    data.eventTime!.hasSpecific())
                  _WinnerCard.fromEventTime(time: data.eventTime!),
                if (others.isNotEmpty) ...[
                  const SizedBox(height: 18),
                  Text(
                    l10n.timePollFinalizedOtherTimes,
                    style: TextStyle(
                      fontSize: 11,
                      fontWeight: FontWeight.w700,
                      color: AppColors.modalTextMuted,
                      letterSpacing: 0.6,
                    ),
                  ),
                  const SizedBox(height: 8),
                  _OtherTimesPills(proposals: others),
                ],
              ],
            ),
          ),
        ),
        Padding(
          padding: const EdgeInsets.fromLTRB(20, 8, 20, 16),
          child: Row(
            children: [
              if (data.isOrganizer) ...[
                Expanded(
                  child: Tappable(
                    semanticsLabel: l10n.timePollFinalizedChange,
                    onTap: () => _openChangeMenu(context, ref),
                    child: _PillButton(
                      label: l10n.timePollFinalizedChange,
                      ghost: true,
                    ),
                  ),
                ),
                const SizedBox(width: 10),
              ],
              Expanded(
                flex: data.isOrganizer ? 2 : 1,
                child: Tappable(
                  semanticsLabel: l10n.timePollFinalizedAddToCalendar,
                  onTap: () => _exportToCalendar(context, ref),
                  child: _PillButton(
                    label: l10n.timePollFinalizedAddToCalendar,
                  ),
                ),
              ),
            ],
          ),
        ),
      ],
    );
  }

  /// Returns the non-winning proposals, deduped by the time they point at
  /// AND filtered to exclude any proposal that resolves to the same instant
  /// as the winner. Two participants proposing the same Friday 6 PM —
  /// once via the calendar picker, once via the LLM hatch — would
  /// otherwise each render as their own pill, and a duplicate-of-the-winner
  /// proposal would show up under "Other Times" even though it's the time
  /// the group landed on. The experience's confirmed [eventTime] is also
  /// seeded into the seen set so proposals matching the winning instant
  /// are excluded.
  ///
  /// Dedup key uses the proposal's specific unix-second timestamp (or the
  /// range start). Proposals with no usable instant fall back to their
  /// proposal id so multiple unkeyable rows each render once.
  List<TimeProposal> _dedupeOthers(
    List<TimeProposal> all,
    TimeProposal? winner,
    ExperienceTime? eventTime,
  ) {
    final seen = <String>{};
    if (winner != null) {
      final winnerKey = _dedupeKey(winner);
      if (winnerKey.isNotEmpty) seen.add(winnerKey);
    }
    if (eventTime != null) {
      if (eventTime.hasSpecific()) {
        seen.add('ts:${eventTime.specific.unixTimestampSec}');
      } else if (eventTime.hasRange()) {
        seen.add('ts:${eventTime.range.startUnixSec}');
      }
    }
    final out = <TimeProposal>[];
    for (final p in all) {
      if (winner != null && p.id == winner.id) continue;
      final key = _dedupeKey(p);
      if (key.isEmpty) {
        if (!seen.add('id:${p.id}')) continue;
        out.add(p);
        continue;
      }
      if (!seen.add(key)) continue;
      out.add(p);
    }
    return out;
  }

  String _dedupeKey(TimeProposal p) {
    if (p.time.hasSpecific()) return 'ts:${p.time.specific.unixTimestampSec}';
    if (p.time.hasRange()) return 'ts:${p.time.range.startUnixSec}';
    return '';
  }

  TimeProposal? _resolveWinner(TimeModalData data) {
    // The lockedProposal getter from time_modal_state.dart already returns
    // the proposal whose isConfirmed flag is set — that's the canonical
    // winner once a time is confirmed.
    final locked = data.lockedProposal;
    if (locked != null) return locked;
    // Fall back to whichever proposal matches the experience's event time.
    final eventTime = data.eventTime;
    if (eventTime == null || !eventTime.hasSpecific()) return null;
    final target = eventTime.specific.unixTimestampSec;
    for (final p in data.proposals) {
      if (p.time.hasSpecific() && p.time.specific.unixTimestampSec == target) {
        return p;
      }
    }
    return null;
  }

  Future<void> _openChangeMenu(BuildContext context, WidgetRef ref) async {
    final l10n = context.l10n;
    final result = await showAccessibleModal<_ChangeAction>(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (_) => PollManageMenuSheet<_ChangeAction>(
        items: [
          PollManageMenuItem(
            icon: Icons.swap_horiz,
            label: l10n.timePollFinalizedChangeMenuSwap,
            description: l10n.timePollFinalizedChangeMenuSwapDesc,
            action: _ChangeAction.swap,
          ),
          PollManageMenuItem(
            icon: Icons.how_to_vote_outlined,
            label: l10n.timePollFinalizedChangeMenuFreshPoll,
            description: l10n.timePollFinalizedChangeMenuFreshPollDesc,
            action: _ChangeAction.newPoll,
          ),
        ],
      ),
    );
    if (result == null || !context.mounted) return;
    switch (result) {
      case _ChangeAction.swap:
        // Single-time replacement via the unified calendar+time picker.
        // Mirrors location's "Pick a different spot" → LocationPickerModal
        // flow; no new poll is opened.
        await _pickReplacementTime(context, ref);
        break;
      case _ChangeAction.newPoll:
        // Standalone: pop the finalized sheet, then open the propose modal.
        // Embedded: keep the morphing sheet open and let the propose modal
        // sit on top; once it opens a poll the sheet morphs underneath.
        if (!embedded) Navigator.of(context).pop();
        if (!context.mounted) return;
        await TimePollProposeModal.show(context, experienceId);
        break;
    }
  }

  Future<void> _pickReplacementTime(
      BuildContext context, WidgetRef ref) async {
    final data = ref.read(timeModalProvider(experienceId)).value;
    final currentTime = data?.eventTime;
    final now = DateTime.now();
    DateTime initial;
    if (currentTime != null && currentTime.hasSpecific()) {
      initial = DateTime.fromMillisecondsSinceEpoch(
        currentTime.specific.unixTimestampSec.toInt() * 1000,
      );
      if (initial.isBefore(now)) {
        initial = DateTime(now.year, now.month, now.day + 1, 18);
      }
    } else {
      initial = DateTime(now.year, now.month, now.day + 1, 18);
    }
    final result = await DateTimePickerModal.show(
      context,
      initialDateTime: initial,
      firstDate: now,
      lastDate: now.add(const Duration(days: 365)),
    );
    if (result is! DateTimePickerResultSaved || !context.mounted) return;
    final picked = result.value;
    final durationMinutes = currentTime != null && currentTime.hasSpecific()
        ? currentTime.specific.durationMinutes
        : 60;
    final timezone = await ref.read(resolvedTimezoneProvider.future);
    if (!context.mounted) return;
    final newTime = ExperienceTime(
      specific: SpecificTime(
        unixTimestampSec: Int64(picked.millisecondsSinceEpoch ~/ 1000),
        timezone: timezone,
        durationMinutes: durationMinutes,
      ),
    );
    try {
      await ref
          .read(experienceProvider(experienceId).notifier)
          .updateTime(newTime);
      if (!context.mounted) return;
      // Standalone: pop so the next chip tap re-routes and lands on a fresh
      // "It's Set" card. Embedded: keep the morphing sheet open — it
      // re-renders the finalized body with the new time automatically.
      if (!embedded) Navigator.of(context).pop();
    } catch (_) {
      if (!context.mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(context.l10n.commonError)),
      );
    }
  }

  Future<void> _exportToCalendar(BuildContext context, WidgetRef ref) async {
    // add_2_calendar has no web implementation; show a notice pointing
    // the visitor at the mobile app. See #2157 plugin audit.
    if (WebUnsupported.showCalendarNotice(context)) return;
    try {
      final result = await ref
          .read(experienceProvider(experienceId).notifier)
          .exportToCalendar();
      if (result == null) {
        if (!context.mounted) return;
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text(context.l10n.timeExportFailed)),
        );
        return;
      }
      // Widget async exception: platform calendar UI.
      final success = await CalendarHelper.addToCalendar(
        result.experience,
        riplsUrl: result.riplsUrl,
        locationDisplay: result.locationDisplay,
      );
      if (!context.mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(
            success
                ? context.l10n.timeExportOpeningCalendar
                : context.l10n.timeExportFailed,
          ),
        ),
      );
    } catch (_) {
      if (!context.mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(context.l10n.timeExportFailed)),
      );
    }
  }
}

enum _ChangeAction { swap, newPoll }

/// Sage-tinted "It's Set" card. Renders either from a winning poll proposal
/// (with a "N of M chose this time" footer) or from the experience's raw
/// `eventTime` when the time was set directly by the owner — in that path
/// there are no proposals and no vote count, so the footer is hidden.
class _WinnerCard extends StatelessWidget {
  const _WinnerCard._({
    required this.time,
    required this.votedCount,
    required this.invitedCount,
  });

  /// Card backed by a confirmed poll proposal — shows the YES vote footer.
  factory _WinnerCard.fromProposal({
    required TimeProposal proposal,
    required int votedCount,
    required int invitedCount,
  }) =>
      _WinnerCard._(
        time: proposal.time,
        votedCount: votedCount,
        invitedCount: invitedCount,
      );

  /// Card backed by the experience's `eventTime` directly — used when the
  /// owner set the time without going through a poll. No footer renders.
  factory _WinnerCard.fromEventTime({required ExperienceTime time}) =>
      _WinnerCard._(time: time, votedCount: 0, invitedCount: 0);

  final ExperienceTime time;
  final int votedCount;
  final int invitedCount;

  @override
  Widget build(BuildContext context) {
    final sage = AppColors.lightAccent;
    final dateLabel = time.hasSpecific()
        ? DateFormat('EEEE, MMM d').format(
            DateTime.fromMillisecondsSinceEpoch(
              time.specific.unixTimestampSec.toInt() * 1000,
            ),
          )
        : 'TBD';
    final timeLabel = time.hasSpecific()
        ? DateFormat('h:mm a').format(
            DateTime.fromMillisecondsSinceEpoch(
              time.specific.unixTimestampSec.toInt() * 1000,
            ),
          )
        : '';
    return Container(
      padding: const EdgeInsets.all(18),
      decoration: BoxDecoration(
        color: sage.withValues(alpha: 0.18),
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: sage, width: 1.5),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            dateLabel,
            style: TextStyle(
              fontSize: 18,
              fontWeight: FontWeight.w700,
              color: AppColors.modalTextPrimary,
            ),
          ),
          if (timeLabel.isNotEmpty) ...[
            const SizedBox(height: 4),
            Text(
              timeLabel,
              style: TextStyle(
                fontSize: 16,
                fontWeight: FontWeight.w500,
                color: AppColors.modalTextSecondary,
              ),
            ),
          ],
          if (invitedCount > 0) ...[
            const SizedBox(height: 10),
            Text(
              context.l10n.timePollFinalizedFooter(votedCount, invitedCount),
              style: TextStyle(
                fontSize: 12,
                fontWeight: FontWeight.w600,
                color: sage,
              ),
            ),
          ],
        ],
      ),
    );
  }
}

class _OtherTimesPills extends StatelessWidget {
  const _OtherTimesPills({required this.proposals});
  final List<TimeProposal> proposals;

  @override
  Widget build(BuildContext context) {
    return Wrap(
      spacing: 8,
      runSpacing: 8,
      children: [
        for (final p in proposals)
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
            decoration: BoxDecoration(
              color: AppColors.modalInsetCardBg,
              borderRadius: BorderRadius.circular(20),
              border: Border.all(
                color: AppColors.modalInsetCardBorder,
                width: 1,
              ),
            ),
            child: Text(
              _formatPill(p),
              style: TextStyle(
                fontSize: 13,
                color: AppColors.modalTextSecondary,
              ),
            ),
          ),
      ],
    );
  }

  String _formatPill(TimeProposal p) {
    if (!p.time.hasSpecific()) return 'TBD';
    final dt = DateTime.fromMillisecondsSinceEpoch(
      p.time.specific.unixTimestampSec.toInt() * 1000,
    );
    return DateFormat('EEE MMM d, h:mm a').format(dt);
  }
}

class _PillButton extends StatelessWidget {
  const _PillButton({required this.label, this.ghost = false});
  final String label;
  final bool ghost;

  @override
  Widget build(BuildContext context) {
    return Container(
      height: 54,
      decoration: BoxDecoration(
        color: ghost
            ? AppColors.modalInsetCardBg
            : AppColors.lightAccent,
        borderRadius: BorderRadius.circular(16),
        border: ghost
            ? Border.all(color: AppColors.modalInsetCardBorder, width: 1)
            : null,
      ),
      alignment: Alignment.center,
      child: Text(
        label,
        style: TextStyle(
          fontSize: 15,
          fontWeight: FontWeight.w700,
          color: ghost ? AppColors.modalTextPrimary : Colors.white,
        ),
      ),
    );
  }
}
