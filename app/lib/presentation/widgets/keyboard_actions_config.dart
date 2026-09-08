import 'package:flutter/material.dart';
import 'package:keyboard_actions/keyboard_actions.dart';

/// buildKeyboardActionsConfig creates a standard iOS keyboard toolbar config.
///
/// Returns a [KeyboardActionsConfig] that shows a "Done" button above the iOS
/// keyboard for each node in [focusNodes]. The toolbar is iOS-only and does
/// not traverse focus between fields — tapping "Done" dismisses the keyboard.
///
/// Usage:
/// ```dart
/// KeyboardActions(
///   disableScroll: true,
///   config: buildKeyboardActionsConfig([_titleFocusNode, _descriptionFocusNode]),
///   child: ...,
/// )
/// ```
KeyboardActionsConfig buildKeyboardActionsConfig(List<FocusNode> focusNodes) {
  return KeyboardActionsConfig(
    keyboardActionsPlatform: KeyboardActionsPlatform.IOS,
    nextFocus: false,
    actions: focusNodes
        .map((node) => KeyboardActionsItem(focusNode: node))
        .toList(),
  );
}
