import 'package:connectrpc/connect.dart' as connect;
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/utils/rpc_utils.dart';
import 'package:ripls/data/gen/ripls/api/location.pb.dart';
import 'package:ripls/data/gen/ripls/api/location_service.connect.client.dart';
import 'package:ripls/data/gen/ripls/api/location_service.pb.dart';

export 'package:ripls/core/errors/rpc_error_handler.dart' show ServiceException;
export 'package:ripls/data/gen/ripls/api/location.pb.dart' show Location;
export 'package:ripls/data/gen/ripls/api/location_service.pb.dart'
    show SaveLocationResponse;

/// LocationService handles location-related operations using the LocationService API.
class LocationService {
  final LocationServiceClient _client;
  final String? Function() _getAccessToken;
  final RpcErrorHandler _errorHandler;
  final Future<void> Function()? _onUnauthenticated;

  LocationService({
    required connect.Transport transport,
    required String? Function() getAccessToken,
    RpcErrorHandler? errorHandler,
    Future<void> Function()? onUnauthenticated,
  }) : _client = LocationServiceClient(transport),
       _getAccessToken = getAccessToken,
       _errorHandler = errorHandler ?? RpcErrorHandler(),
       _onUnauthenticated = onUnauthenticated;

  connect.Headers _buildHeaders() {
    return RpcUtils.buildHeaders(
      _getAccessToken,
      onUnauthenticated: _onUnauthenticated,
    );
  }

  /// SaveLocation saves a new or updated location.
  ///
  /// Returns the ID of the saved location.
  /// Automatically adds the location to the authenticated user's location list.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<String> saveLocation({
    required double latitudeDeg,
    required double longitudeDeg,
    required String regionCode,
    required String postalCode,
    required String locality,
    List<String>? addressLines,
    String? name,
    String? neighborhood,
    String? county,
    String? administrativeArea,
    String? externalPlaceId,
    String? externalPlaceProvider,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = SaveLocationRequest(
          latitudeDeg: latitudeDeg,
          longitudeDeg: longitudeDeg,
          regionCode: regionCode,
          postalCode: postalCode,
          locality: locality,
          name: name ?? '',
          neighborhood: neighborhood ?? '',
          county: county ?? '',
          administrativeArea: administrativeArea ?? '',
          externalPlaceId: externalPlaceId ?? '',
          externalPlaceProvider: externalPlaceProvider ?? '',
        );

        if (addressLines != null && addressLines.isNotEmpty) {
          request.addressLines.addAll(addressLines);
        }

        final response = await _client.saveLocation(
          request,
          headers: _buildHeaders(),
        );

