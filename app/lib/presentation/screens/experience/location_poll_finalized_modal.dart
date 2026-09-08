import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/core/utils/location_picker_helper.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/data/gen/ripls/api/location.pb.dart' as locapi;
import 'package:ripls/data/gen/ripls/api/location.pb.dart'
    show LocationProposal, LocationVoteStatus;
import 'package:ripls/presentation/screens/experience/location_poll_propose_modal.dart';
import 'package:ripls/presentation/viewmodels/experience_view_model.dart';
import 'package:ripls/presentation/viewmodels/location_modal_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/location/location_modal_widgets.dart';
import 'package:ripls/presentation/widgets/location/location_picker_helpers.dart'
    show openDirections;
import 'package:ripls/presentation/widgets/modal/glass/glass.dart';
import 'package:ripls/presentation/widgets/poll/poll_manage_menu_sheet.dart';
import 'package:ripls/presentation/widgets/user_avatar.dart';
import 'package:ripls/services/providers.dart';

final _log = Logger('LocationPollFinalizedModal');

/// Finalized modal — matches LX2 in `docs/cowork/App Design/location-redesign.html`.
///
/// Read-only "It's a plan." view shown once a location has been locked in.
/// Surfaces the winning spot with a sage-tinted card (name + address +
/// "N of M chose this spot" footer), an interactive Mapbox map centered
/// on the chosen spot, a faint pill row of the other (non-winning)
/// proposals for context, and a "Get directions" primary CTA. Owners
/// also get a "Change" secondary action that opens a menu to either
/// swap the confirmed spot directly or open a fresh poll.
class LocationPollFinalizedModal extends ConsumerWidget {
  const LocationPollFinalizedModal({super.key, required this.experienceId});

  final String experienceId;

  static Future<void> show(BuildContext context, String experienceId) async {
    await showAccessibleModal(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (_) =>
          LocationPollFinalizedModal(experienceId: experienceId),
    );
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = context.l10n;
    final dataAsync = ref.watch(locationModalProvider(experienceId));
    return GlassSheet(
      padding: EdgeInsets.zero,
      child: SafeArea(
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
      ),
    );
  }

