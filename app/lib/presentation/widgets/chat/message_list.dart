import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/data/gen/ripls/api/chat_service.pb.dart'
    show ChatSystemAction;
import 'package:ripls/data/gen/ripls/api/experience_service.pb.dart'
    show RSVPIntention;
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart' show TransferState;
import 'package:ripls/presentation/models/chat_message.dart';
import 'package:ripls/presentation/viewmodels/experience_view_model.dart'
    show experienceProvider;
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/chat/mention/mention_types.dart';
import 'package:ripls/presentation/widgets/chat/message_list/chat_bubble.dart';
import 'package:ripls/presentation/widgets/chat/message_list/poll_banner_or_label.dart';
import 'package:ripls/presentation/widgets/chat/message_list/system_message_row.dart';
import 'package:ripls/presentation/widgets/chat/reaction/emoji_picker_sheet.dart';
import 'package:ripls/presentation/widgets/chat/reaction/reaction_detail_sheet.dart';
import 'package:ripls/presentation/widgets/chat/reaction/reaction_picker.dart';
import 'package:ripls/presentation/widgets/chat/system_message_text_resolver.dart';
import 'package:ripls/presentation/widgets/floating_header.dart';

export 'package:ripls/presentation/widgets/chat/message_list/poll_banner_or_label.dart';

/// MessageList displays the list of chat messages in a conversation.
///
/// When [immersive] is true, uses glass-themed bubble styling (semi-transparent
/// backgrounds, white text, backdrop blur) suited for the ambient blurred
/// background. Defaults to false for standard styling.
///
/// When [prependedCard] is provided (e.g. the RSVP invite card for experience
/// conversations), it is rendered before all messages in the list.
///
/// When [appendedCard] is provided (e.g. the attendance or completion card),
/// it is rendered after all messages — at the bottom of the scrollable area.
class MessageList extends ConsumerWidget {
  final List<ChatMessage> messages;
  final String? currentUserId;
  final bool isGroupConversation;
  final ScrollController scrollController;
  final Future<void> Function() onRefresh;
  final void Function(String userId) onUserAvatarTap;
  final Future<void> Function(String messageId) onRetryMessage;
  final Map<String, RSVPIntention> rsvpStatusMap;
  final Map<String, TransferState> transferStatusMap;

  /// Callback when a mention chip is tapped in a message.
  final void Function(MentionType type, String id)? onMentionTap;

  /// Callback when a user long-presses a message and selects an emoji reaction.
  final void Function(String messageId, String emoji)? onReactionSelected;

  /// Callback when a user taps "remove" in the reaction detail sheet.
  final void Function(String messageId)? onReactionRemoved;

  /// Callback when the user picks "Reply" from a message's long-press menu.
  final void Function(String messageId)? onReplyMessage;

  /// Callback when the user picks "Edit" from their own message's long-press menu.
  final void Function(String messageId)? onEditMessage;

  /// Callback when the user picks "Delete" from their own message's long-press menu.
  final void Function(String messageId)? onDeleteMessage;

  /// When true, renders bubbles with glass-themed colors over the ambient background.
  final bool immersive;

  /// Optional workflow card prepended before all messages (e.g. RSVP invite card).
  /// Supplied by WorkflowWidgetFactory for experience conversations.
  final Widget? prependedCard;

  /// Optional workflow card appended after all messages (e.g. attendance card).
  /// Appears at the visual bottom of the conversation — auto-scrolling to
  /// maxScrollExtent brings it into view.
  final Widget? appendedCard;

  /// User IDs who have offered to help with a request. Used to show the
  /// "helping" badge on their chat bubble avatars.
  final Set<String> requestOffererIds;

  /// Override for the bottom list padding.
  ///
  /// When null (default), uses [FloatingHeader.contentBottom] which is sized
  /// for the home nav bar. Pass a larger value for experience conversations
  /// where the taller input bar would otherwise obscure the last messages.
  final double? bottomPadding;

