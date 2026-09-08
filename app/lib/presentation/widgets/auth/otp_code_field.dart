import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

/// The 6-digit one-time-code entry field, shared by the phone and email
/// verification flows so both look and behave identically — the two sit next
/// to each other as sign-in options, and a code box that differs between them
/// reads as two unrelated features.
///
/// The placeholder is deliberately *not* a caller parameter. It was one, and
/// the two flows promptly drifted apart (#2925): phone showed a dash ruler and
/// email spelled out "6-digit code". Anything that must match between the two
/// belongs here as a constant, not in an argument each caller supplies.
///
/// Only the field is shared. The buttons below it are not: the phone flow
/// offers verify/resend, the email flow also offers "use a different email",
/// and folding those differences in here would make the widget a switch
/// statement over its two callers.
///
/// Accessibility: [labelText] is a real `labelText` rather than a bare hint,
/// so the field is announced and — because Flutter Web composes a field's
/// accessible name from its label *and* its hint, which merge into one
/// semantics node — locatable by name from the Playwright harness (see
/// docs/client/testing/semantics_identifiers.md). The label must therefore
/// stay a real localized label: [placeholder] alone would announce as a run of
/// dashes, which is the state this field was already fixed out of once.
class OtpCodeField extends StatelessWidget {
  /// Controller owning the entered digits. The caller watches it to enable
  /// its own submit button.
  final TextEditingController controller;

  /// Accessible label for the field, e.g. "Enter the 6-digit code".
  final String labelText;

  /// Called when the user submits from the keyboard, so a full code can be
  /// verified without reaching for the button. Callers must gate this on the
  /// same condition as their submit button, in-flight check included —
  /// otherwise the keyboard's submit key can fire a second verification while
  /// the first is still running.
  final VoidCallback? onSubmitted;

  /// Distinguishes this field from other text fields on the same screen.
  /// Required rather than optional: the email flow swaps an address field
  /// (`TextInputType.emailAddress`) for this one (`TextInputType.number`)
  /// within a single screen, and without distinct keys Flutter reuses the
  /// element and strands the keyboard on the previous type.
  final Key fieldKey;

  const OtpCodeField({
    super.key,
    required this.controller,
    required this.labelText,
    required this.fieldKey,
    this.onSubmitted,
  });

  /// The number of digits in a one-time code, shared by both flows.
  static const int codeLength = 6;

  /// Placeholder shown inside the box before anything is typed. Renders as a
  /// spaced-out ruler rather than six tight hyphens because the field's own
  /// `letterSpacing` reaches the hint through `InputDecorator`'s `baseStyle`.
  ///
  /// Not an ARB key, unlike every other string on these screens: a hyphen run
  /// is field-shape iconography with no linguistic content, and there is no
  /// locale in which it means something else. Routing it through ARB is what
  /// let the two flows diverge in the first place — a translatable
  /// "6-digit code" string existed, so one caller used it. See the exception
  /// noted in docs/client/i18n.md.
  static const String placeholder = '------';

  /// Whether [text] is a submittable code. Both flows ask this question and
  /// must answer it the same way, so the definition lives here rather than
  /// being spelled out at each call site.
  static bool isComplete(String text) => text.length == codeLength;

  /// Hoisted out of `build` because `FilteringTextInputFormatter.digitsOnly`
  /// is not a constant, so an inline list would be rebuilt on every frame.
  static final List<TextInputFormatter> _digitsOnly = [
    FilteringTextInputFormatter.digitsOnly,
  ];

  @override
  Widget build(BuildContext context) {
    return TextField(
      key: fieldKey,
      controller: controller,
      keyboardType: TextInputType.number,
      // The length cap counts characters, so without this a leading space
      // fills one of the six slots and silently swallows the last digit the
      // user types. Filtering at entry makes the cap count digits.
      inputFormatters: _digitsOnly,
      autofocus: true,
      textAlign: TextAlign.center,
      maxLength: codeLength,
      autofillHints: const [AutofillHints.oneTimeCode],
      onSubmitted: onSubmitted == null ? null : (_) => onSubmitted!(),
      style: Theme.of(
        context,
      ).textTheme.headlineMedium?.copyWith(letterSpacing: 8),
      decoration: InputDecoration(
        labelText: labelText,
        hintText: placeholder,
        border: const OutlineInputBorder(),
        filled: true,
        fillColor: Theme.of(context).colorScheme.surfaceContainerHighest,
        // The "0/6" counter is noise next to a field that already caps at six
        // digits and renders them spaced out.
        counterText: '',
      ),
    );
  }
}
