import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/repositories/media_repository.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/media/video_background_host.dart';
import 'package:ripls/services/providers.dart';

import '../../../helpers/l10n_helpers.dart';

/// Throws on every video fetch — drives `createCachedVideoController` into
/// its catch path without touching a platform binding.
class _ThrowingMediaRepository implements MediaRepository {
  int videoFileCalls = 0;

  @override
  Future<File> getVideoFile(String mediaId, String url) async {
    videoFileCalls++;
    throw StateError('forced failure for $mediaId');
  }

  @override
  dynamic noSuchMethod(Invocation invocation) =>
      super.noSuchMethod(invocation);
}

void main() {
  late _ThrowingMediaRepository repo;

  setUp(() {
    repo = _ThrowingMediaRepository();
  });

  Widget buildHost({required String mediaId, required String mediaPath}) =>
      ProviderScope(
        overrides: [
          mediaRepositoryProvider.overrideWithValue(repo),
        ],
        child: localizedApp(
          Scaffold(
            body: VideoBackgroundHost(
              isVideo: true,
              mediaId: mediaId,
              mediaPath: mediaPath,
            ),
          ),
        ),
      );

  testWidgets('shows snackbar when video controller fails to load', (tester) async {
    await tester.pumpWidget(
      buildHost(mediaId: 'media-1', mediaPath: 'https://example.com/v.mp4'),
    );
    // Settle the async load and the SnackBar's enter animation.
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 750));

    final scaffoldContext = tester.element(find.byType(Scaffold).first);
    final expected = AppLocalizations.of(scaffoldContext).mediaVideoLoadFailed;
    expect(find.text(expected), findsOneWidget);
    expect(repo.videoFileCalls, 1);
  });

  testWidgets('snackbar is shown at most once per mediaId on repeated loads',
      (tester) async {
    // First load fails and surfaces the snackbar.
    await tester.pumpWidget(
      buildHost(mediaId: 'media-1', mediaPath: 'https://example.com/v.mp4'),
    );
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 750));

    final scaffoldContext = tester.element(find.byType(Scaffold).first);
    final expected = AppLocalizations.of(scaffoldContext).mediaVideoLoadFailed;
    expect(find.text(expected), findsOneWidget);
    expect(repo.videoFileCalls, 1);

    // Toggle to media-2 then back to media-1. The dedup Set persists across
    // toggles because pumpWidget reuses the State instance for the same
    // widget type. The third load (media-1 again) re-runs the failure path
    // (videoFileCalls rises) but must not queue a second media-1 snackbar.
    await tester.pumpWidget(
      buildHost(mediaId: 'media-2', mediaPath: 'https://example.com/v2.mp4'),
    );
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 750));
    await tester.pumpWidget(
      buildHost(mediaId: 'media-1', mediaPath: 'https://example.com/v.mp4'),
    );
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 750));
    expect(repo.videoFileCalls, 3);

    // Drain the snackbar queue and count how many distinct snackbars Material
    // dequeues. With dedup, only media-1's and media-2's snackbars ever
    // queued — exactly two. Without dedup, the second media-1 load would
    // queue a third.
    final messenger = ScaffoldMessenger.of(
      tester.element(find.byType(Scaffold).first),
    );
    var displayed = 1; // media-1's snackbar is already on-screen.
    while (true) {
      messenger.hideCurrentSnackBar();
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 750));
      if (find.text(expected).evaluate().isEmpty) break;
      displayed++;
      if (displayed > 5) fail('snackbar queue never drained');
    }
    expect(displayed, 2);
  });
}
