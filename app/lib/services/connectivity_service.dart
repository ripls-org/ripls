import 'dart:async';
import 'package:connectivity_plus/connectivity_plus.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

/// ConnectivityService monitors network connectivity status.
/// Uses connectivity_plus to detect network state changes.
class ConnectivityService {
  final Connectivity _connectivity;
  StreamSubscription<List<ConnectivityResult>>? _subscription;
  final _controller = StreamController<bool>.broadcast();

  ConnectivityService([Connectivity? connectivity])
      : _connectivity = connectivity ?? Connectivity();

  /// Stream of connectivity status changes.
  /// Emits true when online, false when offline.
  Stream<bool> get onConnectivityChanged => _controller.stream;

  /// Checks current connectivity status.
  /// Returns true if any network connection is available.
  Future<bool> checkConnectivity() async {
    final results = await _connectivity.checkConnectivity();
    return _isOnline(results);
  }

  /// Starts monitoring connectivity changes.
  void startMonitoring() {
    _subscription = _connectivity.onConnectivityChanged.listen((results) {
      final isOnline = _isOnline(results);
      _controller.add(isOnline);
    });
  }

  /// Stops monitoring connectivity changes.
  void stopMonitoring() {
    _subscription?.cancel();
    _subscription = null;
  }

  /// Disposes resources.
  void dispose() {
    stopMonitoring();
    _controller.close();
  }

  bool _isOnline(List<ConnectivityResult> results) {
    // Online if any connection type except none
    return results.any((result) => result != ConnectivityResult.none);
  }
}

/// Provider for ConnectivityService singleton.
final connectivityServiceProvider = Provider<ConnectivityService>((ref) {
  final service = ConnectivityService();
  ref.onDispose(() => service.dispose());
  return service;
});

/// StreamProvider for current connectivity status.
final connectivityStatusProvider = StreamProvider<bool>((ref) async* {
  final service = ref.watch(connectivityServiceProvider);

  // Emit initial connectivity status
  final initialStatus = await service.checkConnectivity();
  yield initialStatus;

  // Start monitoring and emit changes
  service.startMonitoring();
  yield* service.onConnectivityChanged;
});
