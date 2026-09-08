import 'package:fixnum/fixnum.dart';
import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/portfolio.pb.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/home/needs_you_group_row.dart';

Future<void> _pump(
  WidgetTester tester,
  HomeDecision decision, {
  VoidCallback? onAction,
  VoidCallback? onTap,
}) {
  return tester.pumpWidget(
    ProviderScope(
      child: MaterialApp(
        localizationsDelegates: const [
          AppLocalizations.delegate,
          GlobalMaterialLocalizations.delegate,
          GlobalWidgetsLocalizations.delegate,
          GlobalCupertinoLocalizations.delegate,
        ],
        supportedLocales: AppLocalizations.supportedLocales,
        home: Scaffold(
          body: NeedsYouGroupRow(
            decision: decision,
            onTap: onTap ?? () {},
            onAction: onAction ?? () {},
          ),
        ),
      ),
    ),
  );
}

DailyPerson _person(String name) =>
    DailyPerson(userId: 'u', displayName: name);

void main() {
  group('NeedsYouGroupRow', () {
    testWidgets(
        'resolves the title and accept-label chip from the typed fields',
        (tester) async {
      await _pump(
        tester,
        HomeDecision(
          kind: HomeDecisionKind.HOME_DECISION_KIND_TRANSFER_UPDATE,
          transferAction: HomeTransferAction.HOME_TRANSFER_ACTION_START_LOAN,
          reason: HomeDecisionReason.HOME_DECISION_REASON_READY_TO_PICK_UP,
          subjectTitle: 'Wheelbarrow',
          counterparty: _person('Tyler Reed'),
        ),
      );
      expect(find.text('Wheelbarrow'), findsOneWidget);
      expect(find.text('Mark picked up'), findsOneWidget);
      // Composed status copy renders unquoted.
      expect(
        find.textContaining('ready to pick up', findRichText: true),
        findsOneWidget,
      );
      expect(
        find.textContaining('"ready to pick up"', findRichText: true),
        findsNothing,
      );
    });

    testWidgets('a held loan resolves the Mark-returned chip', (tester) async {
      await _pump(
        tester,
        HomeDecision(
          kind: HomeDecisionKind.HOME_DECISION_KIND_TRANSFER_UPDATE,
          transferAction:
              HomeTransferAction.HOME_TRANSFER_ACTION_COMPLETE_LOAN,
          reason: HomeDecisionReason.HOME_DECISION_REASON_BORROWING,
          subjectTitle: 'Tent',
          counterparty: _person('Tyler Reed'),
        ),
      );
      expect(find.text('Mark returned'), findsOneWidget);
    });

    testWidgets('composes the lend headline and quotes the message preview',
        (tester) async {
      await _pump(
        tester,
        HomeDecision(
          kind: HomeDecisionKind.HOME_DECISION_KIND_LENDING_REQUEST,
          reason: HomeDecisionReason.HOME_DECISION_REASON_MESSAGE_PREVIEW,
          messagePreview: 'Back Sunday, promise!',
          subjectTitle: 'Wheelbarrow',
          counterparty: _person('Tyler Reed'),
        ),
      );
      expect(find.text('Tyler wants your Wheelbarrow'), findsOneWidget);
      expect(find.text('Lend it'), findsOneWidget);
      // Verbatim user text renders quoted.
      expect(
        find.textContaining('"Back Sunday, promise!"', findRichText: true),
        findsOneWidget,
      );
    });

    testWidgets('formats a dated return reason from the timestamp',
        (tester) async {
      final dueAt = DateTime.now().add(const Duration(days: 2));
      await _pump(
        tester,
        HomeDecision(
          kind: HomeDecisionKind.HOME_DECISION_KIND_TRANSFER_UPDATE,
          transferAction:
              HomeTransferAction.HOME_TRANSFER_ACTION_COMPLETE_LOAN,
          reason: HomeDecisionReason.HOME_DECISION_REASON_RETURN_DUE,
          dueAtUnixSec:
              Int64(dueAt.millisecondsSinceEpoch ~/ 1000),
          subjectTitle: 'Tent',
          counterparty: _person('Tyler Reed'),
        ),
      );
      // Two days out renders as an abbreviated weekday, e.g. "due Mon".
      expect(
        find.textContaining('due ', findRichText: true),
        findsOneWidget,
      );
    });

    testWidgets('tapping the chip fires onAction (not the row tap)',
        (tester) async {
      var actioned = false;
      var tapped = false;
      await _pump(
        tester,
        HomeDecision(
          kind: HomeDecisionKind.HOME_DECISION_KIND_ASK_CLAIM,
          subjectTitle: 'Help cutting pine trees',
          helpers: [_person('Alfred Kim')],
        ),
        onAction: () => actioned = true,
        onTap: () => tapped = true,
      );
      await tester.tap(find.text('Say thanks'));
      expect(actioned, isTrue);
      expect(tapped, isFalse);
    });

    testWidgets('hides the chip when the kind is unknown', (tester) async {
      await _pump(
        tester,
        HomeDecision(
          kind: HomeDecisionKind.HOME_DECISION_KIND_UNSPECIFIED,
          subjectTitle: 'New comment on your request',
        ),
      );
      expect(find.text('New comment on your request'), findsOneWidget);
      // No resolvable action for an unknown kind → no chip rendered.
      expect(find.byType(Container), findsWidgets);
    });
  });
}
