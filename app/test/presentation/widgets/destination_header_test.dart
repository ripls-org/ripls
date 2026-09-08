import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/destination_header.dart';
import 'package:ripls/presentation/widgets/profile_menu_avatar.dart';
import 'package:ripls/services/auth_state.dart';
import 'package:ripls/services/providers.dart';

/// The avatar's top offset from the header must be identical across
/// tabs regardless of title/subtitle length (#2634 v2) — this was the
/// bug where bottom-alignment made the avatar shift up and down between
/// Home's two-line greeting and every other tab's one-line title.
class _FixedAuthStateNotifier extends AuthStateNotifier {
  @override
  AuthStateData build() => AuthStateData(
        accessToken: 'token',
        user: User()
          ..id = 'user-1'
          ..name = 'Thomas',
        isLoading: false,
      );
}

void main() {
  Widget host(DestinationHeader header) {
    return ProviderScope(
      overrides: [
        authStateProvider.overrideWith(_FixedAuthStateNotifier.new),
      ],
      child: MaterialApp(
        localizationsDelegates: const [
          AppLocalizations.delegate,
          GlobalMaterialLocalizations.delegate,
          GlobalWidgetsLocalizations.delegate,
          GlobalCupertinoLocalizations.delegate,
        ],
        supportedLocales: AppLocalizations.supportedLocales,
        home: Scaffold(body: header),
      ),
    );
  }

  double avatarTop(WidgetTester tester) =>
      tester.getTopLeft(find.byType(ProfileMenuAvatar)).dy;

  testWidgets(
      'avatar top offset is identical for a one-line and a two-line title',
      (tester) async {
    await tester.pumpWidget(host(const DestinationHeader(title: 'Library')));
    final oneLineTop = avatarTop(tester);

    await tester
        .pumpWidget(host(const DestinationHeader(title: 'Good afternoon,\nThomas')));
    final twoLineTop = avatarTop(tester);

    expect(twoLineTop, oneLineTop);
  });

  testWidgets('avatar top offset is unaffected by a subtitle', (tester) async {
    await tester.pumpWidget(host(const DestinationHeader(title: 'Plans')));
    final noSubtitleTop = avatarTop(tester);

    await tester.pumpWidget(host(const DestinationHeader(
      title: 'Plans',
      subtitle: 'July 2026',
    )));
    final withSubtitleTop = avatarTop(tester);

    expect(withSubtitleTop, noSubtitleTop);
  });

  testWidgets('avatar top offset is unaffected by trailing controls',
      (tester) async {
    await tester.pumpWidget(host(const DestinationHeader(title: 'Library')));
    final noTrailingTop = avatarTop(tester);

    await tester.pumpWidget(host(DestinationHeader(
      title: 'Library',
      trailing: [
        Container(width: 60, height: 34, color: Colors.black12),
      ],
    )));
    final withTrailingTop = avatarTop(tester);

    expect(withTrailingTop, noTrailingTop);
  });

  testWidgets(
      'search chip renders to the right of the avatar, and both hide on '
      'pushed variants', (tester) async {
    await tester.pumpWidget(host(const DestinationHeader(title: 'Library')));

    final searchIcon = find.byIcon(Icons.search);
    expect(searchIcon, findsOneWidget);
    expect(
      tester.getTopLeft(searchIcon).dx,
      greaterThan(tester.getTopRight(find.byType(ProfileMenuAvatar)).dx),
    );

    await tester.pumpWidget(host(const DestinationHeader(
      title: 'Library',
      showAvatar: false,
    )));
    expect(find.byIcon(Icons.search), findsNothing);
    expect(find.byType(ProfileMenuAvatar), findsNothing);
  });
}
