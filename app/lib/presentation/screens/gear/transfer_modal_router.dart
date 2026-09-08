import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/data/gen/ripls/api/gear.pb.dart' show Availability;
import 'package:ripls/data/gen/ripls/api/gear_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart';
import 'package:ripls/presentation/screens/gear/giveaway_giver_modal.dart';
import 'package:ripls/presentation/screens/gear/giveaway_receiver_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/services/providers.dart';

final _log = Logger('TransferModalRouter');

/// TransferModalRouter routes to the appropriate transfer modal based on
/// transfer type and user role.
///
/// This provides backward compatibility with the old TransferManagementModal
/// while using the new phase-based modals.
class TransferModalRouter {
  /// Show the appropriate transfer modal based on gear, user role, and transfer type.
  ///
  /// Returns true if transfer was initiated/updated, false/null otherwise.
  ///
  /// Routes to:
  /// - LoanOwnerModal or LoanListModal for loan owners
  /// - LoanBorrowerModal for loan borrowers
  /// - GiveawayGiverModal for giveaway givers
  /// - GiveawayReceiverModal for giveaway receivers
  static Future<bool?> show(
    BuildContext context, {
    required GetGearResponse gear,
    required bool isOwner,
    required String communityId,
    Availability? availability,
  }) async {
    _log.info(
      'Routing transfer modal - gearId: ${gear.id}, isOwner: $isOwner, availability: $availability',
    );

    // Need to get ProviderContainer to access repositories
    final container = ProviderScope.containerOf(context);
    final repository = container.read(transferRepositoryProvider);

    try {
      // Determine transfer type from availability
      final isLoan = availability == Availability.AVAILABILITY_FOR_LOAN;
      final isGiveaway = availability == Availability.AVAILABILITY_FOR_GIVEAWAY;

      if (!isLoan && !isGiveaway) {
        _log.warning('Unknown availability type: $availability');
        return null;
      }

      // Get transfers for this gear
      final transferType = isLoan
          ? TransferType.TRANSFER_TYPE_LOAN
          : TransferType.TRANSFER_TYPE_GIVEAWAY;

      List<Transfer> transfers;
      if (isOwner) {
        transfers = await repository.listMyTransfers(
          transferType: transferType,
        );
      } else {
        transfers = await repository.listReceivedTransfers(
          transferType: transferType,
        );
      }

      // Filter to transfers for this gear
      // For giveaways: Include COMPLETED to show completion screen
      // For loans: Exclude COMPLETED to show only active loans
      // Always exclude CANCELLED
      final gearTransfers = transfers
          .where((t) => t.gearId == gear.id)
          .where((t) {
            if (t.state == TransferState.TRANSFER_STATE_CANCELLED) {
              return false; // Never show cancelled
            }
            if (t.state == TransferState.TRANSFER_STATE_COMPLETED) {
              // Show completed for giveaways, hide for loans
              return isGiveaway;
            }
            return true; // Show all other states
          })
          .toList();

      // Sort by state priority first, then by latest request time
      // Priority: ACTIVE > RECIPIENT_SELECTED > INTEREST_EXPRESSED
      gearTransfers.sort((a, b) {
        final aStatePriority = _getStatePriority(a.state);
        final bStatePriority = _getStatePriority(b.state);

        if (aStatePriority != bStatePriority) {
          return bStatePriority.compareTo(aStatePriority); // Higher priority first
        }

        // Same state priority - sort by most recent
        return b.latestRequestUnixSec.compareTo(a.latestRequestUnixSec);
      });

      _log.info('Found ${gearTransfers.length} active transfers for gear ${gear.id}');

      // Check context is still mounted after async operations
      if (!context.mounted) {
        _log.warning('Context no longer mounted after loading transfers');
        return null;
      }

      // Route based on transfer type and role
      if (isLoan) {
        return await _showLoanModal(
          context,
          gearId: gear.id,
          gearName: gear.name,
          isOwner: isOwner,
          transfers: gearTransfers,
        );
      } else {
        return await _showGiveawayModal(
          context,
          gearId: gear.id,
          gearName: gear.name,
          isOwner: isOwner,
          transfers: gearTransfers,
        );
      }
    } catch (e, stackTrace) {
      _log.severe('Failed to route transfer modal', e, stackTrace);
      return null;
    }
  }

  static Future<bool?> _showLoanModal(
    BuildContext context, {
    required String gearId,
    required String gearName,
    required bool isOwner,
    required List<Transfer> transfers,
  }) async {
    if (isOwner) {
      // Owner: no modal — all actions are on the managing menu.
      _log.info('Loan owner: no modal, actions on managing menu');
      return null;
    } else {
      // Borrower: Express interest if no transfer exists, return true.
      // No modal — pill and managing menu handle all state display/actions.
      if (transfers.isEmpty) {
        _log.info('No existing transfer — expressing interest for loan gear: $gearId');
        final container = ProviderScope.containerOf(context);
        final repository = container.read(transferRepositoryProvider);
        await repository.expressInterest(gearId: gearId);
        return true;
      }
      // Already has a transfer — no modal needed.
      return null;
    }
  }

  static Future<bool?> _showGiveawayModal(
    BuildContext context, {
    required String gearId,
    required String gearName,
    required bool isOwner,
    required List<Transfer> transfers,
  }) async {
    if (isOwner) {
      // Giver: Show giveaway giver modal
      _log.info('Showing giveaway giver modal');
      // Only pass transferId if there's an active transfer (not just interest expressions)
      final transferId = transfers.isNotEmpty &&
              transfers.first.state != TransferState.TRANSFER_STATE_INTEREST_EXPRESSED
          ? transfers.first.id
          : null;
      return showAccessibleModal<bool>(context,
        isScrollControlled: true,
        backgroundColor: Colors.transparent,
        barrierColor: AppColors.modalBackdrop,
        builder: (context) => GiveawayGiverModal(
          gearId: gearId,
          gearName: gearName,
          transferId: transferId,
        ),
      );
    } else {
      // Receiver: Express interest if no transfer exists.
      if (transfers.isEmpty) {
        _log.info('No existing transfer — expressing interest for gear: $gearId');
        final container = ProviderScope.containerOf(context);
        final repository = container.read(transferRepositoryProvider);
        await repository.expressInterest(gearId: gearId);
        // Don't show modal — just return true so the detail page refreshes
        // and the action pill updates to "You've expressed interest".
        return true;
      }

      // Already has a transfer — show the receiver modal with interest list.
      final transferId = transfers.first.id;
      _log.info('Showing giveaway receiver modal');
      return showAccessibleModal<bool>(context,
        isScrollControlled: true,
        backgroundColor: Colors.transparent,
        barrierColor: AppColors.modalBackdrop,
        builder: (context) => GiveawayReceiverModal(
          transferId: transferId,
          gearId: gearId,
          gearName: gearName,
        ),
      );
    }
  }

  /// Returns priority for transfer state sorting.
  /// Higher values have higher priority.
  static int _getStatePriority(TransferState state) {
    switch (state) {
      case TransferState.TRANSFER_STATE_ACTIVE:
        return 3; // Highest priority (item currently in use)
      case TransferState.TRANSFER_STATE_RECIPIENT_SELECTED:
        return 2; // Selected recipient (pickup arranged or in progress)
      case TransferState.TRANSFER_STATE_INTEREST_EXPRESSED:
        return 1; // Just interested (not selected yet)
      default:
        return 0; // Other states (shouldn't happen after filtering)
    }
  }
}
