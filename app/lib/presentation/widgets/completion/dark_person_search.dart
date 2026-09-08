import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/data/gen/ripls/api/provisional_user.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/completion/dark_check_circle.dart';
import 'package:ripls/presentation/widgets/completion/dark_placeholder_avatar.dart';
import 'package:ripls/presentation/widgets/completion/dark_search_result_tile.dart';
import 'package:ripls/presentation/widgets/provisional_user_avatar.dart';
import 'package:ripls/presentation/widgets/user_avatar.dart';
import 'package:ripls/services/providers.dart';

/// DarkPersonSearch combines the community quick-add grid and the inline search
/// field used by the completion modals.
///
/// It owns its own search controller, debounce state, and community-grid cache
/// so that parent modals do not need to manage any search state. Callbacks
/// fire when the caller should add a person to the confirmed list.
///
/// [excludedMemberIds] and [excludedProvisionalIds] prevent already-added people
/// from appearing in suggestions or search results.
///
/// When [searchCommunityIds] is provided and non-empty, the search and quick-add
/// grid fan out across all listed communities in parallel (deduped by ID).
/// When null or empty, falls back to [{communityId}].
class DarkPersonSearch extends ConsumerStatefulWidget {
  const DarkPersonSearch({
    super.key,
    required this.communityId,
    required this.excludedMemberIds,
    required this.excludedProvisionalIds,
    required this.onMemberSelected,
    required this.onProvisionalSelected,
    required this.onNewProvisionalRequested,
    this.searchHint = 'Search members or people\u2026',
    this.searchCommunityIds,
  });

  final String communityId;
  final Set<String> excludedMemberIds;
  final Set<String> excludedProvisionalIds;
  final void Function(User) onMemberSelected;
  final void Function(ProvisionalUser) onProvisionalSelected;
  final void Function(String name) onNewProvisionalRequested;
  final String searchHint;

  /// When non-null and non-empty, overrides [communityId] for search fan-out.
  final Set<String>? searchCommunityIds;

  @override
  ConsumerState<DarkPersonSearch> createState() => _DarkPersonSearchState();
}

class _DarkPersonSearchState extends ConsumerState<DarkPersonSearch> {
  final _searchController = TextEditingController();
  bool _searching = false;
  bool _isSearching = false;
  List<User> _memberResults = [];
  List<ProvisionalUser> _provisionalResults = [];

  // Persistent grid data (not cleared on cancel).
  List<User> _communityMembers = [];
  List<ProvisionalUser> _communityProvisionals = [];
  bool _gridLoaded = false;

  @override
  void initState() {
    super.initState();
    _searchController.addListener(() => setState(() {}));
    WidgetsBinding.instance.addPostFrameCallback((_) => _preloadGrid());
  }

  @override
  void dispose() {
    _searchController.dispose();
    super.dispose();
  }

  Future<void> _preloadGrid() async {
    if (_gridLoaded || !mounted) return;
    _gridLoaded = true;
    await _runSearch('');
  }

  Set<String> get _effectiveCommunityIds {
    final ids = widget.searchCommunityIds;
    if (ids != null && ids.isNotEmpty) return ids;
    return {widget.communityId};
  }

  Future<void> _runSearch(String query) async {
    setState(() => _isSearching = true);
    try {
      final communityRepo = ref.read(communityRepositoryProvider);
      final provisionalRepo = ref.read(provisionalUserRepositoryProvider);
      final communityIds = _effectiveCommunityIds.toList();

      // Fan out one search per community in parallel, then deduplicate by ID.
      final futures = communityIds.expand(
        (cid) => [
          communityRepo.searchMembers(
            communityId: cid,
            query: query,
            limit: 10,
          ),
          provisionalRepo.searchProvisionalUsers(
            communityId: cid,
            query: query,
            limit: 10,
          ),
        ],
      );
      final allResults = await Future.wait(futures);

      // allResults interleaves [members0, provs0, members1, provs1, ...].
      final seenMemberIds = <String>{};
      final mergedMembers = <User>[];
      final seenProvisionalIds = <String>{};
      final mergedProvisionals = <ProvisionalUser>[];

      for (var i = 0; i < allResults.length; i += 2) {
        for (final u in allResults[i] as List<User>) {
          if (seenMemberIds.add(u.id)) mergedMembers.add(u);
        }
        for (final s in allResults[i + 1] as List<ProvisionalUser>) {
          if (seenProvisionalIds.add(s.id)) mergedProvisionals.add(s);
        }
      }

      if (mounted) {
        setState(() {
          _memberResults = mergedMembers.take(10).toList();
          _provisionalResults = mergedProvisionals.take(10).toList();
          if (query.isEmpty) {
            _communityMembers = _memberResults;
            _communityProvisionals = _provisionalResults;
          }
        });
      }
    } catch (_) {
      // Non-fatal: keep previous results visible.
    } finally {
      if (mounted) setState(() => _isSearching = false);
    }
  }

