import 'package:connectrpc/connect.dart' as connect;
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/utils/rpc_utils.dart';
import 'package:ripls/data/gen/ripls/api/feed_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/user_service.connect.client.dart';
import 'package:ripls/data/gen/ripls/api/user_service.pb.dart';
import 'package:ripls/services/location_service.dart';

export 'package:ripls/core/errors/rpc_error_handler.dart' show ServiceException;

/// UserLocationInfo holds information about a user's saved location
class UserLocationInfo {
  final String locationId;
  final bool isPrimary;
  final Location locationDetails;

  UserLocationInfo({
    required this.locationId,
    required this.isPrimary,
    required this.locationDetails,
  });
}

/// UserService handles user information retrieval using the UserService API.
class UserService {
  final UserServiceClient _client;
  final String? Function() _getAccessToken;
  final RpcErrorHandler _errorHandler;
  final Future<void> Function()? _onUnauthenticated;
  final LocationService? _locationService;

  UserService({
    required connect.Transport transport,
    required String? Function() getAccessToken,
    RpcErrorHandler? errorHandler,
    Future<void> Function()? onUnauthenticated,
    LocationService? locationService,
  }) : _client = UserServiceClient(transport),
       _getAccessToken = getAccessToken,
       _errorHandler = errorHandler ?? RpcErrorHandler(),
       _onUnauthenticated = onUnauthenticated,
       _locationService = locationService;

  connect.Headers _buildHeaders() {
    return RpcUtils.buildHeaders(
      _getAccessToken,
      onUnauthenticated: _onUnauthenticated,
    );
  }

  /// GetUser retrieves user information by user ID.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<GetUserResponse> getUser(String userId) async {
    return RpcUtils.executeRpc(
      () async {
        final request = GetUserRequest(userId: userId);
        final response = await _client.getUser(
          request,
          headers: _buildHeaders(),
        );
        return response;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetUser',
    );
  }

  /// SaveUser updates user information.
  /// Only fields that are set in the request are updated; omitted fields are ignored.
  ///
  /// Returns the user ID.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<String> saveUser({
    required String userId,
    String? name,
    String? primaryResidenceLocationId,
    List<String>? otherLocationIds,
    List<String>? mediaIds,
    String? description,
    String? preferredTimezone,
    String? preferredLanguage,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = SaveUserRequest(userId: userId);

        if (name != null) {
          request.name = name;
        }
        if (primaryResidenceLocationId != null) {
          request.primaryResidenceLocationId = primaryResidenceLocationId;
        }
        if (otherLocationIds != null) {
          request.otherLocationIds.addAll(otherLocationIds);
        }
        if (mediaIds != null) {
          request.mediaIds.addAll(mediaIds);
        }
        if (description != null) {
          request.description = description;
        }
        if (preferredTimezone != null) {
          request.preferredTimezone = preferredTimezone;
        }
        if (preferredLanguage != null) {
          request.preferredLanguage = preferredLanguage;
        }

        final response = await _client.saveUser(
          request,
          headers: _buildHeaders(),
        );

