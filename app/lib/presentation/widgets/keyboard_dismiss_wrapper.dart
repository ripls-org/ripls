import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// KeyboardDismissWrapper dismisses the software keyboard when the user taps
/// outside any text field.
///
/// Wraps content with a [GestureDetector] that calls [FocusScope.unfocus] on
/// tap, allowing users to dismiss the keyboard by tapping the background. This
/// is a pure presentation widget with no business logic.
///
/// Follows the same pattern as [SwipeToCloseWrapper] — a reusable presentation
/// widget that adds gesture behavior without managing application state.
///
/// Example usage:
/// ```dart
/// KeyboardDismissWrapper(
///   child: Scaffold(
///     body: Column(
///       children: [
///         TextField(...),
///         ElevatedButton(onPressed: _submit, child: Text('Submit')),
///       ],
///     ),
///   ),
/// )
/// ```
class KeyboardDismissWrapper extends StatelessWidget {
  final Widget child;

  const KeyboardDismissWrapper({
    super.key,
    required this.child,
  });

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: context.l10n.a11yMiscDismissKeyboard,
      onTap: () => FocusScope.of(context).unfocus(),
      excludeChildSemantics: false,
      child: child,
    );
  }
}