  void _cancelSearch() {
    _searchController.clear();
    setState(() {
      _searching = false;
      _memberResults = [];
      _provisionalResults = [];
    });
  }

  String _initials(String name) {
    final parts = name.trim().split(' ');
    if (parts.length >= 2) {
      return '${parts.first[0]}${parts.last[0]}'.toUpperCase();
    }
    return name.substring(0, name.length.clamp(0, 2)).toUpperCase();
  }

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        if (!_searching) _buildGrid(context),
        _buildSearchField(context),
        _buildSearchResults(context),
      ],
    );
  }

  /// The signed-in user's id, excluded from every suggestion surface: a
  /// picker that offers the host to herself ("add Leslie" shown to Leslie)
  /// is always wrong, regardless of what the caller passed in
  /// [DarkPersonSearch.excludedMemberIds] (#2724).
  String? get _currentUserId => ref.read(authStateProvider).user?.id;

  bool _isSuggestable(User u) =>
      u.id != _currentUserId && !widget.excludedMemberIds.contains(u.id);

  Widget _buildGrid(BuildContext context) {
    final memberSuggestions =
        _communityMembers.where(_isSuggestable).toList();
    final provisionalSuggestions = _communityProvisionals
        .where((s) => !widget.excludedProvisionalIds.contains(s.id))
        .toList();

    // Each row carries a stable per-person key. The grid reflows whenever a
    // selection adds someone to excludedMemberIds (they drop out of the grid),
    // so without keys Flutter would reconcile the surviving rows by position —
    // recycling one person's element (and its semantics node) for the person
    // who shifted into that slot. On Flutter web that mis-binds the CanvasKit
    // semantic overlay, so a tap on the face labelled "Elena" can select
    // whoever moved into Elena's old slot. Keying by id keeps each element,
    // and its semantics, bound to its own person across the reflow (#2492).
    final items =
        <({Key key, Widget avatar, String firstName, VoidCallback onTap})>[];
    for (final u in memberSuggestions) {
      if (items.length >= 5) break;
      items.add((
        key: ValueKey('member-${u.id}'),
        avatar: UserAvatar(user: u, radius: 19),
        firstName: u.name.split(' ').first,
        onTap: () => widget.onMemberSelected(u),
      ));
    }
    for (final s in provisionalSuggestions) {
      if (items.length >= 5) break;
      items.add((
        key: ValueKey('provisional-${s.id}'),
        avatar: ProvisionalUserAvatar(name: s.name, radius: 19),
        firstName: s.name.split(' ').first,
        onTap: () => widget.onProvisionalSelected(s),
      ));
    }

    if (items.isEmpty) return const SizedBox.shrink();

    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 16, 16, 0),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            _effectiveCommunityIds.length > 1
                ? context.l10n.completionAddFromYourCommunities
                : context.l10n.completionAddFromYourCommunity,
            style: TextStyle(
              fontSize: 10,
              fontWeight: FontWeight.w600,
              letterSpacing: 1,
              color: CompletionColors.textSecondary(context),
            ),
          ),
          const SizedBox(height: 10),
          Row(
            mainAxisAlignment: MainAxisAlignment.start,
            children: items.map((item) {
              return Padding(
                key: item.key,
                padding: const EdgeInsets.only(right: 16),
                child: Tappable(
                  semanticsLabel: item.firstName,
                  onTap: item.onTap,
                  excludeChildSemantics: false,
                  child: Column(
                    children: [
                      Stack(
                        clipBehavior: Clip.none,
                        children: [
                          item.avatar,
                          Positioned(
                            bottom: -1,
                            right: -1,
                            child: Container(
                              width: 16,
                              height: 16,
                              decoration: BoxDecoration(
                                color: CompletionColors.gridBadgeBackground(
                                  context,
                                ),
                                shape: BoxShape.circle,
                                border: Border.all(
                                  color: CompletionColors.gridBadgeBorder(
                                    context,
                                  ),
                                  width: 1.5,
                                ),
                              ),
                              child: const Icon(
                                Icons.add,
                                size: 10,
                                color: Colors.white,
                              ),
                            ),
                          ),
                        ],
                      ),
                      const SizedBox(height: 5),
                      Text(
                        item.firstName,
                        style: TextStyle(
                          fontSize: 11,
                          fontWeight: FontWeight.w500,
                          color: CompletionColors.textPrimary(context),
                        ),
                      ),
                    ],
                  ),
                ),
              );
            }).toList(),
          ),
        ],
      ),
    );
  }

  Widget _buildSearchField(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 12, 16, 0),
      child: Tappable(
        semanticsLabel: widget.searchHint,
        onTap: () {
          if (!_searching) {
            setState(() => _searching = true);
            _runSearch('');
          }
        },
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
          decoration: BoxDecoration(
            color: Colors.transparent,
            borderRadius: BorderRadius.circular(12),
            border: Border.all(
              color: CompletionColors.searchFieldBorder(context),
            ),
          ),
          child: Row(
            children: [
              Text(
                '⌕',
                style: TextStyle(
                  fontSize: 14,
                  color: CompletionColors.searchIcon(context),
                ),
              ),
              const SizedBox(width: 10),
              Expanded(
                child: _searching
                    ? TextField(
                        controller: _searchController,
                        autofocus: true,
                        cursorColor: CompletionColors.textPrimary(context),
                        style: TextStyle(
                          color: CompletionColors.textPrimary(context),
                          fontSize: 14,
                        ),
                        decoration: InputDecoration(
                          isCollapsed: true,
                          border: InputBorder.none,
                          filled: false,
                          hintText: widget.searchHint,
                          hintStyle: TextStyle(
                            color: CompletionColors.searchPlaceholder(context),
                            fontSize: 14,
                          ),
                        ),
                        onChanged: (v) => _runSearch(v.trim()),
                      )
                    : Text(
                        widget.searchHint,
                        style: TextStyle(
                          fontSize: 14,
                          color: CompletionColors.searchPlaceholder(context),
                        ),
                      ),
              ),
              if (_searching)
                Tappable(
                  semanticsLabel: context.l10n.commonCancel,
                  onTap: _cancelSearch,
                  child: Text(
                    'Cancel',
                    style: TextStyle(
                      fontSize: 12,
                      color: CompletionColors.textDim(context),
                    ),
                  ),
                ),
            ],
          ),
        ),
      ),
    );
  }

  Widget _buildSearchResults(BuildContext context) {
    final query = _searchController.text.trim();
    if (!_searching || query.isEmpty) return const SizedBox.shrink();

    final filteredMembers = _memberResults.where(_isSuggestable).toList();
    final filteredProvisionals = _provisionalResults
        .where((s) => !widget.excludedProvisionalIds.contains(s.id))
        .toList();
    final hasResults =
        filteredMembers.isNotEmpty || filteredProvisionals.isNotEmpty;

    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 6, 16, 0),
      child: Container(
        decoration: BoxDecoration(
          color: CompletionColors.containerFill(context),
          borderRadius: BorderRadius.circular(12),
          border: Border.all(color: CompletionColors.glassBorder(context)),
        ),
        child: _isSearching && !hasResults
            ? Padding(
                padding: const EdgeInsets.all(16),
                child: Center(
                  child: SizedBox(
                    width: 20,
                    height: 20,
                    child: CircularProgressIndicator(
                      strokeWidth: 2,
                      color: CompletionColors.textSecondary(context),
                    ),
                  ),
                ),
              )
            : Column(
                children: [
                  ...filteredMembers.map(
                    (u) => DarkSearchResultTile(
                      key: ValueKey('member-${u.id}'),
                      avatar: UserAvatar(user: u, radius: 14),
                      name: u.name,
                      onTap: () {
                        widget.onMemberSelected(u);
                        _cancelSearch();
                      },
                    ),
                  ),
                  ...filteredProvisionals.map(
                    (s) => DarkSearchResultTile(
                      key: ValueKey('provisional-${s.id}'),
                      avatar: ProvisionalUserAvatar(name: s.name, radius: 14),
                      name: s.name,
                      subtitle: 'Not yet on Ripls',
                      onTap: () {
                        widget.onProvisionalSelected(s);
                        _cancelSearch();
                      },
                    ),
                  ),
                  if (!hasResults)
                    DarkSearchResultTile(
                      avatar: DarkPlaceholderAvatar(
                        initials: _initials(query),
                        radius: 14,
                      ),
                      name: 'Add "$query"',
                      subtitle: 'Create placeholder',
                      trailingLabel: '+',
                      onTap: () {
                        widget.onNewProvisionalRequested(query);
                        _cancelSearch();
                      },
                    ),
                ],
              ),
      ),
    );
  }
}
