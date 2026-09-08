import 'package:flutter/material.dart';
import 'package:ripls/presentation/widgets/chat/conversation_overlay_chrome.dart';
import 'package:ripls/presentation/widgets/content/content_morph_panel.dart';

/// ExperienceConversationPanel is the full-screen conversation the discussion
/// ("comments") card expands into — the same morph-reveal surface as the
/// "Who's pitching in?" roster (docs/client/modals.md). It hosts the
/// conversation [child] (an `ExperienceChatPane`, built by the content view so
/// the media callbacks stay there) inside the shared [ContentMorphPanel] chrome
/// (strong media scrim + horizontal swipe-to-close) over the still-playing
/// hero, with the shared [ConversationOverlayChrome] header: a top inset +
/// fade so the transcript never collides with the single close (✕) control
/// (system back and swipe also close).
class ExperienceConversationPanel extends StatelessWidget {
  /// The conversation body (typically an `ExperienceChatPane`).
  final Widget child;

  const ExperienceConversationPanel({super.key, required this.child});

  @override
  Widget build(BuildContext context) {
    return ContentMorphPanel(
      child: ConversationOverlayChrome(child: child),
    );
  }
}
