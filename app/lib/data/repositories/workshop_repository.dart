import 'package:ripls/data/cache/cache_manager.dart';
import 'package:ripls/data/cache/cache_service.dart';
import 'package:ripls/data/gen/ripls/api/workshop_service.pb.dart'
    show
        GenerateWorkshopDraftResponse,
        GetCategoryDetailResponse,
        GetWorkshopBriefResponse,
        GetWorkshopSynthesisResponse,
        HideKnownForCategoryResponse,
        KnownForMode,
        PermanentlyRemoveKnownForCategoryResponse,
        SuppressionScope,
        UndoHideKnownForCategoryResponse;
import 'package:ripls/services/workshop_service.dart';

export 'package:ripls/data/gen/ripls/api/workshop_service.pb.dart'
    show
        BriefCTARow,
        BriefLever,
        BriefPayload,
        BriefSegment,
        GenerateWorkshopDraftResponse,
        GetCategoryDetailResponse,
        GetWorkshopBriefResponse,
        GetWorkshopSynthesisResponse,
        HideKnownForCategoryResponse,
        KnownForMode,
        PermanentlyRemoveKnownForCategoryResponse,
        SuppressionScope,
        SynthesisPanel,
        SynthesisPanelBreakdown,
        UndoHideKnownForCategoryResponse;

/// WorkshopRepository wraps the Workshop service with transparent caching.
///
/// Cache namespaces:
///   - `'workshop'` — synthesis and draft responses (global TTL).
///   - `'workshop_brief'` — daily brief responses (15-min TTL, tighter than
///     the global default because brief content is AI-generated on-demand per open).
class WorkshopRepository {
  final CacheService _cache;
  final CacheService _briefCache;
  final WorkshopService _service;

  WorkshopRepository(CacheManager cacheManager, this._service)
      : _cache = CacheService(cacheManager, 'workshop'),
        _briefCache = CacheService(cacheManager, 'workshop_brief');

  /// Generate an experience draft prefilled from a Workshop action
  /// card's prior instance.
  ///
  /// Not cached — each tap calls fresh because the prior instance's
  /// participant list and metadata may have changed.
  Future<GenerateWorkshopDraftResponse> generateDraft({
    required String experienceId,
  }) {
    return _service.generateWorkshopDraft(experienceId: experienceId);
  }

  /// Get the "Together this season" synthesis panels for a circle scope.
  /// Cached under `'synthesis:<sorted_ids>'`.
  Future<GetWorkshopSynthesisResponse> getSynthesis({
    required List<String> communityIds,
  }) {
    final key = 'synthesis:${(communityIds.toList()..sort()).join(",")}';
    return _cache.get(
      key: key,
      fetch: () => _service.getWorkshopSynthesis(communityIds: communityIds),
    );
  }

  /// Get the AI-generated daily brief for a circle scope.
  ///
  /// Cached under `'workshop_brief'` namespace with key
  /// `'brief:<sorted_ids>'`. Uses a separate cache namespace from the
  /// synthesis so its TTL can differ — brief content is AI-generated
  /// on-demand and more time-sensitive.
  Future<GetWorkshopBriefResponse> getBrief({
    required List<String> communityIds,
  }) {
    final key = 'brief:${(communityIds.toList()..sort()).join(",")}';
    return _briefCache.get(
      key: key,
      fetch: () => _service.getWorkshopBrief(communityIds: communityIds),
    );
  }

  /// Invalidates the brief cache for a given scope (or all when null).
  Future<void> invalidateBrief({List<String>? communityIds}) {
    if (communityIds == null) {
      return _briefCache.invalidatePattern('brief:*');
    }
    final key = 'brief:${(communityIds.toList()..sort()).join(",")}';
    return _briefCache.invalidate(key);
  }

  /// Clears both the synthesis and brief caches. Used by the
  /// pull-to-refresh path so the next read hits the server.
  Future<void> invalidateAll() async {
    await _cache.invalidatePattern('synthesis:*');
    await _briefCache.invalidatePattern('brief:*');
  }

