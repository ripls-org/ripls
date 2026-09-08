import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/data/gen/ripls/api/chat_service.pb.dart'
    show ConversationItem;
import 'package:ripls/data/gen/ripls/api/conversation.pb.dart';
import 'package:ripls/data/gen/ripls/api/experience_service.pb.dart'
    show GetExperienceResponse, RSVPIntention;
import 'package:ripls/data/gen/ripls/api/request.pb.dart' show Request;
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart'
    show Transfer, TransferState;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/models/chat_message.dart';
import 'package:video_player/video_player.dart';

part 'conversation_state.freezed.dart';

/// ConversationState holds the full UI state for a conversation screen.
///
/// Owned by [ConversationNotifier]. All action mixins read and write this
/// state via [Notifier.state] and [Notifier.state.copyWith].
@freezed
sealed class ConversationState with _$ConversationState {
  const factory ConversationState({
    required String currentUserId,
    required String currentUserName,
    // Null only before [ConversationNotifier.initialize] has run.
    ConversationItem? conversation,
    @Default(0) int unreadCount,
    @Default([]) List<ChatMessage> messages,
    @Default([]) List<User> participants,
    @Default(true) bool isLoadingMessages,
    @Default(false) bool isSending,
    @Default(false) bool isFetchingEntityForModal,
    UserError? error,
    String? cachedOtherParticipantName,
    Transfer? cachedTransfer,
    String? gearThumbnailUrl,
    Request? cachedRequest,
    String? requestMediaUrl,
    GetExperienceResponse? cachedExperience,
    String? experienceMediaUrl,
    String? experienceLocationName,
    @Default({}) Map<String, RSVPIntention> rsvpStatusMap,
    @Default({}) Map<String, TransferState> transferStatusMap,
    ConversationContext? conversationContext,
    String? backgroundImageUrl,
    String? backgroundImageMediaId,
    @Default(false) bool isVideo,
    VideoPlayerController? videoController,
    @Default([]) List<String> pendingAttachments,
    @Default(0) int uploadProgress,
    @Default(0) int uploadTotal,
    @Default(false) bool isStreamDisconnected,
    // Compose-bar modes. At most one of these is set at a time.
    // When [editingMessageId] is set, the compose bar is editing that message.
    String? editingMessageId,
    // When [replyingToMessageId] is set, the compose bar is composing a reply
    // quoting that message; [replyingToSenderName] and [replyingToText] hold a
    // snapshot for the reply banner.
    String? replyingToMessageId,
    String? replyingToSenderName,
    String? replyingToText,
  }) = _ConversationState;
}