        return response.userId;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'SaveUser',
    );
  }

  /// AddPhoneNumber attaches a verified phone number to the current account as
  /// an additional sign-in method; the account's existing sign-in method keeps
  /// working. [firebaseIdToken] is the phone OTP verification token. Returns the
  /// phone number now set on the account (E.164).
  ///
  /// Throws [ServiceException] with a user-friendly message on failure (e.g. the
  /// number already belongs to a different account).
  Future<String> addPhoneNumber({required String firebaseIdToken}) async {
    return RpcUtils.executeRpc(
      () async {
        final response = await _client.addPhoneNumber(
          AddPhoneNumberRequest(firebaseIdToken: firebaseIdToken),
          headers: _buildHeaders(),
        );
        return response.phoneNumber;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'AddPhoneNumber',
    );
  }

  /// DeleteUser soft-deletes the current user's account.
  ///
  /// This action is irreversible and will:
  /// - Mark the user account as deleted
  /// - Prevent future logins with this account
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<void> deleteUser() async {
    return RpcUtils.executeRpc(
      () async {
        final request = DeleteUserRequest();
        await _client.deleteUser(
          request,
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'DeleteUser',
    );
  }

  /// GetUserStats retrieves aggregate statistics and impact metrics for a user.
  ///
  /// Returns user statistics including community count, item count, loans, borrows,
  /// giveaways, help offered, events hosted, and savings metrics.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<GetUserStatsResponse> getUserStats(String userId) async {
    return RpcUtils.executeRpc(
      () async {
        final request = GetUserStatsRequest(userId: userId);
        final response = await _client.getUserStats(
          request,
          headers: _buildHeaders(),
        );
        return response;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetUserStats',
    );
  }

  /// ListUserStories retrieves stories where the user is a participant.
  ///
  /// Returns list of story payloads ordered by creation time (newest first).
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<List<StoryPayload>> listUserStories({
    required String userId,
    int limit = 10,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = ListUserStoriesRequest(
          userId: userId,
          limit: limit,
        );
        final response = await _client.listUserStories(
          request,
          headers: _buildHeaders(),
        );
        return response.stories;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'ListUserStories',
    );
  }

  /// GetUserNotificationPreferences reads the calling user's user-scoped
  /// notification preferences. Unset toggles in the response mean the
  /// category is enabled — callers must treat unset as "on".
  Future<UserNotificationPreferences> getUserNotificationPreferences() async {
    return RpcUtils.executeRpc(
      () async {
        final request = GetUserNotificationPreferencesRequest();
        final response = await _client.getUserNotificationPreferences(
          request,
          headers: _buildHeaders(),
        );
        return response.preferences;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetUserNotificationPreferences',
    );
  }

  /// UpdateUserNotificationPreferences persists user-scoped preferences.
  /// Only fields explicitly set on [preferences] are written; unset fields
  /// preserve their existing value. Returns the persisted message.
  Future<UserNotificationPreferences> updateUserNotificationPreferences({
    required UserNotificationPreferences preferences,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = UpdateUserNotificationPreferencesRequest(
          preferences: preferences,
        );
        final response = await _client.updateUserNotificationPreferences(
          request,
          headers: _buildHeaders(),
        );
        return response.preferences;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'UpdateUserNotificationPreferences',
    );
  }

  /// GetUserSavedLocations retrieves all saved locations for a user.
  /// Returns a list with the primary residence first, followed by other locations.
  /// Requires LocationService to be provided in the constructor.
  ///
  /// Returns empty list if user has no saved locations.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<List<UserLocationInfo>> getUserSavedLocations(String userId) async {
    if (_locationService == null) {
      throw ServiceException('LocationService not provided to UserService');
    }

    return RpcUtils.executeRpc(
      () async {
        // Get user info
        final user = await getUser(userId);
        final locations = <UserLocationInfo>[];

        // Fetch primary residence location if it exists
        if (user.primaryResidenceLocationId.isNotEmpty) {
          try {
            final locationDetails = await _locationService.getLocation(
              user.primaryResidenceLocationId,
            );
            locations.add(
              UserLocationInfo(
                locationId: user.primaryResidenceLocationId,
                isPrimary: true,
                locationDetails: locationDetails,
              ),
            );
          } catch (e) {
            // Skip if location fetch fails
          }
        }

        // Fetch other locations
        for (final locationId in user.otherLocationIds) {
          if (locationId.isEmpty) continue;
          try {
            final locationDetails = await _locationService.getLocation(
              locationId,
            );
            locations.add(
              UserLocationInfo(
                locationId: locationId,
                isPrimary: false,
                locationDetails: locationDetails,
              ),
            );
          } catch (e) {
            // Skip if location fetch fails
          }
        }

        return locations;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetUserSavedLocations',
    );
  }
}
