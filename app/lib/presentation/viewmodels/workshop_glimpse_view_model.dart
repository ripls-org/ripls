import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/services/providers.dart';

/// One rotating quote shown in the Workshop overview chat glimpse.
class WorkshopGlimpseQuote {
  final String text;
  final String author;
  final int atUnixSec;

  const WorkshopGlimpseQuote({
    required this.text,
    required this.author,
    required this.atUnixSec,
  });
}

/// Fetches the most-recent user-message quotes from a community's perpetual
/// conversation, for the Workshop overview's rotating chat glimpse (#2447).
///
/// Returns an empty list when the community has no conversation yet or no
/// renderable user messages — the widget then falls back to the cta roller.
final workshopGlimpseProvider = FutureProvider.autoDispose
    .family<List<WorkshopGlimpseQuote>, String>((ref, communityId) async {
  final chatRepo = ref.read(chatRepositoryProvider);
  final conversation =
      await chatRepo.getConversationForCommunity(communityId);
  final conversationId = conversation.conversationId;
  if (conversationId.isEmpty) return const <WorkshopGlimpseQuote>[];

  final history = await chatRepo.getConversationHistory(
    conversationId: conversationId,
    maxMessages: 12,
  );

  // History is chronological; surface the latest few user messages newest-first.
  final quotes = <WorkshopGlimpseQuote>[];
  for (final item in history.messages.reversed) {
    if (!item.hasUserMessage()) continue;
    final msg = item.userMessage;
    final text = msg.text.trim();
    if (text.isEmpty) continue;
    quotes.add(WorkshopGlimpseQuote(
      text: text,
      author: msg.sender.name,
      atUnixSec: item.sentAtUnixSec.toInt(),
    ));
    if (quotes.length >= 3) break;
  }
  return quotes;
});
