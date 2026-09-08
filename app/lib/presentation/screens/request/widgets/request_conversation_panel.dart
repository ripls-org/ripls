import 'package:flutter/material.dart';
import 'package:ripls/presentation/widgets/chat/conversation_overlay_chrome.dart';
import 'package:ripls/presentation/widgets/content/content_morph_panel.dart';

/// RequestConversationPanel is the full-screen conversation the discussion card
/// expands into — the same morph-reveal surface as the location / helpers
/// panels (docs/client/modals.md), mirroring `ExperienceConversationPanel`. It
/// hosts the conversation [child] (a `RequestChatPane`, built by the content
/// view so the media callbacks stay there) inside the shared [ContentMorphPanel]
/// chrome over the still-playing hero, with the shared
/// [ConversationOverlayChrome] header: a top inset + fade so the transcript
/// never collides with the single close (✕) control (system back and swipe
/// also close).
class RequestConversationPanel extends StatelessWidget {
  /// The conversation body (typically a `RequestChatPane`).
  final Widget child;

  const RequestConversationPanel({super.key, required this.child});

  @override
  Widget build(BuildContext context) {
    return ContentMorphPanel(
      child: ConversationOverlayChrome(child: child),
    );
  }
}
