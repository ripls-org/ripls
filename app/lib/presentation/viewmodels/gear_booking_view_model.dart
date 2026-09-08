import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/data/repositories/gear_repository.dart' show GearBooking;
import 'package:ripls/services/providers.dart';

/// Immutable state for the gear who's-using booking calendar. Plain (not
/// Freezed) to avoid a codegen step for this focused screen — the field set is
/// small and the copyWith is explicit.
class GearBookingState {
  final List<GearBooking> bookings;
  final bool isLoading;
  final bool isMutating;

  /// The last mutation/load failure, classified for localized display. Cleared
  /// to null on a fresh attempt.
  final UserError? error;

  const GearBookingState({
    this.bookings = const [],
    this.isLoading = false,
    this.isMutating = false,
    this.error,
  });

  GearBookingState copyWith({
    List<GearBooking>? bookings,
    bool? isLoading,
    bool? isMutating,
    UserError? error,
  }) {
    return GearBookingState(
      bookings: bookings ?? this.bookings,
      isLoading: isLoading ?? this.isLoading,
      isMutating: isMutating ?? this.isMutating,
      error: error,
    );
  }
}

/// Manages the booking schedule for one gear item's who's-using calendar:
/// loads bookings, claims/releases day ranges, and edits hand-offs. Parameterized
/// by gearId via the family modifier; the community context is supplied by
/// [initialize] (the gear view-model owns community resolution).
class GearBookingNotifier extends Notifier<GearBookingState> {
  GearBookingNotifier(this.gearId);

  final String gearId;
  String? _communityId;

  @override
  GearBookingState build() => const GearBookingState();

  Future<void> initialize({required String communityId}) async {
    _communityId = communityId;
    await load();
  }

  /// After a booking mutation (which creates/cancels a loan transfer), refresh
  /// the gear so the content view's "who's using it" preview — which reads the
  /// gear's `activeLoan` + (uncached) transfer context, not the booking list —
  /// reflects the change. Mirrors `GearTransferHandlersMixin.refreshAfterTransferAction`.
  Future<void> _refreshGearAfterBookingChange() async {
    final communityId = _communityId;
    final gearRepo = ref.read(gearRepositoryProvider);
    await gearRepo.invalidate(gearId, communityId: communityId);
    await gearRepo.invalidateStats(gearId, communityId: communityId);
    if (!ref.mounted) return;
    // The gear content view listens to this and reloads its gear details +
    // fresh transfer context.
    ref.read(transferCacheInvalidationProvider.notifier).notify();
  }

  Future<void> load() async {
    final communityId = _communityId;
    if (communityId == null) return;
    state = state.copyWith(isLoading: true, error: null);
    try {
      final repo = ref.read(gearRepositoryProvider);
      final bookings =
          await repo.listBookings(gearId: gearId, communityId: communityId);
      if (!ref.mounted) return;
      state = state.copyWith(bookings: bookings, isLoading: false);
    } catch (e) {
      if (!ref.mounted) return;
      state = state.copyWith(isLoading: false, error: RpcErrorHandler.classify(e));
    }
  }

  /// Claims an inclusive day range. By default the caller becomes the recipient;
  /// the owner may reserve for [recipientId] (or themselves to block), or hold
  /// the days behind an accept link with [pending] (the returned booking carries
  /// the accept token). Returns the booking on success, null on failure (with
  /// [error] set).
  Future<GearBooking?> claim({
    required int startDateUnixSec,
    required int endDateUnixSec,
    String? recipientId,
    bool pending = false,
  }) async {
    final communityId = _communityId;
    if (communityId == null) return null;
    state = state.copyWith(isMutating: true, error: null);
    try {
      final repo = ref.read(gearRepositoryProvider);
      final booking = await repo.claimDays(
        gearId: gearId,
        communityId: communityId,
        startDateUnixSec: startDateUnixSec,
        endDateUnixSec: endDateUnixSec,
        recipientId: recipientId,
        pending: pending,
      );
      if (!ref.mounted) return booking;
      state = state.copyWith(isMutating: false);
      await _refreshGearAfterBookingChange();
      await load();
      return booking;
    } catch (e) {
      if (!ref.mounted) return null;
      state = state.copyWith(isMutating: false, error: RpcErrorHandler.classify(e));
      return null;
    }
  }

  /// Accepts a pending owner-created reservation via its link token.
  Future<bool> accept({
    required String bookingId,
    required String acceptToken,
  }) async {
    state = state.copyWith(isMutating: true, error: null);
    try {
      final repo = ref.read(gearRepositoryProvider);
      await repo.acceptBooking(bookingId: bookingId, acceptToken: acceptToken);
      if (!ref.mounted) return true;
      state = state.copyWith(isMutating: false);
      await _refreshGearAfterBookingChange();
      await load();
      return true;
    } catch (e) {
      if (!ref.mounted) return false;
      state = state.copyWith(isMutating: false, error: RpcErrorHandler.classify(e));
      return false;
    }
  }

  /// Releases (drops) a booking the user owns.
  Future<bool> release(String bookingId) async {
    state = state.copyWith(isMutating: true, error: null);
    try {
      final repo = ref.read(gearRepositoryProvider);
      await repo.releaseBooking(bookingId);
      if (!ref.mounted) return true;
      state = state.copyWith(isMutating: false);
      await _refreshGearAfterBookingChange();
      await load();
      return true;
    } catch (e) {
      if (!ref.mounted) return false;
      state = state.copyWith(isMutating: false, error: RpcErrorHandler.classify(e));
      return false;
    }
  }

  /// Sets/clears the pickup + drop-off hand-off on a booking.
  Future<bool> updateHandoff({
    required String bookingId,
    String? pickupLocationId,
    int? pickupTimeUnixSec,
    String? dropoffLocationId,
    int? dropoffTimeUnixSec,
  }) async {
    state = state.copyWith(isMutating: true, error: null);
    try {
      final repo = ref.read(gearRepositoryProvider);
      await repo.updateBookingHandoff(
        bookingId: bookingId,
        pickupLocationId: pickupLocationId,
        pickupTimeUnixSec: pickupTimeUnixSec,
        dropoffLocationId: dropoffLocationId,
        dropoffTimeUnixSec: dropoffTimeUnixSec,
      );
      if (!ref.mounted) return true;
      state = state.copyWith(isMutating: false);
      await _refreshGearAfterBookingChange();
      await load();
      return true;
    } catch (e) {
      if (!ref.mounted) return false;
      state = state.copyWith(isMutating: false, error: RpcErrorHandler.classify(e));
      return false;
    }
  }
}

/// Provider for a gear item's booking schedule, keyed by gearId.
final gearBookingProvider = NotifierProvider.autoDispose
    .family<GearBookingNotifier, GearBookingState, String>(
  GearBookingNotifier.new,
);