  Widget _buildLoaded(
      BuildContext context, WidgetRef ref, LocationModalData data) {
    final l10n = context.l10n;
    // Confirmed location comes straight from the location-modal view-model's
    // [eventLocationId], which is refetched server-side every time the modal
    // is invalidated (e.g., after "Change → Pick a different spot"). Reading
    // from this single source keeps the Final Spot card, address, and map in
    // lockstep with the new spot regardless of how `ExperienceNotifier`
    // schedules its concurrent state writes.
    final confirmedLocationId = data.eventLocationId;

    final winner = _resolveWinner(data, confirmedLocationId);
    final others = _dedupeOthers(data.proposals, winner, data.eventLocationId);

    // YES vote count on the winning proposal + total YES voters across the
    // poll, used for the "4 of 5 chose this spot" footer.
    final winnerYesCount = winner == null
        ? 0
        : winner.votes
            .where((v) => v.status == LocationVoteStatus.LOCATION_VOTE_STATUS_YES)
            .length;
    final totalVoters = data.proposals
        .expand((p) => p.votes
            .where((v) =>
                v.status == LocationVoteStatus.LOCATION_VOTE_STATUS_YES)
            .map((v) => v.user.id))
        .toSet()
        .length;

    final attribution = winner != null && winner.proposedBy.name.isNotEmpty
        ? l10n.locationPollFinalizedAttribution(winner.proposedBy.name)
        : null;

    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(20, 4, 20, 0),
          child: _Eyebrow(
            text: l10n.locationPollFinalizedKicker,
          ),
        ),
        Padding(
          padding: const EdgeInsets.fromLTRB(20, 4, 20, 0),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              _Title(text: l10n.locationPollFinalizedTitle),
              if (attribution != null) ...[
                const SizedBox(height: 6),
                _Subtitle(text: attribution),
              ],
            ],
          ),
        ),
        Flexible(
          child: SingleChildScrollView(
            padding: const EdgeInsets.fromLTRB(20, 16, 20, 12),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                if (confirmedLocationId != null) ...[
                  _FinalSpotCard(
                    confirmedLocationId: confirmedLocationId,
                    proposal: winner,
                    votedCount: winnerYesCount,
                    invitedCount: totalVoters,
                  ),
                  const SizedBox(height: 12),
                  _WinnerMap(confirmedLocationId: confirmedLocationId),
                ],
                if (others.isNotEmpty) ...[
                  const SizedBox(height: 16),
                  _SectionLabel(text: l10n.locationPollFinalizedOtherSpots),
                  const SizedBox(height: 8),
                  _OtherSpotsPills(proposals: others),
                ],
              ],
            ),
          ),
        ),
        Padding(
          padding: const EdgeInsets.fromLTRB(20, 8, 20, 24),
          child: Row(
            children: [
              if (data.isOrganizer) ...[
                Expanded(
                  child: Tappable(
                    semanticsLabel: l10n.locationPollFinalizedChange,
                    onTap: () => _openChangeMenu(context, ref),
                    child: _PillButton(
                      label: l10n.locationPollFinalizedChange,
                      variant: _PillVariant.ghost,
                    ),
                  ),
                ),
                const SizedBox(width: 10),
              ],
              Expanded(
                flex: data.isOrganizer ? 2 : 1,
                child: Tappable(
                  semanticsLabel: l10n.locationPollFinalizedGetDirections,
                  onTap: confirmedLocationId == null
                      ? null
                      : () => _openDirectionsFromLocationId(
                            context,
                            ref,
                            confirmedLocationId,
                          ),
                  child: _PillButton(
                    label: l10n.locationPollFinalizedGetDirections,
                    enabled: confirmedLocationId != null,
                  ),
                ),
              ),
            ],
          ),
        ),
      ],
    );
  }

  /// Returns the non-winning proposals, deduped by the location they
  /// point at AND filtered to exclude any proposal that resolves to the
  /// same location as the winner. Two participants proposing the same
  /// Avery Brewing — once via a saved location, once via an inline
  /// geocoded pin — would otherwise each render as their own pill, and a
  /// duplicate-of-the-winner proposal would show up under "Other Spots"
  /// even though it's the spot the group landed on. The experience's
  /// confirmed [eventLocationId] is also seeded into the seen set so
  /// proposals pointing at the same saved Location are excluded — handy
  /// when the winner was a drop-pin that the server materialized into a
  /// Location row and other proposals reference that materialized row.
  /// Dedup key prefers the stored `location_id` (canonical identity),
  /// then the external place id from the geocoder, falling back to a
  /// lowercased display name.
  List<LocationProposal> _dedupeOthers(
    List<LocationProposal> all,
    LocationProposal? winner,
    String? eventLocationId,
  ) {
    final seen = <String>{};
    if (winner != null) {
      final winnerKey = _dedupeKey(winner);
      if (winnerKey.isNotEmpty) seen.add(winnerKey);
    }
    if (eventLocationId != null && eventLocationId.isNotEmpty) {
      seen.add('loc:$eventLocationId');
    }
    final out = <LocationProposal>[];
    for (final p in all) {
      if (winner != null && p.id == winner.id) continue;
      final key = _dedupeKey(p);
      if (key.isEmpty) {
        // No identity signal at all — keep it to avoid silently dropping
        // a proposal. Use the proposal id so multiple unkeyable rows
        // each render once.
        if (!seen.add('id:${p.id}')) continue;
        out.add(p);
        continue;
      }
      if (!seen.add(key)) continue;
      out.add(p);
    }
    return out;
  }

  String _dedupeKey(LocationProposal p) {
    final loc = p.location;
    if (loc.locationId.isNotEmpty) return 'loc:${loc.locationId}';
    if (loc.hasGeocoded()) {
      final g = loc.geocoded;
      if (g.externalPlaceId.isNotEmpty) return 'ext:${g.externalPlaceId}';
      final name = g.name.isNotEmpty
          ? g.name
          : (g.addressLines.isNotEmpty ? g.addressLines.first : g.locality);
      if (name.isNotEmpty) return 'name:${name.toLowerCase()}';
    }
    return '';
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
            icon: Icons.place_outlined,
            label: l10n.locationPollFinalizedChangePickOther,
            description: l10n.locationPollFinalizedChangePickOtherDesc,
            action: _ChangeAction.pickOther,
          ),
          PollManageMenuItem(
            icon: Icons.how_to_vote_outlined,
            label: l10n.locationPollFinalizedChangeNewPoll,
            description: l10n.locationPollFinalizedChangeNewPollDesc,
            action: _ChangeAction.newPoll,
          ),
        ],
      ),
    );
    if (result == null || !context.mounted) return;
    switch (result) {
      case _ChangeAction.pickOther:
        await _pickReplacementLocation(context, ref);
        break;
      case _ChangeAction.newPoll:
        // Closing the Finalized modal first prevents stacking two glass
        // sheets on top of each other when the propose modal slides up.
        Navigator.of(context).pop();
        if (!context.mounted) return;
        await LocationPollProposeModal.show(context, experienceId);
        break;
    }
  }

  Future<void> _pickReplacementLocation(
      BuildContext context, WidgetRef ref) async {
    final expState = ref.read(experienceProvider(experienceId));
    final exp = expState.experienceDetails?.experience;
    if (exp == null) return;
    final currentLocationId = exp.locationId.isEmpty ? null : exp.locationId;

    final String? newLocationId = await LocationPickerHelper.showLocationPicker(
      context: context,
      ref: ref,
      locationId: currentLocationId,
      checkIsOwner: () async => true,
      showDirections: true,
      allowNonOwnerEdit: false,
    );
    if (newLocationId == null || newLocationId == currentLocationId) return;
    if (!context.mounted) return;
    try {
      await ref
          .read(experienceProvider(experienceId).notifier)
          .updateLocation(newLocationId);
      // Re-fetch the modal view-model so the new spot renders immediately.
      ref.invalidate(locationModalProvider(experienceId));
    } catch (e, st) {
      _log.warning('Failed to replace confirmed location', e, st);
      if (!context.mounted) return;
      ToastHelper.showError(context, context.l10n.commonError);
    }
  }

  /// Returns the proposal that should be treated as the winner, or `null`
  /// if no proposal matches the experience's confirmed location.
  ///
  /// Prefers a proposal whose location matches [confirmedLocationId] (the
  /// experience's actual confirmed location, source of truth). Falls back
  /// to `data.lockedProposalId` only when no proposal matches — useful for
  /// the poll-completed-but-pre-confirmation flow where the experience has
  /// no locationId yet.
  LocationProposal? _resolveWinner(
    LocationModalData data,
    String? confirmedLocationId,
  ) {
    if (confirmedLocationId != null) {
      for (final p in data.proposals) {
        if (p.location.locationId == confirmedLocationId) return p;
      }
      // No proposal matches the experience's confirmed location — the
      // owner picked a different spot via LX2's "Change → Pick a Different
      // Spot" path, which bypasses ConfirmLocation. The caller will render
      // the Final Spot card from experienceProvider state instead.
      return null;
    }
    if (data.lockedProposalId != null) {
      for (final p in data.proposals) {
        if (p.id == data.lockedProposalId) return p;
      }
    }
    return null;
  }

  Future<void> _openDirectionsFromLocationId(
    BuildContext context,
    WidgetRef ref,
    String locationId,
  ) async {
    try {
      final saved = await ref.read(locationRepositoryProvider).get(locationId);
      if (!context.mounted) return;
      await openDirections(context, saved);
    } catch (_) {
      // get() failures are visible in the directions UI itself; nothing to
      // do here beyond aborting.
    }
  }
}

