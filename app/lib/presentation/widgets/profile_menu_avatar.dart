import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/profile_menu_modal.dart';
import 'package:ripls/presentation/widgets/user_avatar.dart';
import 'package:ripls/services/providers.dart' show authStateProvider;

/// Tap target rendering the signed-in user's avatar; opens [ProfileMenuModal]
/// — the account-level entry point for Settings and Feedback. Renders
/// `SizedBox.shrink()` when no user is loaded; the modal would be a no-op
/// anyway.
class ProfileMenuAvatar extends ConsumerWidget {
  const ProfileMenuAvatar({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final user = ref.watch(authStateProvider).user;
    if (user == null) {
      return const SizedBox.shrink();
    }
    return Tappable(
      semanticsLabel: context.l10n.a11yOpenProfileMenu,
      onTap: () => ProfileMenuModal.show(context),
      child: UserAvatar(user: user, radius: 14),
    );
  }
}
