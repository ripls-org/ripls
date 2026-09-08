import 'package:flutter/material.dart';
import 'package:flutter/services.dart' show HapticFeedback;
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/data/gen/ripls/api/experience_service.pb.dart'
    show RSVPIntention;
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart' show TransferState;
import 'package:ripls/presentation/models/avatar_status_badge.dart';
import 'package:ripls/presentation/models/chat_message.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/chat/mention/mention_text.dart';
import 'package:ripls/presentation/widgets/chat/mention/mention_types.dart';
import 'package:ripls/presentation/widgets/chat/message_list/message_media_attachments.dart';
import 'package:ripls/presentation/widgets/chat/reaction/emoji_picker_sheet.dart';
import 'package:ripls/presentation/widgets/chat/reaction/reaction_badge.dart';
import 'package:ripls/presentation/widgets/chat/reaction/reaction_detail_sheet.dart';
import 'package:ripls/presentation/widgets/chat/reaction/reaction_picker.dart';
import 'package:ripls/presentation/widgets/user_avatar.dart';
import 'package:timeago/timeago.dart' as timeago;

/// ChatBubble renders a single chat message row including the sender avatar,
/// name/status row, message bubble, and reaction badge.
///
/// Supports two visual modes: standard (AppColors-based) and [immersive]
/// (glass-themed semi-transparent bubbles for use over an ambient background).
class ChatBubble extends ConsumerWidget {
  const ChatBubble({
    super.key,
    required this.chatMessage,
    required this.isCurrentUser,
    required this.immersive,
    required this.onUserAvatarTap,
    required this.onRetryMessage,
    this.rsvpStatus,
    this.transferStatus,
    this.isOfferer = false,
    this.onMentionTap,
    this.onReactionSelected,
    this.onReactionRemoved,
    this.onReplyMessage,
    this.onEditMessage,
    this.onDeleteMessage,
    this.currentUserId,
    this.onMediaTap,
    this.experienceNeedNames = const {},
    this.experienceContributionNames = const {},
    this.onExperienceNeedTap,
    this.onExperienceContributionTap,
  });

  final ChatMessage chatMessage;
  final bool isCurrentUser;

  /// When true, renders bubbles with glass-themed colors over the ambient background.
  final bool immersive;

  final void Function(String userId) onUserAvatarTap;
  final Future<void> Function(String messageId) onRetryMessage;

  final RSVPIntention? rsvpStatus;
  final TransferState? transferStatus;
  final bool isOfferer;

  /// Callback when a mention chip is tapped in the message.
  final void Function(MentionType type, String id)? onMentionTap;

  /// Callback when a user long-presses and selects an emoji reaction.
  final void Function(String messageId, String emoji)? onReactionSelected;

  /// Callback when a user taps "remove" in the reaction detail sheet.
  final void Function(String messageId)? onReactionRemoved;

  /// Callback when the user picks "Reply" from the long-press menu.
  final void Function(String messageId)? onReplyMessage;

  /// Callback when the user picks "Edit" from the long-press menu (own messages).
  final void Function(String messageId)? onEditMessage;

  /// Callback when the user picks "Delete" from the long-press menu (own messages).
  final void Function(String messageId)? onDeleteMessage;

  /// The current user's ID, used to identify self-reactions.
  final String? currentUserId;

  /// Callback when a media attachment is tapped.
  final void Function(String mediaId)? onMediaTap;

  /// Maps experience need IDs to their display names for the reference chip.
  final Map<String, String> experienceNeedNames;

  /// Maps experience contribution IDs to their display names for the reference chip.
  final Map<String, String> experienceContributionNames;

  /// Called when a user taps the need reference chip inside a message bubble.
  final void Function(String needId)? onExperienceNeedTap;

  /// Called when a user taps the contribution reference chip inside a message bubble.
  final void Function(String contributionId)? onExperienceContributionTap;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final message = chatMessage.message;
    final timestamp = DateTime.fromMillisecondsSinceEpoch(
      message.sentAtUnixSec.toInt() * 1000,
    );
    final badge = _resolveBadge(
      rsvpStatus,
      transferStatus,
      isOfferer: isOfferer,
    );

