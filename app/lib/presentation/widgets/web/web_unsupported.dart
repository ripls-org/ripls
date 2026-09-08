import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';

/// Helpers for surfacing "this feature needs the mobile app" notices on
/// Flutter Web. Several plugins the mobile app depends on (camera,
/// image_picker, gal, geolocator, add_2_calendar) have no web
/// implementation. Rather than guard each plugin call individually,
/// we guard the entry points — the + FAB on HomeScreen, profile/community
/// photo upload, Add to Calendar — and show a snackbar pointing the
/// visitor at the mobile app.
///
/// #2157 ("serve the whole app on web") is where these gaps were accepted;
/// each guarded plugin either gains a web implementation or keeps its notice.
class WebUnsupported {
  WebUnsupported._();

  
  /// Shows a snackbar saying calendar export isn't available on web.
  /// Returns true if the snackbar was shown.
  static bool showCalendarNotice(BuildContext context) {
    if (!kIsWeb) return false;
    _show(context, context.l10n.webUnsupportedCalendar);
    return true;
  }

  /// Shows a snackbar saying photo upload isn't available on web.
  /// Returns true if the snackbar was shown.
  static bool showPhotoUploadNotice(BuildContext context) {
    if (!kIsWeb) return false;
    _show(context, context.l10n.webUnsupportedPhotoUpload);
    return true;
  }

  static void _show(BuildContext context, String message) {
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(content: Text(message)),
    );
  }
}
