import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/location.pb.dart';
import 'package:ripls/data/repositories/location_repository.dart';
import 'package:ripls/presentation/viewmodels/location_modal_view_model.dart';
import 'package:ripls/presentation/viewmodels/location_panel_providers.dart';
import 'package:ripls/services/providers.dart';

/// Minimal fake exposing only [get]; everything else throws via noSuchMethod.
class _FakeLocationRepository implements LocationRepository {
  _FakeLocationRepository(this._byId);
  final Map<String, Location> _byId;

  @override
  Future<Location> get(String locationId) async {
    final loc = _byId[locationId];
    if (loc == null) throw StateError('no location $locationId');
    return loc;
  }

  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

/// Fake modal notifier returning fixed state without touching a repository.
class _FakeModalNotifier extends LocationModalNotifier {
  _FakeModalNotifier(this._data) : super('exp');
  final LocationModalData _data;

  @override
  Future<LocationModalData> build() async => _data;
}

LocationProposal _savedProposal(String id, String locationId) {
  return LocationProposal()
    ..id = id
    ..location = (ProposedLocation()..locationId = locationId);
}

LocationProposal _geocodedProposal(
    String id, String name, double lat, double lng) {
  return LocationProposal()
    ..id = id
    ..location = (ProposedLocation()
      ..geocoded = (GeocodedLocation()
        ..name = name
        ..latitudeDeg = lat
        ..longitudeDeg = lng));
}

ProviderContainer _container({
  required LocationModalData data,
  required Map<String, Location> locations,
}) {
  return ProviderContainer(
    overrides: [
      locationRepositoryProvider
          .overrideWithValue(_FakeLocationRepository(locations)),
      locationModalProvider('exp').overrideWith(() => _FakeModalNotifier(data)),
    ],
  );
}

void main() {
  group('locationPanelGeoProvider', () {
    test('resolves saved-location proposals with name, address and coords',
        () async {
      final container = _container(
        data: LocationModalData(
          proposals: [_savedProposal('p1', 'loc-1')],
        ),
        locations: {
          'loc-1': Location()
            ..name = 'Chautauqua Park'
            ..addressLines.add('900 Baseline Rd')
            ..locality = 'Boulder'
            ..latitudeDeg = 40.0
            ..longitudeDeg = -105.28,
        },
      );
      addTearDown(container.dispose);

      final geo = await container.read(locationPanelGeoProvider('exp').future);

      expect(geo.proposals, hasLength(1));
      final r = geo.proposals.single;
      expect(r.name, 'Chautauqua Park');
      expect(r.address, contains('Boulder'));
      expect(r.latitude, 40.0);
      expect(r.longitude, -105.28);
      expect(r.hasCoordinate, isTrue);
    });

    test('resolves inline geocoded proposals without a repository fetch',
        () async {
      final container = _container(
        data: LocationModalData(
          proposals: [_geocodedProposal('p2', 'The Sink', 40.01, -105.27)],
        ),
        locations: const {},
      );
      addTearDown(container.dispose);

      final geo = await container.read(locationPanelGeoProvider('exp').future);

      final r = geo.proposals.single;
      expect(r.name, 'The Sink');
      expect(r.latitude, 40.01);
      expect(r.hasCoordinate, isTrue);
    });

    test('exposes the confirmed location for the "set" state', () async {
      final container = _container(
        data: LocationModalData(
          proposals: [_savedProposal('p1', 'loc-1')],
          eventLocationId: 'loc-1',
          lockedProposalId: 'p1',
        ),
        locations: {
          'loc-1': Location()
            ..name = 'Chautauqua Park'
            ..latitudeDeg = 40.0
            ..longitudeDeg = -105.28,
        },
      );
      addTearDown(container.dispose);

      final geo = await container.read(locationPanelGeoProvider('exp').future);

      expect(geo.confirmed, isNotNull);
      expect(geo.confirmed!.name, 'Chautauqua Park');
      expect(geo.confirmed!.hasCoordinate, isTrue);
    });

    test('keeps a proposal in the list when its location fails to resolve',
        () async {
      final container = _container(
        data: LocationModalData(
          proposals: [_savedProposal('p1', 'missing')],
        ),
        locations: const {},
      );
      addTearDown(container.dispose);

      final geo = await container.read(locationPanelGeoProvider('exp').future);

      expect(geo.proposals, hasLength(1));
      expect(geo.proposals.single.name, 'missing');
      // Unresolved → no coordinate, so it is omitted from the map markers.
      expect(geo.proposals.single.hasCoordinate, isFalse);
    });

    test('treats (0, 0) as no usable coordinate', () {
      final r = ResolvedLocationProposal(
        proposal: LocationProposal(),
        name: 'Null Island',
        latitude: 0,
        longitude: 0,
      );
      expect(r.hasCoordinate, isFalse);
    });
  });
}
