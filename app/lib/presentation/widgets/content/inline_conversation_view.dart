import 'package:cross_file/cross_file.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:keyboard_actions/keyboard_actions.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/models/media_item_data.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/core/utils/media_picker_helper.dart';
import 'package:ripls/core/utils/navigation_helpers.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/presentation/screens/experience/experience_screen.dart';
import 'package:ripls/presentation/screens/experience/time_poll_vote_modal.dart';
import 'package:ripls/presentation/screens/gear/gear_screen.dart';
import 'package:ripls/presentation/screens/request/request_screen.dart';
import 'package:ripls/presentation/screens/users/user_screen.dart';
import 'package:ripls/presentation/viewmodels/conversation_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/chat/mention/mention_text_controller.dart';
import 'package:ripls/presentation/widgets/chat/mention/mention_types.dart';
import 'package:ripls/presentation/widgets/chat/mention/mentionable_text_field.dart';
import 'package:ripls/presentation/widgets/chat/message_list.dart';
import 'package:ripls/presentation/widgets/content/inline_conversation_attachments.dart';
import 'package:ripls/presentation/widgets/content/inline_conversation_compose_banner.dart';
import 'package:ripls/presentation/widgets/keyboard_actions_config.dart';
import 'package:ripls/presentation/widgets/media/media_carousel.dart'
    show MediaCarousel, MediaItem;
import 'package:ripls/presentation/widgets/media/media_picker_dialog.dart';
import 'package:ripls/presentation/widgets/transfer/confirmation_dialog.dart';
import 'package:ripls/presentation/widgets/web/web_unsupported.dart';
import 'package:ripls/services/providers.dart';

/// InlineConversationView renders the full conversation experience inside the
/// Chat tab of a content view.
///
/// In collapsed mode (isExpanded = false) it shows the last three messages
/// and a simple text input. Tapping anywhere expands it.
///
/// In expanded mode (isExpanded = true) it renders the full [MessageList] with
/// system pills, workflow widget cards, and the complete [MessageInputBar]
/// (progress bar, participant avatars, action button).
///
/// Initialization is lazy: the conversation is fetched the first time
/// [isActive] becomes true and is not repeated on subsequent activations.
class InlineConversationView extends ConsumerStatefulWidget {
  final String conversationId;
  final Color accentColor;
  final bool isActive;
  final bool isExpanded;
  final VoidCallback? onExpand;
  final void Function(String conversationId)? onMessagesMarkedAsRead;

  /// Media items to display in a horizontal strip below the input bar.
  /// When non-empty, a thumbnail row is shown and the input bar's bottom
  /// padding is reduced so the strip sits flush against it.
  final List<MediaItemData> mediaItems;

  final Future<void> Function()? onAddMedia;
  // onDeleteMedia: handles deletion. Visibility per item is determined by
  // isOwner and the current user's uploader match inside MediaCarousel.
  final Future<void> Function(String mediaId)? onDeleteMedia;
  // isOwner: true when the current user owns the parent item (gear/experience/
  // request). Owners can delete any media; non-owners can only delete items
  // they uploaded themselves.
  final bool isOwner;
  // onReorderMedia: only the owner can reorder — pass null to disable.
  final Future<void> Function(List<String> mediaIds)? onReorderMedia;

  /// Maps experience need IDs → display names for reference chips in bubbles.
  final Map<String, String> experienceNeedNames;

  /// Maps experience contribution IDs → display names for reference chips in bubbles.
  final Map<String, String> experienceContributionNames;

  /// Called when the user taps a need reference chip inside a message bubble.
  final void Function(String needId)? onExperienceNeedTap;

  /// Called when the user taps a contribution reference chip inside a message bubble.
  final void Function(String contributionId)? onExperienceContributionTap;

  /// Overrides the unread count used when initializing the conversation.
  ///
  /// Set this when the caller has a more accurate unread count than what
  /// UnreadCountRepository returns — e.g. community-wide conversations are
  /// excluded from GetUnreadCounts but their count is available on the
  /// ConversationItem fetched via GetConversationForCommunity.
  final int? initialUnreadCountOverride;

