// ExperienceService is split across part files by RPC group using Dart's
// `part`/`part of` mechanism combined with mixins on an abstract base class.
// ExperienceServiceBase (defined in this file) exposes the RPC client and
// request-building helpers as library-private getters. The five mixin files
// each declare `mixin <Name> on ExperienceServiceBase` so their methods have
// access to those helpers without circular inheritance. ExperienceService
// extends ExperienceServiceBase and mixes in all five groups.
//
// Callers and Mockito-generated mocks use ExperienceService normally;
// the mixin methods are part of the public class interface.
//
// Part files:
//   experience/lifecycle_methods.dart — save/share/cancel/complete/delete RPCs
//   experience/fetch_methods.dart     — read-only RPCs (get, list, stats, people)
//   experience/rsvp_time_methods.dart — RSVP, attendance, time proposal/voting RPCs
//   experience/gen_methods.dart       — AI generation and time-parsing RPCs
//   experience/needs_methods.dart     — needs and contributions RPCs

import 'package:connectrpc/connect.dart' as connect;
import 'package:fixnum/fixnum.dart';
import 'package:flutter_timezone/flutter_timezone.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/utils/rpc_utils.dart';
import 'package:ripls/data/gen/ripls/api/community_service.connect.client.dart'
    show CommunityServiceClient;
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart'
    show ShareItemRequest, UnshareItemRequest;
import 'package:ripls/data/gen/ripls/api/experience.pb.dart';
import 'package:ripls/data/gen/ripls/api/experience_service.connect.client.dart';
import 'package:ripls/data/gen/ripls/api/experience_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart';
import 'package:ripls/data/gen/ripls/api/location.pb.dart';
import 'package:ripls/data/gen/ripls/api/portfolio.pb.dart';
import 'package:ripls/data/gen/ripls/api/social.pb.dart';
import 'package:ripls/data/gen/ripls/api/time.pb.dart';

export 'package:ripls/core/errors/rpc_error_handler.dart' show ServiceException;
export 'package:ripls/data/gen/ripls/api/experience.pb.dart'
    show Experience, ExperienceState, ExperienceNeedResponse, ExperienceContributionResponse;
export 'package:ripls/data/gen/ripls/api/experience_service.pb.dart'
    show
        GetExperienceResponse,
        GenExperienceResponse,
        ConvertInformalTimeResponse,
        ExperienceMetadata,
        RSVP,
        RSVPIntention,
        AttendedStatus,
        AttendanceRecord,
        GetExperiencePeopleResponse,
        GetExperienceDayWeatherResponse,
        GetExperienceStatsResponse,
        CompleteExperienceResponse,
        ListExperienceNeedsAndContributionsResponse,
        AddExperienceNeedResponse,
        ClaimExperienceNeedResponse,
        AddExperienceContributionResponse,
        EditExperienceContributionResponse,
        BatchNeedItem,
        BatchContributionItem,
        BatchAddExperienceNeedsRequest,
        BatchAddExperienceContributionsRequest,
        StreamGenExperienceRequest,
        StreamGenExperienceResponse,
        StreamGenExperienceResponse_Event;
export 'package:ripls/data/gen/ripls/api/gen_stream.pb.dart'
    show GenStreamError, GenStreamErrorCode, MediaReady;
export 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart' show ImpactEstimate;
export 'package:ripls/data/gen/ripls/api/location.pb.dart'
    show
        GeocodedLocation,
        LocationProposal,
        LocationVote,
        LocationVoteStatus,
        ProposedLocation;
export 'package:ripls/data/gen/ripls/api/portfolio.pb.dart'
    show HourForecast, DayForecast, DayForecastCondition;
export 'package:ripls/data/gen/ripls/api/social.pb.dart' show SocialContext;
export 'package:ripls/data/gen/ripls/api/time.pb.dart'
    show
        ExperienceTime,
        SpecificTime,
        TimeRange,
        TimeTBD,
        TimeProposal,
        TimeVote,
        TimeVoteStatus,
        TimeConfidence;

part 'experience/fetch_methods.dart';
part 'experience/gen_methods.dart';
part 'experience/lifecycle_methods.dart';
part 'experience/location_poll_methods.dart';
part 'experience/needs_methods.dart';
part 'experience/rsvp_time_methods.dart';

/// ExperienceServiceBase provides the RPC client and request-building helpers
/// that the method-group mixins depend on.
abstract class ExperienceServiceBase {
  ExperienceServiceClient get _client;
  // Community-service client used for the converged sharing RPCs (ShareItem /
  // UnshareItem, #2526): experience audience management lives on
  // CommunityService, not ExperienceService.
  CommunityServiceClient get _communityClient;
  RpcErrorHandler get _errorHandler;
  Future<void> Function()? get _onUnauthenticated;

  connect.Headers _buildHeaders();
  Future<String> _getIANATimezone();
}

/// ExperienceService handles experience-related operations using the ExperienceService API.
class ExperienceService extends ExperienceServiceBase
    with
        ExperienceLifecycleMethods,
        ExperienceFetchMethods,
        ExperienceRsvpTimeMethods,
        ExperienceGenMethods,
        ExperienceNeedsMethods,
        ExperienceLocationPollMethods {
  @override
  final ExperienceServiceClient _client;
  @override
  final CommunityServiceClient _communityClient;
  final String? Function() _getAccessToken;
  @override
  final RpcErrorHandler _errorHandler;
  @override
  final Future<void> Function()? _onUnauthenticated;

  ExperienceService({
    required connect.Transport transport,
    required String? Function() getAccessToken,
    RpcErrorHandler? errorHandler,
    Future<void> Function()? onUnauthenticated,
  }) : _client = ExperienceServiceClient(transport),
       _communityClient = CommunityServiceClient(transport),
       _getAccessToken = getAccessToken,
       _errorHandler = errorHandler ?? RpcErrorHandler(),
       _onUnauthenticated = onUnauthenticated;

  @override
  connect.Headers _buildHeaders() {
    return RpcUtils.buildHeaders(
      _getAccessToken,
      onUnauthenticated: _onUnauthenticated,
    );
  }

  @override
  Future<String> _getIANATimezone() async {
    try {
      return (await FlutterTimezone.getLocalTimezone()).identifier;
    } catch (_) {
      return 'UTC';
    }
  }
}
