/// PII redaction utilities for safe logging.
///
/// These functions mask sensitive information before logging to prevent
/// personal data from appearing in crash reports, analytics, or logs.
class LogRedactor {
  /// Masks an email address for safe logging.
  ///
  /// Shows the first 2 characters of the local part, followed by asterisks.
  ///
  /// Examples:
  /// - "alice@example.com" → "al***@example.com"
  /// - "a@example.com" → "a***@example.com"
  /// - "" → ""
  /// - "invalid" → "***@***"
  static String maskEmail(String email) {
    if (email.isEmpty) {
      return '';
    }

    final parts = email.split('@');
    if (parts.length != 2) {
      return '***@***';
    }

    final local = parts[0];
    final domain = parts[1];

    if (domain.isEmpty) {
      return '***@***';
    }

    // Mask the local part, keeping first 2 characters if possible
    String maskedLocal;
    if (local.isEmpty) {
      maskedLocal = '***';
    } else if (local.length <= 2) {
      maskedLocal = '$local***';
    } else {
      maskedLocal = '${local.substring(0, 2)}***';
    }

    return '$maskedLocal@$domain';
  }

  /// Masks a token for safe logging by showing only the first 8 characters.
  ///
  /// Examples:
  /// - "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..." → "eyJhbGci..."
  /// - "12345678" → "12345678" (exactly 8 chars, no ellipsis)
  /// - "short" → "short" (less than 8 chars)
  /// - "" → ""
  static String maskToken(String token) {
    if (token.isEmpty) {
      return '';
    }

    if (token.length <= 8) {
      return token;
    }

    return '${token.substring(0, 8)}...';
  }

  /// Masks a phone number for safe logging by showing only the last 4 digits.
  ///
  /// Preserves the country code prefix if present (+).
  ///
  /// Examples:
  /// - "+1234567890" → "+***7890"
  /// - "5551234567" → "***4567"
  /// - "1234" → "1234" (4 or fewer digits)
  /// - "" → ""
  static String maskPhoneNumber(String phoneNumber) {
    if (phoneNumber.isEmpty) {
      return '';
    }

    // Extract just the digits for processing
    final digitsOnly = phoneNumber.replaceAll(RegExp(r'[^\d]'), '');

    if (digitsOnly.length <= 4) {
      return phoneNumber;
    }

    final lastFour = digitsOnly.substring(digitsOnly.length - 4);
    final hasPlus = phoneNumber.startsWith('+');

    return '${hasPlus ? '+' : ''}***$lastFour';
  }

  /// Masks a user ID for safe logging by showing only the first 8 characters.
  ///
  /// Same behavior as [maskToken] but semantically clearer for user IDs.
  ///
  /// Examples:
  /// - "user_abc123def456" → "user_abc..."
  /// - "short" → "short"
  /// - "" → ""
  static String maskUserId(String userId) {
    return maskToken(userId);
  }

  /// Masks a generic identifier for safe logging.
  ///
  /// Shows the first [visibleChars] characters, followed by asterisks.
  /// Defaults to 4 visible characters.
  ///
  /// Examples (with default visibleChars=4):
  /// - "abc123def456" → "abc1***"
  /// - "abc" → "abc" (shorter than visibleChars)
  /// - "" → ""
  static String maskIdentifier(String identifier, {int visibleChars = 4}) {
    if (identifier.isEmpty) {
      return '';
    }

    if (identifier.length <= visibleChars) {
      return identifier;
    }

    return '${identifier.substring(0, visibleChars)}***';
  }

  /// Redacts all PII from a map of data.
  ///
  /// Automatically detects and masks values based on key names:
  /// - Keys containing "email" → masked with [maskEmail]
  /// - Keys containing "token", "password", "secret" → masked with [maskToken]
  /// - Keys containing "phone" → masked with [maskPhoneNumber]
  /// - Keys containing "user_id" or "userId" → masked with [maskUserId]
  ///
  /// Returns a new map with redacted values. The original map is not modified.
  static Map<String, dynamic> redactMap(Map<String, dynamic> data) {
    final redacted = <String, dynamic>{};

    for (final entry in data.entries) {
      final key = entry.key.toLowerCase();
      final value = entry.value;

      if (value is String) {
        if (key.contains('email')) {
          redacted[entry.key] = maskEmail(value);
        } else if (key.contains('token') ||
            key.contains('password') ||
            key.contains('secret')) {
          redacted[entry.key] = maskToken(value);
        } else if (key.contains('phone')) {
          redacted[entry.key] = maskPhoneNumber(value);
        } else if (key.contains('user_id') || key.contains('userid')) {
          redacted[entry.key] = maskUserId(value);
        } else {
          redacted[entry.key] = value;
        }
      } else if (value is Map<String, dynamic>) {
        redacted[entry.key] = redactMap(value);
      } else {
        redacted[entry.key] = value;
      }
    }

    return redacted;
  }
}
