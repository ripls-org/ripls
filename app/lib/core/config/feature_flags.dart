import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/config/environment.dart';

/// unifiedCreateEnabledProvider exposes the unified-create feature flag via
/// Riverpod so widget tests can override it with ProviderScope.overrides.
///
/// Reads [Environment.unifiedCreateEnabled] (a compile-time constant) by
/// default. Production code that already reads the static getter directly does
/// not need to migrate; this provider exists so secondary creation surfaces and
/// their tests share a single override point.
final Provider<bool> unifiedCreateEnabledProvider = Provider<bool>(
  (ref) => Environment.unifiedCreateEnabled,
);

/// navDockEnabledProvider gates the #2634 converged bottom dock: the
/// five-target glass capsule (Home · Plans · Create · Library · People)
/// plus the detached universal-search orb. Defaults on (cutover); when
/// false the shell renders the legacy three-tab pill + floating Create
/// FAB (#1895). Overridable via ProviderScope.overrides for tests and
/// rollback.
final Provider<bool> navDockEnabledProvider = Provider<bool>(
  (ref) => Environment.navDockEnabled,
);

/// directoryEnabledProvider gates the #2568 Directory (address book) tab
/// that replaces the Workshop tab in the bottom navigation. Defaults **on**
/// at the Phase-3 cutover (see docs/issues/2568-directory-and-profiles.md):
/// the Directory is the live tab, the Workshop tab is retired from the nav,
/// and the momentum engine it relied on is retained and surfaces via the
/// Feed + calendar. Overridable (e.g. for rollback) via
/// ProviderScope.overrides.
final Provider<bool> directoryEnabledProvider = Provider<bool>(
  (ref) => true,
);
