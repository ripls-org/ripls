import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/media/video_mute_toggle_button.dart';

import '../../../helpers/l10n_helpers.dart';

void main() {
  group('VideoMuteToggleButton', () {
    testWidgets('renders volume_off when muted', (tester) async {
      await tester.pumpWidget(
        localizedApp(
          Scaffold(
            body: VideoMuteToggleButton(
              isMuted: true,
              onTap: () {},
            ),
          ),
        ),
      );

      expect(find.byIcon(Icons.volume_off), findsOneWidget);
      expect(find.byIcon(Icons.volume_up), findsNothing);
    });

    testWidgets('renders volume_up when unmuted', (tester) async {
      await tester.pumpWidget(
        localizedApp(
          Scaffold(
            body: VideoMuteToggleButton(
              isMuted: false,
              onTap: () {},
            ),
          ),
        ),
      );

      expect(find.byIcon(Icons.volume_up), findsOneWidget);
      expect(find.byIcon(Icons.volume_off), findsNothing);
    });

    testWidgets('invokes onTap when tapped', (tester) async {
      var taps = 0;
      await tester.pumpWidget(
        localizedApp(
          Scaffold(
            body: VideoMuteToggleButton(
              isMuted: true,
              onTap: () => taps++,
            ),
          ),
        ),
      );

      await tester.tap(find.byType(VideoMuteToggleButton));
      expect(taps, 1);
    });

    testWidgets(
        'semantics label is the localized action ("Unmute" when muted, '
        '"Mute" when audible)', (tester) async {
      // When muted, the next tap unmutes — label describes that action.
      await tester.pumpWidget(
        localizedApp(
          Scaffold(
            body: VideoMuteToggleButton(
              isMuted: true,
              onTap: () {},
            ),
          ),
        ),
      );
      final mutedContext = tester.element(find.byType(VideoMuteToggleButton));
      final l10n = AppLocalizations.of(mutedContext);
      expect(find.bySemanticsLabel(l10n.a11yMiscUnmute), findsOneWidget);

      // When audible, the next tap mutes — label flips.
      await tester.pumpWidget(
        localizedApp(
          Scaffold(
            body: VideoMuteToggleButton(
              isMuted: false,
              onTap: () {},
            ),
          ),
        ),
      );
      expect(find.bySemanticsLabel(l10n.a11yMiscMute), findsOneWidget);
    });
  });
}
