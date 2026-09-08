import 'package:connectrpc/connect.dart' as connect;
import 'package:fixnum/fixnum.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/utils/rpc_utils.dart';
import 'package:ripls/data/gen/ripls/api/gear.pbenum.dart' show Availability;
import 'package:ripls/data/gen/ripls/api/gear_booking.pb.dart';
import 'package:ripls/data/gen/ripls/api/gear_service.connect.client.dart';
import 'package:ripls/data/gen/ripls/api/gear_service.pb.dart';

export 'package:ripls/core/errors/rpc_error_handler.dart' show ServiceException;
export 'package:ripls/data/gen/ripls/api/gear_booking.pb.dart'
    show GearBooking, GearBookingState;
export 'package:ripls/data/gen/ripls/api/gear_service.pb.dart'
    show
        GearItem,
        GetGearResponse,
        DetectedGearItem,
        GetTransferRequestCountResponse,
        GetGearPeopleResponse,
        GetGearStatsResponse,
        GearMetadata,
        DetectGearFromURLResponse,
        StreamGenGearRequest,
        StreamGenGearResponse,
        StreamGenGearResponse_Event;
export 'package:ripls/data/gen/ripls/api/gen_stream.pb.dart'
    show GenStreamError, GenStreamErrorCode, MediaReady;

/// GearService handles gear-related operations using the GearService API.
class GearService {
  final GearServiceClient _client;
  final String? Function() _getAccessToken;
  final RpcErrorHandler _errorHandler;
  final Future<void> Function()? _onUnauthenticated;

  GearService({
    required connect.Transport transport,
    required String? Function() getAccessToken,
    RpcErrorHandler? errorHandler,
    Future<void> Function()? onUnauthenticated,
  }) : _client = GearServiceClient(transport),
       _getAccessToken = getAccessToken,
       _errorHandler = errorHandler ?? RpcErrorHandler(),
       _onUnauthenticated = onUnauthenticated;

  connect.Headers _buildHeaders() {
    return RpcUtils.buildHeaders(
      _getAccessToken,
      onUnauthenticated: _onUnauthenticated,
    );
  }

  /// ListGear retrieves gear items owned by the authenticated user.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<List<GearItem>> listGear() async {
    return RpcUtils.executeRpc(
      () async {
        final request = ListUserGearRequest();
        final response = await _client.listUserGear(
          request,
          headers: _buildHeaders(),
        );

        return response.items;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'ListUserGear',
    );
  }

  /// SaveGear saves (creates or updates) an item of gear in the library.
  /// If id is null or empty, creates a new gear item.
  /// If id is provided, updates the existing gear item.
  ///
  /// Returns the ID of the saved gear.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<String> saveGear({
    String? id,
    String? name,
    String? description,
    List<String>? mediaIds,
    String? locationId,
    GearMetadata? metadata,
    String? generationMode,
    String? sourceUrl,
    Availability? availability,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = SaveGearRequest();

        if (id != null && id.isNotEmpty) {
          request.id = id;
        }
        if (name != null) {
          request.name = name;
        }
        if (description != null) {
          request.description = description;
        }
        if (mediaIds != null && mediaIds.isNotEmpty) {
          request.mediaIds.addAll(mediaIds);
        }
        if (locationId != null) {
          request.locationId = locationId;
        }
        if (metadata != null) {
          request.metadata = metadata;
        }
        if (generationMode != null) {
          request.generationMode = generationMode;
        }
        if (sourceUrl != null && sourceUrl.isNotEmpty) {
          request.sourceUrl = sourceUrl;
        }
        // Creation-time Lend/Give choice for the per-item community's first
        // share (#2687); honored on insert only, see gear_service.proto.
        if (availability != null) {
          request.availability = availability;
        }

        final response = await _client.saveGear(
          request,
          headers: _buildHeaders(),
        );

