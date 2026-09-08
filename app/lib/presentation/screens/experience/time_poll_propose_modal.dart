import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:intl/intl.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/data/gen/ripls/api/time.pb.dart' show TimeProposal;
import 'package:ripls/presentation/viewmodels/date_time_picker_state.dart';
import 'package:ripls/presentation/viewmodels/experience_view_model.dart';
import 'package:ripls/presentation/viewmodels/time_modal_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/modal/glass/date_time_picker_modal.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass.dart';
import 'package:ripls/presentation/widgets/poll/poll_tbd_card.dart';

part 'time_poll_propose_modal_atoms.dart';

final _log = Logger('TimePollProposeModal');

/// Propose modal — mirrors the location-poll "Pick a Spot" flow at
/// [LocationPollProposeModal]:
///   - `empty`: "PICK A TIME" eyebrow + "When should we meet?" + a big
///     sage-circled "Add your first time" card and an LLM-paste textarea.
///   - `one`: same eyebrow + "Lock this in?" with the staged row + a
///     compact "Add another time" affordance. Primary CTA "Set the time".
///   - `many`: "Looks good." / "We'll ask the group..." with all staged
///     rows. Primary CTA "Ask the group".
///
/// When opened against an experience with a confirmed event time and no
/// active poll, the modal pre-seeds the staged list with that time so the
/// "Change → Add times and ask the group" path lands with the existing
/// time pre-staged rather than an empty state. When the poll is already
/// active ("Edit Choices" path), existing proposals are rendered live from
/// server state and each add fires immediately as a new proposal.
class TimePollProposeModal extends ConsumerStatefulWidget {
  const TimePollProposeModal({
    super.key,
    required this.experienceId,
    this.embedded = false,
  });

  final String experienceId;

  /// When true, the body renders without its own [GlassSheet] chrome and
  /// suppresses self-dismiss on poll/time creation so the morphing
  /// [TimePollSheet] can host it and morph in place as state changes.
  final bool embedded;

  static Future<void> show(BuildContext context, String experienceId) async {
    await showAccessibleModal(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (_) => TimePollProposeModal(experienceId: experienceId),
    );
  }

  @override
  ConsumerState<TimePollProposeModal> createState() =>
      _TimePollProposeModalState();
}

class _TimePollProposeModalState extends ConsumerState<TimePollProposeModal> {
  final List<DateTime> _stagedTimes = [];
  bool _isSaving = false;
  bool _seeded = false;

  String get _experienceId => widget.experienceId;