    return Padding(
      padding: const EdgeInsets.only(bottom: 4),
      child: Row(
        mainAxisAlignment: isCurrentUser
            ? MainAxisAlignment.end
            : MainAxisAlignment.start,
        crossAxisAlignment: CrossAxisAlignment.end,
        children: [
          if (!isCurrentUser) ...[
            Tappable(
              semanticsLabel: context.l10n.a11yChatViewProfile,
              onTap: () => onUserAvatarTap(chatMessage.senderId),
              child: UserAvatar(
                user: chatMessage.sender,
                radius: 13,
                statusBadge: badge,
              ),
            ),
            const SizedBox(width: 8),
          ],
          Flexible(
            child: Column(
              crossAxisAlignment: isCurrentUser
                  ? CrossAxisAlignment.end
                  : CrossAxisAlignment.start,
              children: [
                // Name / status / time row
                if (immersive)
                  _buildImmersiveNameRow(
                    context: context,
                    timestamp: timestamp,
                    badge: badge,
                  )
                else
                  Padding(
                    padding: const EdgeInsets.only(
                      left: 4,
                      right: 4,
                      bottom: 4,
                    ),
                    child: Text(
                      '${_getFirstName(chatMessage.senderName)} · ${timeago.format(timestamp)}',
                      style: TextStyle(
                        color: AppColors.textSecondary(context),
                        fontSize: 10,
                        fontWeight: FontWeight.w500,
                      ),
                    ),
                  ),
                // Message bubble + reaction badge
                Builder(
                  builder: (bubbleContext) => Tappable(
                    semanticsLabel: chatMessage.state == MessageState.failed
                        ? context.l10n.a11yChatRetryMessage
                        : context.l10n.a11yChatMessage,
                    onTap: chatMessage.state == MessageState.failed
                        ? () => onRetryMessage(chatMessage.messageId)
                        : null,
                    onLongPress: _hasLongPressMenu
                        ? () => _showReactionPicker(bubbleContext, chatMessage)
                        : null,
                    excludeChildSemantics: false,
                    child: Column(
                      crossAxisAlignment: isCurrentUser
                          ? CrossAxisAlignment.end
                          : CrossAxisAlignment.start,
                      children: [
                        Opacity(
                          opacity: chatMessage.state == MessageState.pending
                              ? 0.8
                              : 1.0,
                          child: immersive
                              ? _buildImmersiveBubble(
                                  context: context,
                                  ref: ref,
                                  timestamp: timestamp,
                                )
                              : _buildStandardBubble(
                                  context: context,
                                  ref: ref,
                                  timestamp: timestamp,
                                ),
                        ),
                        if (chatMessage.reactions.isNotEmpty)
                          ReactionBadge(
                            reactions: chatMessage.reactions,
                            onTap: () =>
                                _showReactionDetailSheet(context, chatMessage),
                          ),
                      ],
                    ),
                  ),
                ),
              ],
            ),
          ),
          if (isCurrentUser) ...[
            const SizedBox(width: 8),
            Tappable(
              semanticsLabel: context.l10n.a11yChatViewProfile,
              onTap: () => onUserAvatarTap(chatMessage.senderId),
              child: UserAvatar(
                user: chatMessage.sender,
                radius: 13,
                statusBadge: badge,
              ),
            ),
          ],
        ],
      ),
    );
  }

  /// Name / status badge / time row rendered above each bubble in immersive mode.
  Widget _buildImmersiveNameRow({
    required BuildContext context,
    required DateTime timestamp,
    required AvatarStatusBadge? badge,
  }) {
    final timeStr = timeago.format(timestamp);
    return Padding(
      padding: EdgeInsets.only(
        left: isCurrentUser ? 0 : 4,
        right: isCurrentUser ? 4 : 0,
        bottom: 3,
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          if (!isCurrentUser) ...[
            Text(
              _getFirstName(chatMessage.senderName),
              style: const TextStyle(
                fontSize: 11,
                fontWeight: FontWeight.w600,
                color: GlassTokens.textSecondary,
              ),
            ),
            if (badge != null) ...[
              const SizedBox(width: 4),
              _buildStatusLabel(badge),
            ],
            const SizedBox(width: 4),
          ],
          Text(
            timeStr,
            style: const TextStyle(
              fontSize: 10,
              color: GlassTokens.textFaint,
            ),
          ),
        ],
      ),
    );
  }

  /// Small uppercase status label chip (OWNER, REQUESTED, etc.) shown in the
  /// immersive name row beside the sender's first name.
  Widget _buildStatusLabel(AvatarStatusBadge badge) {
    final color = _badgeLabelColor(badge);
    final label = _badgeLabelText(badge);
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 5, vertical: 2),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.094), // ~18/192 ≈ 9%
        borderRadius: BorderRadius.circular(4),
      ),
      child: Text(
        label,
        style: TextStyle(
          fontSize: 9,
          fontWeight: FontWeight.w700,
          color: color,
          letterSpacing: 0.3,
        ),
      ),
    );
  }

  Color _badgeLabelColor(AvatarStatusBadge badge) {
    switch (badge) {
      case AvatarStatusBadge.interested:
      case AvatarStatusBadge.requester:
        return AppColors.transferCoral;
      case AvatarStatusBadge.maybe:
        return AppColors.statusWarningOnDark;
      case AvatarStatusBadge.selected:
      case AvatarStatusBadge.helping:
      case AvatarStatusBadge.going:
      case AvatarStatusBadge.owner:
      case AvatarStatusBadge.organizer:
        return AppColors.transferSage;
    }
  }

  String _badgeLabelText(AvatarStatusBadge badge) {
    switch (badge) {
      case AvatarStatusBadge.owner:
        return 'OWNER';
      case AvatarStatusBadge.organizer:
        return 'ORGANIZER';
      case AvatarStatusBadge.interested:
        return 'REQUESTED';
      case AvatarStatusBadge.selected:
        return 'SELECTED';
      case AvatarStatusBadge.helping:
        return 'HELPING';
      case AvatarStatusBadge.requester:
        return 'REQUESTER';
      case AvatarStatusBadge.going:
        return 'ATTENDING';
      case AvatarStatusBadge.maybe:
        return 'MAYBE';
    }
  }

  /// Standard (non-immersive) bubble using AppColors.
  Widget _buildStandardBubble({
    required BuildContext context,
    required WidgetRef ref,
    required DateTime timestamp,
  }) {
    return Container(
      decoration: const BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.all(Radius.circular(16)),
      ),
      child: _buildBubbleContent(
        context: context,
        ref: ref,
        textColor: Colors.black87,
      ),
    );
  }

  /// Glass-style bubble used in immersive mode.
  ///
  /// My messages: coral-tinted semi-transparent, bottom-right corner flattened.
  /// Other messages: white semi-transparent, bottom-left corner flattened.
  /// Text is white at high opacity for readability against the dark background.
  Widget _buildImmersiveBubble({
    required BuildContext context,
    required WidgetRef ref,
    required DateTime timestamp,
  }) {
    final bgColor = isCurrentUser
        ? AppColors.transferCoral.withValues(alpha: 0.80)
        : Colors.white.withValues(alpha: 0.80);
    final borderColor = isCurrentUser
        ? AppColors.transferCoral.withValues(alpha: 0.40)
        : Colors.white.withValues(alpha: 0.60);
    final textColor = isCurrentUser ? AppColors.darkBackground : Colors.black;
    final radius = BorderRadius.only(
      topLeft: const Radius.circular(16),
      topRight: const Radius.circular(16),
      bottomLeft: Radius.circular(isCurrentUser ? 16 : 4),
      bottomRight: Radius.circular(isCurrentUser ? 4 : 16),
    );

    return Container(
      decoration: BoxDecoration(
        color: bgColor,
        borderRadius: radius,
        border: Border.all(color: borderColor),
      ),
      child: _buildBubbleContent(
        context: context,
        ref: ref,
        textColor: textColor,
      ),
    );
  }

  /// Small pill shown inside a bubble when the message annotates a need or contribution.
  Widget _buildReferenceChip(
    BuildContext context,
    String label, {
    required bool isCurrentUser,
    VoidCallback? onTap,
  }) {
    // Current-user bubbles are light-sage fills (dark chip reads); received
    // bubbles are near-white glass (deep sage reads).
    final chipColor = isCurrentUser
        ? AppColors.darkBackground
        : AppColors.lightPrimary;
    return Tappable(
      semanticsLabel: label,
      isLink: true,
      onTap: onTap,
      child: Container(
        margin: const EdgeInsets.only(left: 16, right: 16, top: 10, bottom: 2),
        padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
        decoration: BoxDecoration(
          color: chipColor.withValues(alpha: 0.18),
          borderRadius: BorderRadius.circular(20),
          border: Border.all(color: chipColor.withValues(alpha: 0.40)),
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(Icons.link, size: 11, color: chipColor),
            const SizedBox(width: 4),
            Flexible(
              child: Text(
                label,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: TextStyle(
                  fontSize: 11,
                  fontWeight: FontWeight.w600,
                  color: chipColor,
                ),
              ),
            ),
            if (onTap != null) ...[
              const SizedBox(width: 3),
              Icon(Icons.chevron_right, size: 11, color: chipColor),
            ],
          ],
        ),
      ),
    );
  }

  /// Builds the quoted preview block for a reply, rendered above the message
  /// text (WhatsApp/Signal style): a leading accent bar, the quoted sender's
  /// name, and a truncated snippet. Colors derive from the bubble's [textColor]
  /// so it adapts to both standard and immersive (glass) styles.
  Widget _buildQuotedBlock(BuildContext context, Color textColor) {
    final reply = chatMessage.replyTo!;
    final quotedName = reply.hasQuotedSender() ? reply.quotedSender.name : '';
    final snippet = reply.quotedText.isNotEmpty
        ? reply.quotedText
        : (reply.hasQuotedMediaId() ? '📷' : '');

    return Semantics(
      label: context.l10n.a11yChatQuotedMessage(quotedName),
      child: Container(
        margin: const EdgeInsets.only(left: 12, right: 12, top: 8),
        padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
        decoration: BoxDecoration(
          color: textColor.withValues(alpha: 0.08),
          borderRadius: BorderRadius.circular(8),
          border: Border(
            left: BorderSide(
              color: textColor.withValues(alpha: 0.45),
              width: 3,
            ),
          ),
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          mainAxisSize: MainAxisSize.min,
          children: [
            if (quotedName.isNotEmpty)
              Text(
                quotedName,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: TextStyle(
                  color: textColor.withValues(alpha: 0.9),
                  fontSize: 12,
                  fontWeight: FontWeight.w600,
                ),
              ),
            if (snippet.isNotEmpty)
              Text(
                snippet,
                maxLines: 2,
                overflow: TextOverflow.ellipsis,
                style: TextStyle(
                  color: textColor.withValues(alpha: 0.7),
                  fontSize: 13,
                ),
              ),
          ],
        ),
      ),
    );
  }

  /// Shared bubble content (media + text + timestamp) for both bubble variants.
  Widget _buildBubbleContent({
    required BuildContext context,
    required WidgetRef ref,
    required Color textColor,
  }) {
    final needId = chatMessage.needId;
    final contribId = chatMessage.contributionId;
    final needTitle = needId != null
        ? (chatMessage.needName ?? experienceNeedNames[needId])
        : null;
    final contribTitle = contribId != null
        ? (chatMessage.contributionTitle ??
              experienceContributionNames[contribId])
        : null;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        if (chatMessage.replyTo != null) _buildQuotedBlock(context, textColor),
        if (needTitle != null)
          _buildReferenceChip(
            context,
            needTitle,
            isCurrentUser: isCurrentUser,
            onTap: onExperienceNeedTap != null
                ? () => onExperienceNeedTap!(needId!)
                : null,
          ),
        if (contribTitle != null)
          _buildReferenceChip(
            context,
            contribTitle,
            isCurrentUser: isCurrentUser,
            onTap: onExperienceContributionTap != null
                ? () => onExperienceContributionTap!(contribId!)
                : null,
          ),
        if (chatMessage.mediaIds.isNotEmpty) ...[
          ClipRRect(
            borderRadius: BorderRadius.only(
              topLeft: const Radius.circular(16),
              topRight: const Radius.circular(16),
              bottomLeft: Radius.circular(
                chatMessage.text.isEmpty && isCurrentUser ? 16 : 0,
              ),
              bottomRight: Radius.circular(
                chatMessage.text.isEmpty && !isCurrentUser ? 16 : 0,
              ),
            ),
            child: MessageMediaAttachments(
              mediaIds: chatMessage.mediaIds,
              onMediaTap: onMediaTap,
            ),
          ),
        ],
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 10),
          child: Row(
            crossAxisAlignment: CrossAxisAlignment.end,
            mainAxisSize: MainAxisSize.min,
            children: [
              Flexible(
                child: chatMessage.text.isNotEmpty
                    ? MentionText(
                        text: chatMessage.text,
                        onDarkBackground: immersive && isCurrentUser,
                        onMentionTap: onMentionTap,
                        style: TextStyle(color: textColor, fontSize: 15),
                      )
                    : const SizedBox.shrink(),
              ),
              if (chatMessage.isEdited) ...[
                const SizedBox(width: 6),
                Padding(
                  padding: const EdgeInsets.only(bottom: 1),
                  child: Text(
                    context.l10n.chatEdited,
                    style: TextStyle(
                      color: textColor.withValues(alpha: 0.5),
                      fontSize: 11,
                      fontStyle: FontStyle.italic,
                    ),
                  ),
                ),
              ],
              if (isCurrentUser) ...[
                const SizedBox(width: 6),
                Padding(
                  padding: const EdgeInsets.only(bottom: 1),
                  child: _buildDeliveryReceiptIcon(context, chatMessage.state),
                ),
              ],
            ],
          ),
        ),
      ],
    );
  }

  Widget _buildDeliveryReceiptIcon(BuildContext context, MessageState state) {
    IconData icon;
    Color color;

    switch (state) {
      case MessageState.pending:
        icon = Icons.access_time;
        color = immersive
            ? GlassTokens.textFaint
            : isCurrentUser
            ? Colors.black.withValues(alpha: 0.35)
            : AppColors.textSecondary(context);
        break;
      case MessageState.delivered:
        icon = Icons.check;
        color = immersive
            ? GlassTokens.textFaint
            : isCurrentUser
            ? Colors.black.withValues(alpha: 0.35)
            : AppColors.textSecondary(context);
        break;
      case MessageState.failed:
        icon = Icons.error_outline;
        color = Colors.red;
        break;
    }

    return Icon(icon, size: 14, color: color);
  }

  String _getFirstName(String fullName) {
    final parts = fullName.split(' ');
    return parts.isNotEmpty ? parts.first : fullName;
  }

  /// _resolveBadge maps RSVP or transfer status to an AvatarStatusBadge.
  ///
  /// RSVP status takes precedence over transfer status. Returns null when
  /// neither status carries visible meaning.
  AvatarStatusBadge? _resolveBadge(
    RSVPIntention? rsvp,
    TransferState? transfer, {
    bool isOfferer = false,
  }) {
    if (isOfferer) return AvatarStatusBadge.helping;
    if (rsvp != null) {
      switch (rsvp) {
        case RSVPIntention.RSVP_INTENTION_YES:
          return AvatarStatusBadge.going;
        case RSVPIntention.RSVP_INTENTION_MAYBE:
          return AvatarStatusBadge.maybe;
        default:
          break;
      }
    }
    if (transfer != null) {
      switch (transfer) {
        case TransferState.TRANSFER_STATE_INTEREST_EXPRESSED:
          return AvatarStatusBadge.interested;
        case TransferState.TRANSFER_STATE_RECIPIENT_SELECTED:
          return AvatarStatusBadge.selected;
        default:
          break;
      }
    }
    return null;
  }

  // ── Long-press menu (reactions + actions) ─────────────────────────────────

  /// Whether this bubble is a delivered, non-system user message that can host
  /// the long-press menu. Pending/failed/temporary and system messages cannot.
  bool get _isActionableUserMessage =>
      !chatMessage.isSystemMessage &&
      chatMessage.state == MessageState.delivered &&
      !chatMessage.isTemporary;

  /// Number of context actions (Reply / Edit / Delete) the menu will show.
  int get _menuActionCount {
    var count = 0;
    if (onReplyMessage != null) count++;
    if (isCurrentUser && onEditMessage != null) count++;
    if (isCurrentUser && onDeleteMessage != null) count++;
    return count;
  }

  /// Whether long-press should open the menu at all.
  bool get _hasLongPressMenu =>
      _isActionableUserMessage &&
      (onReactionSelected != null || _menuActionCount > 0);

  /// Shows the long-press menu overlay anchored above the long-pressed bubble.
  void _showReactionPicker(BuildContext context, ChatMessage chatMessage) {
    // A short buzz confirms the long-press registered, matching other
    // messaging apps.
    HapticFeedback.mediumImpact();

    // Capture the bubble's screen position before entering the overlay builder.
    final renderBox = context.findRenderObject() as RenderBox?;
    if (renderBox == null) return;
    final bubbleOffset = renderBox.localToGlobal(Offset.zero);

    // Estimate the menu height so the whole menu sits above the bubble: emoji
    // bar (~52) plus the action card (gap + ~46 per row) when present.
    final actionCount = _menuActionCount;
    final menuHeight = 52.0 + (actionCount > 0 ? 8 + actionCount * 46.0 : 0);

    final overlay = Overlay.of(context);
    late OverlayEntry entry;

    entry = OverlayEntry(
      builder: (ctx) {
        final pickerBottom = bubbleOffset.dy - 8;
        final pickerTop = (pickerBottom - menuHeight).clamp(
          8.0,
          double.infinity,
        );

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
                  child: immersive
                      ? Theme(
                          data: Theme.of(
                            context,
                          ).copyWith(brightness: Brightness.dark),
                          child: _buildPickerContent(
                            context,
                            entry,
                            chatMessage,
                          ),
                        )
                      : _buildPickerContent(context, entry, chatMessage),
                ),
              ),
            ],
          ),
        );
      },
    );
    overlay.insert(entry);
  }

  Widget _buildPickerContent(
    BuildContext context,
    OverlayEntry entry,
    ChatMessage chatMessage,
  ) {
    return ReactionPicker(
      onEmojiSelected: (emoji) {
        entry.remove();
        onReactionSelected?.call(chatMessage.messageId, emoji);
      },
      onExpandPicker: () {
        entry.remove();
        _showFullEmojiPicker(context, chatMessage);
      },
      actions: _buildMenuActions(context, entry, chatMessage),
    );
  }

  /// Builds the Reply / Edit / Delete rows for the long-press menu. Reply is
  /// offered for any message; Edit and Delete only on the current user's own.
  List<MessageAction> _buildMenuActions(
    BuildContext context,
    OverlayEntry entry,
    ChatMessage chatMessage,
  ) {
    final actions = <MessageAction>[];

    if (onReplyMessage != null) {
      actions.add(
        MessageAction(
          icon: Icons.reply,
          label: context.l10n.chatActionReply,
          semanticsLabel: context.l10n.a11yChatActionReply,
          onTap: () {
            entry.remove();
            onReplyMessage!(chatMessage.messageId);
          },
        ),
      );
    }

    if (isCurrentUser && onEditMessage != null) {
      actions.add(
        MessageAction(
          icon: Icons.edit_outlined,
          label: context.l10n.chatActionEdit,
          semanticsLabel: context.l10n.a11yChatActionEdit,
          onTap: () {
            entry.remove();
            onEditMessage!(chatMessage.messageId);
          },
        ),
      );
    }

    if (isCurrentUser && onDeleteMessage != null) {
      actions.add(
        MessageAction(
          icon: Icons.delete_outline,
          label: context.l10n.chatActionDelete,
          semanticsLabel: context.l10n.a11yChatActionDelete,
          isDestructive: true,
          onTap: () {
            entry.remove();
            onDeleteMessage!(chatMessage.messageId);
          },
        ),
      );
    }

    return actions;
  }

  /// Shows a bottom sheet with the full emoji picker grid.
  void _showFullEmojiPicker(BuildContext context, ChatMessage chatMessage) {
    showAccessibleModal<void>(
      context,
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
  void _showReactionDetailSheet(BuildContext context, ChatMessage chatMessage) {
    showAccessibleModal<void>(
      context,
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
