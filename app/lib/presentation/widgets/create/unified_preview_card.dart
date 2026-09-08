import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/core/utils/location_picker_helper.dart';
import 'package:ripls/data/gen/ripls/api/time.pb.dart' show ExperienceTime;
import 'package:ripls/data/gen/ripls/api/unified_create_service.pb.dart'
    show DetectedContentType;
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/screens/experience/widgets/preview/preview_time_picker_sheet.dart';
import 'package:ripls/presentation/viewmodels/unified_create_state.dart';
import 'package:ripls/presentation/viewmodels/unified_create_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/create/unified_building_pill.dart';
import 'package:ripls/presentation/widgets/create/unified_item_details_sheet.dart';
import 'package:ripls/presentation/widgets/create/unified_lend_give_toggle.dart';
import 'package:ripls/presentation/widgets/create/unified_primary_button.dart';
import 'package:ripls/presentation/widgets/create/unified_type_selector.dart';
import 'package:ripls/presentation/widgets/creation/seed_needs_field.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass_surface.dart';
import 'package:ripls/services/providers/location_providers.dart';

/// Localized "Save {type}" label for the primary create button — varies by the
/// detected content type so the action reads naturally before the Share sheet
/// opens.
String _saveButtonLabel(BuildContext context, DetectedContentType? type) {
  final l10n = context.l10n;
  switch (type) {
    case DetectedContentType.DETECTED_CONTENT_TYPE_EVENT:
      return l10n.unifiedCreateSaveEvent;
    case DetectedContentType.DETECTED_CONTENT_TYPE_REQUEST:
      return l10n.unifiedCreateSaveRequest;
    case DetectedContentType.DETECTED_CONTENT_TYPE_GEAR:
    default:
      return l10n.unifiedCreateSaveGeneric;
  }
}

String _descriptionHint(BuildContext context, DetectedContentType? type) {
  final l10n = context.l10n;
  switch (type) {
    case DetectedContentType.DETECTED_CONTENT_TYPE_GEAR:
      return l10n.unifiedCreateDescriptionHintItem;
    case DetectedContentType.DETECTED_CONTENT_TYPE_REQUEST:
      return l10n.unifiedCreateDescriptionHintRequest;
    case DetectedContentType.DETECTED_CONTENT_TYPE_EVENT:
      return l10n.unifiedCreateDescriptionHintEvent;
    default:
      return l10n.unifiedCreateDescriptionHintDefault;
  }
}

/// Glass preview card for the unified-create flow.
class UnifiedPreviewCard extends ConsumerWidget {
  const UnifiedPreviewCard({super.key, this.onShare});

  final VoidCallback? onShare;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(unifiedCreateViewModelProvider);
    final vm = ref.read(unifiedCreateViewModelProvider.notifier);

    Future<void> openTimePicker() async {
      final result = await showAccessibleModal<ExperienceTime>(
        context,
        isScrollControlled: true,
        backgroundColor: Colors.transparent,
        barrierColor: AppColors.modalBackdrop,
        builder: (_) => PreviewTimePickerSheet(
          initialTime: state.eventTime?.suggestedTime,
        ),
      );
      if (result != null) vm.setEventTime(result);
    }

    Future<void> openLocationPicker() async {
      // Route through the helper so a saved `locationId` is pre-loaded into a
      // `GeocodedLocation` BEFORE the modal mounts. Without this the modal
      // starts with the Austin fallback and Mapbox drops the catch-up camera
      // command issued during native map init — see
      // docs/client/geospatial.md §Shared Location Components.
      //
      // `allowNonOwnerEdit: true` because this is the unified-create flow —
      // the user is editing their own in-progress draft, so always show the
      // edit UI. The helper's default is `false` (read-only), which matches
      // the gear/experience detail screens but not preview.
      final result = await LocationPickerHelper.showLocationPicker(
        context: context,
        ref: ref,
        locationId: state.locationId,
        geocodedLocation: state.location,
        allowNonOwnerEdit: true,
      );
      vm.setLocationFromPicker(result);
    }

