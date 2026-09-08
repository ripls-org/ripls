import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/data/gen/ripls/api/request.pbenum.dart' as proto;
import 'package:ripls/presentation/viewmodels/needs_scope.dart';
import 'package:ripls/presentation/viewmodels/request_needs_view_model.dart';
import 'package:ripls/presentation/viewmodels/request_view_model.dart';
import 'package:ripls/presentation/widgets/content/content_shared_widgets.dart'
    show ConversationPaneEmptyState;
import 'package:ripls/presentation/widgets/content/inline_conversation_view.dart';
import 'package:ripls/presentation/widgets/needs/needs_actions.dart';
import 'package:ripls/services/providers.dart';

/// RequestChatPane renders the third tab (Discuss) of the request content view.
///
/// Hosts the inline conversation with needs/contributions context so that
/// need-tagged messages are tappable inline.
class RequestChatPane extends ConsumerWidget {
  final RequestState state;
  final String requestId;
  final Color accentColor;
  final Future<void> Function() onAddMedia;
  final Future<void> Function(String mediaId) onDeleteMedia;
  final Future<void> Function(List<String> mediaIds)? onReorderMedia;

  /// Forces the conversation to initialize regardless of tab state. Set when
  /// hosted on its own surface (the morph-reveal conversation panel), which has
  /// no active-tab signal.
  final bool forceActive;

  const RequestChatPane({
    super.key,
    required this.state,
    required this.requestId,
    required this.accentColor,
    required this.onAddMedia,
    required this.onDeleteMedia,
    this.onReorderMedia,
    this.forceActive = false,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final request = state.requestDetails;
    if (request == null || request.conversationId.isEmpty) {
      return const ConversationPaneEmptyState();
    }

    final needsState = ref.watch(requestNeedsProvider(requestId));
    final needById = {
      for (final RequestNeedResponse n in needsState.needs) n.id: n,
    };
    final contribById = {
      for (final RequestContributionResponse c in needsState.contributions)
        c.id: c,
    };
    final isTerminal =
        request.state == proto.RequestState.REQUEST_STATE_FULFILLED ||
        request.state == proto.RequestState.REQUEST_STATE_CANCELLED;
    final needsActions = NeedsActions(
      scope: NeedsScope.request(
        requestId: requestId,
        currentUserId: state.currentUserId,
        isTerminal: isTerminal,
        communityId: state.communityId ?? '',
        isOwner: state.isOwner,
        requestOwnerId: request.requester.id,
        requestName: request.title,
      ),
    );

    return InlineConversationView(
      key: ValueKey(request.conversationId),
      conversationId: request.conversationId,
      accentColor: accentColor,
      isActive: forceActive,
      isExpanded: true,
      mediaItems: state.allMediaItems,
      onAddMedia: onAddMedia,
      onDeleteMedia: onDeleteMedia,
      isOwner: state.isOwner,
      onReorderMedia: onReorderMedia,
      onMessagesMarkedAsRead: (conversationId) =>
          resetUnreadForConversation(ref, conversationId),
      experienceNeedNames: {
        for (final RequestNeedResponse n in needsState.needs) n.id: n.name,
      },
      experienceContributionNames: {
        for (final RequestContributionResponse c in needsState.contributions)
          c.id: c.title,
      },
      onExperienceNeedTap: (needId) {
        final need = needById[needId];
        if (need == null || !context.mounted) return;
        needsActions.onNeedTap(context, ref, need);
      },
      onExperienceContributionTap: (contribId) {
        final contrib = contribById[contribId];
        if (contrib == null || !context.mounted) return;
        needsActions.onContributionTap(context, ref, contrib);
      },
    );
  }
}
