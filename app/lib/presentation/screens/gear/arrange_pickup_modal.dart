import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:intl/intl.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/viewmodels/loan_borrower_state.dart';
import 'package:ripls/presentation/viewmodels/loan_borrower_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass.dart';
import 'package:ripls/presentation/widgets/transfer/duration_chips.dart';
import 'package:ripls/presentation/widgets/transfer/quick_date_chips.dart';
import 'package:ripls/presentation/widgets/transfer/quick_time_chips.dart';
import 'package:ripls/presentation/widgets/transfer/transfer_modal_shell.dart';
import 'package:ripls/presentation/widgets/transfer/transfer_phase_bottom_bar.dart';
import 'package:ripls/services/providers.dart';

/// ArrangePickupModal lets either the owner or borrower propose a pickup date,
/// time, and duration for a loan.
///
/// When a pickup time is already set the modal displays a read-only summary.
/// Either party can tap "Edit" to enter edit mode and change the details.
/// The read-only state persists until the time is edited or the item is marked
/// as picked up (handled by the parent loan modal).
class ArrangePickupModal extends ConsumerStatefulWidget {
  final String transferId;
  final String gearName;

  /// When true the modal shows owner-appropriate labels and descriptions.
  final bool isOwner;

  /// When true the modal skips the duration selector (giveaways don't need it).
  final bool isGiveaway;

  const ArrangePickupModal({
    super.key,
    required this.transferId,
    required this.gearName,
    this.isOwner = false,
    this.isGiveaway = false,
  });

  /// show opens the ArrangePickupModal as a bottom sheet.
  static Future<void> show(
    BuildContext context, {
    required String transferId,
    required String gearName,
    bool isOwner = false,
    bool isGiveaway = false,
  }) async {
    await showAccessibleModal<void>(context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (_) => ArrangePickupModal(
        transferId: transferId,
        gearName: gearName,
        isOwner: isOwner,
        isGiveaway: isGiveaway,
      ),
    );
  }

  @override
  ConsumerState<ArrangePickupModal> createState() => _ArrangePickupModalState();
}

