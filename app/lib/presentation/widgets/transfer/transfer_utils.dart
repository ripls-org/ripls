import 'package:intl/intl.dart';

/// Formats unix timestamp as relative time (e.g., "2d ago", "Just now").
String formatRelativeTimestamp(int unixSec) {
  final dateTime = DateTime.fromMillisecondsSinceEpoch(unixSec * 1000);
  final now = DateTime.now();
  final difference = now.difference(dateTime);

  if (difference.inSeconds < 60) {
    return 'Just now';
  } else if (difference.inMinutes < 60) {
    final minutes = difference.inMinutes;
    return '${minutes}m ago';
  } else if (difference.inHours < 24) {
    final hours = difference.inHours;
    return '${hours}h ago';
  } else if (difference.inDays < 7) {
    final days = difference.inDays;
    return '${days}d ago';
  } else if (difference.inDays < 30) {
    final weeks = (difference.inDays / 7).floor();
    return '${weeks}w ago';
  } else if (difference.inDays < 365) {
    final months = (difference.inDays / 30).floor();
    return months == 1 ? '1mo ago' : '${months}mo ago';
  } else {
    final years = (difference.inDays / 365).floor();
    return years == 1 ? '1y ago' : '${years}y ago';
  }
}

/// Formats DateTime for pickup display (e.g., "EEE, MMM d").
String formatPickupDate(DateTime date) {
  return DateFormat('EEE, MMM d').format(date);
}

/// Formats DateTime for time display (e.g., "h:mm a").
String formatPickupTime(DateTime date) {
  return DateFormat('h:mm a').format(date);
}

/// Formats duration in days as human-readable string.
String formatDuration(int days) {
  if (days == 1) {
    return '1 day';
  } else if (days < 7) {
    return '$days days';
  } else if (days == 7) {
    return '1 week';
  } else if (days == 14) {
    return '2 weeks';
  } else if (days < 30) {
    final weeks = (days / 7).floor();
    return '$weeks weeks';
  } else if (days < 365) {
    final months = (days / 30).floor();
    return months == 1 ? '1 month' : '$months months';
  } else {
    final years = (days / 365).floor();
    return years == 1 ? '1 year' : '$years years';
  }
}
