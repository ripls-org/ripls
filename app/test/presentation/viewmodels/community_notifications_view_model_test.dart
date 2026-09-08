import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart';
import 'package:ripls/data/repositories/community_repository.dart';
import 'package:ripls/presentation/viewmodels/community_notifications_view_model.dart';
import 'package:ripls/services/providers.dart';

import 'community_notifications_view_model_test.mocks.dart';

@GenerateMocks([CommunityRepository])
void main() {
  late MockCommunityRepository mockRepo;
  late ProviderContainer container;

  const communityId = 'comm1';

  setUp(() {
    mockRepo = MockCommunityRepository();
    container = ProviderContainer(
      overrides: [
        communityRepositoryProvider.overrideWithValue(mockRepo),
      ],
    );
  });

  tearDown(() => container.dispose());

  test('build resolves unset toggles to true', () async {
    when(mockRepo.getNotificationPreferences(communityId))
        .thenAnswer((_) async => CommunityNotificationPreferences());

    final state = await container
        .read(communityNotificationsProvider(communityId).future);

    expect(state.notifyNewRequests, isTrue);
    expect(state.notifyChats, isTrue);
    expect(state.notifyPlanningUpdates, isTrue);
  });

  test('build respects explicit false in stored prefs', () async {
    final prefs = CommunityNotificationPreferences()
      ..notifyChats = false
      ..notifyTransferUpdates = false;
    when(mockRepo.getNotificationPreferences(communityId))
        .thenAnswer((_) async => prefs);

    final state = await container
        .read(communityNotificationsProvider(communityId).future);

    expect(state.notifyChats, isFalse);
    expect(state.notifyTransferUpdates, isFalse);
    expect(state.notifyNewRequests, isTrue); // unset → on
  });

  test('setCategory persists and updates state', () async {
    when(mockRepo.getNotificationPreferences(communityId))
        .thenAnswer((_) async => CommunityNotificationPreferences());
    when(mockRepo.updateNotificationPreferences(
      communityId: communityId,
      preferences: anyNamed('preferences'),
    )).thenAnswer((invocation) async {
      final input = invocation.namedArguments[#preferences]
          as CommunityNotificationPreferences;
      return input;
    });

    await container.read(communityNotificationsProvider(communityId).future);
    final notifier =
        container.read(communityNotificationsProvider(communityId).notifier);

    await notifier.setCategory(notificationCategoryChats, false);

    final state = container.read(communityNotificationsProvider(communityId)).value!;
    expect(state.notifyChats, isFalse);
    verify(mockRepo.updateNotificationPreferences(
      communityId: communityId,
      preferences: anyNamed('preferences'),
    )).called(1);
  });

  test('disposal during async update does not crash', () async {
    when(mockRepo.getNotificationPreferences(communityId))
        .thenAnswer((_) async => CommunityNotificationPreferences());
    when(mockRepo.updateNotificationPreferences(
      communityId: communityId,
      preferences: anyNamed('preferences'),
    )).thenAnswer((_) async {
      await Future.delayed(const Duration(milliseconds: 100));
      return CommunityNotificationPreferences()..notifyChats = false;
    });

    await container.read(communityNotificationsProvider(communityId).future);
    final notifier =
        container.read(communityNotificationsProvider(communityId).notifier);
    final future = notifier.setCategory(notificationCategoryChats, false);

    // Dispose while the update is in flight.
    container.dispose();

    // Future should settle without throwing.
    await expectLater(future, completes);
  });
}