        return response.id;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'SaveGear',
    );
  }

  /// GetGear retrieves a specific item of gear by ID.
  ///
  /// If [communityId] is provided, also returns community-specific fields
  /// (conversation_id, active_loan, availability).
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<GetGearResponse> getGear(String id, {String? communityId}) async {
    return RpcUtils.executeRpc(
      () async {
        final request = GetGearRequest(
          id: id,
          communityId: communityId ?? '',
        );
        final response = await _client.getGear(
          request,
          headers: _buildHeaders(),
        );

        return response;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetGear',
    );
  }

  /// GenGear generates gear from text prompt using AI.
  ///
  /// StreamGenGear emits title / media_ready events as each resolves, then a
  /// terminal `final` or `error`. Errors surface through the returned
  /// stream's `onError`.
  ///
  /// Mode is determined by which of [prompt], [websiteUrl], or [mediaId] is
  /// provided. Exactly one must be non-empty.
  Stream<StreamGenGearResponse> streamGenGear({
    String? prompt,
    String? websiteUrl,
    String? mediaId,
    String? locationId,
    double? latitudeDeg,
    double? longitudeDeg,
  }) {
    final request = StreamGenGearRequest(
      locationId: locationId ?? '',
      latitudeDeg: latitudeDeg ?? 0.0,
      longitudeDeg: longitudeDeg ?? 0.0,
    );
    if (mediaId != null && mediaId.isNotEmpty) {
      request.mediaId = mediaId;
    } else if (websiteUrl != null && websiteUrl.isNotEmpty) {
      request.websiteUrl = websiteUrl;
    } else if (prompt != null && prompt.isNotEmpty) {
      request.prompt = prompt;
    }
    return _client.streamGenGear(request, headers: _buildHeaders());
  }

  /// DetectGearFromURL extracts product metadata from a product page URL.
  ///
  /// Fetches the webpage and uses AI to extract brand, model, category, material,
  /// weight, and value estimate. Returns an error if the URL cannot be fetched —
  /// no fallback to text generation.
  Future<DetectedGearItem?> detectGearFromURL(String url) async {
    return RpcUtils.executeRpc(
      () async {
        final request = DetectGearFromURLRequest(url: url);
        final response = await _client.detectGearFromURL(
          request,
          headers: _buildHeaders(),
        );
        return response.hasDetectedGear() ? response.detectedGear : null;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'DetectGearFromURL',
    );
  }

  /// DeleteGear deletes a gear item by ID.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<void> deleteGear(String gearId) async {
    return RpcUtils.executeRpc(
      () async {
        final request = DeleteGearRequest(gearId: gearId);
        await _client.deleteGear(
          request,
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'DeleteGear',
    );
  }

  /// GetTransferRequestCount retrieves the count of transfer requests for a gear item.
  ///
  /// Returns information about transfer requests for the gear item:
  /// - For owners: count of non-archived requests received across all communities
  /// - For borrowers: whether they have an active request and its conversation ID
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<GetTransferRequestCountResponse> getTransferRequestCount({
    required String gearId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = GetTransferRequestCountRequest(gearId: gearId);
        final response = await _client.getTransferRequestCount(
          request,
          headers: _buildHeaders(),
        );

        return response;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetTransferRequestCount',
    );
  }

  /// GetGearPeople retrieves people associated with a gear item.
  ///
  /// Returns owner, current borrower, past borrowers, and interested parties.
  /// If [communityId] is provided, filters results for that specific community.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<GetGearPeopleResponse> getGearPeople({
    required String gearId,
    String? communityId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = GetGearPeopleRequest(
          gearId: gearId,
          communityId: communityId ?? '',
        );
        final response = await _client.getGearPeople(
          request,
          headers: _buildHeaders(),
        );

        return response;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetGearPeople',
    );
  }

  /// GetGearStats retrieves statistics for a gear item.
  ///
  /// Returns times loaned, people helped, value shared, and interest count.
  /// If [communityId] is provided, returns stats specific to that community.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<GetGearStatsResponse> getGearStats({
    required String gearId,
    String? communityId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = GetGearStatsRequest(
          gearId: gearId,
          communityId: communityId ?? '',
        );
        final response = await _client.getGearStats(
          request,
          headers: _buildHeaders(),
        );

        return response;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetGearStats',
    );
  }

  /// ListGearBookings returns the who-has-it-which-days schedule for a gear item.
  Future<List<GearBooking>> listGearBookings({
    required String gearId,
    required String communityId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final response = await _client.listGearBookings(
          ListGearBookingsRequest(gearId: gearId, communityId: communityId),
          headers: _buildHeaders(),
        );
        return response.bookings;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'ListGearBookings',
    );
  }

  /// ClaimGearDays reserves an inclusive day range. By default the caller is the
  /// recipient. The gear owner may reserve for someone else ([recipientId]),
  /// block the days ([recipientId] = the owner), or hold them behind an accept
  /// link ([pending]).
  Future<GearBooking> claimGearDays({
    required String gearId,
    required String communityId,
    required int startDateUnixSec,
    required int endDateUnixSec,
    String? recipientId,
    bool pending = false,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = ClaimGearDaysRequest(
          gearId: gearId,
          communityId: communityId,
          startDateUnixSec: Int64(startDateUnixSec),
          endDateUnixSec: Int64(endDateUnixSec),
          pending: pending,
        );
        if (recipientId != null) request.recipientId = recipientId;
        final response = await _client.claimGearDays(
          request,
          headers: _buildHeaders(),
        );
        return response.booking;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'ClaimGearDays',
    );
  }

  /// AcceptGearBooking claims a pending owner-created reservation via its token.
  Future<GearBooking> acceptGearBooking({
    required String bookingId,
    required String acceptToken,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final response = await _client.acceptGearBooking(
          AcceptGearBookingRequest(
            bookingId: bookingId,
            acceptToken: acceptToken,
          ),
          headers: _buildHeaders(),
        );
        return response.booking;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'AcceptGearBooking',
    );
  }

  /// ReleaseGearBooking drops a booking the authenticated user owns.
  Future<void> releaseGearBooking({required String bookingId}) async {
    return RpcUtils.executeRpc(
      () async {
        await _client.releaseGearBooking(
          ReleaseGearBookingRequest(bookingId: bookingId),
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'ReleaseGearBooking',
    );
  }

  /// UpdateGearBookingHandoff sets/clears the pickup + drop-off hand-off on a
  /// booking. A null arg leaves that field unchanged; an empty location id
  /// clears it.
  Future<GearBooking> updateGearBookingHandoff({
    required String bookingId,
    String? pickupLocationId,
    int? pickupTimeUnixSec,
    String? dropoffLocationId,
    int? dropoffTimeUnixSec,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = UpdateGearBookingHandoffRequest(bookingId: bookingId);
        if (pickupLocationId != null) {
          request.pickupLocationId = pickupLocationId;
        }
        if (pickupTimeUnixSec != null) {
          request.pickupTimeUnixSec = Int64(pickupTimeUnixSec);
        }
        if (dropoffLocationId != null) {
          request.dropoffLocationId = dropoffLocationId;
        }
        if (dropoffTimeUnixSec != null) {
          request.dropoffTimeUnixSec = Int64(dropoffTimeUnixSec);
        }
        final response = await _client.updateGearBookingHandoff(
          request,
          headers: _buildHeaders(),
        );
        return response.booking;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'UpdateGearBookingHandoff',
    );
  }
}
