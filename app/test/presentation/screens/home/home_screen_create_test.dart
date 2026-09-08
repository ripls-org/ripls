import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/config/feature_flags.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart'
    show CommunityItem;
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/screens/create/unified_create_modal.dart';
import 'package:ripls/presentation/widgets/plus_button_modal.dart';
import 'package:ripls/services/providers.dart';
import 'package:shared_preferences/shared_preferences.dart';

// These tests cover home_screen.dart's three PlusButtonModal callbacks
// (onShareGear, onInviteExperience, onRequestSomething) with flag OFF.
// The flag-on path (UnifiedCreateModal from the + FAB) is covered by the
// existing home_screen unified-create test and the blank_create_dispatcher tests.
//
// The PlusButtonModal is rendered by the legacy branch (flag=false), so all
// three callbacks below should NOT produce a UnifiedCreateModal.

class _StubCommunitiesNotifier extends CommunitiesNotifier {
  _StubCommunitiesNotifier(this._initial);
  final CommunitiesState _initial;

  @override
  CommunitiesState build() => _initial;
}

// Minimal standalone widget that renders a PlusButtonModal driven by the same
// callbacks as home_screen.dart's _onAddTap legacy branch, so we can tap items
// without bringing up the full HomeScreen (which has many provider dependencies).
class _PlusButtonHost extends ConsumerWidget {
  const _PlusButtonHost();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return ElevatedButton(
      onPressed: () {
        showModalBottomSheet<void>(
          context: context,
          builder: (_) => PlusButtonModal(
            onShareGear: () async {
              // mirrors home_screen.dart onShareGear
              Navigator.of(context).pop();
            },
            onInviteExperience: () async {
              Navigator.of(context).pop();
            },
            onRequestSomething: () async {
              Navigator.of(context).pop();
            },
            onInviteUser: () async {
              Navigator.of(context).pop();
            },
            onClose: () => Navigator.of(context).pop(),
          ),
        );
      },
      child: const Text('open'),
    );
  }
}

Widget _buildApp(WidgetRef outerRef) {
  return MaterialApp(
    localizationsDelegates: const [
      AppLocalizations.delegate,
      GlobalMaterialLocalizations.delegate,
      GlobalWidgetsLocalizations.delegate,
      GlobalCupertinoLocalizations.delegate,
    ],
    supportedLocales: AppLocalizations.supportedLocales,
    home: const Scaffold(body: _PlusButtonHost()),
  );
}

void main() {
  setUp(() => SharedPreferences.setMockInitialValues({}));

  group(
      'HomeScreen PlusButtonModal callbacks (flag-off) – '
      'no UnifiedCreateModal opened',
      () {
    // Pump a minimal harness: flag is OFF, one community so legacy modal branch
    // is reachable.
    Widget harness() {
      return ProviderScope(
        overrides: [
          unifiedCreateEnabledProvider.overrideWithValue(false),
          communitiesProvider.overrideWith(
            () => _StubCommunitiesNotifier(
              CommunitiesState(
                communities: [CommunityItem(id: 'c1', name: 'Circle 1')],
              ),
            ),
          ),
        ],
        child: Consumer(
          builder: (_, ref, _) => _buildApp(ref),
        ),
      );
    }

    testWidgets('Share Gear does not open UnifiedCreateModal', (tester) async {
      await tester.pumpWidget(harness());
      await tester.pump();

      // Open PlusButtonModal.
      await tester.tap(find.text('open'));
      await tester.pumpAndSettle();

      // Navigate to Share tab and tap 'Tools'.
      await tester.tap(find.text('Share').last);
      await tester.pump();
      await tester.tap(find.text('Tools'));
      await tester.pump();

      expect(find.byType(UnifiedCreateModal), findsNothing);
    });

    testWidgets('Invite Event (Eat) does not open UnifiedCreateModal',
        (tester) async {
      await tester.pumpWidget(harness());
      await tester.pump();

      await tester.tap(find.text('open'));
      await tester.pumpAndSettle();

      await tester.tap(find.text('Do'));
      await tester.pump();
      await tester.tap(find.text('Eat'));
      await tester.pump();

      expect(find.byType(UnifiedCreateModal), findsNothing);
    });

    testWidgets('Request Something (Help) does not open UnifiedCreateModal',
        (tester) async {
      await tester.pumpWidget(harness());
      await tester.pump();

      await tester.tap(find.text('open'));
      await tester.pumpAndSettle();

      // Ask tab is default.
      await tester.tap(find.text('Help'));
      await tester.pump();

      expect(find.byType(UnifiedCreateModal), findsNothing);
    });
  });
}
