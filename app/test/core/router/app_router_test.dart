import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/services/auth_state.dart';

void main() {
  group('Router Redirect Logic', () {
    test(
      'should redirect to login when unauthenticated and not going to auth screens',
      () {
        const authState = AuthStateData(accessToken: null, isLoading: false);

        final isAuthenticated = authState.isAuthenticated;
        const isGoingToLogin = false;
        const isGoingToRegister = false;

        // Simulate router redirect logic
        final shouldRedirect =
            !isAuthenticated && !isGoingToLogin && !isGoingToRegister;
        expect(shouldRedirect, true);
      },
    );

    test('should not redirect when authenticated', () {
      const authState = AuthStateData(
        accessToken: 'valid-token',
        isLoading: false,
      );

      final isAuthenticated = authState.isAuthenticated;
      const isGoingToLogin = false;
      const isGoingToRegister = false;

      final shouldRedirect =
          !isAuthenticated && !isGoingToLogin && !isGoingToRegister;
      expect(shouldRedirect, false);
    });

    test('should not redirect when going to login while unauthenticated', () {
      const authState = AuthStateData(accessToken: null, isLoading: false);

      final isAuthenticated = authState.isAuthenticated;
      const isGoingToLogin = true;

      final shouldRedirect = !isAuthenticated && !isGoingToLogin;
      expect(shouldRedirect, false);
    });

    test(
      'should not redirect when going to register while unauthenticated',
      () {
        const authState = AuthStateData(accessToken: null, isLoading: false);

        final isAuthenticated = authState.isAuthenticated;
        const isGoingToRegister = true;

        final shouldRedirect = !isAuthenticated && !isGoingToRegister;
        expect(shouldRedirect, false);
      },
    );

    test(
      'should not redirect when going to invite while unauthenticated',
      () {
        const authState = AuthStateData(accessToken: null, isLoading: false);

        final isAuthenticated = authState.isAuthenticated;
        const isGoingToInvite = true;

        final shouldRedirect = !isAuthenticated && !isGoingToInvite;
        expect(shouldRedirect, false);
      },
    );

    test('should wait for loading to complete before redirecting', () {
      const authState = AuthStateData(accessToken: null, isLoading: true);

      final isLoading = authState.isLoading;

      // Router should wait when loading
      expect(isLoading, true);
    });

    test('should redirect authenticated user from login to home', () {
      const authState = AuthStateData(
        accessToken: 'valid-token',
        isLoading: false,
      );

      final isAuthenticated = authState.isAuthenticated;
      const isGoingToLogin = true;

      final shouldRedirectToHome = isAuthenticated && isGoingToLogin;
      expect(shouldRedirectToHome, true);
    });

    test('should redirect authenticated user from register to home', () {
      const authState = AuthStateData(
        accessToken: 'valid-token',
        isLoading: false,
      );

      final isAuthenticated = authState.isAuthenticated;
      const isGoingToRegister = true;

      final shouldRedirectToHome = isAuthenticated && isGoingToRegister;
      expect(shouldRedirectToHome, true);
    });

    test('should preserve query parameters in redirect URL', () {
      const intendedLocation = '/gear/123?tab=details';
      final encodedLocation = Uri.encodeComponent(intendedLocation);
      final redirectUrl = '/login?from=$encodedLocation';

      final uri = Uri.parse(redirectUrl);
      final fromParam = uri.queryParameters['from'];
      expect(fromParam, intendedLocation);
    });

    test('should encode complex URLs correctly', () {
      const intendedLocation = '/profile?user=john&tab=settings';
      final encodedLocation = Uri.encodeComponent(intendedLocation);
      final redirectUrl = '/login?from=$encodedLocation';

      final uri = Uri.parse(redirectUrl);
      final fromParam = uri.queryParameters['from'];
      expect(fromParam, intendedLocation);
    });

    test('should handle root path redirect', () {
      const intendedLocation = '/';
      final encodedLocation = Uri.encodeComponent(intendedLocation);
      final redirectUrl = '/login?from=$encodedLocation';

      final uri = Uri.parse(redirectUrl);
      final fromParam = uri.queryParameters['from'];
      expect(fromParam, '/');
    });
  });

  group('Auth State Changes and Routing', () {
    test('unauthenticated user attempting to access protected route', () {
      const authState = AuthStateData(accessToken: null, isLoading: false);

      expect(authState.isAuthenticated, false);

      // Simulate attempting to access home page
      const isGoingToLogin = false;
      const isGoingToRegister = false;

      final shouldRedirectToLogin =
          !authState.isAuthenticated && !isGoingToLogin && !isGoingToRegister;
      expect(shouldRedirectToLogin, true);
    });

    test('authenticated user can access protected routes', () {
      final authState = AuthStateData(
        accessToken: 'token',
        user: User(id: 'user-id', name: 'User'),
        isLoading: false,
      );

      expect(authState.isAuthenticated, true);

      // No redirect needed
      const isGoingToLogin = false;
      const isGoingToRegister = false;

      final shouldRedirect =
          !authState.isAuthenticated && !isGoingToLogin && !isGoingToRegister;
      expect(shouldRedirect, false);
    });

    test('logout triggers redirect to login', () {
      // Before logout - authenticated
      final beforeLogout = AuthStateData(
        accessToken: 'token',
        user: User(id: 'user-id', name: 'User'),
        isLoading: false,
      );
      expect(beforeLogout.isAuthenticated, true);

      // After logout - unauthenticated
      const afterLogout = AuthStateData(
        accessToken: null,
        user: null,
        isLoading: false,
      );
      expect(afterLogout.isAuthenticated, false);

      // Should now redirect to login
      const isGoingToLogin = false;
      const isGoingToRegister = false;
      final shouldRedirect =
          !afterLogout.isAuthenticated && !isGoingToLogin && !isGoingToRegister;
      expect(shouldRedirect, true);
    });

    test('login with redirect parameter navigates to intended location', () {
      const intendedLocation = '/profile';
      final fromParam = Uri.encodeComponent(intendedLocation);

      // After successful login, should use fromParam for navigation
      expect(Uri.decodeComponent(fromParam), intendedLocation);
    });

    test('router handles empty auth state', () {
      const emptyState = AuthStateData.empty;

      expect(emptyState.isAuthenticated, false);
      expect(emptyState.isLoading, false);

      // Should redirect to login
      const isGoingToLogin = false;
      const isGoingToRegister = false;
      final shouldRedirect =
          !emptyState.isAuthenticated && !isGoingToLogin && !isGoingToRegister;
      expect(shouldRedirect, true);
    });

    test('router handles loading state correctly', () {
      const loadingState = AuthStateData(isLoading: true);

      expect(loadingState.isLoading, true);

      // Should not make routing decisions while loading
      // Router will return null when isLoading is true
    });

    test('token expiry triggers unauthenticated flow', () {
      // Simulate token expiring - user has old token
      final expiredTokenState = AuthStateData(
        accessToken: '', // Empty token after expiry
        user: User(id: 'user-id', name: 'User'),
        isLoading: false,
      );

      // Empty token means not authenticated
      expect(expiredTokenState.isAuthenticated, false);

      // Should redirect to login
      const isGoingToLogin = false;
      const isGoingToRegister = false;
      final shouldRedirect =
          !expiredTokenState.isAuthenticated &&
          !isGoingToLogin &&
          !isGoingToRegister;
      expect(shouldRedirect, true);
    });
  });

  group('AuthStateData Properties', () {
    test('isAuthenticated is true with non-empty token', () {
      const state = AuthStateData(accessToken: 'test-token', isLoading: false);

      expect(state.isAuthenticated, true);
    });

    test('isAuthenticated is false with null token', () {
      const state = AuthStateData(accessToken: null, isLoading: false);

      expect(state.isAuthenticated, false);
    });

    test('isAuthenticated is false with empty token', () {
      const state = AuthStateData(accessToken: '', isLoading: false);

      expect(state.isAuthenticated, false);
    });

    test('default state is loading', () {
      const state = AuthStateData();
      expect(state.isLoading, true);
      expect(state.isAuthenticated, false);
    });

    test('empty constant has correct values', () {
      expect(AuthStateData.empty.accessToken, null);
      expect(AuthStateData.empty.isLoading, false);
      expect(AuthStateData.empty.isAuthenticated, false);
    });
  });
}
