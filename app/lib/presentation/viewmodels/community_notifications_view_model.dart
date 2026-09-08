import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:logging/logging.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart';
import 'package:ripls/services/providers.dart';

part 'community_notifications_view_model.freezed.dart';

final _log = Logger('CommunityNotificationsViewModel');

/// Resolved view of per-community notification preferences. Each field is a
/// concrete bool: unset toggles in the underlying proto are resolved to
/// `true` here, so widgets never have to check presence.
@freezed
sealed class CommunityNotificationsState with _$CommunityNotificationsState {
  const factory CommunityNotificationsState({
    @Default(true) bool notifyNewRequests,
    @Default(true) bool notifyNewExperiences,
    @Default(true) bool notifyGearShared,
    @Default(true) bool notifyExperienceCompleted,
    @Default(true) bool notifyTransferUpdates,
    @Default(true) bool notifyRequestUpdates,
    @Default(true) bool notifyExperienceRsvps,
    @Default(true) bool notifyPlanningUpdates,
    @Default(true) bool notifyChats,
    @Default(true) bool notifyNewMembers,
    @Default(true) bool notifyRequestFollowupPrompts,
  }) = _CommunityNotificationsState;
}

/// AsyncNotifier for the per-community notification preferences screen.
///
/// `build()` fetches the row via the repository (cached). Toggle setters
/// optimistically update local state, persist, then reconcile with the
/// server's response.
class CommunityNotificationsNotifier
    extends AsyncNotifier<CommunityNotificationsState> {
  CommunityNotificationsNotifier(this.communityId);

  final String communityId;

  @override
  Future<CommunityNotificationsState> build() async {
    final repo = ref.read(communityRepositoryProvider);
    final prefs = await repo.getNotificationPreferences(communityId);
    return _resolve(prefs);
  }

  /// Setter for a single category. Optimistically updates the visible
  /// state, persists, and reconciles. On failure, surfaces an
  /// [AsyncError] and reverts to the prior state.
  Future<void> setCategory(NotificationCategoryToggle category, bool value) async {
    final current = state.value;
    if (current == null) return;

    final optimistic = _withCategory(current, category, value);
    if (!ref.mounted) return;
    state = AsyncData(optimistic);

    final next = CommunityNotificationPreferences()
      ..notifyNewRequests = optimistic.notifyNewRequests
      ..notifyNewExperiences = optimistic.notifyNewExperiences
      ..notifyGearShared = optimistic.notifyGearShared
      ..notifyExperienceCompleted = optimistic.notifyExperienceCompleted
      ..notifyTransferUpdates = optimistic.notifyTransferUpdates
      ..notifyRequestUpdates = optimistic.notifyRequestUpdates
      ..notifyExperienceRsvps = optimistic.notifyExperienceRsvps
      ..notifyPlanningUpdates = optimistic.notifyPlanningUpdates
      ..notifyChats = optimistic.notifyChats
      ..notifyNewMembers = optimistic.notifyNewMembers
      ..notifyRequestFollowupPrompts = optimistic.notifyRequestFollowupPrompts;

    final result = await AsyncValue.guard(() async {
      final repo = ref.read(communityRepositoryProvider);
      final saved = await repo.updateNotificationPreferences(
        communityId: communityId,
        preferences: next,
      );
      return _resolve(saved);
    });

    if (!ref.mounted) return;
    if (result.hasError) {
      _log.warning('failed to update notification preference', result.error);
      state = AsyncData(current);
      // Surface error so the screen can show a snackbar.
      state = result;
    } else {
      state = result;
    }
  }

  static CommunityNotificationsState _resolve(
    CommunityNotificationPreferences prefs,
  ) {
    bool unsetIsOn(bool has, bool value) => has ? value : true;
    return CommunityNotificationsState(
      notifyNewRequests:
          unsetIsOn(prefs.hasNotifyNewRequests(), prefs.notifyNewRequests),
      notifyNewExperiences: unsetIsOn(
          prefs.hasNotifyNewExperiences(), prefs.notifyNewExperiences),
      notifyGearShared:
          unsetIsOn(prefs.hasNotifyGearShared(), prefs.notifyGearShared),
      notifyExperienceCompleted: unsetIsOn(
          prefs.hasNotifyExperienceCompleted(),
          prefs.notifyExperienceCompleted),
      notifyTransferUpdates: unsetIsOn(
          prefs.hasNotifyTransferUpdates(), prefs.notifyTransferUpdates),
      notifyRequestUpdates: unsetIsOn(
          prefs.hasNotifyRequestUpdates(), prefs.notifyRequestUpdates),
      notifyExperienceRsvps: unsetIsOn(
          prefs.hasNotifyExperienceRsvps(), prefs.notifyExperienceRsvps),
      notifyPlanningUpdates: unsetIsOn(
          prefs.hasNotifyPlanningUpdates(), prefs.notifyPlanningUpdates),
      notifyChats: unsetIsOn(prefs.hasNotifyChats(), prefs.notifyChats),
      notifyNewMembers:
          unsetIsOn(prefs.hasNotifyNewMembers(), prefs.notifyNewMembers),
      notifyRequestFollowupPrompts: unsetIsOn(
          prefs.hasNotifyRequestFollowupPrompts(),
          prefs.notifyRequestFollowupPrompts),
    );
  }

  static CommunityNotificationsState _withCategory(
    CommunityNotificationsState s,
    NotificationCategoryToggle cat,
    bool v,
  ) {
    switch (cat) {
      case NotificationCategoryToggle.newRequests:
        return s.copyWith(notifyNewRequests: v);
      case NotificationCategoryToggle.newExperiences:
        return s.copyWith(notifyNewExperiences: v);
      case NotificationCategoryToggle.gearShared:
        return s.copyWith(notifyGearShared: v);
      case NotificationCategoryToggle.experienceCompleted:
        return s.copyWith(notifyExperienceCompleted: v);
      case NotificationCategoryToggle.transferUpdates:
        return s.copyWith(notifyTransferUpdates: v);
      case NotificationCategoryToggle.requestUpdates:
        return s.copyWith(notifyRequestUpdates: v);
      case NotificationCategoryToggle.experienceRsvps:
        return s.copyWith(notifyExperienceRsvps: v);
      case NotificationCategoryToggle.planningUpdates:
        return s.copyWith(notifyPlanningUpdates: v);
      case NotificationCategoryToggle.chats:
        return s.copyWith(notifyChats: v);
      case NotificationCategoryToggle.newMembers:
        return s.copyWith(notifyNewMembers: v);
      case NotificationCategoryToggle.requestFollowupPrompts:
        return s.copyWith(notifyRequestFollowupPrompts: v);
    }
  }
}

