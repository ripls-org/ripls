import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/data/gen/ripls/api/gear_service.pb.dart'
    show GearItem;
import 'package:ripls/data/repositories/search_repository.dart'
    show SearchItemType;
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/screens/create/unified_create_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/accessibility/toggle.dart';
import 'package:ripls/presentation/widgets/chat/cached_media_image.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass_sheet.dart';
import 'package:ripls/services/device_location_service.dart'
    show LocationIntent;
import 'package:ripls/services/providers.dart'
    show gearRepositoryProvider, mediaUrlProvider, searchRepositoryProvider;
import 'package:ripls/services/providers/user_location_provider.dart';

final _log = Logger('NeedsClaimGearPicker');

/// Result returned from [NeedsClaimGearPicker] when the user confirms.
///
/// A null [gearId] means the user explicitly cleared the link
/// ("No item"). A null result (sheet dismissed without confirming)
/// should be treated as "no change" by callers. The optional [gear]
/// snapshot is populated whenever the result was triggered by a row
/// tap or a freshly-captured item — callers can use it to render the
/// selection inline without an extra round-trip. It's null when the
/// user dismissed the picker via the close button or cleared the
/// selection.
class NeedsClaimGearPickerResult {
  final String? gearId;
  final GearItem? gear;
  const NeedsClaimGearPickerResult({required this.gearId, this.gear});
}

/// Single-select picker that lets a contributor link one of the items
/// in their library to a claim or contribution.
///
/// Search reuses the same infrastructure as the @-mention lookup in
/// chat: [searchRepositoryProvider] with `SEARCH_STRATEGY_EXACT` and
/// `SEARCH_ITEM_TYPE_GEAR`. Search results are intersected with the
/// caller's owned gear so the picker only ever surfaces items the
/// user is allowed to link. When [communityId] is empty the picker
/// degrades gracefully — the suggested section is hidden and the full
/// owned-library is rendered as the only section.
class NeedsClaimGearPicker extends ConsumerStatefulWidget {
  /// Optional community context for the gear search scope. Empty
  /// string falls back to a library-only picker (no search).
  final String? communityId;

  /// Default query text. Typically the parent Need's title so the
  /// suggested section auto-fills with the most relevant items.
  final String? initialQuery;

  /// Pre-selected gear ID (when editing an existing claim).
  final String? initialGearId;

  const NeedsClaimGearPicker({
    super.key,
    this.communityId,
    this.initialQuery,
    this.initialGearId,
  });

  /// Opens the picker as an accessible modal sheet. Returns the
  /// confirmed result, or null when the user dismisses without
  /// confirming.
  static Future<NeedsClaimGearPickerResult?> show(
    BuildContext context, {
    String? communityId,
    String? initialQuery,
    String? initialGearId,
  }) {
    return showAccessibleModal<NeedsClaimGearPickerResult>(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (_) => NeedsClaimGearPicker(
        communityId: communityId,
        initialQuery: initialQuery,
        initialGearId: initialGearId,
      ),
    );
  }

  @override
  ConsumerState<NeedsClaimGearPicker> createState() =>
      _NeedsClaimGearPickerState();
}

