import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/data/gen/ripls/api/search_service.pb.dart'
    show SearchItemType;
import 'package:ripls/presentation/models/discover_item.dart'
    show SearchDiscoverItem;
import 'package:ripls/presentation/viewmodels/search_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/accessibility/toggle.dart';
import 'package:ripls/presentation/widgets/modal/solid_sheet.dart';
import 'package:ripls/presentation/widgets/user_avatar.dart';
import 'package:ripls/services/location_search_service.dart'
    show LocationResult;
import 'package:ripls/services/providers.dart' show communitiesProvider;
import 'package:ripls/services/providers/auth_providers.dart';
import 'package:ripls/services/providers/location_providers.dart'
    show locationSearchServiceProvider;

/// What the location sheet resolved to: [place] is null for "back to
/// Near me", otherwise the chosen anchor with coordinates populated.
/// [communityId] is set when the anchor is one of the user's community
/// areas, so the header can mark that row Current on the next open.
class LibraryAnchorResult {
  final LocationResult? place;
  final String? communityId;

  const LibraryAnchorResult(this.place, {this.communityId});
}

/// LibraryLocationSheet is the Library's "Show gear near…" sheet
/// (#2634 v2, Rev 12/14) on the solid (non-glass) sheet surface —
/// where, not what. Community areas lead ("group areas do 80% of the
/// work with zero typing") with honest object counts, then upcoming
/// plan locations, then the place field — deliberately the app's
/// **only** geocoding surface. The result filters (what shows on the
/// map/shelves, People, Include Completed) live inline at the bottom of
/// this same sheet; map styling is its own affordance (the map-style
/// button floats on the map) and no longer bundled here.
class LibraryLocationSheet extends ConsumerStatefulWidget {
  /// Community whose area the map is currently anchored to, if any —
  /// its row renders the Current pill.
  final String? currentCommunityId;

  const LibraryLocationSheet({super.key, this.currentCommunityId});

  /// Opens the sheet; resolves to null when dismissed without choosing.
  ///
  /// Uses [useRootNavigator] so the sheet renders above the bottom dock:
  /// the Library tab hosts its own nested Navigator (for the legacy
  /// Discover screen it's built from), and without this the sheet would
  /// paint into that Navigator's Overlay — which sits *below* the dock
  /// in the outer Stack — leaving the dock visible on top of the sheet.
  static Future<LibraryAnchorResult?> show(
    BuildContext context, {
    String? currentCommunityId,
  }) {
    return showAccessibleModal<LibraryAnchorResult>(
      context,
      isScrollControlled: true,
      useRootNavigator: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (_) =>
          LibraryLocationSheet(currentCommunityId: currentCommunityId),
    );
  }

  @override
  ConsumerState<LibraryLocationSheet> createState() =>
      _LibraryLocationSheetState();
}

class _LibraryLocationSheetState extends ConsumerState<LibraryLocationSheet> {
  final TextEditingController _placeController = TextEditingController();

  /// One autocomplete session per sheet open, so providers that bill
  /// suggestion + detail as a pair see a single session.
  final String _sessionToken = UniqueKey().toString();

  Timer? _debounce;
  List<LocationResult> _suggestions = const [];
  bool _searching = false;

  @override
  void dispose() {
    _debounce?.cancel();
    _placeController.dispose();
    super.dispose();
  }

  void _onPlaceQueryChanged(String query) {
    _debounce?.cancel();
    final trimmed = query.trim();
    if (trimmed.isEmpty) {
      setState(() {
        _suggestions = const [];
        _searching = false;
      });
      return;
    }
    _debounce = Timer(const Duration(milliseconds: 300), () async {
      setState(() => _searching = true);
      try {
        final results = await ref
            .read(locationSearchServiceProvider)
            .searchAddresses(trimmed, sessionToken: _sessionToken);
        if (!mounted) return;
        setState(() {
          _suggestions = results.take(5).toList(growable: false);
          _searching = false;
        });
      } catch (_) {
        if (!mounted) return;
        // Geocoder hiccup: fall back to an empty suggestion list rather
        // than surfacing an error inside the sheet.
        setState(() {
          _suggestions = const [];
          _searching = false;
        });
      }
    });
  }

