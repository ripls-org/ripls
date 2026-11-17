import 'package:material_ui/material_ui.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/l10n/localization_delegates.dart';

/// localizedApp wraps [child] in a [MaterialApp] with full localization
/// delegates configured for widget tests.
///
/// Use this in widget tests anywhere [context.l10n] is called. Viewmodel
/// unit tests using [ProviderContainer] do NOT need this wrapper — viewmodels
/// must never resolve localized strings directly.
Widget localizedApp(Widget child) {
  return MaterialApp(
    localizationsDelegates: appLocalizationsDelegates,
    supportedLocales: AppLocalizations.supportedLocales,
    home: child,
  );
}
