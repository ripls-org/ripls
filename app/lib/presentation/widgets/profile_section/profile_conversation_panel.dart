import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';
import 'package:ripls/presentation/widgets/content/content_morph_panel.dart';
import 'package:ripls/presentation/widgets/content/content_shared_widgets.dart'
    show ConversationPaneEmptyState;
import 'package:ripls/presentation/widgets/content/inline_conversation_view.dart';
import 'package:ripls/services/providers/chat_providers.dart';

/// The full-screen standing conversation the person and community
/// profiles' message bubble expands into (issue #2568/#2634) — the same
/// morph-reveal mechanism `experienceContentView` and the other content
/// views use (`ContentMorphPanel` + `InlineConversationView`), grown from
/// the tapped bubble's own footprint via `openContentMorphPanel`.
///
/// Both profiles route the standing thread through the shared community's
/// conversation (no 1:1 DM surface exists yet, #2568), so this panel takes
/// [communityId] and resolves the conversation itself — mirroring
/// `WorkshopConversationPanel`, but with the plain [ContentMorphPanel]
/// chrome every other content view uses rather than Workshop's
/// photo-backdrop styling.
class ProfileConversationPanel extends ConsumerWidget {
  const ProfileConversationPanel({super.key, required this.communityId});

  final String communityId;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final convAsync = ref.watch(communityConversationProvider(communityId));

    return ContentMorphPanel(
      // Named for the e2e harness (docs/client/testing/semantics_identifiers.md):
      // a deep link can open this panel without a tap (#2876), and "the
      // conversation is open" is otherwise only assertable via the composer,
      // which is indistinguishable from any other text field in the DOM.
      child: Semantics(
        container: true,
        explicitChildNodes: true,
        identifier: 'community-conversation-panel',
        child: SafeArea(
          child: Stack(
            children: [
              convAsync.when(
                loading: () => const Center(
                  child: CircularProgressIndicator(
                    color: GlassTokens.textPrimary,
                  ),
                ),
                error: (e, _) => Center(
                  child: Padding(
                    padding: const EdgeInsets.all(24),
                    child: Text(
                      e.toString(),
                      textAlign: TextAlign.center,
                      style: const TextStyle(color: GlassTokens.textMuted),
                    ),
                  ),
                ),
                data: (conv) {
                  final convId = conv?.conversationId ?? '';
                  if (convId.isEmpty) {
                    return ConversationPaneEmptyState(
                      message: context.l10n.communityChatEmpty,
                    );
                  }
                  return InlineConversationView(
                    key: ValueKey(convId),
                    conversationId: convId,
                    accentColor: AppColors.primary(context),
                    isActive: true,
                    isExpanded: true,
                    initialUnreadCountOverride: conv?.unreadCount,
                    onMessagesMarkedAsRead: (cid) async {
                      resetUnreadForConversation(ref, cid);
                      await ref
                          .read(chatRepositoryProvider)
                          .refreshConversationForCommunity(communityId);
                      ref.invalidate(
                        communityConversationProvider(communityId),
                      );
                    },
                  );
                },
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
      ),
    );
  }
}
