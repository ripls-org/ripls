import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart'
    show CommunityItem;
import 'package:ripls/data/repositories/workshop_repository.dart'
    show
        BriefPayload,
        GetWorkshopBriefResponse,
        GetWorkshopSynthesisResponse,
        SynthesisPanel,
        WorkshopRepository;
import 'package:ripls/presentation/viewmodels/workshop_view_model.dart';
import 'package:ripls/services/providers/community_providers.dart';
import 'package:ripls/services/providers/workshop_community_provider.dart';
import 'package:ripls/services/providers/workshop_providers.dart';

import 'workshop_view_model_test.mocks.dart';

@GenerateMocks([WorkshopRepository])
void main() {
  group('WorkshopState', () {
    test('defaults to empty with no content', () {
      const state = WorkshopState();
      expect(state.hasBrief, isFalse);
      expect(state.hasSynthesis, isFalse);
      expect(state.hasContent, isFalse);
      expect(state.brief, isNull);
    });

    test('hasSynthesis is true when synthesis has panels', () {
      final state = WorkshopState(
        synthesis: GetWorkshopSynthesisResponse(
          panels: [SynthesisPanel(kind: 'repeat')],
        ),
      );
      expect(state.hasSynthesis, isTrue);
      expect(state.hasContent, isTrue);
    });

    test('hasSynthesis is false when synthesis has no panels', () {
      final state = WorkshopState(
        synthesis: GetWorkshopSynthesisResponse(panels: []),
      );
      expect(state.hasSynthesis, isFalse);
    });

    test('hasBrief is true when briefResponse contains a brief', () {
      final state = WorkshopState(
        briefResponse: GetWorkshopBriefResponse(
          brief: BriefPayload(hoursTogether: 10),
        ),
      );
      expect(state.hasBrief, isTrue);
      expect(state.brief, isNotNull);
      expect(state.brief!.hoursTogether, 10);
      expect(state.hasContent, isTrue);
    });

    test('hasBrief is false when briefResponse has no brief set', () {
      final state = WorkshopState(
        briefResponse: GetWorkshopBriefResponse(),
      );
      expect(state.hasBrief, isFalse);
      expect(state.brief, isNull);
    });
  });

  group('WorkshopNotifier', () {
    late MockWorkshopRepository mockRepository;

    setUp(() {
      mockRepository = MockWorkshopRepository();
    });

    ProviderContainer buildContainer({List<String> communityIds = const []}) {
      return ProviderContainer(
        overrides: [
          workshopRepositoryProvider.overrideWithValue(mockRepository),
          communitiesProvider.overrideWith(
            () => _FakeCommunitiesNotifier(communityIds),
          ),
          // The Workshop notifier reads the derived scope; override it directly
          // so this unit test stays decoupled from how the active community is
          // resolved (single-community model — no "Everything" aggregate).
          workshopEnabledCommunityIdsProvider.overrideWithValue(communityIds),
        ],
      );
    }

    test('returns empty state when no communities are selected', () async {
      final container = buildContainer(communityIds: []);
      addTearDown(container.dispose);

      final state = await container.read(workshopProvider.future);

      expect(state.hasBrief, isFalse);
      expect(state.hasSynthesis, isFalse);
      verifyNever(
        mockRepository.getSynthesis(communityIds: anyNamed('communityIds')),
      );
      verifyNever(
        mockRepository.getBrief(communityIds: anyNamed('communityIds')),
      );
    });

    test('fetches synthesis and brief when communities are available', () async {
      const ids = ['c1', 'c2'];
      when(
        mockRepository.getSynthesis(communityIds: ids),
      ).thenAnswer(
        (_) async => GetWorkshopSynthesisResponse(
          panels: [SynthesisPanel(kind: 'repeat')],
        ),
      );
      when(
        mockRepository.getBrief(communityIds: ids),
      ).thenAnswer(
        (_) async => GetWorkshopBriefResponse(
          brief: BriefPayload(hoursTogether: 5),
        ),
      );

      final container = buildContainer(communityIds: ids);
      addTearDown(container.dispose);

      final state = await container.read(workshopProvider.future);

      expect(state.hasSynthesis, isTrue);
      expect(state.hasBrief, isTrue);
      expect(state.brief!.hoursTogether, 5);
    });

    test('refresh invalidates repository and triggers reload', () async {
      const ids = ['c1'];
      when(
        mockRepository.getSynthesis(communityIds: ids),
      ).thenAnswer((_) async => GetWorkshopSynthesisResponse());
      when(
        mockRepository.getBrief(communityIds: ids),
      ).thenAnswer(
        (_) async => GetWorkshopBriefResponse(
          brief: BriefPayload(hoursTogether: 3),
        ),
      );
      when(mockRepository.invalidateAll()).thenAnswer((_) async {});

      final container = buildContainer(communityIds: ids);
      addTearDown(container.dispose);

      await container.read(workshopProvider.future);

      when(
        mockRepository.getBrief(communityIds: ids),
      ).thenAnswer(
        (_) async => GetWorkshopBriefResponse(
          brief: BriefPayload(hoursTogether: 7),
        ),
      );

      await container.read(workshopProvider.notifier).refresh();

      verify(mockRepository.invalidateAll()).called(1);
      final state = await container.read(workshopProvider.future);
      expect(state.brief!.hoursTogether, 7);
    });
  });
}

/// Fake notifier that returns a [CommunitiesState] populated with the
/// given community IDs so the WorkshopNotifier's community guard can be
/// exercised without wiring up real shared preferences.
class _FakeCommunitiesNotifier extends CommunitiesNotifier {
  _FakeCommunitiesNotifier(this._communityIds);

  final List<String> _communityIds;

  @override
  CommunitiesState build() {
    return CommunitiesState(
      communities:
          _communityIds.map((id) => CommunityItem(id: id, name: id)).toList(),
    );
  }
}
