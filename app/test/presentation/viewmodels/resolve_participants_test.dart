import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/viewmodels/gen_experience_view_model.dart';
import 'package:ripls/services/providers.dart';

import '../../core/observability/analytics_test_helper.dart';
import 'gen_experience_view_model_test.mocks.dart'
    show MockExperienceService, MockExperienceRepository, MockMediaRepository;
import 'manage_members_view_model_test.mocks.dart'
    show MockCommunityRepository, MockProvisionalUserRepository;

void main() {
  late MockExperienceService mockExperienceService;
  late MockExperienceRepository mockExperienceRepository;
  late MockMediaRepository mockMediaRepository;
  late MockCommunityRepository mockCommunityRepo;
  late MockProvisionalUserRepository mockProvisionalRepo;
  late MockObservabilityService mockObservability;
  late ProviderContainer container;

  setUp(() {
    mockExperienceService = MockExperienceService();
    mockExperienceRepository = MockExperienceRepository();
    mockMediaRepository = MockMediaRepository();
    mockCommunityRepo = MockCommunityRepository();
    mockProvisionalRepo = MockProvisionalUserRepository();
    mockObservability = MockObservabilityService();

    container = ProviderContainer(
      overrides: [
        experienceServiceProvider.overrideWithValue(mockExperienceService),
        experienceRepositoryProvider.overrideWithValue(
          mockExperienceRepository,
        ),
        mediaRepositoryProvider.overrideWithValue(mockMediaRepository),
        communityRepositoryProvider.overrideWithValue(mockCommunityRepo),
        provisionalUserRepositoryProvider.overrideWithValue(
          mockProvisionalRepo,
        ),
        observabilityServiceProvider.overrideWithValue(mockObservability),
      ],
    );
  });

  tearDown(() {
    container.dispose();
  });

  group('resolveParticipantsForCompletion', () {
    // TODO(#2040): The pre-#1895 dedup tests have been collapsed into a
    // single null-return assertion because the production code now always
    // returns null (it depended on selectedCommunity, which was always null
    // after #1895 and was deleted in #2023). When the experience creation
    // flow is updated to plumb a community ID through, restore the full
    // member/provisional/dedup test matrix.
    test('returns null pending #2040 community-id plumbing', () async {
      final notifier = container.read(genExperienceProvider.notifier);
      notifier.state = notifier.state.copyWith(mentionedNames: ['Someone']);

      final result = await notifier.resolveParticipantsForCompletion();

      expect(result, isNull);
    });
  });
}