  /// Callback when a media attachment in a message is tapped.
  ///
  /// When provided, the caller is responsible for opening the appropriate
  /// viewer (e.g. the full media carousel with all entity media items).
  /// When null, tapping a message image opens a single-item carousel.
  final void Function(String mediaId)? onMediaTap;

  /// When true, reverses the scroll direction so that the newest messages
  /// anchor to the visual bottom of the viewport. Content sticks to the
  /// bottom when there are fewer messages than the available height, and the
  /// scroll position 0.0 corresponds to the visual bottom (newest messages).
  ///
  /// Callers must scroll to position 0.0 (not maxScrollExtent) to reach the
  /// newest messages when this is true.
  final bool reverse;

  /// Maps experience need IDs to their display names for the reference chip.
  final Map<String, String> experienceNeedNames;

  /// Maps experience contribution IDs to their display names for the reference chip.
  final Map<String, String> experienceContributionNames;

  /// Called when a user taps the need reference chip inside a message bubble.
  final void Function(String needId)? onExperienceNeedTap;

  /// Called when a user taps the contribution reference chip inside a message bubble.
  final void Function(String contributionId)? onExperienceContributionTap;

  /// Experience this conversation is attached to. When provided, system
  /// messages with a [ChatSystemAction.CHAT_SYSTEM_ACTION_TIME_PROPOSED]
  /// action render an inline live poll banner instead of plain text.
  final String? experienceId;

  /// Called when the user taps the inline poll banner rendered for a
  /// TIME_PROPOSED system message. Typically opens the vote modal. The
  /// [pollId] parameter is the specific poll that message represents, so
  /// historical messages can open a read-only view of their own poll rather
  /// than always re-opening the currently-live one.
  final void Function(String? pollId)? onTimePollTap;

