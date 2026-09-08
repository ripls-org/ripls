import 'package:flutter/material.dart';
import '../../../core/utils/responsive.dart';

/// AuthScreenWrapper wraps authentication screens to provide a centered,
/// modal-like appearance on desktop while maintaining full-screen on mobile.
class AuthScreenWrapper extends StatelessWidget {
  final Widget child;
  final double maxWidth;

  const AuthScreenWrapper({
    super.key,
    required this.child,
    this.maxWidth = 480,
  });

  @override
  Widget build(BuildContext context) {
    if (Responsive.isMobile(context)) {
      // Mobile: full-screen layout
      return child;
    }

    // Desktop/Tablet: centered modal-like layout
    return Container(
      decoration: BoxDecoration(
        gradient: LinearGradient(
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
          colors: [
            Theme.of(context).colorScheme.primary.withAlpha(26),
            Theme.of(context).colorScheme.secondary.withAlpha(26),
          ],
        ),
      ),
      child: Center(
        child: ConstrainedBox(
          constraints: BoxConstraints(maxWidth: maxWidth),
          child: Card(
            elevation: 8,
            shape: RoundedRectangleBorder(
              borderRadius: BorderRadius.circular(16),
            ),
            child: child,
          ),
        ),
      ),
    );
  }
}