class _ArrangePickupModalState extends ConsumerState<ArrangePickupModal> {
  /// Whether the user is actively editing an already-set pickup time.
  bool _isEditing = false;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      ref.read(loanBorrowerProvider.notifier).initialize(
            transferId: widget.transferId,
          );
    });
  }

  /// _enterEditMode enables editing of an already-set pickup time.
  void _enterEditMode() {
    setState(() => _isEditing = true);
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(loanBorrowerProvider);

    // Show read-only summary only when the TRANSFER has a saved pickup time
    // (not just local selections). This prevents flipping to read-only before
    // the user has confirmed and saved.
    final transferHasPickup = state.transfer?.hasEstimatedPickupUnixSec() ?? false;
    final showReadOnly = transferHasPickup && !_isEditing && !state.isLoading;

    if (showReadOnly) {
      // Content-sized glass-sheet layout for read-only mode. The shared
      // TransferModalShell is used in edit mode below; here the content
      // is short so we wrap directly in a GlassSheet to avoid forcing the
      // shell's Expanded layout.
      return GlassSheet(
        applyMaxHeight: false,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            _buildHeader(context),
            _buildReadOnlyContent(context, state),
            _buildBottomBar(context, state, showReadOnly),
          ],
        ),
      );
    }

    return SizedBox(
      height: MediaQuery.of(context).size.height * 0.85,
      child: TransferModalShell(
        header: _buildHeader(context),
        content: SingleChildScrollView(
          child: _buildEditContent(context, state),
        ),
        bottomBar: _buildBottomBar(context, state, showReadOnly),
        isLoading: state.isLoading,
      ),
    );
  }

  Widget _buildHeader(BuildContext context) {
    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        const SizedBox(height: 4),
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 20),
          child: Text(
            'Arrange Pickup',
            style: TextStyle(
              fontSize: 18,
              fontWeight: FontWeight.w700,
              color: AppColors.modalTextPrimary,
            ),
            textAlign: TextAlign.center,
          ),
        ),
        const SizedBox(height: 16),
        Container(height: 1, color: AppColors.modalFooterDivider),
      ],
    );
  }

  // ── Read-only view ──────────────────────────────────────────────────────────

  Widget _buildReadOnlyContent(BuildContext context, LoanBorrowerState state) {
    final date = state.selectedPickupDate!;
    final time = state.selectedPickupTime!;

    final dateStr = DateFormat('EEEE, MMMM d').format(date);
    final timeStr = time.format(context);

    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 20, 16, 16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            'Pickup scheduled',
            style: TextStyle(
              fontSize: 13,
              fontWeight: FontWeight.w600,
              color: AppColors.modalTextSecondary,
              letterSpacing: 0.4,
            ),
          ),
          const SizedBox(height: 10),
          GlassInsetCard(
            padding: EdgeInsets.zero,
            child: Column(
              children: [
                _buildDetailRow(
                  context,
                  icon: Icons.calendar_today_outlined,
                  label: 'Date',
                  value: dateStr,
                  isFirst: true,
                ),
                _buildDetailRow(
                  context,
                  icon: Icons.access_time_outlined,
                  label: 'Time',
                  value: timeStr,
                  isLast: widget.isGiveaway,
                ),
                if (!widget.isGiveaway && state.selectedDurationDays != null)
                  _buildDetailRow(
                    context,
                    icon: Icons.timer_outlined,
                    label: 'Duration',
                    value: _formatDuration(state.selectedDurationDays!),
                    isLast: true,
                  ),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildDetailRow(
    BuildContext context, {
    required IconData icon,
    required String label,
    required String value,
    bool isFirst = false,
    bool isLast = false,
  }) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
      decoration: BoxDecoration(
        border: isLast
            ? null
            : Border(
                bottom: BorderSide(color: AppColors.modalInsetCardBorder),
              ),
      ),
      child: Row(
        children: [
          Icon(icon, size: 16, color: AppColors.modalTextMuted),
          const SizedBox(width: 12),
          Text(
            label,
            style: TextStyle(
              fontSize: 14,
              color: AppColors.modalTextSecondary,
            ),
          ),
          const Spacer(),
          Text(
            value,
            style: TextStyle(
              fontSize: 14,
              fontWeight: FontWeight.w600,
              color: AppColors.modalTextPrimary,
            ),
          ),
        ],
      ),
    );
  }

  // ── Edit view ───────────────────────────────────────────────────────────────

  Widget _buildEditContent(BuildContext context, LoanBorrowerState state) {
    final notifier = ref.read(loanBorrowerProvider.notifier);

    return Padding(
      padding: const EdgeInsets.only(bottom: 100),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          QuickDateChips(
            label: 'Pickup Date',
            selectedDate: state.selectedPickupDate,
            onDateSelected: notifier.setPickupDate,
          ),
          const SizedBox(height: 24),
          QuickTimeChips(
            label: 'Pickup Time',
            selectedTime: state.selectedPickupTime,
            onTimeSelected: notifier.setPickupTime,
          ),
          if (!widget.isGiveaway) ...[
            const SizedBox(height: 24),
            DurationChips(
              label: widget.isOwner
                  ? context.l10n.loanDurationQuestionOwner
                  : context.l10n.loanDurationQuestionBorrower,
              selectedDurationDays: state.selectedDurationDays,
              onDurationSelected: notifier.setDuration,
            ),
          ],
          if (state.transfer != null &&
              state.selectedPickupDate != null &&
              state.selectedPickupTime != null &&
              (widget.isGiveaway || state.selectedDurationDays != null)) ...[
            const SizedBox(height: 24),
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 16),
              child: Text(
                _notificationDescription(context, state),
                style: TextStyle(
                  fontSize: 13,
                  color: AppColors.modalTextSecondary,
                ),
              ),
            ),
          ],
        ],
      ),
    );
  }

  // ── Bottom bar ──────────────────────────────────────────────────────────────

  Widget _buildBottomBar(
    BuildContext context,
    LoanBorrowerState state,
    bool showReadOnly,
  ) {
    if (showReadOnly) {
      return TransferPhaseBottomBar(
        primaryLabel: 'Edit Pickup Details',
        onPrimaryPressed: _enterEditMode,
        isLoading: false,
      );
    }

    final hasDetails = widget.isGiveaway
        ? state.selectedPickupDate != null && state.selectedPickupTime != null
        : state.hasCompletePickupDetails;
    final isUpdating = _isEditing;

    return TransferPhaseBottomBar(
      primaryLabel:
          isUpdating ? 'Update Pickup Details' : 'Confirm Pickup Details',
      onPrimaryPressed: hasDetails
          ? () async {
              await ref
                  .read(loanBorrowerProvider.notifier)
                  .submitPickupDetails();
              if (!context.mounted) return;
              // Notify gear content view to refresh managing menu.
              ref.read(transferCacheInvalidationProvider.notifier).notify();
              // Stay on modal showing read-only summary — user taps away to dismiss.
              setState(() => _isEditing = false);
            }
          : null,
      isLoading: state.isLoading,
    );
  }

  // ── Helpers ─────────────────────────────────────────────────────────────────

  String _formatDuration(int days) {
    if (days == 7) return '1 week';
    if (days == 14) return '2 weeks';
    return '$days day${days > 1 ? 's' : ''}';
  }

  String _notificationDescription(
    BuildContext context,
    LoanBorrowerState state,
  ) {
    final transfer = state.transfer;
    if (transfer == null ||
        state.selectedPickupDate == null ||
        state.selectedPickupTime == null) {
      return '';
    }
    if (!widget.isGiveaway && state.selectedDurationDays == null) {
      return '';
    }

    final dateStr =
        DateFormat('EEE, MMM d').format(state.selectedPickupDate!);
    final timeStr = state.selectedPickupTime!.format(context);
    final otherParty = widget.isOwner
        ? transfer.recipient.name
        : transfer.owner.name;

    if (widget.isGiveaway) {
      return '$otherParty will be notified: '
          '"Pickup is set for $dateStr at $timeStr"';
    }

    final durationStr = _formatDuration(state.selectedDurationDays!);
    if (widget.isOwner) {
      return '$otherParty will be notified: '
          '"Pickup is set for $dateStr at $timeStr, for $durationStr"';
    }
    return '$otherParty will be notified: '
        '"You want to pick up $dateStr at $timeStr and borrow for $durationStr"';
  }
}
