import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';

/// GoogleSignInButton displays a button for signing in with Google.
///
/// The mark is Material's `g_mobiledata` glyph rather than Google's own
/// logo: the repo ships no Google brand asset, and Google's guidelines
/// don't permit redrawing it (#2724).
class GoogleSignInButton extends StatelessWidget {
  final VoidCallback onPressed;
  final bool isLoading;

  const GoogleSignInButton({
    super.key,
    required this.onPressed,
    this.isLoading = false,
  });

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      width: double.infinity,
      child: OutlinedButton.icon(
        onPressed: isLoading ? null : onPressed,
        icon: isLoading
            ? const SizedBox(
                width: 20,
                height: 20,
                child: CircularProgressIndicator(strokeWidth: 2),
              )
            : const Icon(Icons.g_mobiledata, size: 24),
        label: Text(context.l10n.authContinueWithGoogle),
        style: OutlinedButton.styleFrom(
          padding: const EdgeInsets.symmetric(vertical: 12, horizontal: 16),
        ),
      ),
    );
  }
}

