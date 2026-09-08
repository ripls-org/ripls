import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/screens/communities/community_creation_modal.dart';
import 'package:ripls/presentation/viewmodels/gen_community_view_model.dart';

// Stub notifier that exposes a fixed GenCommunityState without firing
// any repository, media-upload, or analytics traffic. Lets us mount the
// modal in tests and exercise the editing buffer. `reset()` is overridden
// to a no-op so the modal's post-frame reset doesn't clobber the seed.
class _StubGenCommunityNotifier extends GenCommunityNotifier {
  _StubGenCommunityNotifier(this._fixedState);
  final GenCommunityState _fixedState;

  @override
  GenCommunityState build() => _fixedState;

  @override
  void reset() {}
}

Widget _hostWithOpenButton({required GenCommunityState seedState}) {
  return ProviderScope(
    overrides: [
      genCommunityProvider.overrideWith(
        () => _StubGenCommunityNotifier(seedState),
      ),
    ],
    child: MaterialApp(
      localizationsDelegates: const [
        AppLocalizations.delegate,
        GlobalMaterialLocalizations.delegate,
        GlobalWidgetsLocalizations.delegate,
        GlobalCupertinoLocalizations.delegate,
      ],
      supportedLocales: AppLocalizations.supportedLocales,
      home: Builder(
        builder: (context) => Scaffold(
          body: Center(
            child: ElevatedButton(
              onPressed: () => CommunityCreationModal.show(context),
              child: const Text('open'),
            ),
          ),
        ),
      ),
    ),
  );
}

void main() {
  group('CommunityCreationModal — glass creation surface', () {
    testWidgets('renders localized hint text + Create Community CTA',
        (tester) async {
      await tester
          .pumpWidget(_hostWithOpenButton(seedState: const GenCommunityState()));
      await tester.tap(find.text('open'));
      await tester.pumpAndSettle();

      // Hint text on the synced fields when the buffer is empty.
      expect(find.text('Community name'), findsOneWidget);
      expect(find.text('Community description'), findsOneWidget);
      // Primary CTA + "Replace background" affordance.
      expect(find.text('Create Community'), findsOneWidget);
      expect(find.text('Replace background'), findsOneWidget);
    });

    testWidgets('reflects name / description from the view model',
        (tester) async {
      await tester.pumpWidget(_hostWithOpenButton(
        seedState: const GenCommunityState(
          name: 'Mission Tool Library',
          description: 'A neighborhood tool-share co-op in the Mission.',
        ),
      ));
      await tester.tap(find.text('open'));
      await tester.pumpAndSettle();

      expect(find.text('Mission Tool Library'), findsOneWidget);
      expect(
        find.text('A neighborhood tool-share co-op in the Mission.'),
        findsOneWidget,
      );
    });

    testWidgets('shows close icon button when not creating', (tester) async {
      await tester
          .pumpWidget(_hostWithOpenButton(seedState: const GenCommunityState()));
      await tester.tap(find.text('open'));
      await tester.pumpAndSettle();

      expect(find.byIcon(Icons.close), findsOneWidget);
    });
  });
}
