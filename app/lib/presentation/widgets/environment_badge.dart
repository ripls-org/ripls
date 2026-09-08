import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import '../../core/config/environment.dart';

/// EnvironmentBadge displays the current server URL for non-production environments.
///
/// The server URL is only shown for local and dev environments to help users identify
/// which server they are connected to. This is especially useful when switching
/// between local development and staging environments.
class EnvironmentBadge extends StatelessWidget {
  const EnvironmentBadge({super.key});

  @override
  Widget build(BuildContext context) {
    const env = String.fromEnvironment('ENVIRONMENT', defaultValue: 'dev');

    // Hide in production
    if (env == 'prod') {
      return const SizedBox.shrink();
    }

    return Semantics(
      label: context.l10n.a11yMiscEnvironmentBadge(Environment.getServer),
      container: true,
      child: ExcludeSemantics(
        child: Text(
          Environment.getServer,
          style: TextStyle(
            color: Colors.grey.shade600,
            fontSize: 12,
          ),
        ),
      ),
    );
  }
}
