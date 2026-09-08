import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/cache/stash_cache_manager.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/data/repositories/chat_repository.dart';
import 'package:ripls/data/repositories/experience_repository.dart';
import 'package:ripls/data/repositories/feed_repository.dart';
import 'package:ripls/data/repositories/search_repository.dart';
import 'package:ripls/presentation/viewmodels/location_modal_view_model.dart';
import 'package:ripls/services/auth_state.dart';
import 'package:ripls/services/experience_service.dart';
import 'package:ripls/services/providers.dart';

class _FakeAuthStateNotifier extends AuthStateNotifier {
  @override
  AuthStateData build() => AuthStateData(
        isLoading: false,
        user: User(id: 'user-1', name: 'Tester'),
      );
}

class _FakeFeedRepository extends Fake implements FeedRepository {}

class _FakeChatRepository extends Fake implements ChatRepository {}

class _FakeSearchRepository extends Fake implements SearchRepository {
  @override
  Future<void> invalidateSearches() async {}
}

/// Records every call into the layers the
/// [LocationPollModalViewModel] reaches through the real repository.
/// proposeLocation lives on an extension and is statically dispatched,
/// so we can't override it on a Mockito stub — but the extension just
/// delegates to [ExperienceService.proposeLocation], which we can fake.
class _FakeExperienceService extends Fake implements ExperienceService {
  Experience nextExperience = Experience();
  Exception? proposeError;
  Exception? confirmError;
  Exception? saveError;
  String confirmedLocationId = 'loc-resolved';
  String nextProposalId = 'prop-1';
  final List<({String? locationId, GeocodedLocation? geocoded})> proposeCalls =
      [];
  final List<({String experienceId, String proposalId})> confirmCalls = [];
  final List<({String? id, String? locationId})> saveExperienceCalls = [];

  @override
  Future<GetExperienceResponse> getExperience(
    String id, {
    String? communityId,
  }) async {
    return GetExperienceResponse(experience: nextExperience);
  }

  @override
  Future<List<Experience>> listMyExperiences() async => [];

  @override
  Future<String> saveExperience({
    String? id,
    String? name,
    String? description,
    List<String>? mediaIds,
    String? locationId,
    ExperienceTime? time,
    int? maxParticipants,
    String? sourceUrl,
    ExperienceMetadata? metadata,
    SocialContext? socialContext,
  }) async {
    saveExperienceCalls.add((id: id, locationId: locationId));
    if (saveError != null) throw saveError!;
    if (locationId != null) nextExperience.locationId = locationId;
    return id ?? '';
  }

  @override
  Future<LocationProposal> proposeLocation({
    required String experienceId,
    String? locationId,
    GeocodedLocation? geocoded,
  }) async {
    proposeCalls.add((locationId: locationId, geocoded: geocoded));
    if (proposeError != null) throw proposeError!;
    return LocationProposal()..id = nextProposalId;
  }

  @override
  Future<String> confirmLocation({
    required String experienceId,
    required String proposalId,
  }) async {
    confirmCalls
        .add((experienceId: experienceId, proposalId: proposalId));
    if (confirmError != null) throw confirmError!;
    return confirmedLocationId;
  }
}

