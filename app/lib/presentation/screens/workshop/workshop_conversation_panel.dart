import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/content/content_shared_widgets.dart'
    show ConversationPaneEmptyState;
import 'package:ripls/presentation/widgets/content/inline_conversation_view.dart';
import 'package:ripls/presentation/widgets/workshop/workshop_morph.dart';
import 'package:ripls/presentation/widgets/workshop/workshop_overview_theme.dart';
import 'package:ripls/services/providers/chat_providers.dart';

/// The full-screen community conversation the Workshop chat glimpse expands
/// into (#2447). It grows from the glimpse's footprint and — like the roster /
/// known-for / deep-dive destinations — is hosted inside [WorkshopOverlayHost],
/// so the overview body and nav bars are fully covered. This is just the
/// conversation body + a close (✕) in the upper-right, via [WorkshopMorphPanel].
class WorkshopConversationPanel extends ConsumerWidget {
  final String communityId;

  const WorkshopConversationPanel({super.key, required this.communityId});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final convAsync = ref.watch(communityConversationProvider(communityId));

    return WorkshopMorphPanel(
      child: convAsync.when(
        loading: () => const Center(
          child: CircularProgressIndicator(
            color: WorkshopOverviewPalette.onPhoto,
          ),
        ),
        error: (e, _) => Center(
          child: Padding(
            padding: const EdgeInsets.all(24),
            child: Text(
              e.toString(),
              textAlign: TextAlign.center,
              style: const TextStyle(
                color: WorkshopOverviewPalette.onPhotoDim,
              ),
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
              ref.invalidate(communityConversationProvider(communityId));
            },
          );
        },
      ),
    );
  }
}