class _NeedsClaimGearPickerState
    extends ConsumerState<NeedsClaimGearPicker> {
  late final TextEditingController _search;

  /// Currently-selected gear ID; null means "no item linked." Starts
  /// from [initialGearId] so the picker opens with the existing
  /// selection highlighted.
  String? _selectedId;

  /// All gear owned by the current user, lazily resolved.
  Future<List<GearItem>>? _libraryFuture;

  /// Current live search query. Empty until the user types something —
  /// the picker no longer pre-seeds the input from a parent hint.
  String _query = '';

  /// Bumped on every keystroke so an in-flight semantic search whose
  /// result returns after a newer one stamps cleanly aborts instead of
  /// overwriting the visible list.
  int _searchToken = 0;

  /// Gear IDs returned by the latest semantic search, in score order.
  /// Null means "no search has run for the current query yet" — the
  /// view renders the most-recent library fallback. An empty list
  /// means the search ran and produced no matches.
  List<String>? _searchResultIds;

  /// True while a semantic search is in flight; drives a discreet
  /// spinner in place of the section labels.
  bool _isSearching = false;

  @override
  void initState() {
    super.initState();
    // Intentionally NOT seeded from `widget.initialQuery` — the picker
    // opens to the helper's recent library and only kicks off a
    // semantic search once they actually type. The need's title is
    // still available as `widget.initialQuery` for future use (e.g.
    // logging) but no longer drives the visible list.
    _search = TextEditingController();
    _selectedId = widget.initialGearId;
    _libraryFuture = ref.read(gearRepositoryProvider).listUserGear();
  }

  @override
  void dispose() {
    _search.dispose();
    super.dispose();
  }

  Future<void> _onQueryChanged(String query) async {
    final trimmed = query.trim();
    setState(() {
      _query = trimmed;
      if (trimmed.isEmpty) {
        _searchResultIds = null;
        _isSearching = false;
      } else {
        _isSearching = true;
      }
    });
    if (trimmed.isEmpty) return;
    final token = ++_searchToken;
    await _runSemanticSearch(trimmed, token);
  }

  /// Runs the unified semantic-search RPC scoped to gear in the
  /// caller's community, then surfaces the result IDs so the list
  /// rerenders intersected with the viewer's owned library. Older
  /// in-flight calls are dropped via [_searchToken] so the visible
  /// list always reflects the most-recent keystroke.
  Future<void> _runSemanticSearch(String query, int token) async {
    final communityId = widget.communityId;
    if (communityId == null || communityId.isEmpty) {
      // No community context = library-only picker; fall back to a
      // local substring filter so the picker still narrows the list
      // when the user types.
      if (!mounted || token != _searchToken) return;
      setState(() {
        _searchResultIds = const <String>[];
        _isSearching = false;
      });
      return;
    }
    try {
      final position = await ref.read(
        userLocationProvider(LocationIntent.proximityBias).future,
      );
      final lat = position?.latitude ?? 30.2672; // Austin, TX fallback
      final lng = position?.longitude ?? -97.7431;
      if (!mounted || token != _searchToken) return;
      final searchRepo = ref.read(searchRepositoryProvider);
      final results = await searchRepo.search(
        query: query,
        communityIds: [communityId],
        latitudeDeg: lat,
        longitudeDeg: lng,
        itemTypes: const [SearchItemType.SEARCH_ITEM_TYPE_GEAR],
      );
      if (!mounted || token != _searchToken) return;
      setState(() {
        _searchResultIds = [
          for (final r in results)
            if (r.hasGear() && r.gear.id.isNotEmpty) r.gear.id,
        ];
        _isSearching = false;
      });
    } catch (e, st) {
      _log.warning('semantic search failed; falling back to substring', e, st);
      if (!mounted || token != _searchToken) return;
      setState(() {
        // Empty list signals "search ran with no usable result" — the
        // builder picks up the empty-state message; the helper can
        // still clear and pick from the recent list.
        _searchResultIds = const <String>[];
        _isSearching = false;
      });
    }
  }

  void _select(GearItem gear) {
    // Tapping the already-selected row toggles it off (single-
    // select with implicit clear) and stays open so the user can
    // pick another item. Tapping a different row commits the
    // selection and closes — the parent sheet then reflects the
    // picked item inline without an extra confirm step.
    if (gear.id == _selectedId) {
      setState(() => _selectedId = null);
      return;
    }
    Navigator.of(context).pop(
      NeedsClaimGearPickerResult(gearId: gear.id, gear: gear),
    );
  }

  void _submit() {
    Navigator.of(context).pop(
      NeedsClaimGearPickerResult(gearId: _selectedId),
    );
  }

  /// Launches the unified create modal (the same one wired to the +
  /// FAB on the home screen — camera / text / hyperlink switcher).
  /// On a successful share, refreshes the user-gear cache, diffs
  /// against the pre-open snapshot to find the newly created gear,
  /// auto-confirms the picker with that gear linked, and fires a
  /// "Saved to your library" toast so the user sees the confirmation
  /// on the parent claim sheet (the picker pops behind it).
  Future<void> _openCapture() async {
    final repo = ref.read(gearRepositoryProvider);
    final before = (await repo.listUserGear()).map((g) => g.id).toSet();
    if (!mounted) return;
    // Claim-to-fulfill sub-flow: capture gear, then link it to the claim. Do
    // NOT auto-open the Share sheet here — the user is picking an item, not
    // sharing one.
    final result = await UnifiedCreateModal.show(context, ref);
    if (result == null || !mounted) return;
    await repo.refreshUserGear();
    if (!mounted) return;
    final library = await repo.listUserGear();
    if (!mounted) return;
    final newItem = library.where((g) => !before.contains(g.id)).firstOrNull;
    if (newItem == null) {
      // Defensive: UnifiedCreateModal reported success but the
      // refreshed library doesn't expose a new ID. Keep the picker
      // open so the user can pick manually instead of silently
      // failing.
      setState(() => _libraryFuture = Future.value(library));
      return;
    }
    ToastHelper.showSuccess(
      context,
      context.l10n.needsClaimSavedToLibraryToast,
    );
    Navigator.of(context).pop(
      // Carry the captured item so callers can name the contribution without
      // a refetch (the contribution composers read `result.gear.name`).
      NeedsClaimGearPickerResult(gearId: newItem.id, gear: newItem),
    );
  }

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    final keyboardInset = MediaQuery.of(context).viewInsets.bottom;
    // Stable results area — every state (loading spinner, empty
    // library, search results, recent library) renders inside the
    // same fixed-height scroll box so the sheet's overall size never
    // jumps as the helper types. `SizedBox` (not `ConstrainedBox`)
    // is intentional: the inner `SingleChildScrollView` would
    // otherwise shrink-wrap to its content height and the modal
    // would breathe with every keystroke. The clamp keeps the value
    // sensible across phone sizes.
    final screenHeight = MediaQuery.of(context).size.height;
    final resultsHeight = (screenHeight * 0.45).clamp(280.0, 460.0);
    return GlassSheet(
      padding: EdgeInsets.zero,
      child: SafeArea(
        top: false,
        child: ConstrainedBox(
          constraints: BoxConstraints(
            maxHeight: screenHeight * 0.85,
          ),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              _Header(
                title: l10n.needsClaimGearPickerTitle,
                addNewLabel: l10n.needsClaimAddNewGear,
                addNewSemanticsLabel: l10n.a11ySnapYourGear,
                onAddNew: _openCapture,
                onClose: _submit,
                closeLabel: l10n.a11yClose,
              ),
              Padding(
                padding: const EdgeInsets.fromLTRB(20, 8, 20, 0),
                child: _SearchPill(
                  controller: _search,
                  hint: l10n.needsClaimGearPickerSearchHint,
                  onChanged: _onQueryChanged,
                ),
              ),
              SizedBox(
                height: resultsHeight,
                child: FutureBuilder<List<GearItem>>(
                  future: _libraryFuture,
                  builder: (context, snap) {
                    if (snap.connectionState == ConnectionState.waiting) {
                      return const Center(
                        child: SizedBox(
                          width: 24,
                          height: 24,
                          child: CircularProgressIndicator(strokeWidth: 2),
                        ),
                      );
                    }
                    final library = snap.data ?? const <GearItem>[];
                    if (library.isEmpty) {
                      return SingleChildScrollView(
                        padding: EdgeInsets.fromLTRB(
                            20, 20, 20, 24 + keyboardInset),
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.stretch,
                          mainAxisSize: MainAxisSize.min,
                          children: [
                            _AddItemRow(
                              title: l10n.needsClaimAddItemTitle,
                              subtitle: l10n.needsClaimAddItemSubtitle,
                              semanticsLabel: l10n.a11ySnapYourGear,
                              onTap: _openCapture,
                            ),
                            const SizedBox(height: 16),
                            Text(
                              l10n.needsClaimGearPickerEmpty,
                              style: const TextStyle(
                                color: AppColors.modalTextSecondary,
                                fontSize: 13,
                              ),
                            ),
                          ],
                        ),
                      );
                    }
                    return _buildList(
                      l10n,
                      library,
                      keyboardInset: keyboardInset,
                    );
                  },
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }

  Widget _buildList(
    AppLocalizations l10n,
    List<GearItem> library, {
    required double keyboardInset,
  }) {
    final hasQuery = _query.isNotEmpty;

    final suggested = <GearItem>[];
    final other = <GearItem>[];
    if (hasQuery) {
      // Semantic search ran. Walk the result IDs in score order and
      // pick out the ones the viewer actually owns (the picker can
      // only link gear the user has access to). Items that match show
      // up under "Suggested"; the rest of the library is hidden until
      // they clear the query — keeping the list focused on what the
      // server thinks is relevant.
      final resultIds = _searchResultIds ?? const <String>[];
      final libraryById = {for (final g in library) g.id: g};
      for (final id in resultIds) {
        final hit = libraryById[id];
        if (hit != null) suggested.add(hit);
      }
    } else {
      // No active query — surface the helper's library sorted by
      // most-recent so the first thing they see is what they shared
      // last. The semantic search hook is dormant until they type.
      other.addAll(library);
      other.sort(
          (a, b) => b.createdAtUnixSec.compareTo(a.createdAtUnixSec));
    }

    final showSearchSpinner = hasQuery && _isSearching;
    final showEmptySearchState = hasQuery && !_isSearching && suggested.isEmpty;

    return SingleChildScrollView(
      padding: EdgeInsets.fromLTRB(20, 12, 20, 12 + keyboardInset),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        mainAxisSize: MainAxisSize.min,
        children: [
          if (showSearchSpinner) ...[
            const Padding(
              padding: EdgeInsets.symmetric(vertical: 16),
              child: Center(
                child: SizedBox(
                  width: 20,
                  height: 20,
                  child: CircularProgressIndicator(strokeWidth: 2),
                ),
              ),
            ),
          ] else if (suggested.isNotEmpty) ...[
            _SectionLabel(label: l10n.needsClaimGearPickerSuggestedHeader),
            const SizedBox(height: 8),
            for (final g in suggested) _gearRow(l10n, g),
          ] else if (showEmptySearchState) ...[
            Padding(
              padding: const EdgeInsets.symmetric(vertical: 16),
              child: Text(
                l10n.needsClaimGearPickerEmpty,
                style: const TextStyle(
                  color: AppColors.modalTextSecondary,
                  fontSize: 13,
                ),
              ),
            ),
          ],
          if (other.isNotEmpty) ...[
            SizedBox(height: suggested.isEmpty ? 0 : 18),
            _SectionLabel(label: l10n.needsClaimGearPickerOtherHeader),
            const SizedBox(height: 8),
            for (final g in other) _gearRow(l10n, g),
          ],
          const SizedBox(height: 14),
          _AddItemRow(
            title: l10n.needsClaimAddItemTitle,
            subtitle: l10n.needsClaimAddItemSubtitle,
            semanticsLabel: l10n.a11ySnapYourGear,
            onTap: _openCapture,
          ),
        ],
      ),
    );
  }

  Widget _gearRow(AppLocalizations l10n, GearItem g) {
    return _GearRow(
      gear: g,
      selected: _selectedId == g.id,
      a11yLabel: l10n.a11yNeedsClaimGearRow(g.name),
      onTap: () => _select(g),
    );
  }
}

/// Picker chrome — back chevron + title + an "Add new gear" action on the
/// right (same create flow as the "Add a new item" row). The back chevron
/// submits the current selection (a popped sheet returns the user to the
/// parent claim flow with whatever they chose, including no change).
class _Header extends StatelessWidget {
  final String title;
  final String addNewLabel;
  final String addNewSemanticsLabel;
  final VoidCallback onAddNew;
  final String closeLabel;
  final VoidCallback onClose;
  const _Header({
    required this.title,
    required this.addNewLabel,
    required this.addNewSemanticsLabel,
    required this.onAddNew,
    required this.closeLabel,
    required this.onClose,
  });

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(8, 8, 16, 4),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.center,
        children: [
          IconAction(
            icon: Icons.expand_more,
            tooltip: closeLabel,
            semanticsLabel: closeLabel,
            onPressed: onClose,
          ),
          Expanded(
            child: Semantics(
              header: true,
              child: Text(
                title,
                style: const TextStyle(
                  color: AppColors.modalTextPrimary,
                  fontSize: 18,
                  fontWeight: FontWeight.w700,
                ),
              ),
            ),
          ),
          Tappable(
            semanticsLabel: addNewSemanticsLabel,
            onTap: onAddNew,
            inkBorderRadius: BorderRadius.circular(999),
            child: Padding(
              padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 4),
              child: Row(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Icon(
                    Icons.add_a_photo_outlined,
                    size: 15,
                    color: AppColors.experienceSageGreen,
                  ),
                  const SizedBox(width: 5),
                  Text(
                    addNewLabel,
                    style: const TextStyle(
                      color: AppColors.experienceSageGreen,
                      fontSize: 13,
                      fontWeight: FontWeight.w600,
                    ),
                  ),
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class _SearchPill extends StatelessWidget {
  final TextEditingController controller;
  final String hint;
  final ValueChanged<String> onChanged;
  const _SearchPill({
    required this.controller,
    required this.hint,
    required this.onChanged,
  });

  @override
  Widget build(BuildContext context) {
    return Container(
      height: 44,
      padding: const EdgeInsets.symmetric(horizontal: 12),
      decoration: BoxDecoration(
        color: GlassTokens.fillFaint,
        border: Border.all(color: AppColors.modalChipBorder),
        borderRadius: BorderRadius.circular(12),
      ),
      child: Row(
        children: [
          const Icon(Icons.search, size: 16, color: AppColors.modalTextMuted),
          const SizedBox(width: 10),
          Expanded(
            child: TextField(
              controller: controller,
              cursorColor: AppColors.modalTextPrimary,
              // autocorrect/enableSuggestions=false suppress the iOS
              // suggestion-bar background tint that otherwise paints
              // a cream highlight under the live text and renders it
              // unreadable on top of the glass-sheet backdrop.
              autocorrect: false,
              enableSuggestions: false,
              autofillHints: const [],
              keyboardType: TextInputType.text,
              textInputAction: TextInputAction.search,
              style: const TextStyle(
                color: AppColors.modalTextPrimary,
                fontSize: 14,
                decoration: TextDecoration.none,
              ),
              decoration: InputDecoration(
                hintText: hint,
                hintStyle: const TextStyle(
                  color: AppColors.modalTextMuted,
                  fontSize: 14,
                ),
                filled: false,
                fillColor: Colors.transparent,
                border: InputBorder.none,
                enabledBorder: InputBorder.none,
                focusedBorder: InputBorder.none,
                isCollapsed: true,
                contentPadding: EdgeInsets.zero,
              ),
              onChanged: onChanged,
            ),
          ),
        ],
      ),
    );
  }
}

class _SectionLabel extends StatelessWidget {
  final String label;
  const _SectionLabel({required this.label});

  @override
  Widget build(BuildContext context) {
    return Text(
      label.toUpperCase(),
      style: const TextStyle(
        fontSize: 10,
        fontWeight: FontWeight.w700,
        letterSpacing: 1.2,
        color: AppColors.modalTextMuted,
      ),
    );
  }
}

/// Library row: square thumbnail on the left (gear's first media or a
/// category-tinted placeholder), name + optional description meta,
/// and a checkbox on the right. The whole row is a [Toggle] so screen
/// readers announce the selected state.
class _GearRow extends ConsumerWidget {
  final GearItem gear;
  final String a11yLabel;
  final bool selected;
  final VoidCallback onTap;
  const _GearRow({
    required this.gear,
    required this.a11yLabel,
    required this.selected,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final meta = gear.hasDescription() && gear.description.isNotEmpty
        ? gear.description
        : null;
    return Toggle(
      semanticsLabel: a11yLabel,
      selected: selected,
      onTap: onTap,
      child: Container(
        margin: const EdgeInsets.only(bottom: 8),
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
        decoration: BoxDecoration(
          color: selected
              ? AppColors.lightAccent.withValues(alpha: 0.16)
              : GlassTokens.fillFaint,
          border: Border.all(
            color: selected
                ? AppColors.lightAccent
                : AppColors.modalChipBorder,
            width: selected ? 1.5 : 1,
          ),
          borderRadius: BorderRadius.circular(14),
        ),
        child: Row(
          children: [
            _Thumbnail(gear: gear),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                mainAxisSize: MainAxisSize.min,
                children: [
                  Text(
                    gear.name,
                    style: const TextStyle(
                      color: AppColors.modalTextPrimary,
                      fontSize: 15,
                      fontWeight: FontWeight.w700,
                    ),
                    overflow: TextOverflow.ellipsis,
                    maxLines: 1,
                  ),
                  if (meta != null) ...[
                    const SizedBox(height: 2),
                    Text(
                      meta,
                      style: const TextStyle(
                        color: AppColors.modalTextSecondary,
                        fontSize: 12,
                      ),
                      overflow: TextOverflow.ellipsis,
                      maxLines: 1,
                    ),
                  ],
                ],
              ),
            ),
            const SizedBox(width: 10),
            _CheckBox(selected: selected),
          ],
        ),
      ),
    );
  }
}

/// Square thumbnail rendered on the leading edge of a [_GearRow]. Uses
/// the gear's first media as the image; falls back to a tinted
/// placeholder with a generic gear glyph when no media exists.
class _Thumbnail extends ConsumerWidget {
  final GearItem gear;
  const _Thumbnail({required this.gear});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    const size = 44.0;
    final radius = BorderRadius.circular(10);
    if (gear.mediaIds.isEmpty) {
      return Container(
        width: size,
        height: size,
        decoration: BoxDecoration(
          color: AppColors.surface(context),
          borderRadius: radius,
        ),
        child: Icon(
          Icons.inventory_2_outlined,
          size: 22,
          color: AppColors.modalTextMuted,
        ),
      );
    }
    final mediaId = gear.mediaIds.first;
    final urlAsync = ref.watch(mediaUrlProvider(mediaId));
    return urlAsync.when(
      loading: () => Container(
        width: size,
        height: size,
        decoration: BoxDecoration(
          color: AppColors.surface(context),
          borderRadius: radius,
        ),
      ),
      error: (_, _) => Container(
        width: size,
        height: size,
        decoration: BoxDecoration(
          color: AppColors.surface(context),
          borderRadius: radius,
        ),
      ),
      data: (url) => SizedBox(
        width: size,
        height: size,
        // semanticsLabel: null — gear name is announced by the
        // enclosing Toggle, the thumbnail is decorative.
        child: CachedMediaImage(
          imageUrl: url,
          cacheKey: 'gear-picker:$mediaId',
          fit: BoxFit.cover,
          borderRadius: radius,
          semanticsLabel: null,
        ),
      ),
    );
  }
}

/// Rounded-square checkbox indicator used on the trailing edge of a
/// gear row. Filled with the sage accent when [selected].
class _CheckBox extends StatelessWidget {
  final bool selected;
  const _CheckBox({required this.selected});

  @override
  Widget build(BuildContext context) {
    return Container(
      width: 22,
      height: 22,
      decoration: BoxDecoration(
        color: selected
            ? AppColors.lightAccent
            : Colors.transparent,
        border: Border.all(
          color: selected
              ? AppColors.lightAccent
              : AppColors.modalTextMuted,
          width: 1.5,
        ),
        borderRadius: BorderRadius.circular(6),
      ),
      child: selected
          ? const Icon(
              Icons.check,
              size: 14,
              color: AppColors.modalBackdrop,
            )
          : null,
    );
  }
}

/// Dashed-border row at the bottom of the list that launches the
/// unified create flow (camera / text / hyperlink — opens on photo
/// capture, the modal's default). Same chrome as the rest of the rows
/// so the user reads it as "another option" instead of a separate
/// affordance.
class _AddItemRow extends StatelessWidget {
  final String title;
  final String subtitle;
  final String semanticsLabel;
  final VoidCallback onTap;
  const _AddItemRow({
    required this.title,
    required this.subtitle,
    required this.semanticsLabel,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: semanticsLabel,
      onTap: onTap,
      child: DottedBorderBox(
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
          child: Row(
            children: [
              Container(
                width: 44,
                height: 44,
                decoration: BoxDecoration(
                  color: AppColors.surface(context),
                  borderRadius: BorderRadius.circular(10),
                ),
                child: const Icon(
                  Icons.add_a_photo_outlined,
                  size: 22,
                  color: AppColors.modalTextMuted,
                ),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Text(
                      title,
                      style: const TextStyle(
                        color: AppColors.modalTextPrimary,
                        fontSize: 15,
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                    const SizedBox(height: 2),
                    Text(
                      subtitle,
                      style: const TextStyle(
                        color: AppColors.modalTextSecondary,
                        fontSize: 12,
                      ),
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                    ),
                  ],
                ),
              ),
              const SizedBox(width: 8),
              const Icon(
                Icons.chevron_right,
                size: 16,
                color: AppColors.modalTextMuted,
              ),
            ],
          ),
        ),
      ),
    );
  }
}

/// Dashed-border container painter. Flutter's `Border` doesn't ship a
/// dashed style, so we draw the dashes manually with [CustomPaint].
class DottedBorderBox extends StatelessWidget {
  final Widget child;
  const DottedBorderBox({super.key, required this.child});

  @override
  Widget build(BuildContext context) {
    return CustomPaint(
      painter: _DashedBorderPainter(),
      child: child,
    );
  }
}

class _DashedBorderPainter extends CustomPainter {
  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = AppColors.modalChipBorder
      ..strokeWidth = 1
      ..style = PaintingStyle.stroke;
    const radius = Radius.circular(14);
    final rect = RRect.fromRectAndRadius(
      Rect.fromLTWH(0, 0, size.width, size.height),
      radius,
    );
    final path = Path()..addRRect(rect);
    final metrics = path.computeMetrics();
    const dashLength = 6.0;
    const gapLength = 4.0;
    for (final metric in metrics) {
      var distance = 0.0;
      while (distance < metric.length) {
        final next = distance + dashLength;
        canvas.drawPath(
          metric.extractPath(distance, next.clamp(0.0, metric.length)),
          paint,
        );
        distance = next + gapLength;
      }
    }
  }

  @override
  bool shouldRepaint(covariant CustomPainter oldDelegate) => false;
}
