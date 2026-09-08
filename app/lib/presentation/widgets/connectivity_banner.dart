import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/presentation/widgets/accessibility/semantic_announcer.dart';
import 'package:ripls/services/connectivity_service.dart';

/// ConnectivityBanner displays a persistent banner when the app is offline.
/// Automatically shows/hides based on network connectivity status.
class ConnectivityBanner extends ConsumerWidget {
  final Widget child;

  const ConnectivityBanner({
    required this.child,
    super.key,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final connectivityAsync = ref.watch(connectivityStatusProvider);

    return Column(
      children: [
        connectivityAsync.when(
          data: (isOnline) {
            if (isOnline) {
              return const SizedBox.shrink();
            }
            return _buildOfflineBanner(context);
          },
          loading: () => const SizedBox.shrink(),
          error: (error, stackTrace) => const SizedBox.shrink(),
        ),
        Expanded(child: child),
      ],
    );
  }

  Widget _buildOfflineBanner(BuildContext context) {
    final theme = Theme.of(context);

    return LiveRegion(
      child: Material(
        color: theme.colorScheme.errorContainer,
        child: SafeArea(
          bottom: false,
          child: Container(
            width: double.infinity,
            padding: const EdgeInsets.symmetric(
              horizontal: 16,
              vertical: 12,
            ),
            child: Row(
              children: [
                Icon(
                  Icons.cloud_off,
                  size: 20,
                  color: theme.colorScheme.onErrorContainer,
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: Text(
                    'You\'re offline. Showing cached content.',
                    style: theme.textTheme.bodyMedium?.copyWith(
                      color: theme.colorScheme.onErrorContainer,
                    ),
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}