    Future<void> openItemDetails() async {
      final result = await UnifiedItemDetailsSheet.show(
        context,
        initial: state.itemDetails,
      );
      if (result != null) vm.setItemDetails(result);
    }

    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        const Align(
          alignment: Alignment.centerRight,
          child: Padding(
            padding: EdgeInsets.only(bottom: 8, right: 4),
            child: UnifiedBuildingPill(),
          ),
        ),
        GlassSurface(
          borderRadius: BorderRadius.circular(24),
          padding: const EdgeInsets.all(20),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            mainAxisSize: MainAxisSize.min,
            children: [
              const UnifiedTypeSelector(),
              const SizedBox(height: 14),
              _FadeInToast(
                shown: state.title?.isNotEmpty ?? false,
                child: _SyncedTextField(
                  value: state.title ?? '',
                  onChanged: vm.editTitle,
                  style: TextStyle(
                    fontSize: 26,
                    fontWeight: FontWeight.w700,
                    color: AppColors.modalTextPrimary,
                    height: 1.1,
                  ),
                  maxLines: null,
                  textCapitalization: TextCapitalization.sentences,
                ),
              ),
              Padding(
                padding: const EdgeInsets.only(top: 6),
                child: _SyncedTextField(
                  value: state.description ?? '',
                  onChanged: vm.editDescription,
                  hintText: _descriptionHint(context, state.type),
                  style: const TextStyle(
                    fontSize: 14,
                    color: GlassTokens.textSecondary,
                    height: 1.35,
                  ),
                  maxLines: null,
                  textCapitalization: TextCapitalization.sentences,
                ),
              ),
              const SizedBox(height: 14),
              Container(height: 1, color: AppColors.modalBorderSubtle),
              ..._buildPerTypeRows(
                context,
                state,
                onTime: openTimePicker,
                onLocation: openLocationPicker,
                onItemDetails: openItemDetails,
                onSeedNeedsChanged: vm.editRequestSeedNeeds,
              ),
            ],
          ),
        ),
        const SizedBox(height: 16),
        UnifiedPrimaryButton(
          label: _saveButtonLabel(context, state.type),
          enabled: state.isContentValid,
          loading: state.saving,
          onTap: onShare ?? () {},
        ),
      ],
    );
  }

  List<Widget> _buildPerTypeRows(
    BuildContext context,
    UnifiedCreateState state, {
    required VoidCallback onTime,
    required VoidCallback onLocation,
    required VoidCallback onItemDetails,
    required ValueChanged<List<String>> onSeedNeedsChanged,
  }) {
    final l10n = context.l10n;
    // While streaming, only show rows that the server has populated.
    // Once streaming completes, show all rows so the user can tap to set
    // any that didn't auto-fill.
    final showUnset = !state.streaming;
    final rows = <Widget>[];
    if (state.type == DetectedContentType.DETECTED_CONTENT_TYPE_GEAR) {
      rows.add(_FadeInToast(
        shown: state.streamComplete || !state.streaming,
        child: const Padding(
          padding: EdgeInsets.symmetric(vertical: 10),
          child: UnifiedLendGiveToggle(),
        ),
      ));
      final details = state.itemDetails;
      final hasDetails = details != null && !details.isEmpty;
      // A partial detail set (e.g. material/weight only) can still format
      // to '' — fall back to the "Item details" label so the row never
      // renders as a bare sparkle icon + chevron (#2724).
      final detailsSummary =
          hasDetails ? _formatItemDetails(l10n, details) : '';
      rows.add(_FadeInToast(
        shown: hasDetails || showUnset,
        child: _Row(
          icon: Icons.auto_awesome,
          text: detailsSummary.isNotEmpty
              ? detailsSummary
              : l10n.unifiedCreateItemDetails,
          onTap: onItemDetails,
        ),
      ));
    }
    if (state.type == DetectedContentType.DETECTED_CONTENT_TYPE_REQUEST) {
      // The seeded needs (#2702, #2731): the request is born with one claimable
      // need per entry, so the requester sees the extracted list and can prune
      // or add to it before saving — extraction shouldn't be invisible.
      rows.add(_FadeInToast(
        shown: state.requestSeedNeeds.isNotEmpty || showUnset,
        child: Padding(
          padding: const EdgeInsets.symmetric(vertical: 12),
          child: SeedNeedsField(
            needs: state.requestSeedNeeds,
            onChanged: onSeedNeedsChanged,
          ),
        ),
      ));
    }
    if (state.type == DetectedContentType.DETECTED_CONTENT_TYPE_EVENT) {
      final timeSet = state.eventTime?.suggestedTime != null;
      rows.add(_FadeInToast(
        shown: timeSet || showUnset,
        child: _Row(
          icon: Icons.access_time,
          text: timeSet
              ? _formatExperienceTime(state.eventTime!.suggestedTime)
              : l10n.unifiedCreateSetTime,
          onTap: onTime,
        ),
      ));
    }
    rows.add(_FadeInToast(
      shown: _locationKnown(state) || showUnset,
      child: _LocationRow(state: state, onTap: onLocation, isLast: true),
    ));
    return rows;
  }
}