        return response.id;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'SaveLocation',
    );
  }

  /// GetLocation retrieves a specific location by ID.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<Location> getLocation(String id) async {
    return RpcUtils.executeRpc(
      () async {
        final request = GetLocationRequest(id: id);
        final response = await _client.getLocation(
          request,
          headers: _buildHeaders(),
        );

        // Convert GetLocationResponse to Location
        return Location(
          id: response.id,
          latitudeDeg: response.latitudeDeg,
          longitudeDeg: response.longitudeDeg,
          regionCode: response.regionCode,
          postalCode: response.postalCode,
          locality: response.locality,
          addressLines: response.addressLines,
          name: response.name,
          neighborhood: response.neighborhood,
          county: response.county,
          administrativeArea: response.administrativeArea,
        );
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetLocation',
    );
  }

  /// GeocodeAddress converts an address to coordinates.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<GeocodeAddressResponse> geocodeAddress({
    required String regionCode,
    required String postalCode,
    required String locality,
    List<String>? addressLines,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = GeocodeAddressRequest(
          regionCode: regionCode,
          postalCode: postalCode,
          locality: locality,
        );

        if (addressLines != null && addressLines.isNotEmpty) {
          request.addressLines.addAll(addressLines);
        }

        final response = await _client.geocodeAddress(
          request,
          headers: _buildHeaders(),
        );

        return response;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GeocodeAddress',
    );
  }

  /// ReverseGeocode converts coordinates to an address.
  ///
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<ReverseGeocodeResponse> reverseGeocode({
    required double latitudeDeg,
    required double longitudeDeg,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = ReverseGeocodeRequest(
          latitudeDeg: latitudeDeg,
          longitudeDeg: longitudeDeg,
        );

        final response = await _client.reverseGeocode(
          request,
          headers: _buildHeaders(),
        );

        return response;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'ReverseGeocode',
    );
  }

  /// DeleteLocation deletes a location by ID.
  ///
  /// Returns true if the location was deleted successfully.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<bool> deleteLocation(String id) async {
    return RpcUtils.executeRpc(
      () async {
        final request = DeleteLocationRequest(id: id);
        await _client.deleteLocation(request, headers: _buildHeaders());

        return true; // Success is implied by no exception
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'DeleteLocation',
    );
  }

  /// SearchRegions searches for regions by name (typeahead search).
  ///
  /// INTERNAL: Called by LocationRepository, not ViewModels directly.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<List<RegionItem>> searchRegions({
    required String query,
    String? regionType,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = SearchRegionsRequest(
          query: query,
          regionType: regionType ?? '',
        );
        final response = await _client.searchRegions(
          request,
          headers: _buildHeaders(),
        );
        return response.regions;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'SearchRegions',
    );
  }

  /// GetRegion retrieves region details by ID.
  ///
  /// INTERNAL: Called by LocationRepository, not ViewModels directly.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<RegionItem> getRegion(String regionId) async {
    return RpcUtils.executeRpc(
      () async {
        final request = GetRegionRequest(regionId: regionId);
        final response = await _client.getRegion(
          request,
          headers: _buildHeaders(),
        );
        return response.region;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetRegion',
    );
  }

  /// CreateRegionFromAddress creates or finds a region based on address details.
  ///
  /// Used by the community governance flow to create regions for manual override.
  /// Returns the region ID, type, and name.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<CreateRegionFromAddressResponse> createRegionFromAddress({
    required double latitudeDeg,
    required double longitudeDeg,
    required String regionCode,
    required String postalCode,
    required String locality,
    String? neighborhood,
    String? county,
    String? administrativeArea,
    required String preferredRegionType,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = CreateRegionFromAddressRequest(
          latitudeDeg: latitudeDeg,
          longitudeDeg: longitudeDeg,
          regionCode: regionCode,
          postalCode: postalCode,
          locality: locality,
          neighborhood: neighborhood ?? '',
          county: county ?? '',
          administrativeArea: administrativeArea ?? '',
          preferredRegionType: preferredRegionType,
        );
        final response = await _client.createRegionFromAddress(
          request,
          headers: _buildHeaders(),
        );
        return response;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'CreateRegionFromAddress',
    );
  }

  /// GetUserLocations retrieves the authenticated user's locations sorted by last_used_at.
  ///
  /// Returns locations ordered by most recently used first.
  /// INTERNAL: Called by LocationRepository, not ViewModels directly.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<List<UserLocationWithDetails>> getUserLocations({int? limit}) async {
    return RpcUtils.executeRpc(
      () async {
        final request = GetUserLocationsRequest(limit: limit ?? 50);
        final response = await _client.getUserLocations(
          request,
          headers: _buildHeaders(),
        );
        return response.locations;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetUserLocations',
    );
  }

  /// AddUserLocation adds a location to the authenticated user's location list.
  ///
  /// Creates a new user-location association or updates the last_used_at if it already exists.
  /// INTERNAL: Called by LocationRepository, not ViewModels directly.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<void> addUserLocation(String locationId) async {
    return RpcUtils.executeRpc(
      () async {
        final request = AddUserLocationRequest(locationId: locationId);
        await _client.addUserLocation(request, headers: _buildHeaders());
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'AddUserLocation',
    );
  }

  /// RemoveUserLocation removes a location from the authenticated user's location list.
  ///
  /// Soft-deletes the user-location association.
  /// INTERNAL: Called by LocationRepository, not ViewModels directly.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<void> removeUserLocation(String locationId) async {
    return RpcUtils.executeRpc(
      () async {
        final request = RemoveUserLocationRequest(locationId: locationId);
        await _client.removeUserLocation(request, headers: _buildHeaders());
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'RemoveUserLocation',
    );
  }

  /// UpdateLocationLastUsed updates the last_used_at timestamp for a location.
  ///
  /// Call this when the user selects a location for gear/request/experience.
  /// INTERNAL: Called by LocationRepository, not ViewModels directly.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<void> updateLocationLastUsed(String locationId) async {
    return RpcUtils.executeRpc(
      () async {
        final request = UpdateLocationLastUsedRequest(locationId: locationId);
        await _client.updateLocationLastUsed(request, headers: _buildHeaders());
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'UpdateLocationLastUsed',
    );
  }
}
