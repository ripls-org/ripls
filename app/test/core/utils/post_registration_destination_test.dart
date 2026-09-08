import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/utils/post_registration_destination.dart';

/// Where a freshly-authenticated user lands.
///
/// **Coverage boundary.** Every interesting branch in this helper is behind
/// `kIsWeb`, which is a compile-time `false` on the VM test target — the web
/// branches are dead-code-eliminated here and cannot be asserted. Their real
/// coverage is the Playwright suite, which drives the actual web bundle:
/// `phone-community-join-full-loop.spec.ts` (#2875), `phone-rsvp-full-loop`,
/// `phone-gear-interest-full-loop`, `phone-request-offer-full-loop`.
///
/// What this file locks down is the *mobile* contract, which is the half the
/// e2e suite can't see: on mobile the deep-link target is resolved from the
/// share-link row after auth, so this helper must return `/` no matter how
/// many carriers it's handed. A future edit that hoists a branch out of the
/// `kIsWeb` guard would strand mobile users on a web-only route.
void main() {
  group('postRegistrationDestination on mobile', () {
    test('returns home with no context at all', () {
      expect(postRegistrationDestination(), '/');
    });

    test('returns home for every carrier the web flows use', () {
      expect(
        postRegistrationDestination(
          shortCode: 'CODE1234',
          experienceId: 'exp-1',
          rsvpIntention: 'yes',
        ),
        '/',
      );
      expect(
        postRegistrationDestination(shortCode: 'CODE1234', gearId: 'gear-1'),
        '/',
      );
      expect(
        postRegistrationDestination(
          shortCode: 'CODE1234',
          requestId: 'req-1',
        ),
        '/',
      );
      expect(
        postRegistrationDestination(
          shortCode: 'CODE1234',
          communityId: 'comm-1',
        ),
        '/',
      );
    });

    test('returns home when an item and its community both ride along', () {
      // A share link that carries an item always also carries that item's
      // community, so this combination is the common case, not an edge one.
      expect(
        postRegistrationDestination(
          shortCode: 'CODE1234',
          gearId: 'gear-1',
          communityId: 'comm-1',
        ),
        '/',
      );
    });
  });
}
