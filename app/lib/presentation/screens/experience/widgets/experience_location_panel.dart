import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:geolocator/geolocator.dart' show Position;
import 'package:google_maps_flutter/google_maps_flutter.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/core/utils/location_pin_renderer.dart';
import 'package:ripls/data/gen/ripls/api/experience_service.pb.dart'
    show RSVPIntention;
import 'package:ripls/data/gen/ripls/api/location.pb.dart'
    show Location, LocationVote, LocationVoteStatus;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/screens/experience/widgets/experience_location_panel_widgets.dart';
import 'package:ripls/presentation/screens/experience/widgets/experience_location_set_final_view.dart';
import 'package:ripls/presentation/screens/experience/widgets/experience_settings_menu_items.dart';
import 'package:ripls/presentation/viewmodels/experience_view_model.dart';
import 'package:ripls/presentation/viewmodels/location_modal_view_model.dart';
import 'package:ripls/presentation/viewmodels/location_panel_providers.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';
import 'package:ripls/presentation/widgets/accessibility/semantic_announcer.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/content/content_morph_panel.dart';
import 'package:ripls/presentation/widgets/location/location_modal_widgets.dart';
import 'package:ripls/presentation/widgets/location/location_picker_helpers.dart'
    show openDirections;
import 'package:ripls/presentation/widgets/location/location_picker_modal.dart';
import 'package:ripls/presentation/widgets/poll/poll_manage_menu_sheet.dart';
import 'package:ripls/presentation/widgets/poll/poll_option_widgets.dart';
import 'package:ripls/services/device_location_service.dart';
import 'package:ripls/services/providers.dart';

/// What the inline picker should do with the spot the user saves.
enum _PickIntent {
  /// Replace / set the single confirmed event location (no poll).
  setSingle,

  /// Add the spot to a location poll (starting or extending it).
  propose,
}

/// Owner overflow-menu actions: poll management (while a poll runs) plus the
/// experience-settings actions carried over from the content view's Manage
/// sheet.
enum _LocationManageAction {
  addSpot,
  setFinal,
  toggleLock,
  nudge,
  cancelPoll,
  markCompleted,
  closeEvent,
}

/// ExperienceLocationPanel is the full-screen "Where" surface the location card
/// expands into — the same morph-reveal surface as the roster and conversation
/// (docs/client/modals.md). It absorbs the location-poll bottom sheets
/// (where-screen v3): an embedded map, inline poll voting (auto-submit), an
/// inline set-final-spot view, a single-spot (n=1) accept/decline state, owner
/// controls, and an in-panel manage menu from the top bar.
///
/// Reads [locationModalProvider] for state + mutations, [locationPanelGeoProvider]
/// for pre-resolved names/coordinates, and [experienceProvider] for the
/// attendee roster — so the panel never calls a repository directly.
class ExperienceLocationPanel extends ConsumerStatefulWidget {
  final String experienceId;
  final Color accentColor;

  /// Owner experience-settings actions surfaced in the overflow menu (the same
  /// "Mark completed" / "Close event" flows as the content view's Manage
  /// sheet). Null when the viewer isn't the owner.
  final VoidCallback? onMarkCompleted;
  final VoidCallback? onCloseEvent;

  const ExperienceLocationPanel({
    super.key,
    required this.experienceId,
    required this.accentColor,
    this.onMarkCompleted,
    this.onCloseEvent,
  });

  @override
  ConsumerState<ExperienceLocationPanel> createState() =>
      _ExperienceLocationPanelState();
}