  @override
  void initState() {
    super.initState();
    // Pre-seed once with the experience's currently-confirmed time so a
    // "Change → Add times and ask the group" entry lands with the existing
    // time pre-staged rather than an empty card. Skipped when a poll is
    // already running — that path renders existing server proposals
    // straight from the modal state.
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!mounted || _seeded) return;
      _seeded = true;
      final exp = ref
          .read(experienceProvider(_experienceId))
          .experienceDetails
          ?.experience;
      if (exp == null) return;
      if (exp.timePollActive) return;
      if (!exp.hasTime() || !exp.time.hasSpecific()) return;
      final dt = DateTime.fromMillisecondsSinceEpoch(
        exp.time.specific.unixTimestampSec.toInt() * 1000,
      );
      setState(() => _stagedTimes.add(dt));
    });
  }


  bool _isLivePollEdit() =>
      ref.read(timeModalProvider(_experienceId)).value?.timePollActive ?? false;

  /// Returns true when the staged list already contains a time matching
  /// [dt] at the same minute — used to skip duplicates on add.
  bool _isStagedDuplicate(DateTime dt) {
    return _stagedTimes.any((s) =>
        s.year == dt.year &&
        s.month == dt.month &&
        s.day == dt.day &&
        s.hour == dt.hour &&
        s.minute == dt.minute);
  }

  Future<void> _addViaPicker() async {
    final now = DateTime.now();
    // Default: 1 day from now at 6pm so the picker isn't on "now".
    final defaultStart = DateTime(now.year, now.month, now.day + 1, 18);
    final result = await DateTimePickerModal.show(
      context,
      initialDateTime: defaultStart,
      firstDate: now,
      lastDate: now.add(const Duration(days: 365)),
    );
    if (result == null || !mounted) return;
    if (result is! DateTimePickerResultSaved) return;
    final picked = result.value;

    if (_isLivePollEdit()) {
      // Active poll → propose immediately, no staged copy.
      setState(() => _isSaving = true);
      try {
        await ref
            .read(timeModalProvider(_experienceId).notifier)
            .addTimesToActivePoll([
          (
            DateTime(picked.year, picked.month, picked.day),
            TimeOfDay(hour: picked.hour, minute: picked.minute),
          ),
        ]);
      } catch (e) {
        _log.warning('Add to live poll failed', e);
        if (!mounted) return;
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text(context.l10n.timePollProposeError)),
        );
      } finally {
        if (mounted) setState(() => _isSaving = false);
      }
      return;
    }

    if (_isStagedDuplicate(picked)) return;
    setState(() => _stagedTimes.add(picked));
  }

  Future<void> _deleteExistingProposal(String proposalId) async {
    try {
      await ref
          .read(timeModalProvider(_experienceId).notifier)
          .deleteTimeProposal(proposalId);
    } catch (e) {
      _log.warning('Delete time proposal failed', e);
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(context.l10n.commonError)),
      );
    }
  }

  Future<void> _submit() async {
    if (_stagedTimes.isEmpty || _isSaving) return;
    setState(() => _isSaving = true);
    try {
      final notifier = ref.read(timeModalProvider(_experienceId).notifier);
      // Single staged candidate: set it directly as the event time. The
      // singular "Set the time" CTA conveys a direct-set contract, not a
      // one-option poll.
      if (_stagedTimes.length == 1) {
        await notifier.setSingleTime(_stagedTimes.first);
      } else {
        final options = [
          for (final dt in _stagedTimes)
            (
              DateTime(dt.year, dt.month, dt.day),
              TimeOfDay(hour: dt.hour, minute: dt.minute),
            ),
        ];
        await notifier.createTimePoll(options);
      }
      // Wait for the provider to settle before popping so callers that
      // don't keep the provider alive don't race against AsyncLoading.
      try {
        await ref.read(timeModalProvider(_experienceId).future);
      } catch (_) {
        // Poll was created; ignore refresh failure on dispose.
      }
      if (!mounted) return;
      // Embedded inside the morphing sheet: stay open and morph to the
      // poll/set view. Standalone: dismiss.
      if (!widget.embedded) Navigator.of(context).pop();
    } catch (e) {
      _log.warning('Failed to submit time proposals', e);
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(context.l10n.timePollProposeError)),
      );
    } finally {
      if (mounted) setState(() => _isSaving = false);
    }
  }

  /// Empty-state "Keep it TBD for now" CTA — a no-op dismiss. Friends can
  /// RSVP against a TBD time and the owner sets it later. Mirrors the
  /// location flow.
  void _keepTbd() => Navigator.of(context).pop();

  /// Floats the staged times to the group as a poll instead of setting them
  /// directly. Used by the "or float it to the group" link in the
  /// single-staged-time state, where the primary CTA does a direct set.
  Future<void> _floatToGroup() async {
    if (_stagedTimes.isEmpty || _isSaving) return;
    setState(() => _isSaving = true);
    try {
      final notifier = ref.read(timeModalProvider(_experienceId).notifier);
      final options = [
        for (final dt in _stagedTimes)
          (
            DateTime(dt.year, dt.month, dt.day),
            TimeOfDay(hour: dt.hour, minute: dt.minute),
          ),
      ];
      await notifier.createTimePoll(options);
      try {
        await ref.read(timeModalProvider(_experienceId).future);
      } catch (_) {
        // Poll created; ignore refresh failure on dispose.
      }
      if (!mounted) return;
      // Embedded inside the morphing sheet: stay open and let the sheet
      // morph to the poll view. Standalone: dismiss.
      if (!widget.embedded) Navigator.of(context).pop();
    } catch (e) {
      _log.warning('Failed to float time proposals', e);
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(context.l10n.timePollProposeError)),
      );
    } finally {
      if (mounted) setState(() => _isSaving = false);
    }
  }

  String _titleForState(BuildContext context, int count) {
    final l10n = context.l10n;
    if (count == 0) return l10n.timePollProposeTitle;
    if (count == 1) return l10n.timePollProposeLockTitle;
    return l10n.timePollProposeReadyTitle;
  }

  String _subtitleForState(BuildContext context, int count) {
    final l10n = context.l10n;
    if (count == 0) return l10n.timePollProposeSubtitle;
    if (count == 1) return l10n.timePollProposeLockSubtitle;
    return l10n.timePollProposeReadySubtitle;
  }

  String _ctaLabel(BuildContext context, int count) {
    final l10n = context.l10n;
    if (count <= 1) return l10n.timePollProposeSetTime;
    return l10n.timePollProposeAskGroup;
  }

  Future<void> _pickDeadline(int? currentUnixSec) async {
    final now = DateTime.now();
    final hasExisting = currentUnixSec != null && currentUnixSec > 0;
    final initial = hasExisting
        ? DateTime.fromMillisecondsSinceEpoch(currentUnixSec * 1000)
        : now.add(const Duration(days: 2));
    final result = await DateTimePickerModal.show(
      context,
      initialDateTime: initial.isBefore(now) ? now : initial,
      firstDate: now,
      lastDate: now.add(const Duration(days: 365)),
      allowRemove: hasExisting,
    );
    if (result == null || !mounted) return;
    final notifier = ref.read(timeModalProvider(_experienceId).notifier);
    switch (result) {
      case DateTimePickerResultSaved(:final value):
        try {
          await notifier.setTimePollDeadline(value.millisecondsSinceEpoch ~/ 1000);
        } catch (e) {
          _log.warning('Set time-poll deadline failed', e);
          if (!mounted) return;
          ScaffoldMessenger.of(context).showSnackBar(
            SnackBar(content: Text(context.l10n.commonError)),
          );
        }
      case DateTimePickerResultRemoved():
        try {
          await notifier.setTimePollDeadline(0);
        } catch (_) {/* non-fatal */}
    }
  }

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    final dataAsync = ref.watch(timeModalProvider(_experienceId));
    final data = dataAsync.value;

    // Existing proposals scoped to the active poll. Only surface them when
    // a poll is actually live — after a cancel the experience is back to
    // "no poll" with proposals preserved for history, so a fresh propose
    // flow shouldn't repopulate from the old poll.
    final existingProposals = <TimeProposal>[];
    if (data != null && data.timePollActive) {
      final activePollId = data.currentPollId ?? '';
      for (final p in data.proposals) {
        if (activePollId.isEmpty ||
            (p.hasPollId() && p.pollId == activePollId)) {
          existingProposals.add(p);
        }
      }
    }
    final stagedCount = _stagedTimes.length;
    final visibleCount = existingProposals.length + stagedCount;
    final isLivePollEdit = data?.timePollActive ?? false;
    // The "Ask the Group" CTA only makes sense for the fresh-poll flow;
    // live-edit mode fires every add immediately so there is nothing to
    // submit.
    final showSubmitButton = !isLivePollEdit;
    final canSubmit = showSubmitButton && stagedCount > 0 && !_isSaving;
    final serverDeadlineSec = data?.pollDeadlineUnixSec;
    // Show a 24h-from-now display default in fresh-poll mode until the
    // server seeds the real deadline on first proposal. Matches the
    // server's default-deadline behavior (see [defaultTimePollDeadline]).
    final displayDeadlineSec = serverDeadlineSec ??
        (DateTime.now().add(const Duration(hours: 24)).millisecondsSinceEpoch ~/
            1000);

    final content = SafeArea(
      top: false,
      child: ConstrainedBox(
        constraints: BoxConstraints(
          maxHeight: MediaQuery.of(context).size.height * 0.85,
        ),
        child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Padding(
                padding: const EdgeInsets.fromLTRB(20, 4, 12, 0),
                child: Row(
                  children: [
                    Expanded(child: _Eyebrow(text: l10n.timePollProposeKicker)),
                    IconAction(
                      icon: Icons.close,
                      tooltip: l10n.a11yClose,
                      semanticsLabel: l10n.a11yClose,
                      onPressed: () => Navigator.of(context).pop(),
                    ),
                  ],
                ),
              ),
              Padding(
                padding: const EdgeInsets.fromLTRB(20, 4, 20, 0),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    _Title(text: _titleForState(context, visibleCount)),
                    const SizedBox(height: 6),
                    _Subtitle(text: _subtitleForState(context, visibleCount)),
                  ],
                ),
              ),
              Flexible(
                child: SingleChildScrollView(
                  padding: const EdgeInsets.fromLTRB(20, 18, 20, 12),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      ...existingProposals.asMap().entries.map((entry) {
                        final idx = entry.key;
                        final p = entry.value;
                        return Padding(
                          padding: const EdgeInsets.only(bottom: 9),
                          child: _ExistingProposalRow(
                            n: idx + 1,
                            proposal: p,
                            onRemove: () => _deleteExistingProposal(p.id),
                          ),
                        );
                      }),
                      ..._stagedTimes.asMap().entries.map((entry) {
                        final idx = entry.key;
                        final dt = entry.value;
                        return Padding(
                          padding: const EdgeInsets.only(bottom: 9),
                          child: _StagedTimeRow(
                            n: existingProposals.length + idx + 1,
                            dateTime: dt,
                            onRemove: () => setState(() =>
                                _stagedTimes.remove(dt)),
                          ),
                        );
                      }),
                      if (visibleCount == 0) ...[
                        PollTbdCard(
                          icon: Icons.schedule_outlined,
                          title: l10n.timePollProposeTbdCardTitle,
                          subtitle: l10n.timePollProposeTbdCardSubtitle,
                        ),
                        const SizedBox(height: 14),
                      ],
                      _AddTimeCard(
                        label: visibleCount == 0
                            ? l10n.timePollProposeAddTime
                            : l10n.timePollProposeAddAnother,
                        onTap: _isSaving ? null : _addViaPicker,
                      ),
                      if (visibleCount >= 1) ...[
                        const SizedBox(height: 10),
                        _DeadlineStrip(
                          deadlineUnixSec: displayDeadlineSec,
                          onChange: () => _pickDeadline(displayDeadlineSec),
                        ),
                      ],
                    ],
                  ),
                ),
              ),
              if (showSubmitButton)
                Padding(
                  padding: const EdgeInsets.fromLTRB(20, 4, 20, 24),
                  child: visibleCount == 0
                      // TBD-first: an empty poll commits to nothing. "Keep it
                      // TBD for now" dismisses; friends can still RSVP.
                      ? _PrimaryButton(
                          label: l10n.timePollProposeKeepTbd,
                          enabled: true,
                          onTap: _keepTbd,
                        )
                      : Column(
                          mainAxisSize: MainAxisSize.min,
                          children: [
                            _PrimaryButton(
                              label: _ctaLabel(context, visibleCount),
                              enabled: canSubmit,
                              onTap: canSubmit ? _submit : null,
                            ),
                            // A single staged time sets directly by default;
                            // offer floating it to the group as the softer
                            // alternative (matches the prototype).
                            if (visibleCount == 1)
                              Padding(
                                padding: const EdgeInsets.only(top: 4),
                                child: Tappable(
                                  semanticsLabel: l10n.timePollProposeFloatLink,
                                  onTap: _isSaving ? null : _floatToGroup,
                                  child: Padding(
                                    padding: const EdgeInsets.all(8),
                                    child: Text(
                                      l10n.timePollProposeFloatLink,
                                      textAlign: TextAlign.center,
                                      style: TextStyle(
                                        color: AppColors.modalTextSecondary,
                                        fontSize: 13,
                                        fontWeight: FontWeight.w600,
                                      ),
                                    ),
                                  ),
                                ),
                              ),
                          ],
                        ),
                )
              else
                const SizedBox(height: 24),
            ],
          ),
        ),
      );
    if (widget.embedded) return content;
    return GlassSheet(padding: EdgeInsets.zero, child: content);
  }
}