  /// Get the per-category detail payload that backs the "known for"
  /// chip-detail screen. Cached under the `'workshop'` namespace; the
  /// key includes [mode] so per-user and per-community results don't
  /// collide for the same category + community set.
  Future<GetCategoryDetailResponse> getCategoryDetail({
    required KnownForMode mode,
    String? ownerId,
    required List<String> communityIds,
    required String category,
  }) {
    final key = _categoryDetailKey(
      mode: mode,
      ownerId: ownerId,
      communityIds: communityIds,
      category: category,
    );
    return _cache.get(
      key: key,
      fetch: () => _service.getCategoryDetail(
        mode: mode,
        ownerId: ownerId,
        communityIds: communityIds,
        category: category,
      ),
    );
  }

  /// Drops the cached category-detail response for one
  /// (mode, owner, communities, category) tuple. Called by the
  /// detail screen's pull-to-refresh path and by the
  /// hide / permanently-delete mutations.
  Future<void> invalidateCategoryDetail({
    required KnownForMode mode,
    String? ownerId,
    required List<String> communityIds,
    required String category,
  }) {
    final key = _categoryDetailKey(
      mode: mode,
      ownerId: ownerId,
      communityIds: communityIds,
      category: category,
    );
    return _cache.invalidate(key);
  }

  String _categoryDetailKey({
    required KnownForMode mode,
    String? ownerId,
    required List<String> communityIds,
    required String category,
  }) {
    final modeSegment =
        mode == KnownForMode.KNOWN_FOR_MODE_PER_USER ? 'user' : 'community';
    final owner = (ownerId == null || ownerId.isEmpty) ? '' : ownerId;
    final sortedIds = (communityIds.toList()..sort()).join(',');
    final normCategory = category.trim().toLowerCase();
    return 'category_detail:$modeSegment:$owner:$normCategory:$sortedIds';
  }

  /// Hide a "known for" category for the given scope. Invalidates the
  /// brief and the matching detail cache entry on success so the
  /// chip disappears from the next read.
  Future<HideKnownForCategoryResponse> hideKnownForCategory({
    required SuppressionScope scopeKind,
    required String scopeId,
    required String category,
    List<String> invalidateCommunityIds = const <String>[],
    String? invalidateOwnerId,
    required KnownForMode invalidateMode,
  }) async {
    final resp = await _service.hideKnownForCategory(
      scopeKind: scopeKind,
      scopeId: scopeId,
      category: category,
    );
    await _briefCache.invalidatePattern('brief:*');
    await invalidateCategoryDetail(
      mode: invalidateMode,
      ownerId: invalidateOwnerId,
      communityIds: invalidateCommunityIds,
      category: category,
    );
    return resp;
  }

  /// Undo a recent hide by suppression-row id. Invalidates the brief
  /// cache so the chip reappears on the next read.
  Future<UndoHideKnownForCategoryResponse> undoHideKnownForCategory({
    required String suppressionId,
  }) async {
    final resp =
        await _service.undoHideKnownForCategory(suppressionId: suppressionId);
    await _briefCache.invalidatePattern('brief:*');
    await _cache.invalidatePattern('category_detail:*');
    return resp;
  }

  /// Permanently remove a "known for" category for the given scope.
  /// Same cache-invalidation behaviour as [hideKnownForCategory].
  Future<PermanentlyRemoveKnownForCategoryResponse>
      permanentlyRemoveKnownForCategory({
    required SuppressionScope scopeKind,
    required String scopeId,
    required String category,
    List<String> invalidateCommunityIds = const <String>[],
    String? invalidateOwnerId,
    required KnownForMode invalidateMode,
  }) async {
    final resp = await _service.permanentlyRemoveKnownForCategory(
      scopeKind: scopeKind,
      scopeId: scopeId,
      category: category,
    );
    await _briefCache.invalidatePattern('brief:*');
    await invalidateCategoryDetail(
      mode: invalidateMode,
      ownerId: invalidateOwnerId,
      communityIds: invalidateCommunityIds,
      category: category,
    );
    return resp;
  }
}