class _ExperienceLocationPanelState
    extends ConsumerState<ExperienceLocationPanel> {
  /// Proposal ids with an in-flight vote RPC — disables that row meanwhile.
  final Set<String> _inFlight = {};
  bool _busy = false;

  /// Lazily-rendered lettered map-pin bitmaps for the poll markers.
  final LocationPinCache _pins = LocationPinCache();

  /// When true the panel shows the inline set-final-spot view instead of the
  /// main poll/confirmed/tbd body. [_finalTargetId] is the proposal the owner
  /// is about to lock in.
  bool _settingFinal = false;
  String? _finalTargetId;

  /// When set the panel shows the inline location picker (absorbed from
  /// `LocationPickerModal`); the value is what to do with the picked spot.
  _PickIntent? _picking;

  /// The confirmed location id to seed the inline picker's map with (for the
  /// "change spot" flow), or null for a fresh pick.
  String? _pickInitialLocationId;

  String get _experienceId => widget.experienceId;

  @override
  void initState() {
    super.initState();
    // The content view's cache listener doesn't reach this pushed panel, so
    // refresh on any mutation that invalidates the cache (e.g. a vote from the
    // chat banner). listenManual in initState, not ref.listen in build.
    ref.listenManual(contentCacheInvalidationProvider, (previous, next) {
      if (previous == next) return;
      ref.invalidate(locationModalProvider(_experienceId));
    });
  }

  @override
  Widget build(BuildContext context) {
    final dataAsync = ref.watch(locationModalProvider(_experienceId));
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

  /// Serif title, an owner-only manage (···) button while a poll is running,
  /// and the trailing control: a back arrow that cancels the inline picker, or
  /// the close button (pop reverses the morph into the card).
  Widget _header(BuildContext context, LocationModalData? data) {
    final l10n = context.l10n;
    final picking = _picking != null;
    // Always available to the owner — poll-management actions while a poll runs,
    // else the experience settings.
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
                l10n.contentFactWhere,
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
                _pickInitialLocationId = null;
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

  Widget _content(BuildContext context, LocationModalData data) {
    final geoAsync = ref.watch(locationPanelGeoProvider(_experienceId));
    final geo = geoAsync.asData?.value;
    final userPos =
        ref.watch(userLocationProvider(LocationIntent.precisePin)).asData?.value;

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        _mapSection(context, data, geo),
        Expanded(
          child: ListView(
            padding: const EdgeInsets.fromLTRB(20, 16, 20, 28),
            children: _settingFinal
                ? [
                    LocationSetFinalView(
                      experienceId: _experienceId,
                      finalTargetId: _finalTargetId,
                      accentColor: widget.accentColor,
                      busy: _busy,
                      onRetarget: (id) =>
                          setState(() => _finalTargetId = id),
                      onCancel: () => setState(() => _settingFinal = false),
                      onLock: _lockInFinal,
                    ),
                  ]
                : _body(context, data, geo, userPos),
          ),
        ),
      ],
    );
  }

  // ── Map ────────────────────────────────────────────────────────────────

  Widget _mapSection(
    BuildContext context,
    LocationModalData data,
    LocationPanelGeo? geo,
  ) {
    final height = MediaQuery.of(context).size.height * 0.34;
    final markers = _markers(data, geo);
    final center = _center(data, geo);

    Widget map;
    if (markers.isEmpty && center == null) {
      map = Container(
        color: AppColors.darkSurface,
        alignment: Alignment.center,
        child: Icon(
          Icons.map_outlined,
          size: 36,
          color: AppColors.darkTextTertiary,
        ),
      );
    } else {
      map = LocationMapPreview(
        latitude: center?.latitude ?? 39.5,
        longitude: center?.longitude ?? -98.5,
        height: height,
        zoom: markers.isEmpty ? 12 : 15,
        showBorder: false,
        showRoundedCorners: false,
        markers: markers.isEmpty ? null : markers,
        fitToMarkers: markers.length >= 2,
        interactive: true,
      );
    }

    return Semantics(
      label: context.l10n.a11yLocationPanelMap,
      child: SizedBox(height: height, child: map),
    );
  }

  Set<Marker> _markers(LocationModalData data, LocationPanelGeo? geo) {
    if (geo == null) return const {};
    // Set-final view: just the target.
    if (_settingFinal) {
      final r = geo.proposals
          .where((p) => p.proposal.id == _finalTargetId)
          .firstOrNull;
      if (r != null && r.hasCoordinate) {
        return {
          Marker(
            markerId: const MarkerId('final'),
            position: LatLng(r.latitude!, r.longitude!),
            infoWindow: InfoWindow(title: r.name),
          ),
        };
      }
      return const {};
    }
    if (data.locationPollActive) {
      final leadingId = data.leadingProposalId;
      // Letters track the option rows' order (`_visibleProposals`) so a
      // coordinate-less proposal still consumes its letter — pins stay aligned.
      final visible = _visibleProposals(data, geo);
      return {
        for (var i = 0; i < visible.length; i++)
          if (visible[i].hasCoordinate)
            Marker(
              markerId: MarkerId(visible[i].proposal.id),
              position: LatLng(visible[i].latitude!, visible[i].longitude!),
              anchor: const Offset(0.5, 1),
              infoWindow: InfoWindow(
                title: '${pollOptionLetter(i)} · ${visible[i].name}',
              ),
              icon: _pins.letteredIcon(
                pollOptionLetter(i),
                leader: visible[i].proposal.id == leadingId,
                accent: widget.accentColor,
                onReady: () {
                  if (mounted) setState(() {});
                },
              ),
            ),
      };
    }
    final confirmed = geo.confirmed;
    if (confirmed != null && confirmed.hasCoordinate) {
      return {
        Marker(
          markerId: const MarkerId('confirmed'),
          position: LatLng(confirmed.latitude!, confirmed.longitude!),
          infoWindow: InfoWindow(title: confirmed.name),
          icon: BitmapDescriptor.defaultMarkerWithHue(BitmapDescriptor.hueAzure),
        ),
      };
    }
    return const {};
  }

  LatLng? _center(LocationModalData data, LocationPanelGeo? geo) {
    final markers = _markers(data, geo);
    if (markers.isNotEmpty) return markers.first.position;
    final userPos =
        ref.watch(userLocationProvider(LocationIntent.precisePin)).asData?.value;
    if (userPos != null) return LatLng(userPos.latitude, userPos.longitude);
    return null;
  }

  // ── Body (state-dependent) ───────────────────────────────────────────────

  List<Widget> _body(
    BuildContext context,
    LocationModalData data,
    LocationPanelGeo? geo,
    Position? userPos,
  ) {
    if (geo == null) {
      return const [
        Padding(
          padding: EdgeInsets.only(top: 24),
          child: Center(child: CircularProgressIndicator()),
        ),
      ];
    }
    if (data.locationPollActive) {
      final visible = _visibleProposals(data, geo);
      final viewerIsProposer = visible.length == 1 &&
          visible.first.proposal.proposedBy.id == data.currentUserId;
      if (visible.length == 1 && !data.isOrganizer && !viewerIsProposer) {
        return _singleSpotBody(context, data, visible.first, userPos);
      }
      return _pollBody(context, data, visible, userPos);
    }
    if (geo.confirmed != null) {
      return _confirmedBody(context, data, geo, userPos);
    }
    return _tbdBody(context, data);
  }

  /// The distinct YES-voters on a proposal, as users for the avatar stack.
  List<User> _yesVoters(List<LocationVote> votes) => [
        for (final v in votes)
          if (v.status == LocationVoteStatus.LOCATION_VOTE_STATUS_YES) v.user,
      ];

  /// Proposals belonging to the active poll, as resolved for display.
  List<ResolvedLocationProposal> _visibleProposals(
    LocationModalData data,
    LocationPanelGeo geo,
  ) {
    final pollId = data.currentLocationPollId;
    return geo.proposals
        .where((r) =>
            pollId == null ||
            pollId.isEmpty ||
            !r.proposal.hasPollId() ||
            r.proposal.pollId == pollId)
        .toList();
  }

  /// Active poll: replies counter, unreplied/nudge strip, the lettered option
  /// rows (auto-submit voting), an add-spot affordance, and a primary
  /// "Set the final spot" for the owner.
  List<Widget> _pollBody(
    BuildContext context,
    LocationModalData data,
    List<ResolvedLocationProposal> visible,
    Position? userPos,
  ) {
    final l10n = context.l10n;
    final submitted = data.hasVotedOnCurrentPoll;
    final leadingId = data.leadingProposalId;
    final tiedTop = pollTiedTopCount(
      visible.map((r) => r.proposal.id).toList(),
      data.effectiveVoteCount,
    );

    return [
      Text(
        submitted ? l10n.locationPollVoteSubmittedTitle : l10n.locationPollVoteTitle,
        style: const TextStyle(
          color: AppColors.onContentImage,
          fontSize: 22,
          fontWeight: FontWeight.w800,
          height: 1.1,
        ),
      ),
      if (!submitted) ...[
        const SizedBox(height: 6),
        Text(
          l10n.locationPollVoteSubtitle,
          style: const TextStyle(
            color: AppColors.darkTextSecondary,
            fontSize: 14,
            height: 1.4,
          ),
        ),
      ],
      const SizedBox(height: 12),
      ..._unrepliedSection(context, data),
      const SizedBox(height: 14),
      for (var i = 0; i < visible.length; i++)
        PollOptionRow(
          letter: pollOptionLetter(i),
          name: visible[i].name,
          subtitle: visible[i].address,
          evidence: locationDistanceLabel(visible[i], userPos),
          evidenceGlyph: '🚶',
          addedByName: visible[i].proposal.proposedBy.id != data.currentUserId
              ? visible[i].proposal.proposedBy.name
              : null,
          voters: _yesVoters(visible[i].proposal.votes),
          voted: data.currentUserYesProposalIds.contains(visible[i].proposal.id),
          leading: visible[i].proposal.id == leadingId,
          tied: tiedTop > 1 &&
              data.effectiveVoteCount(visible[i].proposal.id) == tiedTop,
          accentColor: widget.accentColor,
          enabled: !_inFlight.contains(visible[i].proposal.id),
          onTap: () => _toggleVote(visible[i].proposal.id),
        ),
      if (!data.locationProposalsLocked)
        PollAddRow(
          label: l10n.locationPollProposeAddAnother,
          accentColor: widget.accentColor,
          onTap: _busy ? null : _addSpotToPoll,
        ),
      if (data.isOrganizer) ...[
        const SizedBox(height: 16),
        PollPanelButton(
          label: l10n.locationPollConfirmKicker,
          icon: Icons.check_circle_outline,
          accentColor: widget.accentColor,
          primary: true,
          onTap: visible.isEmpty || _busy ? null : () => _enterSetFinal(data),
        ),
      ],
    ];
  }

  /// Single-spot (n=1) state for an attendee who didn't propose it: "X set the
  /// spot — Works for me / Doesn't work / Directions."
  List<Widget> _singleSpotBody(
    BuildContext context,
    LocationModalData data,
    ResolvedLocationProposal r,
    Position? userPos,
  ) {
    final l10n = context.l10n;
    final voted = data.currentUserYesProposalIds.contains(r.proposal.id);
    final proposer = r.proposal.proposedBy.name;
    final dist = locationDistanceLabel(r, userPos);

    return [
      if (proposer.isNotEmpty)
        Text(
          l10n.locationPanelSingleKicker(proposer).toUpperCase(),
          style: const TextStyle(
            color: AppColors.darkTextSecondary,
            fontSize: 11.5,
            fontWeight: FontWeight.w800,
            letterSpacing: 1.2,
          ),
        ),
      const SizedBox(height: 6),
      Text(
        r.name,
        style: const TextStyle(
          color: AppColors.onContentImage,
          fontSize: 24,
          fontWeight: FontWeight.w800,
          height: 1.15,
        ),
      ),
      if (r.address != null && r.address!.isNotEmpty) ...[
        const SizedBox(height: 6),
        Text(
          r.address!,
          style: const TextStyle(
            color: AppColors.darkTextSecondary,
            fontSize: 15,
          ),
        ),
      ],
      if (dist != null) ...[
        const SizedBox(height: 8),
        Text(
          '🚶 $dist',
          style: const TextStyle(color: AppColors.darkTextTertiary, fontSize: 13),
        ),
      ],
      const SizedBox(height: 12),
      Text(
        l10n.locationPanelSingleSubtitle,
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
        onTap: (voted || _busy) ? null : () => _setWorks(r.proposal.id),
      ),
      const SizedBox(height: 10),
      Row(
        children: [
          Expanded(
            child: PollPanelButton(
              label: l10n.locationPanelDoesntWork,
              accentColor: widget.accentColor,
              onTap: _busy ? null : _addSpotToPoll,
            ),
          ),
          const SizedBox(width: 10),
          Expanded(
            child: PollPanelButton(
              label: l10n.locationPanelDirectionsShort,
              icon: Icons.directions_outlined,
              accentColor: widget.accentColor,
              semanticsLabel: l10n.a11yLocationPanelDirections(r.name),
              onTap: () => _openDirections(context, r),
            ),
          ),
        ],
      ),
    ];
  }

  /// Inline set-final-spot view (absorbs LocationPollConfirmModal): the chosen
  /// winner card, a tie / not-leader warning, the re-targetable "how everyone
  /// picked" rows, and Cancel / Lock-in.
  /// Confirmed single spot: name + address + your distance, directions, and
  /// owner controls to change the spot or re-open it to the group.
  List<Widget> _confirmedBody(
    BuildContext context,
    LocationModalData data,
    LocationPanelGeo geo,
    Position? userPos,
  ) {
    final l10n = context.l10n;
    final c = geo.confirmed!;
    final dist = locationDistanceLabel(c, userPos);
    return [
      Text(
        c.name,
        style: const TextStyle(
          color: AppColors.onContentImage,
          fontSize: 24,
          fontWeight: FontWeight.w800,
          height: 1.15,
        ),
      ),
      if (c.address != null && c.address!.isNotEmpty) ...[
        const SizedBox(height: 4),
        Text(
          c.address!,
          style: const TextStyle(color: AppColors.darkTextSecondary, fontSize: 15),
        ),
      ],
      if (dist != null) ...[
        const SizedBox(height: 8),
        Text(
          '🚶 $dist',
          style: const TextStyle(color: AppColors.darkTextTertiary, fontSize: 13),
        ),
      ],
      const SizedBox(height: 16),
      PollPanelButton(
        label: l10n.locationPanelDirections,
        icon: Icons.directions_outlined,
        accentColor: widget.accentColor,
        primary: true,
        semanticsLabel: l10n.a11yLocationPanelDirections(c.name),
        onTap: () => _openDirections(context, c),
      ),
      if (data.isOrganizer) ...[
        const SizedBox(height: 10),
        Row(
          children: [
            Expanded(
              child: PollPanelButton(
                label: l10n.locationPanelChangeSpotShort,
                icon: Icons.edit_location_alt_outlined,
                accentColor: widget.accentColor,
                semanticsLabel: l10n.locationPanelChangeSpot,
                onTap: _busy ? null : _pickDifferentSpot,
              ),
            ),
            const SizedBox(width: 10),
            Expanded(
              child: PollPanelButton(
                label: l10n.locationPanelAskGroupShort,
                icon: Icons.how_to_vote_outlined,
                accentColor: widget.accentColor,
                semanticsLabel: l10n.locationPanelAddSpots,
                onTap: _busy ? null : _addSpotToPoll,
              ),
            ),
          ],
        ),
      ],
    ];
  }

  /// No location and no poll: owner can pick a spot or start a poll; everyone
  /// else sees a short "not set yet" note.
  List<Widget> _tbdBody(BuildContext context, LocationModalData data) {
    final l10n = context.l10n;
    return [
      Text(
        l10n.locationPollProposeTbdCardTitle,
        style: const TextStyle(
          color: AppColors.onContentImage,
          fontSize: 22,
          fontWeight: FontWeight.w800,
        ),
      ),
      const SizedBox(height: 6),
      Text(
        data.isOrganizer
            ? l10n.locationPollProposeSubtitle
            : l10n.locationPanelTbdHostBody,
        style: const TextStyle(
          color: AppColors.darkTextSecondary,
          fontSize: 14,
          height: 1.4,
        ),
      ),
      if (data.isOrganizer) ...[
        const SizedBox(height: 16),
        PollPanelButton(
          label: l10n.locationPanelOwnerSetSpot,
          icon: Icons.place_outlined,
          accentColor: widget.accentColor,
          primary: true,
          onTap: _busy ? null : _pickDifferentSpot,
        ),
        const SizedBox(height: 10),
        PollPanelButton(
          label: l10n.locationPanelAddSpots,
          icon: Icons.how_to_vote_outlined,
          accentColor: widget.accentColor,
          onTap: _busy ? null : _addSpotToPoll,
        ),
      ],
    ];
  }

  // ── Unreplied strip ───────────────────────────────────────────────────────

  List<Widget> _unrepliedSection(BuildContext context, LocationModalData data) {
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

  /// Attendees expected at the event (RSVP'd yes/maybe), used as the poll's
  /// reply denominator and unreplied set.
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

  bool _deadlineNear(LocationModalData data) {
    final deadline = data.locationPollDeadlineUnixSec;
    if (deadline == null) return false;
    final secsLeft = deadline - DateTime.now().millisecondsSinceEpoch ~/ 1000;
    return secsLeft > 0 && secsLeft < 24 * 3600;
  }

  // ── Actions ──────────────────────────────────────────────────────────────

  Future<void> _toggleVote(String proposalId) async {
    if (_inFlight.contains(proposalId)) return;
    setState(() => _inFlight.add(proposalId));
    try {
      await ref
          .read(locationModalProvider(_experienceId).notifier)
          .voteOnLocation(proposalId);
      if (!mounted) return;
      unawaited(SemanticAnnouncer.announce(
        context,
        context.l10n.locationPollVoteSubmittedTitle,
      ));
    } catch (_) {
      _showError();
    } finally {
      if (mounted) setState(() => _inFlight.remove(proposalId));
    }
  }

  /// Single-spot accept: cast a YES vote without toggling it back off.
  Future<void> _setWorks(String proposalId) async {
    await _guard(() => ref
        .read(locationModalProvider(_experienceId).notifier)
        .voteOnLocation(proposalId));
    if (mounted) {
      unawaited(SemanticAnnouncer.announce(
        context,
        context.l10n.locationPollVoteSubmittedTitle,
      ));
    }
  }

  /// Opens the inline picker (on the panel itself, not a modal) seeded with the
  /// confirmed location, to replace / set the single event location on save.
  void _pickDifferentSpot() {
    setState(() {
      _pickInitialLocationId = ref
          .read(locationModalProvider(_experienceId))
          .asData
          ?.value
          .eventLocationId;
      _picking = _PickIntent.setSingle;
    });
  }

  /// Opens the inline picker to add a fresh spot to the poll on save.
  void _addSpotToPoll() {
    setState(() {
      _pickInitialLocationId = null;
      _picking = _PickIntent.propose;
    });
  }

  void _onPickerSaved(String id) {
    final intent = _picking;
    setState(() {
      _picking = null;
      _pickInitialLocationId = null;
    });
    if (id.isEmpty || intent == null) return;
    final notifier = ref.read(locationModalProvider(_experienceId).notifier);
    _guard(() => intent == _PickIntent.setSingle
        ? notifier.setSingleLocation(locationId: id)
        : notifier.proposeSavedLocation(id));
  }

  /// The inline location picker (absorbed `LocationPickerModal`) rendered on the
  /// panel surface — no separate modal. Edge-to-edge: the picker supplies its
  /// own per-child padding and full-bleed map band.
  Widget _pickerBody() {
    return ListView(
      padding: EdgeInsets.zero,
      children: [
        LocationPickerView(
          initialLocationId: _pickInitialLocationId,
          showDirections: false,
          onSaved: _onPickerSaved,
        ),
      ],
    );
  }

  void _enterSetFinal(LocationModalData data) {
    setState(() {
      _finalTargetId = data.leadingProposalId ??
          data.currentPollProposals.firstOrNull?.id;
      _settingFinal = true;
    });
  }

  Future<void> _lockInFinal() async {
    final id = _finalTargetId;
    if (id == null) return;
    await _guard(() => ref
        .read(locationModalProvider(_experienceId).notifier)
        .confirmLocation(id));
    if (mounted) setState(() => _settingFinal = false);
  }

  Future<void> _openDirections(
    BuildContext context,
    ResolvedLocationProposal r,
  ) async {
    final loc = Location()..name = r.name;
    if (r.latitude != null) loc.latitudeDeg = r.latitude!;
    if (r.longitude != null) loc.longitudeDeg = r.longitude!;
    await openDirections(context, loc, fallbackName: r.name);
  }

  Future<void> _nudge() async {
    await _guard(() async {
      final count = await ref
          .read(locationModalProvider(_experienceId).notifier)
          .nudgeUnreplied();
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

  /// The owner overflow menu: poll-management actions while a poll runs, else
  /// the experience-settings actions (mark completed / close event).
  Future<void> _openManageMenu(LocationModalData data) async {
    final l10n = context.l10n;
    final pollActive = data.locationPollActive;
    final locked = data.locationProposalsLocked;
    final action = await showAccessibleModal<_LocationManageAction>(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (_) => PollManageMenuSheet<_LocationManageAction>(
        items: [
          if (pollActive) ...[
            if (!locked)
              PollManageMenuItem(
                icon: Icons.place_outlined,
                label: l10n.locationPollManageAddSpot,
                description: l10n.locationPollManageAddSpotDesc,
                action: _LocationManageAction.addSpot,
              ),
            PollManageMenuItem(
              icon: locked ? Icons.lock_open : Icons.lock_outline,
              label:
                  locked ? l10n.locationPollManageUnlock : l10n.locationPollManageLock,
              description: locked
                  ? l10n.locationPollManageUnlockDesc
                  : l10n.locationPollManageLockDesc,
              action: _LocationManageAction.toggleLock,
            ),
            PollManageMenuItem(
              icon: Icons.notifications_outlined,
              label: l10n.locationPollManageNudge,
              description: l10n.locationPollManageNudgeDesc,
              action: _LocationManageAction.nudge,
            ),
            if (data.proposals.isNotEmpty)
              PollManageMenuItem(
                icon: Icons.check,
                label: l10n.locationPollManageSetFinal,
                description: l10n.locationPollManageSetFinalDesc,
                action: _LocationManageAction.setFinal,
              ),
            PollManageMenuItem(
              icon: Icons.close,
              label: l10n.locationPollManageCancelPoll,
              description: l10n.locationPollManageCancelDesc,
              destructive: true,
              action: _LocationManageAction.cancelPoll,
            ),
          ]
          // Event settings — hidden until any open poll completes.
          else
            ...experienceSettingsMenuItems(
              context,
              markCompleted: _LocationManageAction.markCompleted,
              closeEvent: _LocationManageAction.closeEvent,
            ),
        ],
      ),
    );
    if (action == null || !mounted) return;
    final notifier = ref.read(locationModalProvider(_experienceId).notifier);
    switch (action) {
      case _LocationManageAction.addSpot:
        _addSpotToPoll();
      case _LocationManageAction.setFinal:
        _enterSetFinal(data);
      case _LocationManageAction.toggleLock:
        await _guard(() => notifier.setLocationProposalsLocked(!locked));
      case _LocationManageAction.nudge:
        await _nudge();
      case _LocationManageAction.cancelPoll:
        final confirmed = await _confirmCancel();
        if (confirmed ?? false) await _guard(notifier.endLocationPoll);
      case _LocationManageAction.markCompleted:
        widget.onMarkCompleted?.call();
      case _LocationManageAction.closeEvent:
        // Closing the event tears down the content view it's pushed over, so
        // pop this panel first, then run the close flow on the content view.
        Navigator.of(context).pop();
        widget.onCloseEvent?.call();
    }
  }

  Future<bool?> _confirmCancel() {
    final l10n = context.l10n;
    return showDialog<bool>(
      context: context,
      builder: (dialogCtx) => AlertDialog(
        title: Text(l10n.locationPollManageCancelPoll),
        content: Text(l10n.locationPollManageCancelDesc),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(dialogCtx).pop(false),
            child: Text(l10n.commonCancel),
          ),
          TextButton(
            onPressed: () => Navigator.of(dialogCtx).pop(true),
            child: Text(l10n.locationPollManageCancelPoll),
          ),
        ],
      ),
    );
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
