import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/widgets/content/content_headline.dart';

Widget _host(Widget child) => MaterialApp(home: Scaffold(body: child));

void main() {
  group('ContentHeadline', () {
    testWidgets('renders title and description', (tester) async {
      await tester.pumpWidget(
        _host(
          const ContentHeadline(
            title: 'GoPro Mountain Games',
            description: "Mountain sports, music, art.",
          ),
        ),
      );

      expect(find.text('GoPro Mountain Games'), findsOneWidget);
      expect(find.text('Mountain sports, music, art.'), findsOneWidget);
    });

    testWidgets('omits the description when null', (tester) async {
      await tester.pumpWidget(
        _host(const ContentHeadline(title: 'GoPro Mountain Games')),
      );

      expect(find.text('GoPro Mountain Games'), findsOneWidget);
      expect(find.byType(Text), findsOneWidget);
    });
  });
}
