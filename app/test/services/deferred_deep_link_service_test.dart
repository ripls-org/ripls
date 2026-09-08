import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/services/deferred_deep_link_service.dart';

void main() {
  // Deferred deep links carry only the invitation short code. Legacy
  // gear_id/request_id/experience_id params are ignored — the deep-link
  // target is resolved from the share-link row via CheckInvitation (#2562).
  group('DeferredDeepLinkService.parseReferrer', () {
    test('parses valid referrer with short code', () {
      final context = DeferredDeepLinkService.parseReferrer(
        'utm_source=ripls&utm_content=ABC123',
      );

      expect(context.hasContext, isTrue);
      expect(context.shortCode, equals('ABC123'));
    });

    test('extracts short code, ignoring legacy item-id params', () {
      final context = DeferredDeepLinkService.parseReferrer(
        'utm_source=ripls&utm_content=FULL01&gear_id=g1&request_id=r1&experience_id=e1',
      );

      expect(context.hasContext, isTrue);
      expect(context.shortCode, equals('FULL01'));
    });

    test('rejects referrer from non-ripls source', () {
      final context = DeferredDeepLinkService.parseReferrer(
        'utm_source=google&utm_content=ABC123',
      );

      expect(context.hasContext, isFalse);
      expect(context.shortCode, isNull);
    });

    test('rejects referrer with no utm_source', () {
      final context = DeferredDeepLinkService.parseReferrer(
        'utm_content=ABC123',
      );

      expect(context.hasContext, isFalse);
    });

    test('returns empty context for empty referrer', () {
      final context = DeferredDeepLinkService.parseReferrer('');

      expect(context.hasContext, isFalse);
    });

    test('returns empty context for malformed referrer', () {
      final context = DeferredDeepLinkService.parseReferrer(
        'not-a-valid-referrer-string',
      );

      expect(context.hasContext, isFalse);
    });

    test('handles URL-encoded values in referrer', () {
      final context = DeferredDeepLinkService.parseReferrer(
        'utm_source=ripls&utm_content=CODE%2BPLUS',
      );

      expect(context.hasContext, isTrue);
      expect(context.shortCode, equals('CODE+PLUS'));
    });

    test('ignores empty utm_content', () {
      final context = DeferredDeepLinkService.parseReferrer(
        'utm_source=ripls&utm_content=',
      );

      expect(context.hasContext, isFalse);
      expect(context.shortCode, isNull);
    });

    test('handles referrer with extra unknown parameters', () {
      final context = DeferredDeepLinkService.parseReferrer(
        'utm_source=ripls&utm_content=ABC&utm_medium=link&unknown=value',
      );

      expect(context.hasContext, isTrue);
      expect(context.shortCode, equals('ABC'));
    });
  });

  group('DeferredDeepLinkService.parseClipboardUrl', () {
    test('parses valid ripls://invite URL with token', () {
      final context = DeferredDeepLinkService.parseClipboardUrl(
        'ripls://invite?token=ABC123',
      );

      expect(context, isNotNull);
      expect(context!.hasContext, isTrue);
      expect(context.shortCode, equals('ABC123'));
    });

    test('extracts token, ignoring legacy item-id params', () {
      final context = DeferredDeepLinkService.parseClipboardUrl(
        'ripls://invite?token=FULL&gear_id=g1&request_id=r1&experience_id=e1',
      );

      expect(context, isNotNull);
      expect(context!.shortCode, equals('FULL'));
    });

    test('returns null for non-ripls scheme', () {
      final context = DeferredDeepLinkService.parseClipboardUrl(
        'https://example.com/go/ABC123',
      );

      expect(context, isNull);
    });

    test('returns null for wrong host', () {
      final context = DeferredDeepLinkService.parseClipboardUrl(
        'ripls://reset-password?token=ABC123',
      );

      expect(context, isNull);
    });

    test('returns null for missing token', () {
      final context = DeferredDeepLinkService.parseClipboardUrl(
        'ripls://invite?gear_id=g1',
      );

      expect(context, isNull);
    });

    test('returns null for empty string', () {
      final context = DeferredDeepLinkService.parseClipboardUrl('');

      expect(context, isNull);
    });

    test('returns null for plain text', () {
      final context = DeferredDeepLinkService.parseClipboardUrl(
        'just some random clipboard text',
      );

      expect(context, isNull);
    });

    test('handles whitespace around URL', () {
      final context = DeferredDeepLinkService.parseClipboardUrl(
        '  ripls://invite?token=ABC123  ',
      );

      expect(context, isNotNull);
      expect(context!.shortCode, equals('ABC123'));
    });

    test('returns null for URL with empty token', () {
      final context = DeferredDeepLinkService.parseClipboardUrl(
        'ripls://invite?token=',
      );

      expect(context, isNull);
    });
  });

  group('DeferredDeepLinkContext', () {
    test('hasContext is false when shortCode is null', () {
      const context = DeferredDeepLinkContext();
      expect(context.hasContext, isFalse);
    });

    test('hasContext is true with a shortCode', () {
      const context = DeferredDeepLinkContext(shortCode: 'ABC');
      expect(context.hasContext, isTrue);
    });
  });
}
