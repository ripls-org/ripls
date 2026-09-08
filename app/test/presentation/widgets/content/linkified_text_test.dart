import 'package:flutter/gestures.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/widgets/content/linkified_text.dart';

void main() {
  const baseStyle = TextStyle(color: Colors.white);
  const linkStyle = TextStyle(
    color: Colors.blue,
    decoration: TextDecoration.underline,
  );

  List<String> tappedUrls = [];
  void onUrlTap(String url) => tappedUrls.add(url);

  setUp(() => tappedUrls = []);

  group('buildLinkifiedSpans', () {
    test('returns single span for text without URLs', () {
      final spans = buildLinkifiedSpans(
        text: 'No links here',
        style: baseStyle,
        linkStyle: linkStyle,
        onUrlTap: onUrlTap,
      );

      expect(spans, hasLength(1));
      final span = spans[0] as TextSpan;
      expect(span.text, 'No links here');
      expect(span.style, baseStyle);
      expect(span.recognizer, isNull);
    });

    test('splits text around embedded URL', () {
      final spans = buildLinkifiedSpans(
        text: 'Check out https://example.com for details',
        style: baseStyle,
        linkStyle: linkStyle,
        onUrlTap: onUrlTap,
      );

      expect(spans, hasLength(3));

      final before = spans[0] as TextSpan;
      expect(before.text, 'Check out ');
      expect(before.style, baseStyle);

      final link = spans[1] as TextSpan;
      expect(link.text, 'https://example.com');
      expect(link.style, linkStyle);
      expect(link.recognizer, isA<TapGestureRecognizer>());

      final after = spans[2] as TextSpan;
      expect(after.text, ' for details');
      expect(after.style, baseStyle);
    });

    test('handles URL at start of text', () {
      final spans = buildLinkifiedSpans(
        text: 'https://example.com is great',
        style: baseStyle,
        linkStyle: linkStyle,
        onUrlTap: onUrlTap,
      );

      expect(spans, hasLength(2));

      final link = spans[0] as TextSpan;
      expect(link.text, 'https://example.com');
      expect(link.recognizer, isA<TapGestureRecognizer>());

      final after = spans[1] as TextSpan;
      expect(after.text, ' is great');
    });

    test('handles URL at end of text', () {
      final spans = buildLinkifiedSpans(
        text: 'Visit https://example.com',
        style: baseStyle,
        linkStyle: linkStyle,
        onUrlTap: onUrlTap,
      );

      expect(spans, hasLength(2));

      final before = spans[0] as TextSpan;
      expect(before.text, 'Visit ');

      final link = spans[1] as TextSpan;
      expect(link.text, 'https://example.com');
    });

    test('handles multiple URLs', () {
      final spans = buildLinkifiedSpans(
        text: 'See https://a.com and https://b.com for info',
        style: baseStyle,
        linkStyle: linkStyle,
        onUrlTap: onUrlTap,
      );

      // "See " + link1 + " and " + link2 + " for info"
      expect(spans, hasLength(5));

      expect((spans[0] as TextSpan).text, 'See ');
      expect((spans[1] as TextSpan).text, 'https://a.com');
      expect((spans[2] as TextSpan).text, ' and ');
      expect((spans[3] as TextSpan).text, 'https://b.com');
      expect((spans[4] as TextSpan).text, ' for info');
    });

    test('URL-only text returns single link span', () {
      final spans = buildLinkifiedSpans(
        text: 'https://example.com',
        style: baseStyle,
        linkStyle: linkStyle,
        onUrlTap: onUrlTap,
      );

      expect(spans, hasLength(1));
      final link = spans[0] as TextSpan;
      expect(link.text, 'https://example.com');
      expect(link.recognizer, isA<TapGestureRecognizer>());
    });

    test('tap recognizer invokes onUrlTap callback', () {
      final spans = buildLinkifiedSpans(
        text: 'Visit https://example.com now',
        style: baseStyle,
        linkStyle: linkStyle,
        onUrlTap: onUrlTap,
      );

      final link = spans[1] as TextSpan;
      final recognizer = link.recognizer! as TapGestureRecognizer;
      recognizer.onTap!();

      expect(tappedUrls, ['https://example.com']);
    });

    test('handles http URLs', () {
      final spans = buildLinkifiedSpans(
        text: 'Visit http://example.com',
        style: baseStyle,
        linkStyle: linkStyle,
        onUrlTap: onUrlTap,
      );

      expect(spans, hasLength(2));
      expect((spans[1] as TextSpan).text, 'http://example.com');
    });

    test('empty text returns single empty span', () {
      final spans = buildLinkifiedSpans(
        text: '',
        style: baseStyle,
        linkStyle: linkStyle,
        onUrlTap: onUrlTap,
      );

      expect(spans, hasLength(1));
      expect((spans[0] as TextSpan).text, '');
    });
  });

  group('LinkifiedText widget', () {
    Widget wrap(Widget child) {
      return MaterialApp(home: Scaffold(body: child));
    }

    testWidgets('renders and displays text', (tester) async {
      await tester.pumpWidget(wrap(
        const LinkifiedText(text: 'Hello world'),
      ));

      expect(find.byType(LinkifiedText), findsOneWidget);
    });

    testWidgets('respects maxLines and overflow', (tester) async {
      await tester.pumpWidget(wrap(
        const LinkifiedText(
          text: 'Line 1\nLine 2\nLine 3',
          maxLines: 2,
          overflow: TextOverflow.ellipsis,
        ),
      ));

      expect(find.byType(LinkifiedText), findsOneWidget);
    });
  });
}
