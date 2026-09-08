import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/core/theme/gen/overlay_tokens.gen.dart';
import 'package:ripls/core/utils/navigation_helpers.dart';
import 'package:ripls/presentation/screens/users/user_screen.dart';
import 'package:ripls/presentation/viewmodels/community_content_view_model.dart'
    show communityMembersProvider;
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/content/content_morph_panel.dart';
import 'package:ripls/presentation/widgets/profile_section/profile_face_stack.dart';

/// The full-screen members list the community profile's members row
/// expands into (issue #2568/#2634) — the same morph-reveal mechanism
/// the profile conversation uses (`ContentMorphPanel` chrome, grown from
/// the tapped row's footprint via `openContentMorphPanel`). Keyed
/// explicitly by [communityId] (deliberately not the workshop roster
/// sheet, whose stats resolve through workshop-scope providers that a
/// pushed route cannot override). Tapping a member opens their profile.
class ProfileMembersPanel extends ConsumerWidget {
  const ProfileMembersPanel({super.key, required this.communityId});

  final String communityId;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final membersAsync = ref.watch(communityMembersProvider(communityId));

    return ContentMorphPanel(
      child: SafeArea(
        child: Stack(
          children: [
            Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Padding(
                  padding: const EdgeInsets.fromLTRB(24, 10, 56, 8),
                  child: Text(
                    context.l10n.communityMembers,
                    style: const TextStyle(
                      fontFamily: AppTheme.headingFont,
                      fontSize: 24,
                      fontWeight: FontWeight.w600,
                      color: OverlayTokens.textPrimary,
                    ),
                  ),
                ),
                Expanded(
                  child: membersAsync.when(
                    loading: () => const Center(
                      child: CircularProgressIndicator(
                        color: OverlayTokens.textPrimary,
                      ),
                    ),
                    error: (e, _) => Center(
                      child: Padding(
                        padding: const EdgeInsets.all(24),
                        child: Text(
                          e.toString(),
                          textAlign: TextAlign.center,
                          style: const TextStyle(
                            color: OverlayTokens.textFaint,
                          ),
                        ),
                      ),
                    ),
                    data: (members) => ListView.builder(
                      padding: const EdgeInsets.fromLTRB(24, 4, 24, 24),
                      itemCount: members.length,
                      itemBuilder: (context, i) {
                        final user = members[i].user;
                        return Tappable(
                          semanticsLabel: user.name,
                          onTap: () => NavigationHelpers.pushScreen(
                            context: context,
                            screen: UserScreen(userId: user.id),
                            routeName: 'profile',
                            useRootNavigator: true,
                          ),
                          child: Container(
                            padding:
                                const EdgeInsets.symmetric(vertical: 10),
                            decoration: const BoxDecoration(
                              border: Border(
                                bottom: BorderSide(
                                  color: GlassTokens.hairline,
                                ),
                              ),
                            ),
                            child: Row(
                              children: [
                                ProfileFaceStack(
                                  faces: [
                                    FaceStackEntry(
                                      mediaId: user.mediaId.isEmpty
                                          ? null
                                          : user.mediaId,
                                      initial: user.name.isEmpty
                                          ? null
                                          : user.name[0],
                                    ),
                                  ],
                                  size: 38,
                                ),
                                const SizedBox(width: 12),
                                Expanded(
                                  child: Text(
                                    user.name,
                                    maxLines: 1,
                                    overflow: TextOverflow.ellipsis,
                                    style: const TextStyle(
                                      fontSize: 14.5,
                                      fontWeight: FontWeight.w600,
                                      color: OverlayTokens.textPrimary,
                                    ),
                                  ),
                                ),
                              ],
                            ),
                          ),
                        );
                      },
                    ),
                  ),
                ),
              ],
            ),
            Positioned(
              top: 4,
              right: 8,
              child: IconAction(
                icon: Icons.close_rounded,
                semanticsLabel: context.l10n.a11yClose,
                color: AppColors.onContentImage,
                onPressed: () => Navigator.of(context).pop(),
              ),
            ),
          ],
        ),
      ),
    );
  }
}
