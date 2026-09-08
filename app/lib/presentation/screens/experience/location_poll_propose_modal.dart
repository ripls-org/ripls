import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:intl/intl.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/data/gen/ripls/api/location.pb.dart' as locapi;
import 'package:ripls/data/gen/ripls/api/location.pb.dart' show LocationProposal;
import 'package:ripls/presentation/viewmodels/date_time_picker_state.dart';
import 'package:ripls/presentation/viewmodels/experience_view_model.dart';
import 'package:ripls/presentation/viewmodels/location_modal_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/location/location_picker_modal.dart';
import 'package:ripls/presentation/widgets/modal/glass/date_time_picker_modal.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass.dart';
import 'package:ripls/presentation/widgets/poll/poll_tbd_card.dart';
import 'package:ripls/services/providers.dart';

part 'location_poll_propose_modal_atoms.dart';

final _log = Logger('LocationPollProposeModal');

/// Propose modal — matches `docs/cowork/App Design/location-redesign.html`
/// LP1 / LP2 / LP3:
///   - `empty`: "PICK A SPOT" eyebrow + "Where should we meet?" with a
///     big sage-circled "Add your first spot" card.
///   - `one`: same eyebrow + "Lock this in?" with the staged row + a
///     compact "Add another spot" affordance. Primary CTA "Set the spot".
///   - `many`: "Looks good." / "We'll ask the group which they can make."
///     with all staged rows. Primary CTA "Ask the group".
class LocationPollProposeModal extends ConsumerStatefulWidget {
  const LocationPollProposeModal({super.key, required this.experienceId});

  final String experienceId;

  static Future<void> show(BuildContext context, String experienceId) async {
    await showAccessibleModal(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (_) =>
          LocationPollProposeModal(experienceId: experienceId),
    );
  }

  @override
  ConsumerState<LocationPollProposeModal> createState() =>
      _LocationPollProposeModalState();
}

