import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:logging/logging.dart';
import 'package:ripls/data/gen/ripls/api/impact.pb.dart';
import 'package:ripls/data/gen/ripls/api/item.pb.dart';
import 'package:ripls/data/repositories/profile_repository.dart';
import 'package:ripls/services/providers/auth_providers.dart';
import 'package:ripls/services/providers/impact_providers.dart';
import 'package:ripls/services/providers/media_providers.dart';
import 'package:ripls/services/providers/profile_providers.dart';

part 'viewer_profile_view_model.freezed.dart';

final _log = Logger('ViewerProfileViewModel');

/// One community the viewer and target both belong to. The link in
/// the identity-block subtitle taps through to the community profile.
@freezed
sealed class SharedCommunity with _$SharedCommunity {
  const factory SharedCommunity({
    required String id,
    required String name,
  }) = _SharedCommunity;
}

/// State for the user profile screen — the target user as seen by
/// the viewing user. Hero / ticker numbers reflect the target's
/// full activity; the shared-community list governs only what the
/// identity block names.
@freezed
sealed class ViewerProfileState with _$ViewerProfileState {
  const factory ViewerProfileState({
    required String targetUserId,
    required String targetName,
    String? targetDescription,
    String? targetMediaId,
    String? targetMediaUrl,
    @Default(<SharedCommunity>[]) List<SharedCommunity> sharedCommunities,
    @Default(0) int otherCommunityCount,
    BriefPayload? payload,
    UserImpactMetrics? impactMetrics,
    @Default(<Item>[])
    List<Item> availableNowItems,
    @Default(<String>[]) List<String> knownFor,
    @Default(false) bool isSelfView,
  }) = _ViewerProfileState;

  const ViewerProfileState._();

  /// Whether the viewer and target share at least one community.
  bool get hasSharedCommunities => sharedCommunities.isNotEmpty;
  }

/// AsyncNotifier for the viewer-scoped profile of [targetUserId].
///
/// Auto-disposes per-target so multiple profile screens stacked in
/// the navigation history don't keep stale state alive. Refresh
/// invalidates the repository entry and re-runs build().
class ViewerProfileNotifier extends AsyncNotifier<ViewerProfileState> {
  ViewerProfileNotifier(this.targetUserId);

  /// Target user id this notifier instance is bound to.
  final String targetUserId;

  @override
  Future<ViewerProfileState> build() async {
    _log.info('loading profile for target=$targetUserId');
    return _load(targetUserId);
  }

  Future<ViewerProfileState> _load(String userId) async {
    // Read auth state synchronously alongside the other ref.read calls
    // — the screen stays pure of provider lookups, and a post-await
    // ref.read after disposal would surface as an unhandled error.
    // See #1996 for why self-view is a load-time decision.
    final selfId = ref.read(authStateProvider).user?.id ?? '';

    // Kick off profile + impact metrics in parallel. Impact-fetch
    // failure is non-fatal — the V4 impact rows widget hides itself
    // when the field is null, so the rest of the screen still renders.
    // The error handler is attached at registration time (not at
    // await time) so an early rejection cannot become an unhandled
    // async error before the caller awaits.
    final profileFuture = ref.read(profileRepositoryProvider).getProfile(userId);
    final impactFuture = ref
        .read(impactMetricsRepositoryProvider)
        .getUserMetrics(userId)
        .then<UserImpactMetrics?>(
          (r) => r.hasMetrics() ? r.metrics : null,
          onError: (Object e) {
            _log.warning('failed to load impact metrics: $e');
            return null;
          },
        );

    final response = await profileFuture;
    final impactMetrics = await impactFuture;

    final isSelfView = selfId.isNotEmpty && response.targetUserId == selfId;

    // Resolve the target's profile media to a renderable URL. Failures
    // are non-fatal — the screen renders a placeholder avatar.
    String? mediaUrl;
    final mediaId = response.hasTargetMediaId() ? response.targetMediaId : '';
    if (mediaId.isNotEmpty) {
      try {
        final media =
            await ref.read(mediaRepositoryProvider).getFullMediaUrl(mediaId);
        mediaUrl = media.url;
      } catch (e) {
        _log.warning('failed to resolve target media url: $e');
      }
    }

    return ViewerProfileState(
      targetUserId: response.targetUserId,
      targetName: response.targetName,
      targetDescription:
          response.hasTargetDescription() ? response.targetDescription : null,
      targetMediaId: mediaId.isEmpty ? null : mediaId,
      targetMediaUrl: mediaUrl,
      sharedCommunities: response.sharedCommunities
          .map((c) => SharedCommunity(id: c.id, name: c.name))
          .toList(growable: false),
      otherCommunityCount: response.otherCommunityCount,
      payload: response.hasPayload() ? response.payload : null,
      impactMetrics: impactMetrics,
      availableNowItems:
          response.availableNowItems.toList(growable: false),
      knownFor: response.knownFor.toList(growable: false),
      isSelfView: isSelfView,
    );
  }

  /// Pull-to-refresh handler. Invalidates the cached entries then
  /// re-runs the loader. The post-await `ref.mounted` guard prevents
  /// writing to disposed state.
  Future<void> refresh() async {
    await Future.wait([
      ref.read(profileRepositoryProvider).invalidate(targetUserId),
      ref.read(impactMetricsRepositoryProvider).invalidateUserMetrics(targetUserId),
    ]);
    if (!ref.mounted) return;
    state = const AsyncLoading();
    final next = await AsyncValue.guard(() => _load(targetUserId));
    if (!ref.mounted) return;
    state = next;
  }
}

/// Provider for the viewer-scoped user profile state, keyed by target
/// user id.
final viewerProfileProvider = AsyncNotifierProvider.autoDispose
    .family<ViewerProfileNotifier, ViewerProfileState, String>(
  ViewerProfileNotifier.new,
);
