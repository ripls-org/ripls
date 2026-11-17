import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/l10n/localization_delegates.dart';

void main() {
  Future<BuildContext> pumpWithLocale(WidgetTester tester, Locale locale) async {
    late BuildContext captured;
    await tester.pumpWidget(
      MaterialApp(
        locale: locale,
        localizationsDelegates: appLocalizationsDelegates,
        supportedLocales: AppLocalizations.supportedLocales,
        home: Builder(
          builder: (context) {
            captured = context;
            return const SizedBox.shrink();
          },
        ),
      ),
    );
    return captured;
  }

  testWidgets('serves the app strings and translated Material strings', (
    tester,
  ) async {
    final context = await pumpWithLocale(tester, const Locale('es'));

    expect(AppLocalizations.of(context).localeName, 'es');
    // The point of the hand-written list: material_ui widgets get a real
    // translation, not the English fallback MaterialApp installs by default.
    final material = MaterialLocalizations.of(context);
    final english = const DefaultMaterialLocalizations();
    expect(material.cancelButtonLabel, isNot(english.cancelButtonLabel));
    expect(material.cancelButtonLabel.toLowerCase(), 'cancelar');
  });

  testWidgets('serves English Material strings for an English locale', (
    tester,
  ) async {
    final context = await pumpWithLocale(tester, const Locale('en'));

    expect(AppLocalizations.of(context).localeName, 'en');
    expect(MaterialLocalizations.of(context).cancelButtonLabel, 'Cancel');
  });
}
