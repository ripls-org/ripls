import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/observability/logging/redactor.dart';

void main() {
  group('LogRedactor.maskEmail', () {
    test('masks standard email', () {
      expect(LogRedactor.maskEmail('alice@example.com'), 'al***@example.com');
    });

    test('masks email with short local part (1 char)', () {
      expect(LogRedactor.maskEmail('a@example.com'), 'a***@example.com');
    });

    test('masks email with short local part (2 chars)', () {
      expect(LogRedactor.maskEmail('ab@example.com'), 'ab***@example.com');
    });

    test('masks email with long local part', () {
      expect(
          LogRedactor.maskEmail('verylongemail@example.com'), 've***@example.com');
    });

    test('returns empty string for empty input', () {
      expect(LogRedactor.maskEmail(''), '');
    });

    test('returns masked placeholder for invalid email without @', () {
      expect(LogRedactor.maskEmail('invalid'), '***@***');
    });

    test('returns masked placeholder for email without domain', () {
      expect(LogRedactor.maskEmail('user@'), '***@***');
    });

    test('masks email without local part', () {
      expect(LogRedactor.maskEmail('@domain.com'), '***@domain.com');
    });

    test('returns masked placeholder for multiple @ signs', () {
      expect(LogRedactor.maskEmail('user@domain@extra'), '***@***');
    });

    test('masks email with subdomain', () {
      expect(LogRedactor.maskEmail('user@mail.example.com'), 'us***@mail.example.com');
    });

    test('masks email with dots in local part', () {
      expect(LogRedactor.maskEmail('first.last@example.com'), 'fi***@example.com');
    });

    test('masks email with plus sign in local part', () {
      expect(LogRedactor.maskEmail('user+tag@example.com'), 'us***@example.com');
    });
  });

  group('LogRedactor.maskToken', () {
    test('masks long token', () {
      expect(
        LogRedactor.maskToken(
            'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.payload.signature'),
        'eyJhbGci...',
      );
    });

    test('returns full token when exactly 8 chars', () {
      expect(LogRedactor.maskToken('12345678'), '12345678');
    });

    test('returns full token when less than 8 chars', () {
      expect(LogRedactor.maskToken('short'), 'short');
    });

    test('returns empty string for empty input', () {
      expect(LogRedactor.maskToken(''), '');
    });

    test('masks token with 9 chars', () {
      expect(LogRedactor.maskToken('123456789'), '12345678...');
    });

    test('masks UUID-style token', () {
      expect(
        LogRedactor.maskToken('550e8400-e29b-41d4-a716-446655440000'),
        '550e8400...',
      );
    });
  });

  group('LogRedactor.maskPhoneNumber', () {
    test('masks phone number with country code', () {
      expect(LogRedactor.maskPhoneNumber('+1234567890'), '+***7890');
    });

    test('masks phone number without country code', () {
      expect(LogRedactor.maskPhoneNumber('5551234567'), '***4567');
    });

    test('returns full number when 4 or fewer digits', () {
      expect(LogRedactor.maskPhoneNumber('1234'), '1234');
    });

    test('returns full number when 3 digits', () {
      expect(LogRedactor.maskPhoneNumber('123'), '123');
    });

    test('returns empty string for empty input', () {
      expect(LogRedactor.maskPhoneNumber(''), '');
    });

    test('masks formatted phone number with dashes', () {
      expect(LogRedactor.maskPhoneNumber('555-123-4567'), '***4567');
    });

    test('masks formatted phone number with spaces', () {
      expect(LogRedactor.maskPhoneNumber('555 123 4567'), '***4567');
    });

    test('masks formatted phone number with parentheses', () {
      expect(LogRedactor.maskPhoneNumber('(555) 123-4567'), '***4567');
    });

    test('masks international format with country code', () {
      expect(LogRedactor.maskPhoneNumber('+1 (555) 123-4567'), '+***4567');
    });

    test('masks phone number with extension stripped', () {
      // Extension is not digits-only so it gets stripped
      expect(LogRedactor.maskPhoneNumber('5551234567x123'), '***7123');
    });
  });

  group('LogRedactor.maskUserId', () {
    test('masks long user ID', () {
      expect(LogRedactor.maskUserId('user_abc123def456'), 'user_abc...');
    });

    test('returns full ID when short', () {
      expect(LogRedactor.maskUserId('short'), 'short');
    });

    test('returns empty string for empty input', () {
      expect(LogRedactor.maskUserId(''), '');
    });

    test('masks UUID user ID', () {
      expect(
        LogRedactor.maskUserId('550e8400-e29b-41d4-a716-446655440000'),
        '550e8400...',
      );
    });
  });

  group('LogRedactor.maskIdentifier', () {
    test('masks identifier with default visible chars', () {
      expect(LogRedactor.maskIdentifier('abc123def456'), 'abc1***');
    });

    test('masks identifier with custom visible chars', () {
      expect(
          LogRedactor.maskIdentifier('abc123def456', visibleChars: 6), 'abc123***');
    });

    test('returns full identifier when shorter than visibleChars', () {
      expect(LogRedactor.maskIdentifier('abc'), 'abc');
    });

    test('returns empty string for empty input', () {
      expect(LogRedactor.maskIdentifier(''), '');
    });

    test('returns full identifier when equal to visibleChars', () {
      expect(LogRedactor.maskIdentifier('abcd', visibleChars: 4), 'abcd');
    });

    test('masks with visibleChars=1', () {
      expect(LogRedactor.maskIdentifier('abc123', visibleChars: 1), 'a***');
    });
  });

  group('LogRedactor.redactMap', () {
    test('masks email fields', () {
      final input = {'user_email': 'alice@example.com', 'name': 'Alice'};
      final result = LogRedactor.redactMap(input);

      expect(result['user_email'], 'al***@example.com');
      expect(result['name'], 'Alice');
    });

    test('masks token fields', () {
      final input = {
        'access_token': 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.payload',
        'status': 'active',
      };
      final result = LogRedactor.redactMap(input);

      expect(result['access_token'], 'eyJhbGci...');
      expect(result['status'], 'active');
    });

    test('masks password fields', () {
      final input = {'password': 'supersecret123', 'username': 'alice'};
      final result = LogRedactor.redactMap(input);

      expect(result['password'], 'supersec...');
      expect(result['username'], 'alice');
    });

    test('masks secret fields', () {
      final input = {'api_secret': 'sk_live_abc123xyz789', 'version': '1.0'};
      final result = LogRedactor.redactMap(input);

      expect(result['api_secret'], 'sk_live_...');
      expect(result['version'], '1.0');
    });

    test('masks phone fields', () {
      final input = {'phone_number': '+15551234567', 'verified': true};
      final result = LogRedactor.redactMap(input);

      expect(result['phone_number'], '+***4567');
      expect(result['verified'], true);
    });

    test('masks user_id fields', () {
      final input = {'user_id': 'user_abc123def456', 'action': 'login'};
      final result = LogRedactor.redactMap(input);

      expect(result['user_id'], 'user_abc...');
      expect(result['action'], 'login');
    });

    test('masks userId fields (camelCase)', () {
      final input = {'userId': 'user_abc123def456'};
      final result = LogRedactor.redactMap(input);

      expect(result['userId'], 'user_abc...');
    });

    test('handles nested maps', () {
      final input = {
        'user': {
          'email': 'alice@example.com',
          'profile': {
            'phone': '+15551234567',
          },
        },
        'token': 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9',
      };
      final result = LogRedactor.redactMap(input);

      expect((result['user'] as Map)['email'], 'al***@example.com');
      expect(
          ((result['user'] as Map)['profile'] as Map)['phone'], '+***4567');
      expect(result['token'], 'eyJhbGci...');
    });

    test('preserves non-string values', () {
      final input = {
        'count': 42,
        'active': true,
        'rate': 3.14,
        'items': ['a', 'b', 'c'],
      };
      final result = LogRedactor.redactMap(input);

      expect(result['count'], 42);
      expect(result['active'], true);
      expect(result['rate'], 3.14);
      expect(result['items'], ['a', 'b', 'c']);
    });

    test('does not modify original map', () {
      final input = {'email': 'alice@example.com'};
      LogRedactor.redactMap(input);

      expect(input['email'], 'alice@example.com');
    });

    test('handles empty map', () {
      final result = LogRedactor.redactMap({});
      expect(result, isEmpty);
    });

    test('is case-insensitive for key matching', () {
      final input = {
        'EMAIL': 'alice@example.com',
        'Token': 'abc123def456789',
        'PHONE': '+15551234567',
      };
      final result = LogRedactor.redactMap(input);

      expect(result['EMAIL'], 'al***@example.com');
      expect(result['Token'], 'abc123de...');
      expect(result['PHONE'], '+***4567');
    });
  });
}
