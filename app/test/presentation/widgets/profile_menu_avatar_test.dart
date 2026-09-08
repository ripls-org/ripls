import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/profile_menu_avatar.dart';
import 'package:ripls/presentation/widgets/user_avatar.dart';
import 'package:ripls/services/auth_state.dart';
import 'package:ripls/services/providers.dart';

class _TestAuthStateNotifier extends AuthStateNotifier {
  _TestAuthStateNotifier(this._initial);

  final AuthStateData _initial;

  @override
  AuthStateData build() => _initial;
}

void main() {
  Widget createHost(AuthStateData authState) {
    return ProviderScope(
      overrides: [
        authStateProvider.overrideWith(() => _TestAuthStateNotifier(authState)),
      ],
      child: MaterialApp(
        localizationsDelegates: const [
          AppLocalizations.delegate,
          GlobalMaterialLocalizations.delegate,
          GlobalWidgetsLocalizations.delegate,
          GlobalCupertinoLocalizations.delegate,
        ],
        supportedLocales: AppLocalizations.supportedLocales,
        home: const Scaffold(body: ProfileMenuAvatar()),
      ),
    );
  }

  testWidgets('renders SizedBox.shrink when no user is loaded', (tester) async {
    await tester.pumpWidget(createHost(AuthStateData.empty));

    expect(find.byType(UserAvatar), findsNothing);
    final box = tester.widget<SizedBox>(find.byType(SizedBox));
    expect(box.width, 0);
    expect(box.height, 0);
  });

  testWidgets('renders a tappable UserAvatar when a user is loaded', (
    tester,
  ) async {
    final authState = AuthStateData(
      accessToken: 'token',
      user: User()..id = 'user-1',
      isLoading: false,
    );

    await tester.pumpWidget(createHost(authState));
    await tester.pump();

    expect(find.byType(UserAvatar), findsOneWidget);
    expect(
      find.bySemanticsLabel('Open account menu'),
      findsOneWidget,
    );
  });
}