/* ── Atoms ──────────────────────────────────────────────────────────────── */

class _Eyebrow extends StatelessWidget {
  final String text;
  const _Eyebrow({required this.text});

  @override
  Widget build(BuildContext context) {
    return Text(
      text.toUpperCase(),
      style: TextStyle(
        color: AppColors.modalTextPrimary,
        fontSize: 11,
        fontWeight: FontWeight.w700,
        letterSpacing: 1.6,
      ),
    );
  }
}

class _Title extends StatelessWidget {
  final String text;
  const _Title({required this.text});

  @override
  Widget build(BuildContext context) {
    return Semantics(
      header: true,
      child: Text(
        text,
        style: TextStyle(
          color: AppColors.modalTextPrimary,
          fontSize: 28,
          fontWeight: FontWeight.w800,
          height: 1.1,
          letterSpacing: -0.4,
        ),
      ),
    );
  }
}

class _Subtitle extends StatelessWidget {
  final String text;
  const _Subtitle({required this.text});

  @override
  Widget build(BuildContext context) {
    return Text(
      text,
      style: TextStyle(
        color: AppColors.modalTextSecondary,
        fontSize: 14,
        height: 1.4,
      ),
    );
  }
}

class _SectionLabel extends StatelessWidget {
  final String text;
  const _SectionLabel({required this.text});

  @override
  Widget build(BuildContext context) {
    return Text(
      text.toUpperCase(),
      style: TextStyle(
        color: AppColors.modalTextSecondary,
        fontSize: 10,
        fontWeight: FontWeight.w700,
        letterSpacing: 0.8,
      ),
    );
  }
}

/// Sage-tinted card showing the locked-in spot with a "N of M chose this
/// spot" footer.
class _FinalSpotCard extends ConsumerWidget {
  final String confirmedLocationId;
  final LocationProposal? proposal;
  final int votedCount;
  final int invitedCount;

