import 'package:flutter/material.dart';
import 'package:ripls/presentation/widgets/chat/conversation_overlay_chrome.dart';
import 'package:ripls/presentation/widgets/content/content_morph_panel.dart';

/// GearConversationPanel is the full-screen gear conversation the discussion
/// card expands into — the same morph-reveal surface used by the request and
/// experience conversation panels (docs/client/modals.md). It hosts the
/// conversation [child] (an `InlineConversationView`, built by the content view
/// so the media callbacks stay there) inside the shared [ContentMorphPanel]
/// chrome over the still-playing hero, with the shared
/// [ConversationOverlayChrome] header: a top inset + fade so the transcript
/// never collides with the single close (✕) control (system back and swipe
/// also close).
class GearConversationPanel extends StatelessWidget {
  /// The conversation body (typically an `InlineConversationView`).
  final Widget child;

  const GearConversationPanel({super.key, required this.child});

  @override
  Widget build(BuildContext context) {
    return ContentMorphPanel(
      child: ConversationOverlayChrome(child: child),
    );
  }
}
