import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart' show Estimate;
import 'package:ripls/data/gen/ripls/api/impact.pb.dart'
    show UserImpactMetrics;
import 'package:ripls/presentation/screens/users/user_screen.dart';
import 'package:ripls/presentation/viewmodels/profile_sheet_view_model.dart';
import 'package:ripls/presentation/viewmodels/viewer_profile_view_model.dart';

import '../../../helpers/l10n_helpers.dart';

/// On-device regression: pushing the v11 [UserScreen] (tapping an
/// avatar on an item card) threw `!semantics.parentDataDirty` and left
/// the route's overlay never-laid-out. This pumps the *real* screen —
/// loading spinner → resolved profile → resolved presence — with
/// semantics enabled, the exact async restructuring that dirtied
/// semantics parent data on device.
class _StubProfile extends ViewerProfileNotifier {
  _StubProfile(super.targetUserId, {this.withPhoto = false});

  final bool withPhoto;

  @override
  Future<ViewerProfileState> build() async {
    return ViewerProfileState(
      targetUserId: targetUserId,
      targetName: 'Jordan Reyes',
      targetDescription: 'Will lend you anything with a motor',
      // The photo state exercises ProfileHeroBackdrop; in the test
      // environment the network image fails and swaps to its error
      // widget mid-frame — a semantics restructure the device also
      // sees while a slow image resolves.
      targetMediaId: withPhoto ? 'm1' : null,
      targetMediaUrl: withPhoto ? 'https://example.com/m1.jpg' : null,
      sharedCommunities: const [
        SharedCommunity(id: 'c1', name: 'Campus Crew'),
        SharedCommunity(id: 'c2', name: 'Hiking Crew'),
      ],
      knownFor: const ['Power tools', 'Trail days', 'Chili'],
      impactMetrics: UserImpactMetrics(
        timeBankedMinutes: Estimate(mean: 12840),
        costSavingsUsd: Estimate(mean: 2410),
        costSavingsCount: 38,
      ),
    );
  }
}

class _StubSheet extends ProfileSheetNotifier {
  _StubSheet(super.targetUserId);

  @override
  Future<ProfileSheetState> build() async => const QuietSheet();
}

void main() {
  testWidgets(
      'pushing UserScreen and resolving its providers does not throw '
      'a semantics assertion', (tester) async {
    final handle = tester.ensureSemantics();

    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          viewerProfileProvider.overrideWith2(_StubProfile.new),
          profileSheetProvider.overrideWith2(_StubSheet.new),
        ],
        child: localizedApp(const UserScreen(userId: 'u1')),
      ),
    );
    // Loading spinner frame, then the async providers resolve and the
    // full v11 column replaces it.
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 16));
    await tester.pumpAndSettle();

    expect(find.text('Jordan Reyes'), findsOneWidget);

    handle.dispose();
  });

  testWidgets(
      'the photo state (ProfileHeroBackdrop) resolving does not throw '
      'a semantics assertion', (tester) async {
    final handle = tester.ensureSemantics();

    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          viewerProfileProvider
              .overrideWith2((arg) => _StubProfile(arg, withPhoto: true)),
          profileSheetProvider.overrideWith2(_StubSheet.new),
        ],
        child: localizedApp(const UserScreen(userId: 'u1')),
      ),
    );
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 16));
    // Bounded pumps instead of pumpAndSettle: the failing network image
    // leaves retry timers behind.
    await tester.pump(const Duration(milliseconds: 300));
    await tester.pump(const Duration(seconds: 1));

    expect(find.text('Jordan Reyes'), findsOneWidget);

    handle.dispose();
  });

  testWidgets(
      'the on-device path — a transparent zero-duration route pushed '
      'over a host while the entry slide runs — does not throw',
      (tester) async {
    final handle = tester.ensureSemantics();

    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          viewerProfileProvider.overrideWith2(_StubProfile.new),
          profileSheetProvider.overrideWith2(_StubSheet.new),
        ],
        child: localizedApp(
          Scaffold(
            body: Builder(
              builder: (context) => Center(
                child: ElevatedButton(
                  // Mirrors ContentViewHelpers.openUserScreen →
                  // NavigationHelpers.pushScreen: transparent route,
                  // zero transition, root navigator; the screen's own
                  // SwipeToCloseMixin slide animates the entry.
                  onPressed: () =>
                      Navigator.of(context, rootNavigator: true).push(
                    PageRouteBuilder<void>(
                      opaque: false,
                      pageBuilder: (_, _, _) =>
                          const UserScreen(userId: 'u1'),
                      transitionsBuilder: (_, _, _, child) => child,
                      transitionDuration: Duration.zero,
                      reverseTransitionDuration: Duration.zero,
                    ),
                  ),
                  child: const Text('avatar'),
                ),
              ),
            ),
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();

    await tester.tap(find.text('avatar'));
    // First frame of the pushed route (loading spinner mid-slide), the
    // provider resolution restructure, then settle through the slide.
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 16));
    await tester.pump(const Duration(milliseconds: 120));
    await tester.pumpAndSettle();

    expect(find.text('Jordan Reyes'), findsOneWidget);

    handle.dispose();
  });
}