  const MessageList({
    super.key,
    required this.messages,
    required this.currentUserId,
    required this.isGroupConversation,
    required this.scrollController,
    required this.onRefresh,
    required this.onUserAvatarTap,
    required this.onRetryMessage,
    this.rsvpStatusMap = const {},
    this.transferStatusMap = const {},
    this.requestOffererIds = const {},
    this.onMentionTap,
    this.onReactionSelected,
    this.onReactionRemoved,
    this.onReplyMessage,
    this.onEditMessage,
    this.onDeleteMessage,
    this.immersive = false,
    this.prependedCard,
    this.appendedCard,
    this.bottomPadding,
    this.reverse = false,
    this.onMediaTap,
    this.experienceNeedNames = const {},
    this.experienceContributionNames = const {},
    this.onExperienceNeedTap,
    this.onExperienceContributionTap,
    this.experienceId,
    this.onTimePollTap,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    // Collect TIME_PROPOSED messages ordered by sent_at so legacy messages
    // (created before poll_id was introduced) can be mapped back to the
    // poll they opened. Also tracks the latest TIME_PROPOSED so only it can
    // ever render the live banner when a poll is active.
    final timeProposedSorted = [
      for (final m in messages)
        if (m.isSystemMessage &&
            m.message.systemMessage.action ==
                ChatSystemAction.CHAT_SYSTEM_ACTION_TIME_PROPOSED)
          m,
    ]..sort((a, b) =>
        a.message.sentAtUnixSec.compareTo(b.message.sentAtUnixSec));
    final latestTimeProposedMessageId =
        timeProposedSorted.isEmpty ? null : timeProposedSorted.last.messageId;

    // Infer a poll_id for each TIME_PROPOSED message by correlating with the
    // experience's proposals. When a message already carries a stored poll_id
    // we trust it. Otherwise, we gather distinct poll_ids from the proposals
    // list (sorted by first proposed_at) and map the N-th TIME_PROPOSED
    // message to the N-th poll — mirroring the order in which polls were
    // opened. This lets pre-fix system messages still open the correct
    // historical poll when tapped.
    final inferredPollIdByMessageId = <String, String>{};
    final experience = experienceId == null
        ? null
        : ref.watch(
            experienceProvider(experienceId!)
                .select((s) => s.experienceDetails?.experience),
          );
    final proposals = experience?.timeProposals ?? const [];
    if (proposals.isNotEmpty && timeProposedSorted.isNotEmpty) {
      final earliestProposalByPollId = <String, int>{};
      for (final p in proposals) {
        if (!p.hasPollId() || p.pollId.isEmpty) continue;
        final at = p.proposedAtUnixSec.toInt();
        final existing = earliestProposalByPollId[p.pollId];
        if (existing == null || at < existing) {
          earliestProposalByPollId[p.pollId] = at;
        }
      }
      final orderedPollIds = earliestProposalByPollId.entries.toList()
        ..sort((a, b) => a.value.compareTo(b.value));
      for (var i = 0;
          i < timeProposedSorted.length && i < orderedPollIds.length;
          i++) {
        inferredPollIdByMessageId[timeProposedSorted[i].messageId] =
            orderedPollIds[i].key;
      }
    }

    final messageWidgets = _buildMessageWidgets(
      context: context,
      latestTimeProposedMessageId: latestTimeProposedMessageId,
      inferredPollIdByMessageId: inferredPollIdByMessageId,
    );

    // When reversed, the visual bottom is scroll position 0. Swap the
    // prepended/appended cards so they remain at the correct visual ends.
    final children = reverse
        ? [?appendedCard, ...messageWidgets.reversed, ?prependedCard]
        : [?prependedCard, ...messageWidgets, ?appendedCard];

    return RefreshIndicator(
      onRefresh: onRefresh,
      color: Colors.purple,
      backgroundColor: AppColors.cardBackground(context),
      child: ListView(
        controller: scrollController,
        reverse: reverse,
        // Manual dismiss prevents the list scroll from closing the keyboard
        // in the inline chat tab, which would conflict with typing new lines.
        keyboardDismissBehavior: ScrollViewKeyboardDismissBehavior.manual,
        padding: EdgeInsets.only(
          left: 16,
          right: 16,
          top: 16,
          bottom: bottomPadding ?? FloatingHeader.contentBottom(context),
        ),
        children: children,
      ),
    );
  }

  /// _buildMessageWidgets iterates over [messages] and returns a widget for each.
  List<Widget> _buildMessageWidgets({
    required BuildContext context,
    required String? latestTimeProposedMessageId,
    required Map<String, String> inferredPollIdByMessageId,
  }) {
    final widgets = <Widget>[];
    for (final chatMessage in messages) {
      if (chatMessage.isSystemMessage) {
        final hasSystem = chatMessage.message.hasSystemMessage();
        // Phase 4b of #1904: prefer the structured template_key +
        // params payload so the message renders in the viewer's
        // locale. Falls back to the literal description for legacy
        // emit sites not yet migrated and historical rows.
        final description = hasSystem
            ? resolveSystemMessageText(
                chatMessage.message.systemMessage,
                context.l10n,
              )
            : '';
        final action = hasSystem
            ? chatMessage.message.systemMessage.action
            : ChatSystemAction.CHAT_SYSTEM_ACTION_UNSPECIFIED;

        // Live poll: render a tappable inline poll banner instead of text.
        // Falls back to the plain label if the poll is no longer active so
        // historical messages stay readable.
        final isLivePoll =
            action == ChatSystemAction.CHAT_SYSTEM_ACTION_TIME_PROPOSED &&
                experienceId != null &&
                onTimePollTap != null;
        if (isLivePoll) {
          final storedPollId =
              hasSystem && chatMessage.message.systemMessage.hasPollId()
                  ? chatMessage.message.systemMessage.pollId
                  : null;
          final resolvedPollId =
              (storedPollId != null && storedPollId.isNotEmpty)
                  ? storedPollId
                  : inferredPollIdByMessageId[chatMessage.messageId];
          widgets.add(
            PollBannerOrLabel(
              experienceId: experienceId!,
              currentUserId: currentUserId ?? '',
              fallbackText: description,
              messagePollId: resolvedPollId,
              isLatestTimeProposed:
                  chatMessage.messageId == latestTimeProposedMessageId,
              onTap: () => onTimePollTap!(resolvedPollId),
            ),
          );
        } else if (description.isNotEmpty) {
          widgets.add(
            Builder(
              builder: (bubbleContext) => SystemMessageRow(
                text: description,
                chatMessage: chatMessage,
                onLongPress: onReactionSelected != null
                    ? () => _showSystemReactionPicker(
                          bubbleContext,
                          chatMessage,
                        )
                    : null,
                onReactionBadgeTap: () =>
                    _showReactionDetailSheet(context, chatMessage),
              ),
            ),
          );
        }
      } else {
        widgets.add(
          ChatBubble(
            chatMessage: chatMessage,
            isCurrentUser: chatMessage.senderId == currentUserId,
            immersive: immersive,
            onUserAvatarTap: onUserAvatarTap,
            onRetryMessage: onRetryMessage,
            rsvpStatus: rsvpStatusMap[chatMessage.senderId],
            transferStatus: transferStatusMap[chatMessage.senderId],
            isOfferer: requestOffererIds.contains(chatMessage.senderId),
            onMentionTap: onMentionTap,
            onReactionSelected: onReactionSelected,
            onReactionRemoved: onReactionRemoved,
            onReplyMessage: onReplyMessage,
            onEditMessage: onEditMessage,
            onDeleteMessage: onDeleteMessage,
            currentUserId: currentUserId,
            onMediaTap: onMediaTap,
            experienceNeedNames: experienceNeedNames,
            experienceContributionNames: experienceContributionNames,
            onExperienceNeedTap: onExperienceNeedTap,
            onExperienceContributionTap: onExperienceContributionTap,
          ),
        );
      }
    }
    return widgets;
  }

  // ── System-message reaction picker ───────────────────────────────────────

  /// Shows the reaction picker overlay anchored above a long-pressed system label.
  void _showSystemReactionPicker(
    BuildContext context,
    ChatMessage chatMessage,
  ) {
    final renderBox = context.findRenderObject() as RenderBox?;
    if (renderBox == null) return;
    final bubbleOffset = renderBox.localToGlobal(Offset.zero);

    final overlay = Overlay.of(context);
    late OverlayEntry entry;

    entry = OverlayEntry(
      builder: (ctx) {
        final pickerBottom = bubbleOffset.dy - 8;
        final pickerTop = (pickerBottom - 52).clamp(8.0, double.infinity);

        return Tappable(
          semanticsLabel: context.l10n.a11yDismiss,
          onTap: () => entry.remove(),
          excludeChildSemantics: false,
          child: Stack(
            children: [
              Positioned(
                left: 24,
                right: 24,
                top: pickerTop,
                child: Center(
                  child: ReactionPicker(
                    onEmojiSelected: (emoji) {
                      entry.remove();
                      onReactionSelected?.call(chatMessage.messageId, emoji);
                    },
                    onExpandPicker: () {
                      entry.remove();
                      _showFullEmojiPicker(context, chatMessage);
                    },
                  ),
                ),
              ),
            ],
          ),
        );
      },
    );
    overlay.insert(entry);
  }

  /// Shows a bottom sheet with the full emoji picker grid for a system message.
  void _showFullEmojiPicker(BuildContext context, ChatMessage chatMessage) {
    showAccessibleModal<void>(context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (ctx) => EmojiPickerSheet(
        onEmojiSelected: (emoji) {
          Navigator.pop(ctx);
          onReactionSelected?.call(chatMessage.messageId, emoji);
        },
      ),
    );
  }

  /// Shows a bottom sheet with the reaction detail (who reacted + remove).
  void _showReactionDetailSheet(
    BuildContext context,
    ChatMessage chatMessage,
  ) {
    showAccessibleModal<void>(context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (ctx) => ReactionDetailSheet(
        reactions: chatMessage.reactions,
        currentUserId: currentUserId ?? '',
        onRemove: onReactionRemoved != null
            ? () => onReactionRemoved!(chatMessage.messageId)
            : null,
      ),
    );
  }
}