/// Categories of notification toggles in the Manage Notifications screen.
enum NotificationCategoryToggle {
  newRequests,
  newExperiences,
  gearShared,
  experienceCompleted,
  transferUpdates,
  requestUpdates,
  experienceRsvps,
  planningUpdates,
  chats,
  newMembers,
  requestFollowupPrompts,
}

const notificationCategoryNewRequests = NotificationCategoryToggle.newRequests;
const notificationCategoryNewExperiences = NotificationCategoryToggle.newExperiences;
const notificationCategoryGearShared = NotificationCategoryToggle.gearShared;
const notificationCategoryExperienceCompleted = NotificationCategoryToggle.experienceCompleted;
const notificationCategoryTransferUpdates = NotificationCategoryToggle.transferUpdates;
const notificationCategoryRequestUpdates = NotificationCategoryToggle.requestUpdates;
const notificationCategoryExperienceRsvps = NotificationCategoryToggle.experienceRsvps;
const notificationCategoryPlanningUpdates = NotificationCategoryToggle.planningUpdates;
const notificationCategoryChats = NotificationCategoryToggle.chats;
const notificationCategoryNewMembers = NotificationCategoryToggle.newMembers;
const notificationCategoryRequestFollowupPrompts = NotificationCategoryToggle.requestFollowupPrompts;

final communityNotificationsProvider = AsyncNotifierProvider.autoDispose
    .family<CommunityNotificationsNotifier, CommunityNotificationsState, String>(
  CommunityNotificationsNotifier.new,
);
