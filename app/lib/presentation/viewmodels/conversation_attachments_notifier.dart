import 'package:cross_file/cross_file.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';
import 'package:ripls/presentation/viewmodels/conversation_state.dart';
import 'package:ripls/services/providers.dart';

final _log = Logger('ConversationAttachments');

/// ConversationAttachmentsMixin manages pending attachment staging and
/// upload progress for [ConversationNotifier].
mixin ConversationAttachmentsMixin on Notifier<ConversationState> {
  /// maxPendingAttachments is the maximum number of attachments per chat message.
  static const int maxPendingAttachments = 10;

  /// addPendingAttachments stages local file paths for sending with the next
  /// message. Caps at [maxPendingAttachments].
  void addPendingAttachments(List<String> filePaths) {
    if (filePaths.isEmpty) return;
    final current = state.pendingAttachments;
    final remaining = maxPendingAttachments - current.length;
    if (remaining <= 0) return;
    final toAdd = filePaths.take(remaining).toList();
    state = state.copyWith(pendingAttachments: [...current, ...toAdd]);
  }

  /// removePendingAttachment removes a staged attachment by index.
  void removePendingAttachment(int index) {
    final current = List<String>.from(state.pendingAttachments);
    if (index < 0 || index >= current.length) return;
    current.removeAt(index);
    state = state.copyWith(pendingAttachments: current);
  }

  /// clearPendingAttachments removes all staged attachments.
  void clearPendingAttachments() {
    if (state.pendingAttachments.isEmpty) return;
    state = state.copyWith(pendingAttachments: []);
  }

  /// sendMessageWithPendingAttachments uploads all staged files sequentially,
  /// then sends a single message with all media IDs.
  Future<void> sendMessageWithPendingAttachments({String? text}) async {
    final filePaths = List<String>.from(state.pendingAttachments);
    if (filePaths.isEmpty) return;

    state = state.copyWith(
      isSending: true,
      uploadProgress: 0,
      uploadTotal: filePaths.length,
    );

    final mediaRepository = ref.read(mediaRepositoryProvider);
    final uploadedIds = <String>[];

    try {
      for (var i = 0; i < filePaths.length; i++) {
        if (!ref.mounted) {
          _log.info('Provider disposed during batch upload, stopping.');
          return;
        }

        // pendingAttachments carries filesystem paths from mobile
        // pickers. Chat-attachment-on-web is gated upstream (see
        // inline_conversation_view._pickAndStageMedia) because the
        // staged-preview renderer uses dart:io File. When that
        // changes, switch pendingAttachments to List<XFile> here too.
        final mediaId = await mediaRepository.addMedia(
          file: XFile(filePaths[i]),
        );
        uploadedIds.add(mediaId);

        if (!ref.mounted) return;
        state = state.copyWith(uploadProgress: i + 1);
        _log.info('Uploaded ${i + 1}/${filePaths.length}: $mediaId');
      }

      if (!ref.mounted) return;

      // Clear attachments and send with all uploaded IDs.
      state = state.copyWith(pendingAttachments: []);
      await sendMessage(text ?? '', mediaIds: uploadedIds);
    } catch (e) {
      _log.severe('Failed to send message with attachments: $e');
      if (!ref.mounted) return;
      state = state.copyWith(
        isSending: false,
        uploadProgress: 0,
        uploadTotal: 0,
      );
      rethrow;
    } finally {
      if (ref.mounted) {
        state = state.copyWith(uploadProgress: 0, uploadTotal: 0);
      }
    }
  }

  // ── Cross-mixin hook ──────────────────────────────────────────────────────

  Future<void> sendMessage(
    String text, {
    List<String>? mediaIds,
    String? replyToMessageId,
  });
}
