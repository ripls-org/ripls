import 'package:material_ui/material_ui.dart';
import 'package:ripls/l10n/app_localizations.dart';

/// The localization delegates every `MaterialApp` in the app and its tests
/// should use: the app's own strings plus the Material, Cupertino and widgets
/// delegates that ship with `package:material_ui`.
///
/// Use this instead of the generated `AppLocalizations.localizationsDelegates`.
/// `flutter gen-l10n` still builds that list from `flutter_localizations`,
/// whose Material and Cupertino delegates serve the deprecated in-framework
/// widget libraries. Widgets from `package:material_ui` look up a different
/// `MaterialLocalizations` type, so with the generated list they would fall
/// back to English whatever the locale.
const List<LocalizationsDelegate<dynamic>> appLocalizationsDelegates =
    <LocalizationsDelegate<dynamic>>[
      AppLocalizations.delegate,
      ...GlobalMaterialLocalizations.delegates,
    ];