  const _FinalSpotCard({
    required this.confirmedLocationId,
    required this.proposal,
    required this.votedCount,
    required this.invitedCount,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    // The card keeps its sage-tinted background/border for visual weight,
    // but every glyph on top renders in white so the spot name + address
    // stay legible against the wash.
    final accent = AppColors.lightAccent;
    final onCardPrimary = AppColors.modalTextPrimary;
    final onCardSecondary = AppColors.modalTextSecondary;
    return Container(
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(
        color: accent.withValues(alpha: 0.22),
        border: Border.all(color: accent.withValues(alpha: 0.55), width: 1.5),
        borderRadius: BorderRadius.circular(16),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: [
          Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(Icons.check, size: 12, color: onCardPrimary),
              const SizedBox(width: 4),
              Text(
                context.l10n.locationPollConfirmFinalSpot,
                style: TextStyle(
                  color: onCardPrimary,
                  fontSize: 9,
                  fontWeight: FontWeight.w800,
                  letterSpacing: 1,
                ),
              ),
            ],
          ),
          const SizedBox(height: 6),
          _FinalSpotName(confirmedLocationId: confirmedLocationId),
          const SizedBox(height: 3),
          _FinalSpotAddress(
            confirmedLocationId: confirmedLocationId,
            color: onCardSecondary,
          ),
          if (invitedCount > 0) ...[
            const SizedBox(height: 12),
            const Divider(height: 1, color: GlassTokens.borderSoft),
            const SizedBox(height: 10),
            Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                if (proposal != null &&
                    proposal!.proposedBy.id.isNotEmpty) ...[
                  UserAvatar(user: proposal!.proposedBy, radius: 8),
                  const SizedBox(width: 6),
                ],
                Text(
                  context.l10n.locationPollFinalizedChoseThis(
                      votedCount, invitedCount),
                  style: TextStyle(
                    color: AppColors.modalTextSecondary,
                    fontSize: 11,
                    fontWeight: FontWeight.w600,
                  ),
                ),
              ],
            ),
          ],
        ],
      ),
    );
  }
}

/// Name of the Final Spot, sourced from the experience view-model when
/// available and falling back to the saved Location row otherwise.
class _FinalSpotName extends ConsumerWidget {
  final String confirmedLocationId;
  const _FinalSpotName({required this.confirmedLocationId});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final style = TextStyle(
      color: AppColors.modalTextPrimary,
      fontSize: 22,
      fontWeight: FontWeight.w800,
      height: 1.15,
    );
    return FutureBuilder<locapi.Location>(
      future: ref.read(locationRepositoryProvider).get(confirmedLocationId),
      builder: (context, snap) {
        final saved = snap.data;
        final name = (saved != null && saved.name.isNotEmpty)
            ? saved.name
            : (saved != null && saved.addressLines.isNotEmpty
                ? saved.addressLines.first
                : (saved?.locality ?? '…'));
        return Text(name, style: style, overflow: TextOverflow.ellipsis);
      },
    );
  }
}

/// Address line under the Final Spot title, resolved from the experience's
/// confirmed locationId.
class _FinalSpotAddress extends ConsumerWidget {
  final String confirmedLocationId;
  final Color color;
  const _FinalSpotAddress({
    required this.confirmedLocationId,
    required this.color,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return FutureBuilder<locapi.Location>(
      future: ref.read(locationRepositoryProvider).get(confirmedLocationId),
      builder: (context, snap) {
        final saved = snap.data;
        if (saved == null) return const SizedBox.shrink();
        final parts = <String>[];
        if (saved.addressLines.isNotEmpty) parts.add(saved.addressLines.first);
        if (saved.locality.isNotEmpty) parts.add(saved.locality);
        if (parts.isEmpty) return const SizedBox.shrink();
        return Padding(
          padding: const EdgeInsets.only(top: 2),
          child: Text(
            parts.join(', '),
            style: TextStyle(color: color, fontSize: 12),
          ),
        );
      },
    );
  }
}

/// Interactive Mapbox preview centered on the experience's confirmed
/// location. Prefers explicit lat/lng (already loaded by the experience
/// view-model) over an async lookup so the map updates in lockstep with
/// the experience's locationId.
class _WinnerMap extends ConsumerWidget {
  final String confirmedLocationId;
  const _WinnerMap({required this.confirmedLocationId});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return FutureBuilder<locapi.Location>(
      future: ref.read(locationRepositoryProvider).get(confirmedLocationId),
      builder: (context, snap) {
        final saved = snap.data;
        final lat = saved?.latitudeDeg ?? 0;
        final lng = saved?.longitudeDeg ?? 0;
        if (lat == 0 && lng == 0) return const SizedBox.shrink();
        return _MapBox(lat: lat, lng: lng);
      },
    );
  }
}