class _LocationPollProposeModalState
    extends ConsumerState<LocationPollProposeModal> {
  final List<String> _stagedLocationIds = [];
  final List<locapi.GeocodedLocation> _stagedGeocoded = [];
  bool _isSaving = false;

  String get _experienceId => widget.experienceId;

  @override
  void initState() {
    super.initState();
    final exp =
        ref.read(experienceProvider(_experienceId)).experienceDetails?.experience;
    if (exp == null) return;
    // For "Edit Choices" — an active poll already has proposals; they are
    // rendered straight from server state with their own delete handler,
    // so don't pre-seed anything locally.
    if (exp.locationPollActive) return;
    // Otherwise seed the staged list with the experience's currently-confirmed
    // location so an owner opening "Change → Add spots and ask the group"
    // from the Finalized modal lands with the existing spot pre-staged
    // rather than an empty "Add your first spot" card. New experiences
    // (no confirmed location) still open empty.
    final existingId = exp.locationId;
    if (existingId.isNotEmpty) {
      _stagedLocationIds.add(existingId);
    }
  }

  Future<void> _deleteExistingProposal(String proposalId) async {
    try {
      await ref
          .read(locationModalProvider(_experienceId).notifier)
          .deleteProposal(proposalId);
    } catch (_) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(context.l10n.commonError)),
      );
    }
  }

  int get _totalStagedCount =>
      _stagedLocationIds.length + _stagedGeocoded.length;

  /// Whether the user is editing an already-running poll (the "Edit
  /// Choices" path opened from the Manage menu). In this mode every add /
  /// bulk-extract fires immediately as a proposal so there is nothing
  /// staged and no "Ask the Group" submit button — the existing list of
  /// proposals stays the canonical state.
  bool _isLivePollEdit() =>
      ref.read(locationModalProvider(_experienceId)).value?.locationPollActive ?? false;

  Future<void> _addCandidate() async {
    final id = await LocationPickerModal.show(context);
    if (!mounted) return;
    if (id == null || id.isEmpty) return;

    if (_isLivePollEdit()) {
      // Active poll → push to server immediately and let the modal
      // re-render against the refreshed proposal list. No staged copy.
      setState(() => _isSaving = true);
      try {
        await ref
            .read(locationModalProvider(_experienceId).notifier)
            .proposeSavedLocation(id);
      } catch (_) {
        if (!mounted) return;
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text(context.l10n.locationPollProposeError)),
        );
      } finally {
        if (mounted) setState(() => _isSaving = false);
      }
      return;
    }

    // Fresh poll → stage locally; "Ask the Group" sends the batch.
    if (!_stagedLocationIds.contains(id)) {
      setState(() => _stagedLocationIds.add(id));
    }
  }


  Future<void> _submit() async {
    if (_totalStagedCount == 0 || _isSaving) return;
    setState(() => _isSaving = true);
    try {
      final notifier =
          ref.read(locationModalProvider(_experienceId).notifier);
      // Single staged candidate: set it directly as the event location. The
      // singular "Set the spot" CTA conveys a direct-set contract, not a
      // one-option poll.
      if (_totalStagedCount == 1) {
        if (_stagedLocationIds.length == 1) {
          await notifier.setSingleLocation(
            locationId: _stagedLocationIds.first,
          );
        } else {
          await notifier.setSingleLocation(geocoded: _stagedGeocoded.first);
        }
      } else {
        if (_stagedLocationIds.isNotEmpty) {
          await notifier.proposeLocationsSequentially(_stagedLocationIds);
          if (!mounted) return;
        }
        if (_stagedGeocoded.isNotEmpty) {
          // Batch helper so a per-call `invalidateSelf` inside the loop
          // doesn't dispose the notifier mid-submit and drop later proposals
          // (e.g. "submitted 3 spots, poll shows 2").
          await notifier.proposeGeocodedLocationsSequentially(_stagedGeocoded);
        }
      }
      if (!mounted) return;
      Navigator.of(context).pop();
    } catch (e) {
      _log.warning('Failed to submit location proposals', e);
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(context.l10n.locationPollProposeError)),
      );
    } finally {
      if (mounted) setState(() => _isSaving = false);
    }
  }

  /// Empty-state "Keep it TBD for now" CTA. Per the TBD-first design this is
  /// a no-op dismiss — friends can RSVP against a TBD location and the owner
  /// drops the pin later. No poll or value is created.
  void _keepTbd() => Navigator.of(context).pop();

  /// Floats the staged candidates to the group as a poll instead of setting
  /// them directly. Used by the "or float it to the group" link in the
  /// single-staged-candidate state, where the primary CTA does a direct set.
  Future<void> _floatToGroup() async {
    if (_totalStagedCount == 0 || _isSaving) return;
    setState(() => _isSaving = true);
    try {
      final notifier =
          ref.read(locationModalProvider(_experienceId).notifier);
      if (_stagedLocationIds.isNotEmpty) {
        await notifier.proposeLocationsSequentially(_stagedLocationIds);
        if (!mounted) return;
      }
      if (_stagedGeocoded.isNotEmpty) {
        await notifier.proposeGeocodedLocationsSequentially(_stagedGeocoded);
      }
      if (!mounted) return;
      Navigator.of(context).pop();
    } catch (e) {
      _log.warning('Failed to float location proposals', e);
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(context.l10n.locationPollProposeError)),
      );
    } finally {
      if (mounted) setState(() => _isSaving = false);
    }
  }

  String _titleForState(BuildContext context, int count) {
    final l10n = context.l10n;
    if (count == 0) return l10n.locationPollProposeTitle;
    if (count == 1) return l10n.locationPollProposeLockTitle;
    return l10n.locationPollProposeReadyTitle;
  }

  String _subtitleForState(BuildContext context, int count) {
    final l10n = context.l10n;
    if (count == 0) return l10n.locationPollProposeSubtitle;
    if (count == 1) return l10n.locationPollProposeLockSubtitle;
    return l10n.locationPollProposeReadySubtitle;
  }

  String _ctaLabel(BuildContext context, int count) {
    final l10n = context.l10n;
    if (count <= 1) return l10n.locationPollProposeSetSpot;
    return l10n.locationPollProposeAskGroup;
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
    switch (result) {
      case DateTimePickerResultSaved(:final value):
        try {
          await ref
              .read(locationModalProvider(_experienceId).notifier)
              .setLocationPollDeadline(value.millisecondsSinceEpoch ~/ 1000);
        } catch (e) {
          _log.warning('Failed to set deadline', e);
          if (!mounted) return;
          ScaffoldMessenger.of(context).showSnackBar(
            SnackBar(content: Text(context.l10n.commonError)),
          );
        }
      case DateTimePickerResultRemoved():
        await _clearDeadline();
    }
  }

  Future<void> _clearDeadline() async {
    try {
      await ref
          .read(locationModalProvider(_experienceId).notifier)
          .setLocationPollDeadline(null);
    } catch (e) {
      _log.warning('Failed to clear deadline', e);
    }
  }

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    final dataAsync = ref.watch(locationModalProvider(_experienceId));
    final data = dataAsync.value;
    // Existing proposals scoped to the active poll, in their server order.
    // Only surface them when a poll is actually live — after a cancel the
    // experience is back to "no poll" with proposals preserved for history,
    // so a fresh propose flow shouldn't repopulate from the old poll.
    final existingProposals = <LocationProposal>[];
    if (data != null && data.locationPollActive) {
      final activePollId = data.currentLocationPollId ?? '';
      for (final p in data.proposals) {
        if (activePollId.isEmpty ||
            (p.hasPollId() && p.pollId == activePollId)) {
          existingProposals.add(p);
        }
      }
    }
    final visibleCount = existingProposals.length + _totalStagedCount;
    final isLivePollEdit = data?.locationPollActive ?? false;
    // The "Ask the Group" CTA only makes sense for the fresh-poll flow:
    // in live-edit mode every add / delete fires immediately so there is
    // nothing to submit, and a perpetually-disabled button reads as a bug.
    final showSubmitButton = !isLivePollEdit;
    final canSubmit =
        showSubmitButton && _totalStagedCount > 0 && !_isSaving;
    final serverDeadlineSec = dataAsync.maybeWhen(
      data: (d) => d.locationPollDeadlineUnixSec,
      orElse: () => null,
    );
    // Show a 24h-from-now default until the server seeds the real deadline
    // on first proposal. Matches the server's default-deadline behavior.
    final displayDeadlineSec = serverDeadlineSec ??
        (DateTime.now().add(const Duration(hours: 24)).millisecondsSinceEpoch ~/
            1000);
    return GlassSheet(
      padding: EdgeInsets.zero,
      child: SafeArea(
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
                    Expanded(
                      child: _Eyebrow(
                          text: l10n.locationPollProposeKicker),
                    ),
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
                      ..._stagedLocationIds.asMap().entries.map((entry) {
                        final idx = entry.key;
                        final id = entry.value;
                        return Padding(
                          padding: const EdgeInsets.only(bottom: 9),
                          child: _StagedLocationRow(
                            n: existingProposals.length + idx + 1,
                            locationId: id,
                            onRemove: () => setState(
                                () => _stagedLocationIds.remove(id)),
                          ),
                        );
                      }),
                      ..._stagedGeocoded.asMap().entries.map((entry) {
                        final idx = entry.key;
                        final g = entry.value;
                        return Padding(
                          padding: const EdgeInsets.only(bottom: 9),
                          child: _GeocodedStagedRow(
                            n: existingProposals.length +
                                _stagedLocationIds.length +
                                idx +
                                1,
                            geocoded: g,
                            onRemove: () => setState(
                                () => _stagedGeocoded.removeAt(idx)),
                          ),
                        );
                      }),
                      if (visibleCount == 0) ...[
                        PollTbdCard(
                          icon: Icons.place_outlined,
                          title: l10n.locationPollProposeTbdCardTitle,
                          subtitle: l10n.locationPollProposeTbdCardSubtitle,
                        ),
                        const SizedBox(height: 14),
                      ],
                      _AddSpotCard(
                        label: visibleCount == 0
                            ? l10n.locationPollProposeAddSpot
                            : l10n.locationPollProposeAddAnother,
                        onTap: _isSaving ? null : _addCandidate,
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
                          label: l10n.locationPollProposeKeepTbd,
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
                            // A single staged spot sets directly by default;
                            // offer floating it to the group as the softer
                            // alternative (matches the prototype).
                            if (visibleCount == 1)
                              Padding(
                                padding: const EdgeInsets.only(top: 4),
                                child: Tappable(
                                  semanticsLabel:
                                      l10n.locationPollProposeFloatLink,
                                  onTap: _isSaving ? null : _floatToGroup,
                                  child: Padding(
                                    padding: const EdgeInsets.all(8),
                                    child: Text(
                                      l10n.locationPollProposeFloatLink,
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
      ),
    );
  }
}

