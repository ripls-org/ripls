import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/widgets/auth/otp_code_field.dart';

/// Tests for the one-time-code field shared by the phone and email sign-in
/// flows.
///
/// The parity this widget exists to guarantee (#2925) is now structural — the
/// placeholder is a constant and there is no per-caller override — so what is
/// worth testing here is the input handling the two flows both depend on, and
/// the accessibility property that must survive any future edit to the
/// decoration.
void main() {
  Widget wrap(Widget child) {
    return MaterialApp(home: Scaffold(body: child));
  }

  Widget field(TextEditingController controller) {
    return OtpCodeField(
      fieldKey: const ValueKey('code'),
      controller: controller,
      labelText: 'Enter the 6-digit code',
    );
  }

  testWidgets('the code field caps entry at the code length', (tester) async {
    final controller = TextEditingController();
    addTearDown(controller.dispose);

    await tester.pumpWidget(wrap(field(controller)));
    await tester.pumpAndSettle();

    await tester.enterText(find.byType(TextField), '1234567890');
    await tester.pumpAndSettle();

    // Asserted exactly rather than as an upper bound: a `lessThanOrEqualTo`
    // check also passes when nothing was accepted at all, which is how a
    // truncation bug can hide behind a green test.
    expect(controller.text, '123456');
  });

  // A leading space used to occupy one of the six slots the length cap counts,
  // so the sixth digit was silently dropped and the code was submitted — or
  // refused — one digit short.
  testWidgets('whitespace cannot enter the field or displace a digit',
      (tester) async {
    final controller = TextEditingController();
    addTearDown(controller.dispose);

    await tester.pumpWidget(wrap(field(controller)));
    await tester.pumpAndSettle();

    await tester.enterText(find.byType(TextField), ' 12345');
    await tester.pumpAndSettle();
    expect(controller.text, '12345', reason: 'the space is filtered at entry');
    expect(
      OtpCodeField.isComplete(controller.text),
      isFalse,
      reason: 'five digits is not a submittable code',
    );

    await tester.enterText(find.byType(TextField), ' 123456 ');
    await tester.pumpAndSettle();
    expect(
      controller.text,
      '123456',
      reason: 'surrounding whitespace must not cost the user a digit',
    );
    expect(OtpCodeField.isComplete(controller.text), isTrue);
  });

  test('isComplete accepts exactly a full-length code', () {
    expect(OtpCodeField.isComplete('123456'), isTrue);
    expect(OtpCodeField.isComplete('12345'), isFalse);
    expect(OtpCodeField.isComplete(''), isFalse);
  });

  // The field's accessible name is what screen readers announce and what the
  // Playwright harness locates it by. Label and placeholder merge into a single
  // semantics node — the name is "Enter the 6-digit code\n------", not a label
  // plus a separate hint — so the guarantee worth pinning is that the name
  // leads with the localized label and is never the dashes alone. That is the
  // state this field was already fixed out of once, and a decoration change
  // that dropped the label would silently restore it.
  testWidgets('the accessible name leads with the label, not the placeholder',
      (tester) async {
    final handle = tester.ensureSemantics();
    final controller = TextEditingController();
    addTearDown(controller.dispose);

    await tester.pumpWidget(wrap(field(controller)));
    // Settling matters: the field autofocuses, and an unfocused field drops the
    // placeholder from the semantics tree entirely, which would let the second
    // assertion pass without proving anything.
    await tester.pumpAndSettle();

    expect(
      find.bySemanticsLabel(RegExp(r'^Enter the 6-digit code')),
      findsOneWidget,
    );
    expect(
      find.bySemanticsLabel(OtpCodeField.placeholder),
      findsNothing,
      reason: 'the placeholder must never be the whole accessible name',
    );

    handle.dispose();
  });
}
