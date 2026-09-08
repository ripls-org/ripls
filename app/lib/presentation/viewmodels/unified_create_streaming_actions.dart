import 'dart:async';

import 'package:ripls/data/gen/ripls/api/experience_service.pb.dart'
    show ExperienceTimeExtraction;
import 'package:ripls/data/gen/ripls/api/gen_stream.pb.dart'
    show MediaCandidate;
import 'package:ripls/data/gen/ripls/api/location.pb.dart'
    show GeocodedLocation;
import 'package:ripls/data/gen/ripls/api/unified_create_service.pb.dart'
    show
        DetectedContentType,
        StreamGenUnifiedCreateFinal,
        StreamGenUnifiedCreateResponse,
        StreamGenUnifiedCreateResponse_Event;
import 'package:ripls/presentation/viewmodels/unified_create_state.dart';

/// Reducer-facing event for the unified-create stream. Wire-side
/// variants from `StreamGenUnifiedCreateResponse` are normalised into
/// this sealed union before being applied to state.
sealed class UnifiedCreateEvent {
  const UnifiedCreateEvent();
}

final class UnifiedCreateEventType extends UnifiedCreateEvent {
  const UnifiedCreateEventType(this.type);
  final DetectedContentType type;
}

final class UnifiedCreateEventTitle extends UnifiedCreateEvent {
  const UnifiedCreateEventTitle(this.title);
  final String title;
}

final class UnifiedCreateEventDescription extends UnifiedCreateEvent {
  const UnifiedCreateEventDescription(this.description);
  final String description;
}

final class UnifiedCreateEventLocation extends UnifiedCreateEvent {
  const UnifiedCreateEventLocation(this.geocoded);
  final GeocodedLocation geocoded;
}

final class UnifiedCreateEventMedia extends UnifiedCreateEvent {
  const UnifiedCreateEventMedia(this.mediaIds, this.candidates);
  final List<String> mediaIds;
  final List<MediaCandidate> candidates;
}

final class UnifiedCreateEventTime extends UnifiedCreateEvent {
  const UnifiedCreateEventTime(this.time);
  final ExperienceTimeExtraction time;
}

final class UnifiedCreateEventFinal extends UnifiedCreateEvent {
  const UnifiedCreateEventFinal(this.finalPayload);
  final StreamGenUnifiedCreateFinal finalPayload;
}

final class UnifiedCreateEventError extends UnifiedCreateEvent {
  const UnifiedCreateEventError(this.message);
  final String message;
}

/// Streaming actions facade. Concrete implementation translates wire
/// events into [UnifiedCreateEvent]s and dispatches them through a
/// broadcast stream. Tests substitute a fake.
abstract class UnifiedCreateStreamingActions {
  Stream<UnifiedCreateEvent> stream({
    required UnifiedCreateState state,
    DetectedContentType? forceType,
  });
}

/// Production [UnifiedCreateStreamingActions] backed by the connect-go
/// `streamGenUnifiedCreate` RPC. Maps each
/// `StreamGenUnifiedCreateResponse` wire variant into a typed event the
/// view-model reducer can apply.
class LiveUnifiedCreateStreamingActions implements UnifiedCreateStreamingActions {
  LiveUnifiedCreateStreamingActions(this._streamCall);

  /// Injection seam: the host wires in a closure that calls
  /// UnifiedCreateService.streamGenUnifiedCreate.
  final Stream<StreamGenUnifiedCreateResponse> Function({
    String? text,
    String? mediaId,
    String? websiteUrl,
    DetectedContentType? forceType,
  }) _streamCall;

  @override
  Stream<UnifiedCreateEvent> stream({
    required UnifiedCreateState state,
    DetectedContentType? forceType,
  }) async* {
    // Pick exactly one input based on the active mode so the server's
    // "exactly one of text / media_id / website_url" validation isn't
    // tripped by leftover data in another panel.
    String? text;
    String? mediaId;
    String? websiteUrl;
    switch (state.inputMode) {
      case CreateInputMode.text:
        text = state.prompt.isNotEmpty ? state.prompt : null;
        break;
      case CreateInputMode.image:
        mediaId = state.mediaId;
        break;
      case CreateInputMode.url:
        websiteUrl = state.urlInput.isNotEmpty ? state.urlInput : null;
        break;
    }
    final wire = _streamCall(
      text: text,
      mediaId: mediaId,
      websiteUrl: websiteUrl,
      forceType: forceType,
    );
    await for (final ev in wire) {
      final mapped = _mapWireEvent(ev);
      if (mapped != null) yield mapped;
    }
  }
}

/// Map a wire-side [StreamGenUnifiedCreateResponse] to a reducer event.
UnifiedCreateEvent? _mapWireEvent(StreamGenUnifiedCreateResponse wire) {
  switch (wire.whichEvent()) {
    case StreamGenUnifiedCreateResponse_Event.type:
      return UnifiedCreateEventType(wire.type);
    case StreamGenUnifiedCreateResponse_Event.title:
      return UnifiedCreateEventTitle(wire.title);
    case StreamGenUnifiedCreateResponse_Event.description:
      return UnifiedCreateEventDescription(wire.description);
    case StreamGenUnifiedCreateResponse_Event.geocoded:
      return UnifiedCreateEventLocation(wire.geocoded);
    case StreamGenUnifiedCreateResponse_Event.mediaReady:
      return UnifiedCreateEventMedia(
        List<String>.unmodifiable(wire.mediaReady.mediaIds),
        List<MediaCandidate>.unmodifiable(wire.mediaReady.candidates),
      );
    case StreamGenUnifiedCreateResponse_Event.time:
      return UnifiedCreateEventTime(wire.time);
    case StreamGenUnifiedCreateResponse_Event.final_7:
      return UnifiedCreateEventFinal(wire.final_7);
    case StreamGenUnifiedCreateResponse_Event.error:
      return UnifiedCreateEventError(wire.error.message);
    case StreamGenUnifiedCreateResponse_Event.notSet:
      return null;
  }
}
