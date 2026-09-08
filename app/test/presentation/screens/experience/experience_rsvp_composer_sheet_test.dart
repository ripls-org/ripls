import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/experience.pb.dart' as pb;
import 'package:ripls/data/gen/ripls/api/experience_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/presentation/screens/experience/widgets/experience_rsvp_composer_sheet.dart';
import 'package:ripls/presentation/viewmodels/experience_needs_view_model.dart';
import 'package:ripls/presentation/viewmodels/experience_view_model.dart';

import '../../../helpers/l10n_helpers.dart';

const _id = 'exp-1';

class _FakeExp extends ExperienceNotifier {
  _FakeExp(super.experienceId, this._s);
  final ExperienceState _s;
  @override
  ExperienceState build() => _s;
}

class _FakeNeeds extends ExperienceNeedsNotifier {
  _FakeNeeds(super.experienceId, this._s);
  final ExperienceNeedsState _s;
  @override
  ExperienceNeedsState build() => _s;
}

ExperienceState _state() => ExperienceState(
  experienceId: _id,
  currentUserId: 'viewer',
  locationName: 'Vail Village',
  experienceDetails: GetExperienceResponse(
    experience: pb.Experience(
      id: _id,
      name: 'GoPro Mountain Games',
      owner: User(id: 'anna', name: 'Anna Reyes'),
      state: pb.ExperienceState.EXPERIENCE_STATE_ACTIVE,
    ),
  ),
);

ExperienceNeedsState _needs() => ExperienceNeedsState(
  suggestions: const ['Sunscreen', 'Cooler'],
  contributions: [
    ExperienceContributionResponse(
      id: 'c1',
      contributor: User(id: 'viewer', name: 'You'),
      title: 'Folding chair',
    ),
  ],
);

Future<void> _pump(WidgetTester tester, {bool contributionsOnly = false}) async {
  await tester.pumpWidget(
    ProviderScope(
      overrides: [
        experienceProvider(_id).overrideWith(() => _FakeExp(_id, _state())),
        experienceNeedsProvider(
          _id,
        ).overrideWith(() => _FakeNeeds(_id, _needs())),
      ],
      child: localizedApp(
        ExperienceRsvpComposerSheet(
          experienceId: _id,
          accentColor: const Color(0xFF7A9B8C),
          initialIntention: RSVPIntention.RSVP_INTENTION_YES,
          contributionsOnly: contributionsOnly,
        ),
      ),
    ),
  );
  await tester.pump();
}

void main() {
  group('ExperienceRsvpComposerSheet', () {
    testWidgets('renders intent title, bringing chips and send', (
      tester,
    ) async {
      await _pump(tester);

      expect(find.text("You're going"), findsOneWidget);
      expect(find.text('BRINGING'), findsOneWidget);
      expect(find.text('Sunscreen'), findsOneWidget); // suggestion
      expect(find.text('Cooler'), findsOneWidget); // suggestion
      expect(
        find.text('Folding chair'),
        findsOneWidget,
      ); // existing contribution
      expect(find.text('Add something else'), findsOneWidget);
      expect(find.text('Send'), findsOneWidget);
    });

    testWidgets('switching to Maybe updates the title', (tester) async {
      await _pump(tester);
      expect(find.text("You're going"), findsOneWidget);

      await tester.tap(find.bySemanticsLabel('RSVP: Maybe').first);
      await tester.pump();

      expect(find.text('Maybe for now'), findsOneWidget);
    });

    testWidgets('tapping an unbrought item opens the claim-a-need sheet', (
      tester,
    ) async {
      await _pump(tester);
      expect(find.text("I'll bring this"), findsNothing);

      // 'Sunscreen' is a suggestion the viewer isn't bringing → claim mode.
      await tester.tap(find.text('Sunscreen'));
      await tester.pumpAndSettle();

      expect(find.text('PITCH IN'), findsOneWidget);
      expect(find.text("I'll bring this"), findsOneWidget);
    });

    testWidgets('tapping an item already brought opens the edit sheet', (
      tester,
    ) async {
      await _pump(tester);

      // 'Folding chair' is the viewer's own contribution → edit mode.
      await tester.tap(find.text('Folding chair'));
      await tester.pumpAndSettle();

      expect(find.text('EDIT YOUR CONTRIBUTION'), findsOneWidget);
      expect(find.text('Save changes'), findsOneWidget);
    });

    testWidgets(
      'contributions-only mode drops the RSVP toggle, retitles, and saves',
      (tester) async {
        await _pump(tester, contributionsOnly: true);

        // Retitled, no RSVP intention toggle, Save instead of Send/summary.
        expect(find.text('Your Contribution'), findsOneWidget);
        expect(find.text("You're going"), findsNothing);
        expect(find.bySemanticsLabel('RSVP: Maybe'), findsNothing);
        expect(find.text('Send'), findsNothing);
        expect(find.text('Save'), findsOneWidget);

        // The bringing surface (chips + gear link) is still present.
        expect(find.text('BRINGING'), findsOneWidget);
        expect(find.text('Folding chair'), findsOneWidget);
        expect(find.text('Add something else'), findsOneWidget);
      },
    );
  });
}
