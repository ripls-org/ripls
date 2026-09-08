import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/user_service.pb.dart';
import 'package:ripls/data/repositories/user_repository.dart';
import 'package:ripls/presentation/viewmodels/user_notifications_view_model.dart';
import 'package:ripls/services/providers.dart';

import 'user_notifications_view_model_test.mocks.dart';

@GenerateMocks([UserRepository])
void main() {
  late MockUserRepository mockUserRepository;

  setUp(() {
    mockUserRepository = MockUserRepository();
  });

  ProviderContainer createContainer() {
    return ProviderContainer(
      overrides: [
        userRepositoryProvider.overrideWithValue(mockUserRepository),
      ],
    );
  }

  group('UserNotificationsNotifier.build()', () {
    test('resolves unset fields to true (unset = on)', () async {
      // Empty preferences message: no fields set.
      when(mockUserRepository.getNotificationPreferences())
          .thenAnswer((_) async => UserNotificationPreferences());

      final container = createContainer();
      addTearDown(container.dispose);

      final state =
          await container.read(userNotificationsProvider.future);

      expect(state.notifyLoanReturnReminders, isTrue);
      expect(state.notifyEventClosePrompts, isTrue);
    });

    test('respects explicit-false fields', () async {
      when(mockUserRepository.getNotificationPreferences()).thenAnswer(
        (_) async => UserNotificationPreferences()
          ..notifyLoanReturnReminders = false
          ..notifyEventClosePrompts = false,
      );

      final container = createContainer();
      addTearDown(container.dispose);

      final state = await container.read(userNotificationsProvider.future);

      expect(state.notifyLoanReturnReminders, isFalse);
      expect(state.notifyEventClosePrompts, isFalse);
    });

    test('respects explicit-true fields', () async {
      when(mockUserRepository.getNotificationPreferences()).thenAnswer(
        (_) async => UserNotificationPreferences()
          ..notifyLoanReturnReminders = true
          ..notifyEventClosePrompts = true,
      );

      final container = createContainer();
      addTearDown(container.dispose);

      final state = await container.read(userNotificationsProvider.future);

      expect(state.notifyLoanReturnReminders, isTrue);
      expect(state.notifyEventClosePrompts, isTrue);
    });
  });

  group('UserNotificationsNotifier.setCategory()', () {
    test('optimistically toggles state, persists, and reconciles', () async {
      when(mockUserRepository.getNotificationPreferences())
          .thenAnswer((_) async => UserNotificationPreferences());
      when(mockUserRepository.updateNotificationPreferences(
        preferences: anyNamed('preferences'),
      )).thenAnswer((_) async => UserNotificationPreferences()
        ..notifyLoanReturnReminders = false
        ..notifyEventClosePrompts = true);

      final container = createContainer();
      addTearDown(container.dispose);

      // Force the initial build.
      await container.read(userNotificationsProvider.future);

      final notifier = container.read(userNotificationsProvider.notifier);
      await notifier.setCategory(
        userNotificationCategoryLoanReturnReminders,
        false,
      );

      final state = container.read(userNotificationsProvider).value!;
      expect(state.notifyLoanReturnReminders, isFalse);
      expect(state.notifyEventClosePrompts, isTrue);

      // Verify the persisted message carried both fields (the notifier
      // serializes the full optimistic state so the server doesn't
      // confuse a missing field with an explicit-false).
      final captured = verify(mockUserRepository.updateNotificationPreferences(
        preferences: captureAnyNamed('preferences'),
      )).captured.single as UserNotificationPreferences;
      expect(captured.notifyLoanReturnReminders, isFalse);
      expect(captured.hasNotifyLoanReturnReminders(), isTrue);
      expect(captured.notifyEventClosePrompts, isTrue);
      expect(captured.hasNotifyEventClosePrompts(), isTrue);
    });

    test('reverts state and surfaces error when persistence fails', () async {
      when(mockUserRepository.getNotificationPreferences())
          .thenAnswer((_) async => UserNotificationPreferences()
            ..notifyLoanReturnReminders = true
            ..notifyEventClosePrompts = true);
      when(mockUserRepository.updateNotificationPreferences(
        preferences: anyNamed('preferences'),
      )).thenThrow(Exception('network is on fire'));

      final container = createContainer();
      addTearDown(container.dispose);

      await container.read(userNotificationsProvider.future);

      final notifier = container.read(userNotificationsProvider.notifier);
      await notifier.setCategory(
        userNotificationCategoryEventClosePrompts,
        false,
      );

      final async = container.read(userNotificationsProvider);
      expect(async.hasError, isTrue);
    });

    test('updates only the requested category without touching the other', () async {
      when(mockUserRepository.getNotificationPreferences())
          .thenAnswer((_) async => UserNotificationPreferences()
            ..notifyLoanReturnReminders = true
            ..notifyEventClosePrompts = false);
      when(mockUserRepository.updateNotificationPreferences(
        preferences: anyNamed('preferences'),
      )).thenAnswer((invocation) async {
        final next = invocation.namedArguments[const Symbol('preferences')]
            as UserNotificationPreferences;
        return next;
      });

      final container = createContainer();
      addTearDown(container.dispose);

      await container.read(userNotificationsProvider.future);

      final notifier = container.read(userNotificationsProvider.notifier);
      await notifier.setCategory(
        userNotificationCategoryEventClosePrompts,
        true,
      );

      final state = container.read(userNotificationsProvider).value!;
      // The category we just set:
      expect(state.notifyEventClosePrompts, isTrue);
      // The category we did NOT touch should still be true (preserved).
      expect(state.notifyLoanReturnReminders, isTrue);
    });
  });
}
