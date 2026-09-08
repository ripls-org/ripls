import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/models/media_item_data.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/viewmodels/community_content_view_model.dart';
import 'package:ripls/presentation/widgets/content/content_shared_widgets.dart'
    show ConversationPaneEmptyState;
import 'package:ripls/presentation/widgets/content/inline_conversation_view.dart';
import 'package:ripls/services/providers.dart';

/// CommunityChatPane renders the Discuss tab pane for a community, wrapping
/// the shared [InlineConversationView] with community-scoped state.
///
/// Media props follow the same contract as gear/experience/request chat panes:
/// - [mediaItems] — the community's current media gallery items.
/// - [onAddMedia] — any community member can upload (not creator-gated,
///   unlike the curated carousel on the Community tab which is creator-only).
/// - [onDeleteMedia] — per-item delete; [InlineConversationView] enforces
///   that non-owners can only delete their own uploads.
/// - [onReorderMedia] — reorder is creator-only; pass null for non-creators.
/// - [isOwner] — maps to [widget.canEdit] on the parent community view.
class CommunityChatPane extends ConsumerWidget {
  final String communityId;
  final String? conversationId;
  final double paneHeight;

  /// Server-reported unread count for this conversation.
  ///
  /// Community-wide conversations are excluded from GetUnreadCounts (which
  /// uses participant lists), so the count must be passed in from the caller
  /// who fetches it via GetConversationForCommunity.
  final int unreadCount;

  final List<MediaItemData> mediaItems;
  final Future<void> Function() onAddMedia;
  final Future<void> Function(String mediaId) onDeleteMedia;
  final Future<void> Function(List<String> mediaIds)? onReorderMedia;
  final bool isOwner;

  const CommunityChatPane({
    super.key,
    required this.communityId,
    required this.conversationId,
    required this.paneHeight,
    required this.unreadCount,
    required this.mediaItems,
    required this.onAddMedia,
    required this.onDeleteMedia,
    this.onReorderMedia,
    this.isOwner = false,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final convId = conversationId;
    if (convId == null || convId.isEmpty) {
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
      mediaItems: mediaItems,
      onAddMedia: onAddMedia,
      onDeleteMedia: onDeleteMedia,
      onReorderMedia: onReorderMedia,
      isOwner: isOwner,
      initialUnreadCountOverride: unreadCount,
      onMessagesMarkedAsRead: (cid) async {
        resetUnreadForConversation(ref, cid);
        await ref
            .read(chatRepositoryProvider)
            .refreshConversationForCommunity(communityId);
        ref.read(communityContentProvider.notifier).markConversationRead();
        // Force the inbox provider to re-fetch now that the cache is cleared.
        // The provider stays alive while the inbox is in the navigation stack,
        // so clearing the cache alone does not update its held AsyncValue.
        ref.invalidate(communityConversationProvider(communityId));
      },
    );
  }
}