void main() {
  late _FakeExperienceService service;
  late ExperienceRepository repository;
  late StashCacheManager cacheManager;
  late ProviderContainer container;

  const experienceId = 'exp-1';
  const oldLocationId = 'loc-old';
  const newLocationId = 'loc-new';

  ProviderContainer buildContainer() {
    return ProviderContainer(
      overrides: [
        experienceRepositoryProvider.overrideWithValue(repository),
        authStateProvider.overrideWith(_FakeAuthStateNotifier.new),
      ],
    );
  }

  setUp(() async {
    cacheManager = StashCacheManager();
    await cacheManager.initialize();
    service = _FakeExperienceService();
    repository = ExperienceRepository(
      cacheManager,
      service,
      _FakeFeedRepository(),
      _FakeChatRepository(),
      _FakeSearchRepository(),
    );
    container = buildContainer();
  });

  tearDown(() {
    container.dispose();
  });

  group('eventLocationId reflects Experience.locationId', () {
    // Guards the "It's set" / Finalized modal regression: after the owner
    // taps Change → Pick a different spot, the Final Spot card must
    // re-render against the experience's new locationId. The modal sources
    // its confirmedLocationId from LocationModalData.eventLocationId, so
    // this view-model field is the single seam to cover.
    test('exposes confirmed locationId in eventLocationId', () async {
      service.nextExperience = Experience()..locationId = oldLocationId;

      final data =
          await container.read(locationModalProvider(experienceId).future);
      expect(data.eventLocationId, oldLocationId);
    });

    test(
        'eventLocationId tracks the latest Experience.locationId across a '
        'pick-a-different-spot flow', () async {
      service.nextExperience = Experience()..locationId = oldLocationId;

      final initial =
          await container.read(locationModalProvider(experienceId).future);
      expect(initial.eventLocationId, oldLocationId);

      // updateLocation swaps Experience.locationId and the LX2 Change menu
      // invalidates the modal. The next build must reflect the new id.
      service.nextExperience = Experience()..locationId = newLocationId;
      container.invalidate(locationModalProvider(experienceId));

      final refreshed =
          await container.read(locationModalProvider(experienceId).future);
      expect(refreshed.eventLocationId, newLocationId);
    });
  });

  group('proposeGeocodedLocationsSequentially', () {
    // Regression: submitting N geocoded spots from the propose modal used
    // to call proposeGeocodedLocation per item, each of which fired
    // ref.invalidateSelf() and disposed the notifier mid-loop. Later items
    // would silently drop, so a 3-option poll showed "2 possible spots"
    // on the event row. The batch helper invalidates exactly once at the
    // end of the loop.
    GeocodedLocation geocoded(String name) =>
        GeocodedLocation(name: name, latitudeDeg: 1, longitudeDeg: 2);

    test('fires proposeLocation once per staged geocoded item', () async {
      await container.read(locationModalProvider(experienceId).future);
      final notifier =
          container.read(locationModalProvider(experienceId).notifier);

      final spots = [geocoded('Park'), geocoded('Cafe'), geocoded('Beach')];
      await notifier.proposeGeocodedLocationsSequentially(spots);

      expect(service.proposeCalls.length, 3);
      expect(service.proposeCalls[0].geocoded?.name, 'Park');
      expect(service.proposeCalls[1].geocoded?.name, 'Cafe');
      expect(service.proposeCalls[2].geocoded?.name, 'Beach');
      // None of the geocoded calls should also carry a saved locationId.
      for (final call in service.proposeCalls) {
        expect(call.locationId, isNull);
      }
    });

    test('empty list is a no-op', () async {
      await container.read(locationModalProvider(experienceId).future);
      final notifier =
          container.read(locationModalProvider(experienceId).notifier);

      await notifier.proposeGeocodedLocationsSequentially(const []);

      expect(service.proposeCalls, isEmpty);
    });

    test('rethrows on failure (modal surfaces the error toast)', () async {
      await container.read(locationModalProvider(experienceId).future);
      final notifier =
          container.read(locationModalProvider(experienceId).notifier);

      service.proposeError = Exception('network down');
      await expectLater(
        notifier.proposeGeocodedLocationsSequentially([geocoded('A')]),
        throwsException,
      );
    });
  });

  group('setSingleLocation', () {
    // Regression for #2598: setSingleLocation with a saved locationId must call
    // SaveExperience directly — not ProposeLocation + ConfirmLocation — so no
    // transient poll is opened and no spurious "Poll ended" chat banner appears.
    // The geocoded branch still uses ProposeLocation (required to materialize a
    // canonical location_id); its residual banner leak is tracked separately.
    GeocodedLocation geocoded(String name) =>
        GeocodedLocation(name: name, latitudeDeg: 1, longitudeDeg: 2);

    test('uses saveExperience directly for a saved locationId (no poll opened)',
        () async {
      service.nextExperience = Experience();

      await container.read(locationModalProvider(experienceId).future);
      final notifier =
          container.read(locationModalProvider(experienceId).notifier);

      await notifier.setSingleLocation(locationId: 'loc-staged');

      // saveExperience was called with the new locationId.
      expect(service.saveExperienceCalls.length, 1);
      expect(service.saveExperienceCalls.first.locationId, 'loc-staged');
      // Regression: propose/confirm poll path must not be taken.
      expect(service.proposeCalls, isEmpty);
      expect(service.confirmCalls, isEmpty);

      // After invalidateSelf(), build() reloads — fake returns locationId set
      // by saveExperience, so mode should be 'confirmed'.
      final state =
          await container.read(locationModalProvider(experienceId).future);
      expect(state.eventLocationId, 'loc-staged');
      expect(state.mode, 'confirmed');
    });

    test('rethrows + rolls state back if saveExperience fails (locationId path)',
        () async {
      service.saveError = Exception('save failed');

      await container.read(locationModalProvider(experienceId).future);
      final notifier =
          container.read(locationModalProvider(experienceId).notifier);
      final before =
          container.read(locationModalProvider(experienceId)).requireValue;

      await expectLater(
        notifier.setSingleLocation(locationId: 'loc-staged'),
        throwsException,
      );

      final after =
          container.read(locationModalProvider(experienceId)).requireValue;
      expect(after.mode, before.mode);
      expect(after.locationPollActive, before.locationPollActive);
      expect(after.locationPollCompleted, before.locationPollCompleted);
    });

    test('proposes then confirms for an inline geocoded location', () async {
      service.nextProposalId = 'prop-geo';
      service.confirmedLocationId = 'loc-materialized';

      await container.read(locationModalProvider(experienceId).future);
      final notifier =
          container.read(locationModalProvider(experienceId).notifier);

      await notifier.setSingleLocation(geocoded: geocoded('Park'));

      expect(service.proposeCalls.length, 1);
      expect(service.proposeCalls.first.locationId, isNull);
      expect(service.proposeCalls.first.geocoded?.name, 'Park');
      expect(service.confirmCalls.length, 1);
      expect(service.confirmCalls.first.proposalId, 'prop-geo');

      final state =
          container.read(locationModalProvider(experienceId)).requireValue;
      expect(state.eventLocationId, 'loc-materialized');
      expect(state.locationPollActive, isFalse);
      expect(state.locationPollCompleted, isTrue);
    });

    test('rethrows + rolls state back if confirm fails (geocoded path)',
        () async {
      service.confirmError = Exception('confirm failed');

      await container.read(locationModalProvider(experienceId).future);
      final notifier =
          container.read(locationModalProvider(experienceId).notifier);
      final before =
          container.read(locationModalProvider(experienceId)).requireValue;

      await expectLater(
        notifier.setSingleLocation(geocoded: geocoded('Park')),
        throwsException,
      );

      final after =
          container.read(locationModalProvider(experienceId)).requireValue;
      expect(after.mode, before.mode);
      expect(after.locationPollActive, before.locationPollActive);
      expect(after.locationPollCompleted, before.locationPollCompleted);
    });
  });

  group('disposal safety', () {
    test('container.dispose() mid-flight does not crash setSingleLocation',
        () async {
      await container.read(locationModalProvider(experienceId).future);
      final notifier =
          container.read(locationModalProvider(experienceId).notifier);

      final future = notifier.setSingleLocation(locationId: 'loc-staged');
      container.dispose();
      try {
        await future;
      } catch (_) {
        // Riverpod may throw on ref access after disposal — expected.
      }
    });
  });
}
