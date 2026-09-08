import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/widgets/modal/modal_header.dart';

void main() {
  testWidgets('ModalHeader exposes header semantic role', (tester) async {
    await tester.pumpWidget(const MaterialApp(
      home: Scaffold(body: ModalHeader(title: 'Settings')),
    ));

    final node = tester.getSemantics(find.text('Settings'));
    expect(node.label, 'Settings');
    expect(node.flagsCollection.isHeader, isTrue);
  });
}