class _MapBox extends StatelessWidget {
  final double lat;
  final double lng;
  const _MapBox({required this.lat, required this.lng});

  @override
  Widget build(BuildContext context) {
    return ClipRRect(
      borderRadius: BorderRadius.circular(14),
      child: LocationMapPreview(
        latitude: lat,
        longitude: lng,
        height: 160,
        zoom: 15,
        showBorder: false,
      ),
    );
  }
}

/// Faint chip row of the non-winning proposals. Wraps so 1-2 rows fit
/// comfortably under the map without dragging the layout long.
class _OtherSpotsPills extends ConsumerWidget {
  final List<LocationProposal> proposals;
  const _OtherSpotsPills({required this.proposals});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return Opacity(
      opacity: 0.55,
      child: Wrap(
        spacing: 6,
        runSpacing: 6,
        children: proposals
            .map((p) => _ProposalNameResolver(
                  proposal: p,
                  builder: (name) => _OtherSpotPill(name: name),
                ))
            .toList(),
      ),
    );
  }
}

class _OtherSpotPill extends StatelessWidget {
  final String name;
  const _OtherSpotPill({required this.name});

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 5),
      decoration: BoxDecoration(
        color: AppColors.surface(context).withValues(alpha: 0.06),
        border: Border.all(color: AppColors.border(context)),
        borderRadius: BorderRadius.circular(100),
      ),
      child: Text(
        name,
        style: TextStyle(
          color: AppColors.modalTextPrimary,
          fontSize: 12,
          fontWeight: FontWeight.w500,
        ),
      ),
    );
  }
}

enum _ChangeAction { pickOther, newPoll }

// Change-menu rendering is delegated to [PollManageMenuSheet] via
// [_openChangeMenu]. The legacy `_ChangeMenuSheet` / `_ChangeMenuItem`
// classes were removed when both poll flows consolidated onto the shared
// row chrome.

class _ProposalNameResolver extends ConsumerWidget {
  final LocationProposal proposal;
  final Widget Function(String name) builder;
  const _ProposalNameResolver({
    required this.proposal,
    required this.builder,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final loc = proposal.location;
    if (loc.locationId.isNotEmpty) {
      return FutureBuilder<locapi.Location>(
        future: ref.read(locationRepositoryProvider).get(loc.locationId),
        builder: (context, snap) {
          final name = _resolveSavedName(snap.data) ?? '…';
          return builder(name);
        },
      );
    }
    if (loc.hasGeocoded()) {
      final g = loc.geocoded;
      final name = g.name.isNotEmpty
          ? g.name
          : (g.addressLines.isNotEmpty ? g.addressLines.first : g.locality);
      return builder(name.isEmpty ? '—' : name);
    }
    return builder('—');
  }

  String? _resolveSavedName(locapi.Location? saved) {
    if (saved == null) return null;
    if (saved.hasName() && saved.name.isNotEmpty) return saved.name;
    if (saved.addressLines.isNotEmpty) return saved.addressLines.first;
    if (saved.locality.isNotEmpty) return saved.locality;
    return null;
  }
}

enum _PillVariant { primary, ghost }

class _PillButton extends StatelessWidget {
  final String label;
  final bool enabled;
  final _PillVariant variant;
  const _PillButton({
    required this.label,
    this.enabled = true,
    this.variant = _PillVariant.primary,
  });

  @override
  Widget build(BuildContext context) {
    final accent = AppColors.lightAccent;
    final isPrimary = variant == _PillVariant.primary;
    final bg = !isPrimary
        ? Colors.transparent
        : (enabled ? accent : accent.withValues(alpha: 0.30));
    final fg = isPrimary
        ? AppColors.cardBackground(context)
        : AppColors.modalTextPrimary;
    return Container(
      height: 52,
      decoration: BoxDecoration(
        color: bg,
        border: isPrimary
            ? null
            : Border.all(color: AppColors.border(context)),
        borderRadius: BorderRadius.circular(16),
        boxShadow: isPrimary && enabled
            ? [
                BoxShadow(
                  color: accent.withValues(alpha: 0.30),
                  offset: const Offset(0, 8),
                  blurRadius: 20,
                ),
              ]
            : null,
      ),
      alignment: Alignment.center,
      child: Text(
        label,
        style: TextStyle(
          color: fg,
          fontWeight: FontWeight.w700,
          fontSize: 16,
          letterSpacing: 0.1,
        ),
      ),
    );
  }
}