/// Whether the preview has any location info — AI-geocoded or user-picked.
bool _locationKnown(UnifiedCreateState state) {
  if (state.location != null && state.location!.name.isNotEmpty) return true;
  return state.locationId?.isNotEmpty ?? false;
}

/// Renders the location row. When the user has picked a locationId,
/// resolves it to a formatted display string via [locationDisplayProvider]
/// so the row matches the single-create flow (street > locality + region
/// > region) instead of the generic "Location set" fallback.
class _LocationRow extends ConsumerWidget {
  const _LocationRow({
    required this.state,
    required this.onTap,
    this.isLast = false,
  });
  final UnifiedCreateState state;
  final VoidCallback onTap;
  final bool isLast;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = context.l10n;
    // Priority 1: AI-geocoded name (from streaming `geocoded` event).
    if (state.location != null && state.location!.name.isNotEmpty) {
      return _Row(
        icon: Icons.location_on,
        text: state.location!.name,
        onTap: onTap,
        isLast: isLast,
      );
    }
    // Priority 2: user-picked locationId — resolve via repository.
    final id = state.locationId;
    if (id != null && id.isNotEmpty) {
      final display = ref.watch(locationDisplayProvider(id));
      final text = display.when(
        data: (s) => s ?? l10n.unifiedCreateLocationSet,
        loading: () => l10n.unifiedCreateLocationSet,
        error: (_, _) => l10n.unifiedCreateLocationSet,
      );
      return _Row(
        icon: Icons.location_on,
        text: text,
        onTap: onTap,
        isLast: isLast,
      );
    }
    // Unset.
    return _Row(
      icon: Icons.location_on,
      text: l10n.unifiedCreateSetLocation,
      onTap: onTap,
      isLast: isLast,
    );
  }
}

/// Formats an [ExperienceTime] for display in the preview row. Matches
/// the format used by the existing single-creation experience preview
/// modal (`Friday, May 8 at 9:00 AM` / `TBD` / range description).
String _formatExperienceTime(ExperienceTime time) {
  if (time.hasTbd()) return 'TBD';
  if (time.hasSpecific()) {
    final ts = time.specific.unixTimestampSec.toInt();
    final date = DateTime.fromMillisecondsSinceEpoch(ts * 1000);
    const weekdays = [
      'Monday', 'Tuesday', 'Wednesday', 'Thursday',
      'Friday', 'Saturday', 'Sunday',
    ];
    const months = [
      'January', 'February', 'March', 'April', 'May', 'June',
      'July', 'August', 'September', 'October', 'November', 'December',
    ];
    final hour =
        date.hour == 0 ? 12 : (date.hour > 12 ? date.hour - 12 : date.hour);
    final minute = date.minute.toString().padLeft(2, '0');
    final period = date.hour >= 12 ? 'PM' : 'AM';
    return '${weekdays[date.weekday - 1]}, ${months[date.month - 1]} ${date.day} at $hour:$minute $period';
  }
  if (time.hasRange()) {
    return time.range.description.isNotEmpty
        ? time.range.description
        : 'Custom range';
  }
  return 'TBD';
}

/// Renders a short summary string for the item-details row from whatever
/// fields are set: brand/model, plus the approximate estimated value when
/// present ("Craftsman · M110 · ~$120"; value-only → "Estimated value
/// ~$120"). Returns '' when none of those formatted — the caller falls
/// back to the localized "Item details" label so the row never renders
/// blank (#2724).
String _formatItemDetails(AppLocalizations l10n, ItemDetailsValue v) {
  final brandModel = [
    if (v.brand.isNotEmpty) v.brand,
    if (v.model.isNotEmpty) v.model,
  ].join(' · ');
  final value = _formatApproxValue(v.estValueUsd);
  if (brandModel.isNotEmpty && value.isNotEmpty) {
    return '$brandModel · $value';
  }
  if (brandModel.isNotEmpty) return brandModel;
  if (value.isNotEmpty) return l10n.unifiedCreateItemValueOnly(value);
  return '';
}

