import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:logging/logging.dart';
import 'package:ripls/data/gen/ripls/api/user_service.pb.dart';
import 'package:ripls/services/providers.dart';

part 'user_notifications_view_model.freezed.dart';

final _log = Logger('UserNotificationsViewModel');

/// Resolved view of user-scoped notification preferences. Each field is
/// a concrete bool: unset toggles in the underlying proto are resolved
/// to `true` here, so widgets never have to check presence.
///
/// These preferences are NOT per-community — they follow the user
/// across every community they're a member of. Loan return reminders
/// are about "your stuff in your hands"; close prompts are about "your
/// event" — neither is meaningfully community-scoped.
@freezed
sealed class UserNotificationsState with _$UserNotificationsState {
  const factory UserNotificationsState({
    @Default(true) bool notifyLoanReturnReminders,
    @Default(true) bool notifyEventClosePrompts,
  }) = _UserNotificationsState;
}

/// Categories of toggles on the user-scoped notifications screen.
enum UserNotificationCategoryToggle {
  loanReturnReminders,
  eventClosePrompts,
}

const userNotificationCategoryLoanReturnReminders =
    UserNotificationCategoryToggle.loanReturnReminders;
const userNotificationCategoryEventClosePrompts =
    UserNotificationCategoryToggle.eventClosePrompts;

/// AsyncNotifier for the user-scoped notification preferences screen.
///
/// `build()` fetches the row via the repository (cached). Toggle setters
/// optimistically update local state, persist, then reconcile with the
/// server's response.
class UserNotificationsNotifier extends AsyncNotifier<UserNotificationsState> {
  @override
  Future<UserNotificationsState> build() async {
    final repo = ref.read(userRepositoryProvider);
    final prefs = await repo.getNotificationPreferences();
    return _resolve(prefs);
  }

  /// Setter for a single category. Optimistically updates the visible
  /// state, persists, and reconciles. On failure, surfaces an
  /// [AsyncError] and reverts to the prior state.
  Future<void> setCategory(
    UserNotificationCategoryToggle category,
    bool value,
  ) async {
    final current = state.value;
    if (current == null) return;

    final optimistic = _withCategory(current, category, value);
    if (!ref.mounted) return;
    state = AsyncData(optimistic);

    final next = UserNotificationPreferences()
      ..notifyLoanReturnReminders = optimistic.notifyLoanReturnReminders
      ..notifyEventClosePrompts = optimistic.notifyEventClosePrompts;

    final result = await AsyncValue.guard(() async {
      final repo = ref.read(userRepositoryProvider);
      final saved =
          await repo.updateNotificationPreferences(preferences: next);
      return _resolve(saved);
    });

    if (!ref.mounted) return;
    if (result.hasError) {
      _log.warning('failed to update user notification preference', result.error);
      state = AsyncData(current);
      // Surface error so the screen can show a snackbar.
      state = result;
    } else {
      state = result;
    }
  }

  static UserNotificationsState _resolve(UserNotificationPreferences prefs) {
    bool unsetIsOn(bool has, bool value) => has ? value : true;
    return UserNotificationsState(
      notifyLoanReturnReminders: unsetIsOn(
        prefs.hasNotifyLoanReturnReminders(),
        prefs.notifyLoanReturnReminders,
      ),
      notifyEventClosePrompts: unsetIsOn(
        prefs.hasNotifyEventClosePrompts(),
        prefs.notifyEventClosePrompts,
      ),
    );
  }

  static UserNotificationsState _withCategory(
    UserNotificationsState s,
    UserNotificationCategoryToggle cat,
    bool v,
  ) {
    switch (cat) {
      case UserNotificationCategoryToggle.loanReturnReminders:
        return s.copyWith(notifyLoanReturnReminders: v);
      case UserNotificationCategoryToggle.eventClosePrompts:
        return s.copyWith(notifyEventClosePrompts: v);
    }
  }
}

final userNotificationsProvider =
    AsyncNotifierProvider.autoDispose<UserNotificationsNotifier, UserNotificationsState>(
  UserNotificationsNotifier.new,
);
