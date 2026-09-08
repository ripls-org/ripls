import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/feed_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/data/repositories/esm_repository.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/story/story_embedded_esm.dart';
import 'package:ripls/services/providers/esm_providers.dart';

import 'story_embedded_esm_test.mocks.dart';

@GenerateMocks([EsmRepository])
void main() {
  late MockEsmRepository mockRepository;

  setUp(() {
    mockRepository = MockEsmRepository();
  });

  EmbeddedEsmPrompt buildPrompt({String? currentVote}) {
    final p = EmbeddedEsmPrompt();
    p.promptId = 'prompt-1';
    p.question = 'Want to join if they do something like this again?';
    p.responseOptions.addAll([
      EmbeddedEsmOption(key: 'do_again', label: 'Yes'),
      EmbeddedEsmOption(key: 'maybe', label: 'Maybe'),
      EmbeddedEsmOption(key: 'not_for_me', label: 'Not for me'),
    ]);
    if (currentVote != null) {
      p.currentResponseOptionKey = currentVote;
    }
    return p;
  }

  StoryPayload buildStory({
    EmbeddedEsmPrompt? prompt,
    List<User> socialProof = const [],
    int remainder = 0,
  }) {
    final s = StoryPayload();
    s.storyType = StoryType.STORY_TYPE_EXPERIENCE_CONCLUDED;
    s.title = 'Rafting Trip';
    s.experienceId = 'exp-1';
    if (prompt != null) {
      s.embeddedEsmPrompt = prompt;
    }
    s.embeddedEsmSocialProofUsers.addAll(socialProof);
    s.embeddedEsmSocialProofRemainder = remainder;
    return s;
  }

  Widget harness(StoryPayload story) {
    return ProviderScope(
      overrides: [
        esmRepositoryProvider.overrideWithValue(mockRepository),
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
          backgroundColor: Colors.black,
          body: Center(child: StoryEmbeddedEsm(story: story)),
        ),
      ),
    );
  }

  testWidgets('voting state shows the question and three options', (tester) async {
    await tester.pumpWidget(harness(buildStory(prompt: buildPrompt())));

    expect(find.text('Want to join if they do something like this again?'), findsOneWidget);
    expect(find.text('Yes'), findsOneWidget);
    expect(find.text('Maybe'), findsOneWidget);
    expect(find.text('Not for me'), findsOneWidget);
  });

  testWidgets('voted state with Yes shows good-company header and change link', (tester) async {
    final viewer = User()
      ..id = 'u1'
      ..name = 'Alice Aaa';
    await tester.pumpWidget(harness(buildStory(
      prompt: buildPrompt(currentVote: 'do_again'),
      socialProof: [viewer],
    )));

    expect(find.text("You'd be in good company"), findsOneWidget);
    expect(find.text('change'), findsOneWidget);
  });

  testWidgets('voted state with Maybe shows soft acknowledgement', (tester) async {
    await tester.pumpWidget(harness(buildStory(
      prompt: buildPrompt(currentVote: 'maybe'),
    )));

    expect(find.text('Noted as a maybe'), findsOneWidget);
  });

  testWidgets('voted state with Not for me shows declining descriptor', (tester) async {
    await tester.pumpWidget(harness(buildStory(
      prompt: buildPrompt(currentVote: 'not_for_me'),
    )));

    expect(find.text('Noted — not your thing'), findsOneWidget);
    expect(find.text("We'll show you fewer events like this."), findsOneWidget);
  });

  testWidgets('voting submits via the repository', (tester) async {
    when(mockRepository.respond(
      promptId: anyNamed('promptId'),
      responseOptionKey: anyNamed('responseOptionKey'),
    )).thenAnswer((_) async => Future.value());

    await tester.pumpWidget(harness(buildStory(prompt: buildPrompt())));

    await tester.tap(find.text('Yes'));
    await tester.pumpAndSettle();

    verify(mockRepository.respond(
      promptId: 'prompt-1',
      responseOptionKey: 'do_again',
    )).called(1);
    expect(find.text("You'd be in good company"), findsOneWidget);
  });

  testWidgets('rolls back optimistic update when the RPC fails', (tester) async {
    when(mockRepository.respond(
      promptId: anyNamed('promptId'),
      responseOptionKey: anyNamed('responseOptionKey'),
    )).thenThrow(Exception('boom'));

    await tester.pumpWidget(harness(buildStory(prompt: buildPrompt())));

    await tester.tap(find.text('Maybe'));
    await tester.pumpAndSettle();

    expect(find.text('Want to join if they do something like this again?'), findsOneWidget);
    expect(find.text('Maybe'), findsOneWidget);
  });

  testWidgets('change affordance returns to the voting state', (tester) async {
    await tester.pumpWidget(harness(buildStory(
      prompt: buildPrompt(currentVote: 'do_again'),
    )));

    await tester.tap(find.text('change'));
    await tester.pumpAndSettle();

    expect(find.text('Want to join if they do something like this again?'), findsOneWidget);
    expect(find.text('Yes'), findsOneWidget);
  });
}
