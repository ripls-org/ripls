// Coordination pattern: [GearSharingNotifier] is the single Riverpod provider.
// It composes behaviour from three mixins:
//   GearSharingLoadMixin             — initialize, all transfer/conversation loads
//   GearSharingTransferActionsMixin  — transfer lifecycle (interest, approve, pickup, return)
//   GearSharingAudienceMixin         — community sharing management
//
// All mixins share [state] (the coordinator's own [Notifier.state]) and write
// via [state.copyWith]. There is exactly one [gearSharingProvider] so call
// sites are unchanged.
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/presentation/viewmodels/gear_sharing_audience_actions.dart';
import 'package:ripls/presentation/viewmodels/gear_sharing_load_actions.dart';
// Mixin imports — one per concern.
import 'package:ripls/presentation/viewmodels/gear_sharing_state.dart';
import 'package:ripls/presentation/viewmodels/gear_sharing_transfer_actions.dart';

// Re-export GearSharingState so existing importers of this file work unchanged.
export 'package:ripls/presentation/viewmodels/gear_sharing_state.dart'
    show GearSharingState;

/// GearSharingNotifier manages state for the gear sharing UI.
///
/// A single global instance is used per gear detail screen via
/// [gearSharingProvider]. Callers initialize via [initialize] before using
/// action methods.
class GearSharingNotifier extends Notifier<GearSharingState>
    with
        GearSharingLoadMixin,
        GearSharingTransferActionsMixin,
        GearSharingAudienceMixin {
  @override
  GearSharingState build() {
    return const GearSharingState();
  }
}

/// Provider for gear sharing state
final gearSharingProvider =
    NotifierProvider<GearSharingNotifier, GearSharingState>(
      GearSharingNotifier.new,
    );
