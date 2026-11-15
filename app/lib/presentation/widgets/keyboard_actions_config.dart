import 'package:flutter/foundation.dart';
import 'package:flutter/widgets.dart';
import 'package:keyboard_actions/keyboard_actions.dart';

/// DoneKeyboardActions shows a "Done" toolbar above the iOS keyboard for every
/// text field beneath [child].
///
/// The toolbar is iOS-only and does not traverse focus between fields —
/// tapping "Done" dismisses the keyboard. Fields are discovered automatically,
/// so no per-field [FocusNode] registration is needed.
///
/// Set [dismissOnTapOutside] to also close the keyboard when the user taps
/// outside the focused field and the toolbar.
///
/// Usage:
/// ```dart
/// DoneKeyboardActions(
///   dismissOnTapOutside: true,
///   child: ...,
/// )
/// ```
class DoneKeyboardActions extends StatelessWidget {
  const DoneKeyboardActions({
    super.key,
    required this.child,
    this.dismissOnTapOutside = false,
  });

  final Widget child;
  final bool dismissOnTapOutside;

  @override
  Widget build(BuildContext context) {
    return KeyboardActions.done(
      enabled: defaultTargetPlatform == TargetPlatform.iOS,
      dismissOnTapOutside: dismissOnTapOutside,
      child: child,
    );
  }
}
