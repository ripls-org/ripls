import 'package:fixnum/fixnum.dart' as fixnum;
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/data/gen/ripls/api/location.pb.dart' show Location;
import 'package:ripls/data/gen/ripls/api/location_service.pb.dart' show UserLocationWithDetails;
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/data/repositories/location_repository.dart';
import 'package:ripls/presentation/viewmodels/profile_locations_view_model.dart';
import 'package:ripls/services/auth_state.dart';
import 'package:ripls/services/providers.dart';
import 'package:ripls/services/user_service.dart';

import 'profile_locations_view_model_test.mocks.dart';

@GenerateMocks([LocationRepository, UserService])
void main() {
  late ProviderContainer container;
  late MockLocationRepository mockLocationRepository;
  late MockUserService mockUserService;

  // Helper to create a mock AuthStateNotifier that returns specific data
  AuthStateNotifier createMockAuthNotifier(User? user) {
    return _MockAuthStateNotifier(user);
  }

  setUp(() {
    mockLocationRepository = MockLocationRepository();
    mockUserService = MockUserService();

    container = ProviderContainer(
      overrides: [
        locationRepositoryProvider.overrideWithValue(mockLocationRepository),
        userServiceProvider.overrideWithValue(mockUserService),
        authStateProvider.overrideWith(
          () => createMockAuthNotifier(User(id: 'user-123', name: 'Test User')),
        ),
      ],
    );
  });

  tearDown(() {
    container.dispose();
    reset(mockLocationRepository);
    reset(mockUserService);
  });

  group('ProfileLocationsNotifier', () {
    test('initial state is loading', () {
      final state = container.read(profileLocationsProvider);

      expect(state.isLoading, isTrue);
      expect(state.locations, isEmpty);
      expect(state.primaryLocationId, isNull);
      expect(state.error, isNull);
    });

    test('state getters work correctly', () {
      // Test hasError
      const errorState = ProfileLocationsState(
          error: UserError.generic(fallback: 'Some error'));
      expect(errorState.hasError, isTrue);

      // Test isEmpty
      final emptyState = ProfileLocationsState(locations: [], isLoading: false);
      expect(emptyState.isEmpty, isTrue);

      // Test hasLocations
      final withLocationsState = ProfileLocationsState(
        locations: [
          UserLocationWithDetails(
            location: Location(id: 'loc-1'),
            lastUsedAtUnixSec: fixnum.Int64(1000),
          ),
        ],
      );
      expect(withLocationsState.hasLocations, isTrue);
      expect(withLocationsState.isEmpty, isFalse);
    });

    test('clearError removes error message', () {
      // Start with error state
      final notifier = container.read(profileLocationsProvider.notifier);
      notifier.state = notifier.state.copyWith(
        error: const UserError.generic(fallback: 'Test error'),
        isLoading: false,
      );

      expect(container.read(profileLocationsProvider).hasError, isTrue);

      // Clear error
      notifier.clearError();

      final state = container.read(profileLocationsProvider);
      expect(state.hasError, isFalse);
      expect(state.error, isNull);
    });
  });
}

// Mock AuthStateNotifier for testing
class _MockAuthStateNotifier extends AuthStateNotifier {
  final User? _user;

  _MockAuthStateNotifier(this._user);

  @override
  AuthStateData build() {
    return AuthStateData(
      user: _user,
      accessToken: _user != null ? 'mock-token' : null,
      isLoading: false,
    );
  }
}
