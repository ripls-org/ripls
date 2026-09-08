import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/app_bar_back_button.dart';
import 'package:ripls/presentation/widgets/swipe_to_close_mixin.dart';

/// ManageMembershipsScreen displays a user's community memberships.
///
/// Placeholder implementation; full membership management is not yet shipped.
class ManageMembershipsScreen extends StatefulWidget {
  const ManageMembershipsScreen({super.key, required this.userId});

  final String userId;

  @override
  State<ManageMembershipsScreen> createState() =>
      _ManageMembershipsScreenState();
}

class _ManageMembershipsScreenState extends State<ManageMembershipsScreen>
    with SingleTickerProviderStateMixin, SwipeToCloseMixin {
  @override
  Widget build(BuildContext context) {
    return buildSwipeableScaffold(
      backgroundColor: AppColors.background(context),
      appBar: AppBar(
        backgroundColor: AppColors.appBarBackground(context),
        elevation: 0,
        leading: AppBarBackButton(
          onPressed: handleClose,
        ),
        title: Text(
          context.l10n.profileMemberships,
          style: TextStyle(color: AppColors.textPrimary(context)),
        ),
      ),
      body: Center(
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Column(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              Icon(
                Icons.construction,
                size: 64,
                color: AppColors.textSecondary(context),
              ),
              const SizedBox(height: 16),
              Text(
                'Coming Soon',
                style: Theme.of(context).textTheme.headlineSmall?.copyWith(
                      color: AppColors.textPrimary(context),
                      fontWeight: FontWeight.bold,
                    ),
              ),
              const SizedBox(height: 8),
              Text(
                'Membership management is not yet available.',
                textAlign: TextAlign.center,
                style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                      color: AppColors.textSecondary(context),
                    ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
