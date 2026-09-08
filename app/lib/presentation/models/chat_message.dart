import '../../data/gen/ripls/api/chat_service.pb.dart';
import '../../data/gen/ripls/api/user.pb.dart' show User;

/// Message delivery state for optimistic UI updates
enum MessageState {
  /// Message is being sent to server, awaiting confirmation
  pending,

  /// Message confirmed by server via stream echo
  delivered,

  /// Message failed to send (timeout or error)
  failed,
}

/// ChatMessage wraps MessageHistoryItem with client-side state for optimistic UI
class ChatMessage {
  /// The underlying protobuf message
  final MessageHistoryItem message;

  /// The user who sent this message
  final User sender;

  /// Current delivery state
  final MessageState state;

  /// Whether this message has a temporary (client-generated) ID
  final bool isTemporary;

  ChatMessage({
    required this.message,
    required this.sender,
    this.state = MessageState.delivered,
    this.isTemporary = false,
  });

  /// Creates a copy with updated fields
  ChatMessage copyWith({
    MessageHistoryItem? message,
    User? sender,
    MessageState? state,
    bool? isTemporary,
  }) {
    return ChatMessage(
      message: message ?? this.message,
      sender: sender ?? this.sender,
      state: state ?? this.state,
      isTemporary: isTemporary ?? this.isTemporary,
    );
  }

  /// Convenience getters that delegate to the underlying message
  String get messageId => message.messageId;
  String get senderId => sender.id;
  String get senderName => sender.name;

  /// Get text from either user or system message
  String get text {
    if (message.hasUserMessage()) {
      return message.userMessage.text;
    } else if (message.hasSystemMessage()) {
      return message.systemMessage.description;
    }
    return '';
  }

  int get sentAtUnixSec => message.sentAtUnixSec.toInt();

  /// Returns true if this is a system-generated message
  bool get isSystemMessage => message.hasSystemMessage();

  
  /// Emoji reactions on this message.
  List<Reaction> get reactions => message.reactions;

  /// Whether this user message has been edited.
  bool get isEdited =>
      message.hasUserMessage() && message.userMessage.hasEditedAtUnixSec();

  /// The quoted reply context, if this message is a reply. Null otherwise.
  ReplyContext? get replyTo {
    if (message.hasUserMessage() && message.userMessage.hasReplyTo()) {
      return message.userMessage.replyTo;
    }
    return null;
  }

  /// Get media IDs from user message (empty list if system message)
  List<String> get mediaIds {
    if (message.hasUserMessage()) {
      return message.userMessage.mediaIds;
    }
    return [];
  }

  /// The need ID this message is annotating, if any.
  String? get needId {
    if (message.hasUserMessage() && message.userMessage.hasNeedId()) {
      return message.userMessage.needId;
    }
    return null;
  }

  /// The display name of the referenced need, embedded in the message.
  String? get needName {
    if (message.hasUserMessage() && message.userMessage.hasNeedName()) {
      return message.userMessage.needName;
    }
    return null;
  }

  /// The contribution ID this message is annotating, if any.
  String? get contributionId {
    if (message.hasUserMessage() && message.userMessage.hasContributionId()) {
      return message.userMessage.contributionId;
    }
    return null;
  }

  /// The display title of the referenced contribution, embedded in the message.
  String? get contributionTitle {
    if (message.hasUserMessage() && message.userMessage.hasContributionTitle()) {
      return message.userMessage.contributionTitle;
    }
    return null;
  }
}