/// Approximate display form of the free-text estimated value ("120.00" →
/// "~$120", "1300" → "~$1.3k"), matching `GearMetadataSummary`'s value
/// formatting on the gear detail page. Empty when the field is unset or
/// doesn't parse to a positive number.
String _formatApproxValue(String estValueUsd) {
  final usd = double.tryParse(estValueUsd.trim());
  if (usd == null || usd <= 0) return '';
  if (usd >= 1000) return '~\$${(usd / 1000).toStringAsFixed(1)}k';
  return '~\$${usd.toStringAsFixed(0)}';
}

/// Editable text input whose internal [TextEditingController] is kept
/// in sync with the externally-supplied [value]. When the stream
/// updates state.title / state.description, the field re-reflects the
/// latest content without clobbering an active cursor selection.
/// Mirrors the pattern used by [_TextPanelState] in the drawer.
class _SyncedTextField extends StatefulWidget {
  const _SyncedTextField({
    required this.value,
    required this.onChanged,
    this.style,
    this.maxLines,
    this.hintText,
    this.textCapitalization = TextCapitalization.none,
  });
  final String value;
  final ValueChanged<String> onChanged;
  final TextStyle? style;
  final int? maxLines;
  final String? hintText;
  final TextCapitalization textCapitalization;

  @override
  State<_SyncedTextField> createState() => _SyncedTextFieldState();
}

class _SyncedTextFieldState extends State<_SyncedTextField> {
  late final TextEditingController _controller =
      TextEditingController(text: widget.value);

  @override
  void didUpdateWidget(covariant _SyncedTextField old) {
    super.didUpdateWidget(old);
    if (widget.value != _controller.text) {
      _controller.value = TextEditingValue(
        text: widget.value,
        selection: TextSelection.collapsed(offset: widget.value.length),
      );
    }
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return TextField(
      controller: _controller,
      onChanged: widget.onChanged,
      style: widget.style,
      maxLines: widget.maxLines,
      minLines: 1,
      textCapitalization: widget.textCapitalization,
      cursorColor: AppColors.modalTextPrimary,
      decoration: InputDecoration(
        isCollapsed: true,
        filled: false,
        fillColor: Colors.transparent,
        border: InputBorder.none,
        enabledBorder: InputBorder.none,
        focusedBorder: InputBorder.none,
        contentPadding: EdgeInsets.zero,
        hintText: widget.hintText,
        hintStyle: widget.style?.copyWith(color: GlassTokens.textFaint),
      ),
    );
  }
}

/// Renders [child] with a toast-style fade-in + slide-up the first
/// time [shown] becomes true. **Always participates in flow layout** —
/// when [shown] is false the child is invisible (opacity 0, slid down
/// slightly) but still measures and reserves vertical space, so other
/// rows don't shift when fields populate.
class _FadeInToast extends StatelessWidget {
  const _FadeInToast({required this.shown, required this.child});
  final bool shown;
  final Widget child;

  static const _duration = Duration(milliseconds: 280);
  static const _curve = Curves.easeOutCubic;

  @override
  Widget build(BuildContext context) {
    return AnimatedOpacity(
      duration: _duration,
      curve: _curve,
      opacity: shown ? 1 : 0,
      child: AnimatedSlide(
        duration: _duration,
        curve: _curve,
        offset: shown ? Offset.zero : const Offset(0, 0.15),
        // IgnorePointer when hidden so invisible rows don't intercept
        // taps (e.g., a hidden "Set time" row wouldn't fire its
        // picker).
        child: IgnorePointer(ignoring: !shown, child: child),
      ),
    );
  }
}

class _Row extends StatelessWidget {
  const _Row({
    required this.icon,
    required this.text,
    required this.onTap,
    this.isLast = false,
  });
  final IconData icon;
  final String text;
  final VoidCallback onTap;
  final bool isLast;

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: text,
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(vertical: 12),
        decoration: BoxDecoration(
          border: isLast
              ? null
              : Border(bottom: BorderSide(color: AppColors.modalBorderSubtle)),
        ),
        child: Row(
          children: [
            Icon(icon, size: 18, color: AppColors.modalTextPrimary),
            const SizedBox(width: 10),
            Expanded(
              child: Text(
                text,
                style: TextStyle(
                  color: AppColors.modalTextPrimary,
                  fontSize: 14,
                ),
              ),
            ),
            Icon(
              Icons.chevron_right,
              size: 20,
              color: AppColors.modalInlineActionChevron,
            ),
          ],
        ),
      ),
    );
  }
}
