import 'package:ripls/data/gen/ripls/api/gen_stream.pb.dart' show MediaCandidate;

/// One entry in the Replace Media row. Either an externally-sourced
/// candidate from the streaming Gen* response (carries a URL + optional
/// provider metadata so import can preserve attribution) or a reference
/// to a media row already in the user's library (used when swapping
/// "back" to a previously-active media — no import needed).
sealed class ReplaceMediaSlot {
  const ReplaceMediaSlot();
}

/// A streamed candidate that requires server-side import on tap. The
/// underlying [MediaCandidate] carries the URL the server fetches and
/// any provider+id metadata that routes through the trusted-provider
/// path (see media.AddMediaFromURL Phase 3b).
final class ProtoSlot extends ReplaceMediaSlot {
  const ProtoSlot(this.candidate);
  final MediaCandidate candidate;
}

/// A reference to a media row already in the user's library. Used to
/// represent the previously-active media so the user can swap back to
/// it from the Replace Media row. Tapping a [MediaIdSlot] does no
/// network work — the view-model just sets [UnifiedCreateState.mediaIds]
/// to the carried id.
final class MediaIdSlot extends ReplaceMediaSlot {
  const MediaIdSlot(this.mediaId);
  final String mediaId;
}