  const InlineConversationView({
    super.key,
    required this.conversationId,
    required this.accentColor,
    required this.isActive,
    this.isExpanded = false,
    this.onExpand,
    this.onMessagesMarkedAsRead,
    this.mediaItems = const [],
    this.onAddMedia,
    this.onDeleteMedia,
    this.isOwner = false,
    this.onReorderMedia,
    this.experienceNeedNames = const {},
    this.experienceContributionNames = const {},
    this.onExperienceNeedTap,
    this.onExperienceContributionTap,
    this.initialUnreadCountOverride,
  });

  @override
  ConsumerState<InlineConversationView> createState() =>
      _InlineConversationViewState();
}

class _InlineConversationViewState
    extends ConsumerState<InlineConversationView>
    with AutomaticKeepAliveClientMixin, WidgetsBindingObserver {
  bool _initialized = false;
  final MentionTextController _inputController = MentionTextController();
  final ScrollController _scrollController = ScrollController();
  final FocusNode _inputFocusNode = FocusNode();
  int _previousMessageCount = 0;
  // Cached so KeyboardActions receives a stable config across rebuilds.
  // A new config object on each build causes the keyboard overlay to tear
  // down and recreate, briefly dismissing the keyboard.
  late final KeyboardActionsConfig _keyboardConfig;

  @override
  bool get wantKeepAlive => true;

  @override
  void initState() {
    super.initState();
    _keyboardConfig = buildKeyboardActionsConfig([_inputFocusNode]);
    _inputFocusNode.addListener(_onFocusChange);
    WidgetsBinding.instance.addObserver(this);
    if (widget.isActive) {
      WidgetsBinding.instance.addPostFrameCallback((_) => _initialize());
    }
  }

  @override
  void didUpdateWidget(InlineConversationView old) {
    super.didUpdateWidget(old);
    if (widget.conversationId != old.conversationId) {
      _initialized = false;
    }
    if (widget.isActive && !_initialized) {
      _initialize();
    }
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    _inputFocusNode.removeListener(_onFocusChange);
    _inputFocusNode.dispose();
    _inputController.dispose();
    _scrollController.dispose();
    super.dispose();
  }

  /// Stable method tear-off forwarded to [MediaCarousel.show] as
  /// `getMediaItems`. The carousel calls this on each `_refreshMediaItems`,
  /// so it always reads the latest `widget.mediaItems` (which Flutter
  /// updates on every parent rebuild). A lambda capturing `widget.mediaItems`
  /// would freeze the list at strip-build time and starve the carousel of
  /// updates from in-progress uploads.
  List<MediaItem> _getCarouselMediaItems() {
    return widget.mediaItems
        .map(
          (m) => MediaItem(
            url: m.url,
            isVideo: m.isVideo,
            contentType: m.contentType,
            mediaId: m.id,
            attribution: m.attribution,
            uploader: m.uploader,
            uploadedAtUnixSec: m.uploadedAtUnixSec,
          ),
        )
        .toList();
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state == AppLifecycleState.resumed &&
        mounted &&
        widget.isActive &&
        _initialized) {
      final notifier = ref.read(
        conversationProvider(widget.conversationId).notifier,
      );
      // Fire-and-forget: the notifier handles its own errors and ref.mounted checks.
      notifier.refreshAfterResume();
    }
  }

  void _onFocusChange() {
    if (_inputFocusNode.hasFocus && !widget.isExpanded) {
      widget.onExpand?.call();
    }
  }

  Future<void> _initialize() async {
    if (_initialized || !mounted) return;
    _initialized = true;

    final authState = ref.read(authStateProvider);
    final currentUserId = authState.user?.id ?? '';
    final currentUserName = authState.user?.name ?? 'You';

    try {
      final chatRepository = ref.read(chatRepositoryProvider);
      // Invalidate cached conversation and context so we always fetch fresh
      // data. Without this, opening a chat from a notification can show stale
      // messages because the cache still holds the pre-notification state.
      await Future.wait([
        chatRepository.refreshConversation(widget.conversationId),
        chatRepository.refreshConversationContext(widget.conversationId),
      ]);
      final conversation = await chatRepository.getConversation(
        conversationId: widget.conversationId,
      );
      final context = await chatRepository.getConversationContext(
        conversationId: widget.conversationId,
      );

      if (!mounted) return;

      final unreadRepository = ref.read(unreadCountRepositoryProvider);
      final communityId = conversation.communityId;
      final unreadCount = widget.initialUnreadCountOverride ??
          await unreadRepository.getUnreadCount(
            communityId,
            widget.conversationId,
          );

      if (!mounted) return;

      await ref.read(conversationProvider(widget.conversationId).notifier).initialize(
            currentUserId: currentUserId,
            currentUserName: currentUserName,
            conversation: conversation,
            unreadCount: unreadCount,
            conversationContext: context,
            onMessagesMarkedAsRead: widget.onMessagesMarkedAsRead,
          );

    } catch (e) {
      if (mounted) {
        ToastHelper.showError(context, 'Failed to load chat: $e');
      }
    }
  }

  // ── Auto-scroll ───────────────────────────────────────────────────────────

  // With reverse: true, scroll position 0.0 is the visual bottom (newest
  // messages). New messages are inserted at position 0 by the ListView so
  // they appear at the bottom without any explicit scrolling when the user
  // is already at position 0. We only need to scroll on first load or when
  // the user has scrolled up and a new message arrives.
  void _maybeScrollToBottom(int messageCount) {
    if (messageCount != _previousMessageCount) {
      final jumpInstantly = _previousMessageCount == 0;
      _previousMessageCount = messageCount;
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (!mounted || !_scrollController.hasClients) return;
        final pos = _scrollController.position;
        // Already at the bottom (position 0 in a reversed list) — nothing to do.
        if (pos.pixels <= 1.0 && !jumpInstantly) return;
        if (jumpInstantly) {
          _scrollController.jumpTo(0);
        } else {
          _scrollController.animateTo(
            0,
            duration: accessibleDuration(context, const Duration(milliseconds: 300)),
            curve: Curves.easeOut,
          );
        }
      });
    }
  }

  void _scrollToBottom() {
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted && _scrollController.hasClients) {
        _scrollController.animateTo(
          0,
          duration: accessibleDuration(context, const Duration(milliseconds: 200)),
          curve: Curves.easeOut,
        );
      }
    });
  }

  // ── Build ─────────────────────────────────────────────────────────────────

  @override
  Widget build(BuildContext context) {
    super.build(context);
    final convState = ref.watch(conversationProvider(widget.conversationId));
    _maybeScrollToBottom(convState.messages.length);

    if (widget.isExpanded) {
      return _buildExpanded(convState);
    }
    return _buildCollapsed(convState);
  }

  // ── Collapsed mode ────────────────────────────────────────────────────────

  Widget _buildCollapsed(ConversationState convState) {
    return Tappable(
      semanticsLabel: context.l10n.a11yOpenConversation,
      excludeChildSemantics: false,
      onTap: widget.onExpand != null ? () => widget.onExpand!() : null,
      child: Column(
        children: [
          Expanded(child: _buildMessageArea(convState, lastN: 3)),
          _buildSimpleInput(convState),
          if (widget.mediaItems.isNotEmpty) buildMediaStrip(
            context,
            mediaItems: widget.mediaItems,
            onAddMedia: widget.onAddMedia,
            onDeleteMedia: widget.onDeleteMedia,
            onReorderMedia: widget.onReorderMedia,
            getCarouselMediaItems: _getCarouselMediaItems,
          ),
        ],
      ),
    );
  }

  Widget _buildSimpleInput(ConversationState convState) {
    final hasMedia = widget.mediaItems.isNotEmpty;
    return Tappable(
      // Absorb taps so they don't bubble up to the collapsed-mode expand
      // gesture; the inner TextField handles its own focus.
      semanticsLabel: context.l10n.a11yContentMessageInput,
      excludeChildSemantics: false,
      onTap: () {},
      child: Padding(
        padding: EdgeInsets.fromLTRB(16, 4, 16, hasMedia ? 4 : 20),
        child: Row(
          children: [
            Expanded(
              child: Container(
                decoration: BoxDecoration(
                  color: GlassTokens.scrimTint,
                  borderRadius: BorderRadius.circular(20),
                  border: Border.all(
                    color: GlassTokens.borderSoft,
                  ),
                ),
                child: TextField(
                  controller: _inputController,
                  focusNode: _inputFocusNode,
                  onTap: widget.onExpand != null ? () => widget.onExpand!() : null,
                  style: const TextStyle(color: GlassTokens.textPrimary, fontSize: 13),
                  decoration: InputDecoration(
                    hintText: context.l10n.contentTypeTabAddComment,
                    hintStyle: TextStyle(
                      color: GlassTokens.textFaint,
                      fontSize: 13,
                    ),
                    contentPadding: const EdgeInsets.symmetric(
                      horizontal: 14,
                      vertical: 10,
                    ),
                    border: InputBorder.none,
                  ),
                  onSubmitted: (_) => _handleSendMessage(),
                ),
              ),
            ),
            const SizedBox(width: 8),
            ValueListenableBuilder<TextEditingValue>(
              valueListenable: _inputController,
              builder: (_, value, _) {
                final hasText = value.text.trim().isNotEmpty;
                return Tappable(
                  semanticsLabel: context.l10n.a11yContentSendMessage,
                  onTap: hasText ? _handleSendMessage : null,
                  child: Container(
                    width: 38,
                    height: 38,
                    decoration: BoxDecoration(
                      shape: BoxShape.circle,
                      color: hasText
                          ? widget.accentColor
                          : GlassTokens.fillSubtle,
                    ),
                    child: const Center(
                      child: Text(
                        '↑',
                        style: TextStyle(color: GlassTokens.textPrimary, fontSize: 16),
                      ),
                    ),
                  ),
                );
              },
            ),
          ],
        ),
      ),
    );
  }

  // ── Expanded mode ─────────────────────────────────────────────────────────

  Widget _buildExpanded(ConversationState convState) {
    return KeyboardActions(
      disableScroll: true,
      // Do NOT use tapOutsideBehavior here — it treats the send button,
      // plus button, and text field padding as "outside" and dismisses
      // the keyboard on every tap. Instead, dismiss is handled by a
      // GestureDetector on the message list area only.
      tapOutsideBehavior: TapOutsideBehavior.none,
      config: _keyboardConfig,
      child: Column(
        children: [
          Expanded(
            child: Tappable(
              semanticsLabel: context.l10n.a11yContentDismissKeyboard,
              excludeChildSemantics: false,
              onTap: () => FocusScope.of(context).unfocus(),
              child: _buildMessageArea(convState),
            ),
          ),
          if (convState.pendingAttachments.isNotEmpty)
            buildPendingAttachments(context, ref, convState, widget.conversationId),
          if (convState.isSending && convState.uploadTotal > 0)
            buildUploadProgress(convState, widget.accentColor),
          _buildComposeBanner(convState),
          _buildPillInput(convState),
          if (widget.mediaItems.isNotEmpty) buildMediaStrip(
            context,
            mediaItems: widget.mediaItems,
            onAddMedia: widget.onAddMedia,
            onDeleteMedia: widget.onDeleteMedia,
            onReorderMedia: widget.onReorderMedia,
            getCarouselMediaItems: _getCarouselMediaItems,
          ),
        ],
      ),
    );
  }

  /// Pill-style input bar: attach button + text field + circular send button.
  ///
  /// Matches the reference chat design — floats inside the dark content pane
  /// without the workflow action bar (workflow controls are on the Type tab).
  Widget _buildPillInput(ConversationState convState) {
    final hasMedia = widget.mediaItems.isNotEmpty;
    return Padding(
      padding: EdgeInsets.fromLTRB(12, 8, 12, hasMedia ? 4 : 24),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.end,
        children: [
          // Attach button
          Tappable(
            semanticsLabel: context.l10n.a11yContentAttachMedia,
            onTap: convState.isSending ? null : _handlePickImage,
            child: Padding(
              padding: const EdgeInsets.only(bottom: 10, right: 6),
              child: Icon(
                Icons.add,
                size: 22,
                color: GlassTokens.textFaint,
              ),
            ),
          ),
          // Pill text field + optional send button
          Expanded(
            child: Container(
              decoration: BoxDecoration(
                color: GlassTokens.fillSubtle,
                borderRadius: BorderRadius.circular(22),
              ),
              padding: const EdgeInsets.fromLTRB(14, 0, 4, 0),
              child: Row(
                crossAxisAlignment: CrossAxisAlignment.end,
                children: [
                  Expanded(
                    child: MentionableTextField(
                      controller: _inputController,
                      focusNode: _inputFocusNode,
                      immersive: true,
                      onFetchSuggestions: (query) => ref
                          .read(conversationProvider(widget.conversationId)
                              .notifier)
                          .fetchMentionSuggestions(query),
                      mediaUrlLoader: (mediaId) async {
                        final mediaRepository =
                            ref.read(mediaRepositoryProvider);
                        final mediaUrl =
                            await mediaRepository.getMediaUrl(mediaId);
                        return mediaUrl.url;
                      },
                      style: const TextStyle(color: GlassTokens.textPrimary, fontSize: 14),
                      decoration: InputDecoration(
                        hintText: context.l10n.conversationComposerHint,
                        hintStyle: TextStyle(
                          color: GlassTokens.textFaint,
                          fontSize: 14,
                        ),
                        border: InputBorder.none,
                        enabledBorder: InputBorder.none,
                        focusedBorder: InputBorder.none,
                        filled: true,
                        fillColor: Colors.transparent,
                        contentPadding: const EdgeInsets.symmetric(vertical: 10),
                        isDense: true,
                      ),
                      maxLines: null,
                      textCapitalization: TextCapitalization.sentences,
                      // Enter inserts a newline; the send button handles
                      // submission. This avoids the platform-level keyboard
                      // dismiss that TextInputAction.send triggers on iOS.
                      textInputAction: TextInputAction.newline,
                    ),
                  ),
                  // Send button — always occupies the same space to prevent
                  // layout shifts when text is cleared after sending. A layout
                  // shift can cause KeyboardActions to briefly dismiss the
                  // keyboard. The button fades between active/inactive states.
                  // Visible when text is entered OR attachments are staged.
                  ValueListenableBuilder<TextEditingValue>(
                    valueListenable: _inputController,
                    builder: (_, value, _) {
                      final hasText = value.text.trim().isNotEmpty;
                      final hasAttachments =
                          convState.pendingAttachments.isNotEmpty;
                      final canSend = hasText || hasAttachments;
                      return Padding(
                        padding: const EdgeInsets.only(bottom: 4),
                        child: Tappable(
                          semanticsLabel: context.l10n.a11yContentSendMessage,
                          onTap: (canSend && !convState.isSending)
                              ? _handleSendMessage
                              : null,
                          child: AnimatedOpacity(
                            opacity: canSend ? 1.0 : 0.0,
                            duration: accessibleDuration(context, const Duration(milliseconds: 150)),
                            child: Container(
                              width: 32,
                              height: 32,
                              decoration: BoxDecoration(
                                shape: BoxShape.circle,
                                color: widget.accentColor,
                              ),
                              child: Icon(
                                convState.editingMessageId != null
                                    ? Icons.check
                                    : Icons.arrow_upward,
                                size: 16,
                                color: GlassTokens.textPrimary,
                              ),
                            ),
                          ),
                        ),
                      );
                    },
                  ),
                  const SizedBox(width: 4),
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }

  // ── Message area (shared) ─────────────────────────────────────────────────

  Widget _buildMessageArea(
    ConversationState convState, {
    int? lastN,
  }) {
    // While history is still loading and nothing has arrived yet, show a
    // quiet spinner — never the "Be the first to message" empty state, which
    // would flash over conversations that do have history (#2724).
    if (convState.isLoadingMessages && convState.messages.isEmpty) {
      return const Center(
        child: SizedBox(
          width: 20,
          height: 20,
          child: CircularProgressIndicator(
            strokeWidth: 2,
            color: GlassTokens.textFaint,
          ),
        ),
      );
    }

    final allMessages = convState.messages;
    final messages = lastN != null && allMessages.length > lastN
        ? allMessages.sublist(allMessages.length - lastN)
        : allMessages;

    Widget content;
    if (messages.isEmpty) {
      // Loaded-and-empty: the invitation copy is now truthful.
      content = Center(
        child: Text(
          context.l10n.contentTypeTabBeFirstToMessage,
          style: const TextStyle(
            color: GlassTokens.textPrimary,
            fontSize: 13,
          ),
        ),
      );
    } else {
      final isGroup = convState.participants.length > 2;
      final experienceId =
          convState.conversation?.topic.experienceId ?? '';
      content = MessageList(
      messages: messages,
      currentUserId: convState.currentUserId,
      isGroupConversation: isGroup,
      scrollController: _scrollController,
      reverse: true,
      experienceId: experienceId.isEmpty ? null : experienceId,
      onTimePollTap: experienceId.isEmpty
          ? null
          : (pollId) => TimePollVoteModal.show(
                context,
                experienceId,
                pollId: pollId,
              ),
      onRefresh: () async {
        try {
          await ref.read(conversationProvider(widget.conversationId).notifier).loadMessages();
        } catch (e) {
          if (mounted) {
            ToastHelper.showError(context, 'Failed to refresh: $e');
          }
        }
      },
      onUserAvatarTap: (_) {},
      onRetryMessage: (messageId) async {
        try {
          await ref.read(conversationProvider(widget.conversationId).notifier).retryMessage(messageId);
        } catch (e) {
          if (mounted) {
            ToastHelper.showError(context, 'Failed to retry: $e');
          }
        }
      },
      rsvpStatusMap: convState.rsvpStatusMap,
      transferStatusMap: convState.transferStatusMap,
      requestOffererIds: convState.cachedRequest?.offerers
              .map((u) => u.id)
              .toSet() ??
          const {},
      onMentionTap: _handleMentionTap,
      onReactionSelected: (messageId, emoji) => ref
          .read(conversationProvider(widget.conversationId).notifier)
          .addReaction(messageId, emoji),
      onReactionRemoved: (messageId) => ref
          .read(conversationProvider(widget.conversationId).notifier)
          .removeReaction(messageId),
      onDeleteMessage: _confirmAndDeleteMessage,
      onEditMessage: _handleEditMessage,
      onReplyMessage: _handleReplyMessage,
      experienceNeedNames: widget.experienceNeedNames,
      experienceContributionNames: widget.experienceContributionNames,
      onExperienceNeedTap: widget.onExperienceNeedTap,
      onExperienceContributionTap: widget.onExperienceContributionTap,
      immersive: true,
      bottomPadding: 8,
      onMediaTap: widget.mediaItems.isEmpty
          ? null
          : (mediaId) {
              final index =
                  widget.mediaItems.indexWhere((m) => m.id == mediaId);
              MediaCarousel.show(
                context: context,
                initialIndex: index >= 0 ? index : 0,
                getMediaItems: () => widget.mediaItems
                    .map(
                      (m) => MediaItem(
                        url: m.url,
                        isVideo: m.isVideo,
                        contentType: m.contentType,
                        mediaId: m.id,
                        attribution: m.attribution,
                        uploader: m.uploader,
                        uploadedAtUnixSec: m.uploadedAtUnixSec,
                      ),
                    )
                    .toList(),
                onAddMedia: widget.onAddMedia,
                onDeleteMedia: widget.onDeleteMedia,
                isOwner: widget.isOwner,
                onReorderMedia: widget.onReorderMedia,
              );
            },
      );
    }

    return Column(
      children: [
        if (convState.isStreamDisconnected) _buildDisconnectedBanner(),
        Expanded(child: content),
      ],
    );
  }

  Widget _buildDisconnectedBanner() {
    return Container(
      color: AppColors.statusWarningOnDark.withValues(alpha: 0.85),
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
      child: Row(
        children: [
          Icon(
            Icons.wifi_off,
            size: 14,
            color: AppColors.darkTextPrimary,
          ),
          const SizedBox(width: 6),
          Expanded(
            child: Text(
              context.l10n.chatStreamDisconnected,
              style: TextStyle(
                color: AppColors.darkTextPrimary,
                fontSize: 12,
              ),
            ),
          ),
        ],
      ),
    );
  }

  // ── Mention tap ──────────────────────────────────────────────────────────

  void _handleMentionTap(MentionType type, String id) {
    switch (type) {
      case MentionType.user:
        NavigationHelpers.pushScreen(
          context: context,
          screen: UserScreen(userId: id, onReturn: () {}),
          routeName: 'profile',
        );
        break;
      case MentionType.loan:
      case MentionType.giveaway:
        NavigationHelpers.pushScreen(
          context: context,
          screen: GearScreen(gearId: id),
          routeName: 'gear_detail',
        );
        break;
      case MentionType.request:
        NavigationHelpers.pushScreen(
          context: context,
          screen: RequestScreen(requestId: id),
          routeName: 'request_detail',
        );
        break;
      case MentionType.experience:
        NavigationHelpers.pushScreen(
          context: context,
          screen: ExperienceScreen(experienceId: id),
          routeName: 'experience_detail',
        );
        break;
    }
  }

  // ── Message send ──────────────────────────────────────────────────────────

  /// Confirms via dialog, then deletes the message through the notifier.
  ///
  /// Widget async exception: the confirmation dialog result is awaited in the
  /// widget; guarded with a mounted check before using context again.
  Future<void> _confirmAndDeleteMessage(String messageId) async {
    final confirmed = await showConfirmationDialog(
      context: context,
      title: context.l10n.chatDeleteConfirmTitle,
      message: context.l10n.chatDeleteConfirmBody,
      cancelLabel: context.l10n.commonCancel,
      confirmLabel: context.l10n.commonDelete,
      isDestructive: true,
    );
    if (confirmed != true) return;
    if (!mounted) return;
    try {
      await ref
          .read(conversationProvider(widget.conversationId).notifier)
          .deleteMessage(messageId);
    } catch (e) {
      if (mounted) {
        ToastHelper.showError(context, 'Failed to delete message: $e');
      }
    }
  }

  Future<void> _handleSendMessage() async {
    final notifier =
        ref.read(conversationProvider(widget.conversationId).notifier);
    final convState = ref.read(conversationProvider(widget.conversationId));
    final message = _inputController.encodedText.trim();

    // Edit mode: save the edit instead of sending a new message.
    final editingId = convState.editingMessageId;
    if (editingId != null) {
      if (message.isEmpty) return;
      _inputController.clear();
      _inputFocusNode.requestFocus();
      try {
        await notifier.editMessage(editingId, message);
      } catch (e) {
        if (mounted) {
          ToastHelper.showError(context, 'Failed to edit message: $e');
        }
      }
      return;
    }

    final hasPending = convState.pendingAttachments.isNotEmpty;
    if (message.isEmpty && !hasPending) return;
    final replyToId = convState.replyingToMessageId;
    _inputController.clear();
    // Re-request focus so the keyboard stays open after sending.
    _inputFocusNode.requestFocus();
    try {
      if (hasPending) {
        await notifier.sendMessageWithPendingAttachments(
          text: message.isNotEmpty ? message : null,
        );
      } else {
        await notifier.sendMessage(message, replyToMessageId: replyToId);
      }
      if (replyToId != null) notifier.cancelReplying();
      if (mounted) _scrollToBottom();
    } catch (e) {
      if (mounted) {
        ToastHelper.showError(context, 'Failed to send message: $e');
      }
    }
  }

  /// Enters edit mode for [messageId]: prefills the input with the current
  /// text, expands the view, and focuses the field.
  void _handleEditMessage(String messageId) {
    final convState = ref.read(conversationProvider(widget.conversationId));
    final idx = convState.messages.indexWhere((m) => m.messageId == messageId);
    if (idx == -1) return;
    final msg = convState.messages[idx];

    widget.onExpand?.call();
    _inputController.text = msg.text;
    _inputController.selection = TextSelection.collapsed(
      offset: _inputController.text.length,
    );
    ref
        .read(conversationProvider(widget.conversationId).notifier)
        .beginEditing(messageId);
    _inputFocusNode.requestFocus();
  }

  /// Enters reply mode quoting [messageId]: snapshots the sender name and text
  /// for the banner, expands the view, and focuses the field.
  void _handleReplyMessage(String messageId) {
    final convState = ref.read(conversationProvider(widget.conversationId));
    final idx = convState.messages.indexWhere((m) => m.messageId == messageId);
    if (idx == -1) return;
    final msg = convState.messages[idx];

    widget.onExpand?.call();
    ref.read(conversationProvider(widget.conversationId).notifier).beginReplying(
          messageId,
          msg.senderName,
          msg.text,
        );
    _inputFocusNode.requestFocus();
  }

  /// Builds the edit/reply banner shown above the input bar, or an empty box
  /// when neither mode is active.
  Widget _buildComposeBanner(ConversationState convState) {
    if (convState.editingMessageId != null) {
      return _composeBanner(
        title: context.l10n.chatEditBannerTitle,
        cancelSemanticsLabel: context.l10n.a11yChatCancelEdit,
        onCancel: () {
          _inputController.clear();
          ref
              .read(conversationProvider(widget.conversationId).notifier)
              .cancelEditing();
        },
      );
    }
    if (convState.replyingToMessageId != null) {
      return _composeBanner(
        title: context.l10n.chatReplyBannerTitle(
          convState.replyingToSenderName ?? '',
        ),
        subtitle: convState.replyingToText,
        cancelSemanticsLabel: context.l10n.a11yChatCancelReply,
        onCancel: () => ref
            .read(conversationProvider(widget.conversationId).notifier)
            .cancelReplying(),
      );
    }
    return const SizedBox.shrink();
  }

  Widget _composeBanner({
    required String title,
    String? subtitle,
    required String cancelSemanticsLabel,
    required VoidCallback onCancel,
  }) {
    return ComposeBanner(
      title: title,
      subtitle: subtitle,
      cancelSemanticsLabel: cancelSemanticsLabel,
      onCancel: onCancel,
      accentColor: widget.accentColor,
    );
  }

  void _handlePickImage() {
    if (!mounted) return;
    MediaPickerDialog.show(
      context: context,
      hasMedia: false,
      onVideoTap: () => _pickAndStageMedia(isVideo: true),
      onPhotoTap: () => _pickAndStageMedia(),
      onCameraTap: () => _pickAndStageMedia(fromCamera: true),
    );
  }

  /// Picks media and stages it as a pending attachment.
  ///
  /// Widget async exception: Platform image/video picker requires UI context.
  ///
  /// Chat attachments on web are gated separately from item creation:
  /// the in-chat compose flow uses `Image.file(File(path))` to render
  /// staged previews (`inline_conversation_attachments.dart`), and
  /// `dart:io File` doesn't exist on web. Refactoring the staged-
  /// attachment renderer + `pendingAttachments` state field to flow
  /// `XFile` through is its own piece of work; the item-creation
  /// path is fully web-enabled in #2157, chat-attachment-on-web is
  /// tracked as a follow-up. Until then, show the photo-upload
  /// notice and return.
  Future<void> _pickAndStageMedia({
    bool isVideo = false,
    bool fromCamera = false,
  }) async {
    if (WebUnsupported.showPhotoUploadNotice(context)) return;
    try {
      final notifier =
          ref.read(conversationProvider(widget.conversationId).notifier);

      if (!isVideo && !fromCamera) {
        // Multi-select gallery for photos.
        final files =
            await MediaPickerHelper.pickMultipleImagesFromGallery();
        if (files.isEmpty || !mounted) return;
        notifier.addPendingAttachments(
          files.map((f) => f.path).toList(),
        );
      } else {
        // Single-pick for camera and video.
        final XFile? picked;
        if (fromCamera) {
          picked = await MediaPickerHelper.pickImageFromCamera();
        } else {
          picked = await MediaPickerHelper.pickVideoFromGallery();
        }
        if (picked == null || !mounted) return;
        notifier.addPendingAttachments([picked.path]);
      }
    } catch (e) {
      if (mounted) {
        ToastHelper.showError(context, 'Failed to pick media: $e');
      }
    }
  }
}
