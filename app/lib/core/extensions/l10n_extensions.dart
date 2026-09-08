import 'package:flutter/widgets.dart';
import 'package:ripls/l10n/app_localizations.dart';

/// L10nExtension provides a convenient [l10n] getter on [BuildContext].
///
/// Usage: `context.l10n.commonOk`
extension L10nExtension on BuildContext {
  AppLocalizations get l10n => AppLocalizations.of(this);
}