  Future<void> _selectSuggestion(LocationResult suggestion) async {
    final resolved = await ref
        .read(locationSearchServiceProvider)
        .resolveSuggestion(suggestion, sessionToken: _sessionToken);
    if (!mounted) return;
    if (resolved == null) return;
    Navigator.of(context).pop(LibraryAnchorResult(resolved));
  }

  /// A synthetic anchor for objects that already carry coordinates
  /// (community areas, plan locations) — no geocoding involved.
  LocationResult _anchorFor(String name, double latitude, double longitude) {
    return LocationResult(
      name: name,
      fullName: name,
      type: 'anchor',
      latitude: latitude,
      longitude: longitude,
      streetAddress: '',
      locality: '',
      region: '',
      postcode: '',
      regionCode: '',
    );
  }

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    final keyboardInset = MediaQuery.of(context).viewInsets.bottom;
    final communities = ref.watch(communitiesProvider).communities;
    final areas = communities
        .where((c) => c.hasAreaLatitudeDeg() && c.hasAreaLongitudeDeg())
        .toList(growable: false);
    final planStops = _upcomingPlanStops();

    return SolidSheet(
      child: Padding(
        // Lift the sheet content above the keyboard while the place
        // field is focused (see docs/client/modals.md).
        padding: EdgeInsets.only(bottom: keyboardInset),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Text(
              l10n.librarySheetTitle,
              style: TextStyle(
                fontFamily: AppTheme.headingFont,
                fontSize: 20,
                fontWeight: FontWeight.w600,
                color: AppColors.textPrimary(context),
              ),
            ),
            const SizedBox(height: 10),
            _placeField(context),
            if (_searching)
              const Padding(
                padding: EdgeInsets.symmetric(vertical: 12),
                child: Center(
                  child: SizedBox(
                    width: 18,
                    height: 18,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  ),
                ),
              ),
            for (final suggestion in _suggestions)
              _row(
                icon: Icons.place_outlined,
                title: suggestion.name,
                subtitle: suggestion.fullName,
                onTap: () => unawaited(_selectSuggestion(suggestion)),
              ),
            _row(
              icon: Icons.my_location,
              title: l10n.librarySheetCurrentLocation,
              subtitle: l10n.librarySheetBackToNearMe,
              onTap: () =>
                  Navigator.of(context).pop(const LibraryAnchorResult(null)),
            ),
            if (areas.isNotEmpty) ...[
              _sectionLabel(context, l10n.librarySheetGroupAreas),
              for (final community in areas)
                _row(
                  icon: Icons.home_outlined,
                  title: community.name,
                  subtitle: l10n.librarySheetAreaCount(community.gearCount),
                  currentPill:
                      community.id == widget.currentCommunityId,
                  onTap: () => Navigator.of(context).pop(
                    LibraryAnchorResult(
                      _anchorFor(
                        community.name,
                        community.areaLatitudeDeg,
                        community.areaLongitudeDeg,
                      ),
                      communityId: community.id,
                    ),
                  ),
                ),
            ],
            if (planStops.isNotEmpty) ...[
              _sectionLabel(context, l10n.librarySheetWhereYoullBe),
              for (final stop in planStops)
                _row(
                  icon: Icons.calendar_month_outlined,
                  amber: true,
                  title: stop.title,
                  subtitle: stop.subtitle,
                  onTap: () => Navigator.of(context).pop(
                    LibraryAnchorResult(
                      _anchorFor(stop.title, stop.latitude, stop.longitude),
                    ),
                  ),
                ),
            ],
            const SizedBox(height: 12),
            Container(height: 1, color: AppColors.divider(context)),
            _buildFilters(context),
          ],
        ),
      ),
    );
  }

  /// Upcoming plans with coordinates, drawn from the search state the
  /// map already holds — what can you borrow near where you'll be.
  List<_PlanStop> _upcomingPlanStops() {
    final results =
        ref.watch(searchProvider).value?.filteredResults ?? const [];
    final stops = <_PlanStop>[];
    for (final item in results) {
      if (item is! SearchDiscoverItem) continue;
      if (item.searchResult.itemType !=
          SearchItemType.SEARCH_ITEM_TYPE_EXPERIENCE) {
        continue;
      }
      final experience = item.searchResult.experience;
      if (experience.latitudeDeg == 0 && experience.longitudeDeg == 0) {
        continue;
      }
      if (experience.name.isEmpty) continue;
      stops.add(_PlanStop(
        title: experience.name,
        subtitle: experience.time.informalDescription,
        latitude: experience.latitudeDeg,
        longitude: experience.longitudeDeg,
      ));
      if (stops.length == 2) break;
    }
    return stops;
  }

  Widget _sectionLabel(BuildContext context, String label) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(0, 12, 0, 4),
      child: Text(
        label.toUpperCase(),
        style: TextStyle(
          fontSize: 10.5,
          fontWeight: FontWeight.w700,
          letterSpacing: 1.2,
          color: AppColors.textSecondary(context),
        ),
      ),
    );
  }

  Widget _placeField(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 12),
      decoration: BoxDecoration(
        color: AppColors.surface(context),
        borderRadius: BorderRadius.circular(14),
        border: Border.all(color: AppColors.border(context)),
      ),
      child: Row(
        children: [
          Icon(Icons.search,
              size: 18, color: AppColors.textSecondary(context)),
          const SizedBox(width: 8),
          Expanded(
            child: TextField(
              controller: _placeController,
              onChanged: _onPlaceQueryChanged,
              style: TextStyle(
                fontSize: 14,
                color: AppColors.textPrimary(context),
              ),
              decoration: InputDecoration(
                hintText: context.l10n.librarySheetPlaceHint,
                hintStyle: TextStyle(
                  fontSize: 14,
                  color: AppColors.textSecondary(context),
                ),
                border: InputBorder.none,
                isDense: true,
                contentPadding: const EdgeInsets.symmetric(vertical: 13),
              ),
            ),
          ),
        ],
      ),
    );
  }

  /// The Me-sheet row grammar on the solid surface: glyph box, title +
  /// subtitle, and either the Current pill or a chevron.
  Widget _row({
    required IconData icon,
    required String title,
    String? subtitle,
    bool amber = false,
    bool currentPill = false,
    required VoidCallback onTap,
  }) {
    return Tappable(
      semanticsLabel: title,
      onTap: onTap,
      inkBorderRadius: BorderRadius.circular(12),
      child: Padding(
        padding: const EdgeInsets.symmetric(vertical: 8),
        child: Row(
          children: [
            Container(
              width: 34,
              height: 34,
              decoration: BoxDecoration(
                color: amber
                    ? AppColors.rsvpMaybeBackground(context)
                    : AppColors.surface(context),
                borderRadius: BorderRadius.circular(11),
              ),
              child: Icon(
                icon,
                size: 17,
                color: amber
                    ? AppColors.rsvpMaybeText(context)
                    : AppColors.textPrimary(context),
              ),
            ),
            const SizedBox(width: 11),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    title,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: TextStyle(
                      fontSize: 13.5,
                      fontWeight: FontWeight.w600,
                      color: AppColors.textPrimary(context),
                    ),
                  ),
                  if (subtitle != null && subtitle.isNotEmpty)
                    Text(
                      subtitle,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: TextStyle(
                        fontSize: 11.5,
                        color: AppColors.textSecondary(context),
                      ),
                    ),
                ],
              ),
            ),
            if (currentPill)
              Container(
                padding:
                    const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
                decoration: BoxDecoration(
                  color: AppColors.primary(context).withAlpha(31),
                  borderRadius: BorderRadius.circular(9),
                ),
                child: Text(
                  context.l10n.librarySheetCurrentPill,
                  style: TextStyle(
                    fontSize: 11,
                    fontWeight: FontWeight.w600,
                    color: AppColors.primary(context),
                  ),
                ),
              )
            else
              Icon(
                Icons.chevron_right,
                size: 18,
                color: AppColors.textSecondary(context),
              ),
          ],
        ),
      ),
    );
  }

  // ---------------------------------------------------------------------------
  // Result filters — re-homed onto this sheet (was a separate glass modal).
  // "What shows on the map and shelves": categories, People, and Include
  // Completed. Applied immediately via [searchProvider]. Map Style, Distance,
  // and Sort were intentionally dropped from this surface.
  // ---------------------------------------------------------------------------

  Widget _buildFilters(BuildContext context) {
    final filters = ref.watch(
      searchProvider.select((s) => s.value?.filters ?? const DiscoverFilters()),
    );
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        _filterHeader(context, filters),
        _buildCategorySection(context, filters),
        _buildPeopleSection(context, filters),
        const SizedBox(height: 8),
        _buildIncludeCompleted(context, filters),
      ],
    );
  }

  Widget _filterHeader(BuildContext context, DiscoverFilters filters) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(0, 12, 0, 0),
      child: Row(
        children: [
          Expanded(
            child: Text(
              context.l10n.discoverFilterTitle.toUpperCase(),
              style: TextStyle(
                fontSize: 10.5,
                fontWeight: FontWeight.w700,
                letterSpacing: 1.2,
                color: AppColors.textSecondary(context),
              ),
            ),
          ),
          if (filters.activeFilterCount > 0)
            Tappable(
              semanticsLabel: context.l10n.discoverFilterReset,
              onTap: () => ref.read(searchProvider.notifier).resetFilters(),
              child: Padding(
                padding: const EdgeInsets.symmetric(horizontal: 4, vertical: 2),
                child: Text(
                  context.l10n.discoverFilterReset,
                  style: TextStyle(
                    fontSize: 12,
                    fontWeight: FontWeight.w600,
                    color: AppColors.primary(context),
                  ),
                ),
              ),
            ),
        ],
      ),
    );
  }

  // --- Categories (Show on Map) ---

  Widget _buildCategorySection(BuildContext context, DiscoverFilters filters) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        _sectionLabel(context, context.l10n.discoverFilterShowOnMap),
        Row(
          children: [
            Expanded(
              child: _categoryChip(
                context,
                label: context.l10n.discoverFilterRequests,
                category: 'requests',
                icon: Icons.help_outline,
                isActive: filters.showRequests,
              ),
            ),
            const SizedBox(width: 10),
            Expanded(
              child: _categoryChip(
                context,
                label: context.l10n.discoverFilterEvents,
                category: 'events',
                icon: Icons.event_outlined,
                isActive: filters.showEvents,
              ),
            ),
          ],
        ),
        const SizedBox(height: 10),
        Row(
          children: [
            Expanded(
              child: _categoryChip(
                context,
                label: context.l10n.discoverFilterSharing,
                category: 'sharing',
                icon: Icons.swap_horiz_outlined,
                isActive: filters.showSharing,
              ),
            ),
            const SizedBox(width: 10),
            Expanded(
              child: _categoryChip(
                context,
                label: context.l10n.discoverFilterGiving,
                category: 'giving',
                icon: Icons.card_giftcard_outlined,
                isActive: filters.showGiving,
              ),
            ),
          ],
        ),
      ],
    );
  }

  Widget _categoryChip(
    BuildContext context, {
    required String label,
    required String category,
    required IconData icon,
    required bool isActive,
  }) {
    final accent = AppColors.primary(context);
    return Toggle(
      semanticsLabel: context.l10n.a11yDiscoverFilterCategory(label),
      selected: isActive,
      onTap: () => ref.read(searchProvider.notifier).toggleCategory(category),
      inkBorderRadius: BorderRadius.circular(14),
      child: AnimatedContainer(
        duration: accessibleDuration(context, const Duration(milliseconds: 180)),
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
        decoration: BoxDecoration(
          color: isActive
              ? accent.withAlpha(31)
              : AppColors.surface(context),
          borderRadius: BorderRadius.circular(14),
          border: Border.all(
            color: isActive ? accent : AppColors.border(context),
            width: isActive ? 1.5 : 1,
          ),
        ),
        child: Row(
          children: [
            Icon(
              icon,
              size: 18,
              color: isActive ? accent : AppColors.textSecondary(context),
            ),
            const SizedBox(width: 8),
            Expanded(
              child: Text(
                label,
                style: TextStyle(
                  fontSize: 13.5,
                  fontWeight: FontWeight.w600,
                  color:
                      isActive ? accent : AppColors.textPrimary(context),
                ),
              ),
            ),
            Icon(
              isActive ? Icons.check_circle : Icons.circle_outlined,
              size: 18,
              color: isActive ? accent : AppColors.textSecondary(context),
            ),
          ],
        ),
      ),
    );
  }

  // --- People (restrict results to one community member) ---

  Widget _buildPeopleSection(BuildContext context, DiscoverFilters filters) {
    final searchState = ref.watch(searchProvider).value ?? const SearchState();
    final currentUserId =
        ref.watch(authStateProvider.select((s) => s.user?.id));
    final members = searchState.communityMembers(currentUserId: currentUserId);
    if (members.isEmpty) return const SizedBox.shrink();

    final selectedPersonId = filters.selectedPersonId;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        _sectionLabel(context, context.l10n.discoverFilterPeople),
        SizedBox(
          height: 44,
          child: ListView.separated(
            scrollDirection: Axis.horizontal,
            padding: EdgeInsets.zero,
            itemCount: members.length + 1,
            separatorBuilder: (_, _) => const SizedBox(width: 8),
            itemBuilder: (context, index) {
              if (index == 0) {
                return _personChip(
                  context,
                  isSelected: selectedPersonId == null,
                  semanticsLabel: context.l10n.a11yDiscoverPersonAll,
                  label: context.l10n.discoverAllChip,
                  leadingIcon: Icons.star,
                  onTap: () =>
                      ref.read(searchProvider.notifier).filterByPerson(null),
                );
              }
              final member = members[index - 1];
              final isSelected = selectedPersonId == member.userId;
              return _personChip(
                context,
                isSelected: isSelected,
                semanticsLabel:
                    context.l10n.a11yDiscoverPerson(_firstName(member)),
                label: _firstName(member),
                leadingAvatar: member,
                onTap: () => ref.read(searchProvider.notifier).filterByPerson(
                      isSelected ? null : member.userId,
                    ),
              );
            },
          ),
        ),
        const SizedBox(height: 4),
      ],
    );
  }

  Widget _personChip(
    BuildContext context, {
    required bool isSelected,
    required String semanticsLabel,
    required String label,
    required VoidCallback onTap,
    IconData? leadingIcon,
    CommunityMember? leadingAvatar,
  }) {
    final accent = AppColors.primary(context);
    final fg = isSelected ? accent : AppColors.textPrimary(context);
    return Toggle(
      semanticsLabel: semanticsLabel,
      selected: isSelected,
      onTap: onTap,
      inkBorderRadius: BorderRadius.circular(22),
      child: AnimatedContainer(
        duration: accessibleDuration(context, const Duration(milliseconds: 180)),
        curve: Curves.easeOut,
        padding: EdgeInsets.only(
          right: 10,
          left: leadingIcon != null ? 12 : 0,
        ),
        decoration: BoxDecoration(
          color: isSelected
              ? accent.withAlpha(31)
              : AppColors.surface(context),
          borderRadius: BorderRadius.circular(22),
          border: Border.all(
            color: isSelected ? accent : AppColors.border(context),
          ),
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            if (leadingAvatar != null) ...[
              ClipOval(
                child: SizedBox(
                  width: 44,
                  height: 44,
                  child: UserAvatar(user: leadingAvatar.user, radius: 22),
                ),
              ),
              const SizedBox(width: 8),
            ] else if (leadingIcon != null) ...[
              Icon(leadingIcon, size: 14, color: fg),
              const SizedBox(width: 4),
            ],
            Text(
              label,
              style: TextStyle(
                fontSize: 13,
                fontWeight: FontWeight.w600,
                color: fg,
              ),
            ),
          ],
        ),
      ),
    );
  }

  static String _firstName(CommunityMember member) {
    final name = member.displayName.trim();
    if (name.isEmpty) return '';
    return name.split(' ').first;
  }

  // --- Include Completed ---

  Widget _buildIncludeCompleted(BuildContext context, DiscoverFilters filters) {
    return Row(
      children: [
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                context.l10n.discoverFilterIncludeCompleted,
                style: TextStyle(
                  fontSize: 13.5,
                  fontWeight: FontWeight.w600,
                  color: AppColors.textPrimary(context),
                ),
              ),
              const SizedBox(height: 2),
              Text(
                context.l10n.discoverFilterIncludeCompletedSubtitle,
                style: TextStyle(
                  fontSize: 11.5,
                  color: AppColors.textSecondary(context),
                ),
              ),
            ],
          ),
        ),
        const SizedBox(width: 12),
        Switch.adaptive(
          value: filters.includeCompleted,
          onChanged: (_) =>
              ref.read(searchProvider.notifier).toggleIncludeCompleted(),
          activeTrackColor: AppColors.primary(context),
        ),
      ],
    );
  }
}

/// One "Where you'll be" candidate: an upcoming plan with coordinates.
class _PlanStop {
  final String title;
  final String subtitle;
  final double latitude;
  final double longitude;

  const _PlanStop({
    required this.title,
    required this.subtitle,
    required this.latitude,
    required this.longitude,
  });
}
