import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';

/// The four place destinations of the converged bottom dock (#2634), in
/// their visual order. Create and the search orb are actions, not
/// destinations, so they are not members.
///
/// Each destination maps to a fixed index in the home shell's
/// IndexedStack. The stack also holds entries no destination points to
/// (0 = the orphaned Feed, plus flag-off legacy tabs); those indices are
/// kept stable so pre-dock callers of `navigateToTab` keep working.
enum RiplsNavDestination {
  home(stackIndex: 1, icon: Icons.home_outlined, activeIcon: Icons.home),
  plans(
    stackIndex: 4,
    icon: Icons.calendar_month_outlined,
    activeIcon: Icons.calendar_month,
  ),
  library(
    stackIndex: 2,
    icon: Icons.inventory_2_outlined,
    activeIcon: Icons.inventory_2,
  ),
  people(stackIndex: 3, icon: Icons.people_outline, activeIcon: Icons.people);

  const RiplsNavDestination({
    required this.stackIndex,
    required this.icon,
    required this.activeIcon,
  });

  /// Index of this destination's screen in the home shell IndexedStack.
  final int stackIndex;

  final IconData icon;
  final IconData activeIcon;

  /// Visible tab label; doubles as the semantics label.
  String label(BuildContext context) => switch (this) {
        home => context.l10n.a11yMiscNavHome,
        plans => context.l10n.navDockPlans,
        library => context.l10n.navDockLibrary,
        people => context.l10n.navDockPeople,
      };

  /// The destination whose screen lives at [stackIndex], or null when the
  /// index belongs to an orphaned stack entry (e.g. the Feed at 0).
  static RiplsNavDestination? fromStackIndex(int stackIndex) {
    for (final destination in values) {
      if (destination.stackIndex == stackIndex) return destination;
    }
    return null;
  }
}
