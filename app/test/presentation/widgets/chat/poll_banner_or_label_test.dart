import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/experience.pb.dart' show Experience;
import 'package:ripls/data/gen/ripls/api/experience_service.pb.dart' show GetExperienceResponse;
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/viewmodels/experience_view_model.dart';
import 'package:ripls/presentation/widgets/chat/message_list.dart';

void main() {
  const expId = 'exp-test-123';
  const activePollId = 'poll-active';
  const stalePollId = 'poll-stale';

  // Builds the widget under test inside a localised ProviderScope.
  Widget buildWidget({
    required String? messagePollId,
    required ExperienceState experienceState,
    bool isLatestTimeProposed = true,
  }) {
    return ProviderScope(
      overrides: [
        experienceProvider(expId).overrideWith(
          () => _StubExperienceNotifier(experienceState),
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
        home: Scaffold(
          body: PollBannerOrLabel(
            experienceId: expId,
            currentUserId: 'user-1',
            fallbackText: 'A poll was opened',
            messagePollId: messagePollId,
            isLatestTimeProposed: isLatestTimeProposed,
            onTap: () {},
          ),
        ),
      ),
    );
  }

  // Creates an ExperienceState whose experience has the given poll fields.
  ExperienceState stateWithPoll({
    required bool timePollActive,
    required String currentPollId,
  }) {
    final exp = GetExperienceResponse()
      ..experience = (Experience()
        ..timePollActive = timePollActive
        ..currentPollId = currentPollId);
    return ExperienceState(
      experienceDetails: exp,
    );
  }

  group('PollBannerOrLabel — poll_id-aware rendering', () {
    testWidgets(
        'shows live banner when messagePollId matches active currentPollId',
        (tester) async {
      await tester.pumpWidget(buildWidget(
        messagePollId: activePollId,
        experienceState: stateWithPoll(
          timePollActive: true,
          currentPollId: activePollId,
        ),
      ));
      await tester.pumpAndSettle();

      // The live poll banner shows the "Poll is live" eyebrow label.
      expect(find.text('Poll is live'), findsOneWidget);
      expect(find.text('Poll ended'), findsNothing);
    });

    testWidgets(
        'shows ended banner when messagePollId differs from currentPollId',
        (tester) async {
      await tester.pumpWidget(buildWidget(
        messagePollId: stalePollId,
        experienceState: stateWithPoll(
          timePollActive: true,
          currentPollId: activePollId, // different poll is now active
        ),
      ));
      await tester.pumpAndSettle();

      // The stale message's poll is no longer current — show "Poll ended".
      expect(find.text('Poll ended'), findsOneWidget);
      expect(find.text('Poll is live'), findsNothing);
    });

    testWidgets(
        'shows ended banner when messagePollId matches but poll is no longer active',
        (tester) async {
      await tester.pumpWidget(buildWidget(
        messagePollId: activePollId,
        experienceState: stateWithPoll(
          timePollActive: false,
          currentPollId: activePollId,
        ),
      ));
      await tester.pumpAndSettle();

      expect(find.text('Poll ended'), findsOneWidget);
      expect(find.text('Poll is live'), findsNothing);
    });

    testWidgets(
        'falls back to legacy behaviour (live banner) when messagePollId is null and poll is active',
        (tester) async {
      await tester.pumpWidget(buildWidget(
        messagePollId: null,
        experienceState: stateWithPoll(
          timePollActive: true,
          currentPollId: activePollId,
        ),
      ));
      await tester.pumpAndSettle();

      // Legacy path: no stored poll_id, so we trust the live experience flag
      // when this is the latest TIME_PROPOSED message in the conversation.
      expect(find.text('Poll is live'), findsOneWidget);
    });

    testWidgets(
        'legacy message that is not the latest TIME_PROPOSED renders ended '
        'even when a poll is active — regression for second-poll stale banner',
        (tester) async {
      await tester.pumpWidget(buildWidget(
        messagePollId: null,
        experienceState: stateWithPoll(
          timePollActive: true,
          currentPollId: activePollId,
        ),
        isLatestTimeProposed: false,
      ));
      await tester.pumpAndSettle();

      // A pre-fix message without poll_id that sits below a newer poll must
      // not masquerade as the live poll card.
      expect(find.text('Poll ended'), findsOneWidget);
      expect(find.text('Poll is live'), findsNothing);
    });
  });
}

/// Synchronous stub notifier that immediately returns a fixed [ExperienceState].
class _StubExperienceNotifier extends ExperienceNotifier {
  _StubExperienceNotifier(this._fixedState) : super('exp-test-123');

  final ExperienceState _fixedState;

  @override
  ExperienceState build() => _fixedState;
}
