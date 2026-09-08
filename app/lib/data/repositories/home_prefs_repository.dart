import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:shared_preferences/shared_preferences.dart';

/// HomePrefsRepository persists per-user, device-local Home tab preferences
/// (#2435): the Up-next list/week toggle and the Recent-activity last-opened
/// timestamp that drives the "N new" pill.
///
/// These are presentation preferences, not server state — they follow the
/// async-first SharedPreferences pattern used by TutorialStateRepository.
class HomePrefsRepository {
  SharedPreferences? _prefsInstance;

  Future<SharedPreferences> get _prefs async {
    _prefsInstance ??= await SharedPreferences.getInstance();
    return _prefsInstance!;
  }

  static const String _keyActivityOpenedPrefix = 'home_activity_opened_';

  
  /// Returns when [userId] last opened the Recent-activity screen (Unix
  /// seconds), or 0 when never opened. Activity entries newer than this
  /// count toward the "N new" pill.
  Future<int> getActivityLastOpened(String userId) async {
    final prefs = await _prefs;
    return prefs.getInt('$_keyActivityOpenedPrefix$userId') ?? 0;
  }
  }

/// Provider for the Home tab preferences repository.
final homePrefsRepositoryProvider = Provider<HomePrefsRepository>(
  (ref) => HomePrefsRepository(),
);
