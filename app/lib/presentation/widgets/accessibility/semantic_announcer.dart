import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/semantics.dart';

/// SemanticAnnouncer announces a transient string to assistive technologies.
/// Use this for one-shot status messages that aren't tied to a persistent
/// widget — for example, "Loaded" after a network fetch completes, or
/// "Copied to clipboard" after a copy action.
///
/// For *persistent* status content (e.g. an error banner that stays on
/// screen), use [LiveRegion] to wrap the visible widget instead so screen
/// readers re-announce the content whenever it changes.
class SemanticAnnouncer {
  const SemanticAnnouncer._();

  /// Announces [message] to assistive tech for the given [context]. The text
  /// direction is read from the ambient [Directionality], or LTR when none is
  /// available.
  ///
  /// [assertiveness] defaults to polite — the announcement waits for the
  /// reader's current utterance to finish. Use [Assertiveness.assertive] for
  /// errors and other content that must interrupt.
  static Future<void> announce(
    BuildContext context,
    String message, {
    Assertiveness assertiveness = Assertiveness.polite,
  }) async {
    if (message.isEmpty) return;
    final view = View.maybeOf(context);
    if (view == null) return;
    final textDirection =
        Directionality.maybeOf(context) ?? TextDirection.ltr;
    // Fire-and-forget the platform channel call: on some platforms (notably the
    // iOS simulator with no assistive tech active) sendAnnouncement's Future
    // can hang indefinitely. Callers awaiting this method must never block the
    // UI on screen-reader delivery — the announcement is best-effort.
    unawaited(SemanticsService.sendAnnouncement(
      view,
      message,
      textDirection,
      assertiveness: assertiveness,
    ));
  }
}

/// LiveRegion wraps a widget so that any change to its descendant content is
/// re-announced to screen readers. Use for in-page status messages: error
/// banners, connectivity banners, form validation errors, loading completion
/// labels.
///
/// Implemented as a thin wrapper over `Semantics(liveRegion: true)` so the
/// rule of "every status message must be a LiveRegion" can be enforced by
/// lint rules without coupling callers to the underlying
/// `Semantics` API.
class LiveRegion extends StatelessWidget {
  final Widget child;

  /// When false, the live region is disabled (e.g. while the widget is
  /// hidden). Defaults to true.
  final bool enabled;

  const LiveRegion({super.key, required this.child, this.enabled = true});

  @override
  Widget build(BuildContext context) {
    if (!enabled) return child;
    return Semantics(liveRegion: true, child: child);
  }
}
