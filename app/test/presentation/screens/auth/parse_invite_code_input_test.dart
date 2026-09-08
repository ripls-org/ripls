import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/screens/auth/register_screen.dart';

void main() {
  group('parseInviteCodeInput', () {
    // parseInviteCodeInput returns only the invitation short code. Legacy
    // gear_id/request_id/experience_id query params are ignored — the
    // deep-link target is resolved from the share-link row via
    // CheckInvitation (#2562).
    group('bare short codes', () {
      test('parses simple code', () {
        expect(parseInviteCodeInput('hTW82dhM'), equals('hTW82dhM'));
      });

      test('trims whitespace', () {
        expect(parseInviteCodeInput('  hTW82dhM  '), equals('hTW82dhM'));
      });

      test('handles empty string', () {
        expect(parseInviteCodeInput(''), equals(''));
      });
    });

    group('https://example.com/go/ URLs', () {
      test('extracts code from URL', () {
        expect(
          parseInviteCodeInput('https://example.com/go/hTW82dhM'),
          equals('hTW82dhM'),
        );
      });

      test('extracts code, ignoring legacy gear_id query param', () {
        expect(
          parseInviteCodeInput('https://example.com/go/hTW82dhM?gear_id=gear-123'),
          equals('hTW82dhM'),
        );
      });

      test('extracts code, ignoring legacy request_id query param', () {
        expect(
          parseInviteCodeInput(
              'https://example.com/go/hTW82dhM?request_id=req-456'),
          equals('hTW82dhM'),
        );
      });

      test('extracts code, ignoring legacy experience_id query param', () {
        expect(
          parseInviteCodeInput(
              'https://example.com/go/hTW82dhM?experience_id=exp-789'),
          equals('hTW82dhM'),
        );
      });

      test('extracts code, ignoring all legacy item-id params', () {
        expect(
          parseInviteCodeInput(
              'https://example.com/go/CODE?gear_id=g1&request_id=r1&experience_id=e1'),
          equals('CODE'),
        );
      });

      test('works with localhost URL', () {
        expect(
          parseInviteCodeInput(
              'http://localhost:8080/go/hTW82dhM?gear_id=test'),
          equals('hTW82dhM'),
        );
      });

      test('works with dev server URL', () {
        expect(
          parseInviteCodeInput('https://server-dev.example.com/go/hTW82dhM'),
          equals('hTW82dhM'),
        );
      });

      test('ignores extra query params', () {
        expect(
          parseInviteCodeInput(
              'https://example.com/go/CODE?gear_id=g1&utm_source=test&foo=bar'),
          equals('CODE'),
        );
      });
    });

    group('ripls://invite URLs', () {
      test('extracts code from custom scheme', () {
        expect(
          parseInviteCodeInput('ripls://invite?token=hTW82dhM'),
          equals('hTW82dhM'),
        );
      });

      test('extracts code, ignoring legacy item-id params', () {
        expect(
          parseInviteCodeInput(
              'ripls://invite?token=CODE&gear_id=g1&request_id=r1'),
          equals('CODE'),
        );
      });

      test('falls back to bare code when token is missing', () {
        // No token → treated as bare code (the full string).
        expect(
          parseInviteCodeInput('ripls://invite?gear_id=g1'),
          equals('ripls://invite?gear_id=g1'),
        );
      });

      test('falls back to bare code for wrong host', () {
        expect(
          parseInviteCodeInput('ripls://reset-password?token=ABC'),
          equals('ripls://reset-password?token=ABC'),
        );
      });
    });

    group('malicious input', () {
      test('SQL injection in bare code is passed as-is (server validates)', () {
        expect(
          parseInviteCodeInput("'; DROP TABLE community_invitation_link; --"),
          equals("'; DROP TABLE community_invitation_link; --"),
        );
      });

      test('SQL injection in query param is ignored; code extracted', () {
        expect(
          parseInviteCodeInput(
              "https://example.com/go/CODE?gear_id='; DROP TABLE users; --"),
          equals('CODE'),
        );
      });

      test('script injection in code', () {
        expect(
          parseInviteCodeInput('<script>alert("xss")</script>'),
          equals('<script>alert("xss")</script>'),
        );
      });

      test('script injection in URL query param is ignored; code extracted', () {
        expect(
          parseInviteCodeInput(
              'https://example.com/go/CODE?gear_id=<script>alert(1)</script>'),
          equals('CODE'),
        );
      });

      test('extremely long input is passed through', () {
        final longCode = 'A' * 10000;
        expect(parseInviteCodeInput(longCode), equals(longCode));
      });

      test('null bytes in input', () {
        expect(
          parseInviteCodeInput('CODE\x00; DROP TABLE x;'),
          equals('CODE\x00; DROP TABLE x;'),
        );
      });

      test('URL with javascript scheme is treated as bare code', () {
        // No /go/ path and not ripls:// → bare code.
        expect(
          parseInviteCodeInput('javascript:alert(document.cookie)'),
          equals('javascript:alert(document.cookie)'),
        );
      });

      test('URL with data scheme is treated as bare code', () {
        expect(
          parseInviteCodeInput('data:text/html,<script>alert(1)</script>'),
          equals('data:text/html,<script>alert(1)</script>'),
        );
      });

      test('path traversal in code segment falls through to bare code', () {
        // Uri normalization removes /go/../.., so there's no /go/ match →
        // the input is returned as a bare code (no hijacked short code).
        expect(
          parseInviteCodeInput('https://example.com/go/../../etc/passwd'),
          equals('https://example.com/go/../../etc/passwd'),
        );
      });

      test('URL-encoded SQL injection in code segment', () {
        // Uri.parse keeps path segments percent-encoded.
        expect(
          parseInviteCodeInput(
              "https://example.com/go/%27%3B%20DROP%20TABLE%20users%3B"),
          equals('%27%3B%20DROP%20TABLE%20users%3B'),
        );
      });
    });

    group('non-invite URLs', () {
      test('https URL without /go/ path treated as bare code', () {
        expect(
          parseInviteCodeInput('https://example.com/about'),
          equals('https://example.com/about'),
        );
      });

      test('URL with trailing whitespace is trimmed', () {
        expect(
          parseInviteCodeInput('  https://example.com/go/CODE  '),
          equals('CODE'),
        );
      });
    });
  });
}
