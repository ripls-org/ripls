import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:google_maps_flutter/google_maps_flutter.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/data/gen/ripls/api/location.pb.dart' show Location;
import 'package:ripls/presentation/viewmodels/gear_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';
import 'package:ripls/presentation/widgets/content/content_morph_panel.dart';
import 'package:ripls/presentation/widgets/location/location_modal_widgets.dart';
import 'package:ripls/presentation/widgets/location/location_picker_helpers.dart'
    show openDirections;
import 'package:ripls/presentation/widgets/location/location_picker_modal.dart';
import 'package:ripls/presentation/widgets/poll/poll_option_widgets.dart'
    show PollPanelButton;

/// GearLocationPanel is the full-screen "Where" surface the gear read shell's
/// WHERE card morph-expands into — the same morph-reveal surface the experience
/// and request "Where" cards use (docs/client/modals.md), minus the poll/voting
/// machinery (gear has a single owner-set spot, no group polling). It shows the
/// confirmed spot on an embedded map with a directions button, and for the owner
/// an inline location picker (absorbed from `LocationPickerModal`) to change the
/// spot without a stacked sheet.
///
/// Reads [gearProvider] for the location and drives `GearNotifier.updateLocation`
/// on save, so the panel never calls a repository directly.
class GearLocationPanel extends ConsumerStatefulWidget {
  final String gearId;

  const GearLocationPanel({super.key, required this.gearId});

  @override
  ConsumerState<GearLocationPanel> createState() => _GearLocationPanelState();
}

class _GearLocationPanelState extends ConsumerState<GearLocationPanel> {
  /// When true the panel shows the inline location picker instead of the
  /// confirmed-spot body.
  bool _picking = false;

  String get _gearId => widget.gearId;

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(gearProvider(_gearId));
    return ContentMorphPanel(
      child: SafeArea(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            _header(context),
            Expanded(
              child: _picking ? _pickerBody(state) : _content(context, state),
            ),
          ],
        ),
      ),
    );
  }

  /// Serif "Where" title and the trailing control: a back arrow that cancels
  /// the inline picker, or the close button (pop reverses the morph).
  Widget _header(BuildContext context) {
    final l10n = context.l10n;
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
          if (_picking)
            IconAction(
              icon: Icons.arrow_back,
              semanticsLabel: l10n.commonCancel,
              color: AppColors.onContentImage,
              onPressed: () => setState(() => _picking = false),
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

  Widget _content(BuildContext context, GearState state) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        _mapSection(context, state),
        Expanded(
          child: ListView(
            padding: const EdgeInsets.fromLTRB(20, 16, 20, 28),
            children: _body(context, state),
          ),
        ),
      ],
    );
  }

  Widget _mapSection(BuildContext context, GearState state) {
    final height = MediaQuery.of(context).size.height * 0.34;
    final lat = state.locationLatitude;
    final lng = state.locationLongitude;
    final hasCoord = lat != null && lng != null && (lat != 0 || lng != 0);

    Widget map;
    if (!hasCoord) {
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
        latitude: lat,
        longitude: lng,
        height: height,
        zoom: 15,
        showBorder: false,
        showRoundedCorners: false,
        interactive: true,
        markers: {
          Marker(
            markerId: const MarkerId('gear_location'),
            position: LatLng(lat, lng),
            infoWindow: InfoWindow(title: state.locationName ?? ''),
            icon: BitmapDescriptor.defaultMarkerWithHue(
              BitmapDescriptor.hueAzure,
            ),
          ),
        },
      );
    }

    return Semantics(
      label: context.l10n.a11yLocationPanelMap,
      child: SizedBox(height: height, child: map),
    );
  }

  List<Widget> _body(BuildContext context, GearState state) {
    final l10n = context.l10n;
    final name = state.locationName ?? '';
    // Gear has no group polling — only the owner sets/changes the spot.
    final canEdit = state.isOwner;

    return [
      if (name.isNotEmpty)
        Text(
          name,
          style: const TextStyle(
            color: AppColors.onContentImage,
            fontSize: 24,
            fontWeight: FontWeight.w800,
            height: 1.15,
          ),
        ),
      const SizedBox(height: 16),
      if (name.isNotEmpty)
        PollPanelButton(
          label: l10n.locationPanelDirections,
          icon: Icons.directions_outlined,
          accentColor: AppColors.experienceSageGreen,
          primary: true,
          onTap: () => _openDirections(context, state),
        ),
      if (canEdit) ...[
        const SizedBox(height: 10),
        PollPanelButton(
          label: name.isEmpty
              ? l10n.locationPanelOwnerSetSpot
              : l10n.locationPanelChangeSpotShort,
          icon: Icons.edit_location_alt_outlined,
          accentColor: AppColors.experienceSageGreen,
          semanticsLabel: name.isEmpty
              ? l10n.locationPanelOwnerSetSpot
              : l10n.locationPanelChangeSpot,
          onTap: () => setState(() => _picking = true),
        ),
      ],
    ];
  }

  /// The inline location picker rendered on the panel surface — no separate
  /// modal. Edge-to-edge: the picker supplies its own per-child padding.
  Widget _pickerBody(GearState state) {
    return ListView(
      padding: EdgeInsets.zero,
      children: [
        LocationPickerView(
          initialLocationId: state.gearDetails?.locationId,
          showDirections: false,
          allowNonOwnerEdit: false,
          checkIsOwner: () async => ref.read(gearProvider(_gearId)).isOwner,
          onSaved: _onPickerSaved,
        ),
      ],
    );
  }

  Future<void> _onPickerSaved(String id) async {
    setState(() => _picking = false);
    if (id.isEmpty) return;
    await ref.read(gearProvider(_gearId).notifier).updateLocation(id);
  }

  Future<void> _openDirections(BuildContext context, GearState state) async {
    final name = state.locationName ?? '';
    final loc = Location()
      ..name = name
      ..latitudeDeg = state.locationLatitude ?? 0
      ..longitudeDeg = state.locationLongitude ?? 0;
    await openDirections(context, loc, fallbackName: name);
  }
}
