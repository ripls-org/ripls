import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:intl/intl.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart';
import 'package:ripls/data/gen/ripls/api/provisional_user.pb.dart';
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart' show TransferType;
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/accessibility/toggle.dart';
import 'package:ripls/presentation/widgets/completion/completion_widgets.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass_pickers.dart';
import 'package:ripls/presentation/widgets/provisional_user_avatar.dart';
import 'package:ripls/presentation/widgets/user_avatar.dart';
import 'package:ripls/services/post_creation_service.dart';
import 'package:ripls/services/providers.dart';

/// PastTransferModal is a bottom sheet for logging a past giveaway or loan.
///
/// The owner selects a recipient (registered user or provisional user), a completion
/// date in the past, and — for loans — an optional return date. Submitting calls
/// CreateTransfer and bypasses the normal interest/selection state machine.
class PastTransferModal extends ConsumerStatefulWidget {
  const PastTransferModal({
    super.key,
    required this.gearId,
    required this.gearName,
    required this.communityId,
    required this.transferType,
    required this.ownerId,
    this.sharedCommunityIds = const {},
  });

  final String gearId;
  final String gearName;
  final String communityId;
  final TransferType transferType;

  /// The gear owner's user ID — excluded from recipient search and grid.
  final String ownerId;

  /// All community IDs the gear is shared with; passed to [DarkPersonSearch]
  /// so the quick-add grid and search fan out across every shared community.
  final Set<String> sharedCommunityIds;

  /// Shows the PastTransferModal as a bottom sheet.
  static Future<bool?> show(
    BuildContext context, {
    required String gearId,
    required String gearName,
    required String communityId,
    required TransferType transferType,
    required String ownerId,
    Set<String> sharedCommunityIds = const {},
  }) {
    return showAccessibleModal<bool>(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      builder: (_) => FractionallySizedBox(
        heightFactor: 0.92,
        child: PastTransferModal(
          gearId: gearId,
          gearName: gearName,
          communityId: communityId,
          transferType: transferType,
          ownerId: ownerId,
          sharedCommunityIds: sharedCommunityIds,
        ),
      ),
    );
  }

  @override
  ConsumerState<PastTransferModal> createState() => _PastTransferModalState();
}

class _PastTransferModalState extends ConsumerState<PastTransferModal> {
  // Selected recipient — exactly one of these is set at a time.
  User? _selectedUser;
  ProvisionalUser? _selectedProvisional;

  // Date state.
  DateTime? _completedDate;
  bool _alreadyReturned = false;
  DateTime? _returnedDate;

  bool _isSubmitting = false;
  String? _errorMessage;
  ImpactEstimate? _impactEstimate;

  bool get _isLoan => widget.transferType == TransferType.TRANSFER_TYPE_LOAN;
  bool get _hasRecipient =>
      _selectedUser != null || _selectedProvisional != null;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) => _loadImpact());
  }

  Future<void> _loadImpact() async {
    if (!mounted) return;
    try {
      final stats = await ref
          .read(gearRepositoryProvider)
          .getStats(widget.gearId, communityId: widget.communityId);
      if (!mounted) return;
      final impact = stats.hasPotentialImpact() ? stats.potentialImpact : null;
      setState(() => _impactEstimate = impact);
    } catch (_) {
      // Non-fatal — impact bar simply won't show.
    }
  }

  bool get _canSubmit =>
      _hasRecipient &&
      _completedDate != null &&
      (!_isLoan || !_alreadyReturned || _returnedDate != null) &&
      !_isSubmitting;

  Future<void> _showPastDatePicker({
    required DateTime? initial,
    required ValueChanged<DateTime> onPicked,
  }) async {
    final now = DateTime.now();
    final today = DateTime(now.year, now.month, now.day);
    final picked = await showGlassDatePicker(
      context: context,
      initialDate: initial ?? today,
      firstDate: today.subtract(const Duration(days: 365 * 5)),
      lastDate: today,
    );
    if (picked != null) onPicked(picked);
  }

  Future<void> _submit() async {
    if (!_canSubmit) return;

    if (_isLoan && _alreadyReturned && _returnedDate != null) {
      if (_returnedDate!.isBefore(_completedDate!)) {
        setState(
          () => _errorMessage = 'Return date must be after the loan date',
        );
        return;
      }
    }

    setState(() {
      _isSubmitting = true;
      _errorMessage = null;
    });

    try {
      final repo = ref.read(transferRepositoryProvider);
      final completedAtSec = _completedDate!.millisecondsSinceEpoch ~/ 1000;
      final returnedAtSec =
          (_isLoan && _alreadyReturned && _returnedDate != null)
          ? _returnedDate!.millisecondsSinceEpoch ~/ 1000
          : null;

      await repo.createTransfer(
        gearId: widget.gearId,
        communityId: widget.communityId,
        transferType: widget.transferType,
        completedAtUnixSec: completedAtSec,
        recipientUserId: _selectedUser?.id,
        provisionalUserId: _selectedProvisional?.id,
        returnedAtUnixSec: returnedAtSec,
      );

      if (!mounted) return;
      Navigator.pop(context, true);
      await ref.read(postCreationServiceProvider).handlePostCreation();
    } catch (_) {
      if (mounted) {
        setState(() {
          _isSubmitting = false;
          _errorMessage = 'Something went wrong. Please try again.';
        });
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: BoxDecoration(
        color: CompletionColors.background(context),
        borderRadius: const BorderRadius.vertical(top: Radius.circular(20)),
      ),
      clipBehavior: Clip.hardEdge,
      child: Stack(
        children: [
          // Decorative glow
          Positioned(
            top: -40,
            right: -40,
            child: Container(
              width: 180,
              height: 180,
              decoration: BoxDecoration(
                shape: BoxShape.circle,
                gradient: RadialGradient(
                  colors: [
                    CompletionColors.glowColor(context),
                    Colors.transparent,
                  ],
                ),
              ),
            ),
          ),
          Column(
            children: [
              _buildHandle(),
              Expanded(
                child: ListView(
                  padding: const EdgeInsets.only(bottom: 8),
                  children: [
                    _buildHeader(),
                    _buildImpactBar(),
                    if (_errorMessage != null) _buildErrorBanner(),
                    _buildRecipientSection(),
                    _buildDateSection(),
                    if (_isLoan) _buildReturnSection(),
                  ],
                ),
              ),
              _buildSubmitButton(),
            ],
          ),
        ],
      ),
    );
  }

  Widget _buildHandle() {
    return Padding(
      padding: const EdgeInsets.only(top: 12, bottom: 4),
      child: Center(
        child: Container(
          width: 36,
          height: 4,
          decoration: BoxDecoration(
            color: CompletionColors.handle(context),
            borderRadius: BorderRadius.circular(2),
          ),
        ),
      ),
    );
  }

  Widget _buildHeader() {
    final eyebrow = _isLoan ? 'PAST LOAN' : 'PAST GIVEAWAY';
    return Padding(
      padding: const EdgeInsets.fromLTRB(24, 20, 24, 16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            eyebrow,
            style: TextStyle(
              fontSize: 11,
              fontWeight: FontWeight.w600,
              letterSpacing: 1.5,
              color: CompletionColors.eyebrow(context),
            ),
          ),
          const SizedBox(height: 6),
          Text(
            widget.gearName,
            style: TextStyle(
              fontFamily: AppTheme.headingFont,
              fontSize: 24,
              color: CompletionColors.textPrimary(context),
              height: 1.2,
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildErrorBanner() {
    return Container(
      margin: const EdgeInsets.fromLTRB(16, 0, 16, 8),
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: Colors.red.withValues(alpha: 0.15),
        borderRadius: BorderRadius.circular(10),
        border: Border.all(color: Colors.red.withValues(alpha: 0.3)),
      ),
      child: Text(
        _errorMessage!,
        style: const TextStyle(fontSize: 13, color: AppColors.errorBannerText),
      ),
    );
  }

  // ── Impact bar ──────────────────────────────────────────────────

  Widget _buildImpactBar() {
    final impact = _impactEstimate;
    if (impact == null) return const SizedBox.shrink();

    final hasMoney = impact.hasMoneySaved() && impact.moneySaved.hasValueUsd();
    final hasTime = impact.hasTimeSaved() && impact.timeSaved.hasMinutes();
    final hasCO2 = impact.hasEmissionsPrevented();

    if (!hasMoney && !hasTime) return const SizedBox.shrink();

    Widget metric(String label, String value, Color color) {
      return Column(
        children: [
          Text(
            value,
            style: TextStyle(
              fontFamily: AppTheme.headingFont,
              fontSize: 22,
              fontWeight: FontWeight.w700,
              color: color,
            ),
          ),
          const SizedBox(height: 3),
          Text(
            label,
            style: TextStyle(
              fontSize: 10,
              color: CompletionColors.textDim(context),
              letterSpacing: 0.3,
            ),
          ),
        ],
      );
    }

    return Container(
      margin: const EdgeInsets.fromLTRB(16, 0, 16, 4),
      padding: const EdgeInsets.symmetric(vertical: 18, horizontal: 20),
      decoration: BoxDecoration(
        color: CompletionColors.containerFill(context),
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: CompletionColors.glassBorder(context)),
      ),
      child: Row(
        mainAxisAlignment: MainAxisAlignment.spaceAround,
        children: [
          metric(
            'Saved',
            hasMoney
                ? '\$${impact.moneySaved.valueUsd.mean.toStringAsFixed(0)}'
                : '\$0',
            kCompletionGreenLight,
          ),
          metric(
            'Quality Time',
            hasTime
                ? _formatMinutesShort(impact.timeSaved.minutes.mean.toInt())
                : '0m',
            kCompletionAccentLight,
          ),
          metric(
            'CO₂',
            hasCO2 ? _formatEmissionsShort(impact.emissionsPrevented) : '0.0kg',
            const Color(0xFF7DB8D4),
          ),
        ],
      ),
    );
  }

  String _formatMinutesShort(int minutes) {
    if (minutes >= 60) return '${minutes ~/ 60}h';
    return '${minutes}m';
  }

  String _formatEmissionsShort(PreventedEmissions emissions) {
    double grams = 0;
    if (emissions.hasManufactureAvoidedCarbon() &&
        emissions.manufactureAvoidedCarbon.hasCo2eGrams()) {
      grams += emissions.manufactureAvoidedCarbon.co2eGrams.mean;
    }
    if (emissions.hasWasteReducedCarbon() &&
        emissions.wasteReducedCarbon.hasCo2eGrams()) {
      grams += emissions.wasteReducedCarbon.co2eGrams.mean;
    }
    return '${(grams / 1000).toStringAsFixed(1)}kg';
  }

  // ── Recipient section ───────────────────────────────────────────

  Widget _buildRecipientSection() {
    final label = _isLoan ? 'WHO BORROWED IT?' : 'WHO DID YOU GIVE IT TO?';
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 4, 16, 0),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          _sectionLabel(label),
          const SizedBox(height: 10),
          if (_hasRecipient) _buildSelectedRecipient(),
          if (!_hasRecipient)
            DarkPersonSearch(
              communityId: widget.communityId,
              searchCommunityIds: widget.sharedCommunityIds.isNotEmpty
                  ? widget.sharedCommunityIds
                  : null,
              excludedMemberIds: {widget.ownerId},
              excludedProvisionalIds: const {},
              onMemberSelected: (user) => setState(() => _selectedUser = user),
              onProvisionalSelected: (prov) =>
                  setState(() => _selectedProvisional = prov),
              onNewProvisionalRequested: _createNewProvisional,
              searchHint: _isLoan
                  ? 'Search for borrower\u2026'
                  : 'Search for recipient\u2026',
            ),
        ],
      ),
    );
  }

  Widget _buildSelectedRecipient() {
    final name = _selectedUser?.name ?? _selectedProvisional?.name ?? '';
    final isProvisional = _selectedProvisional != null;
    final avatar = isProvisional
        ? ProvisionalUserAvatar(name: name, radius: 16)
        : UserAvatar(user: _selectedUser!, radius: 16);

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        DarkPersonTile(
          leading: avatar,
          name: name,
          subtitle: isProvisional ? 'Not yet on Ripls' : null,
          included: true,
          onTap: () => setState(() {
            _selectedUser = null;
            _selectedProvisional = null;
          }),
        ),
        Padding(
          padding: const EdgeInsets.only(left: 4, top: 2, bottom: 8),
          child: Text(
            'Tap to change',
            style: TextStyle(
              fontSize: 10,
              color: CompletionColors.textDim(context),
            ),
          ),
        ),
      ],
    );
  }

  Future<void> _createNewProvisional(String name) async {
    try {
      final prov = await ref
          .read(provisionalUserRepositoryProvider)
          .createProvisionalUser(communityId: widget.communityId, name: name);
      if (mounted) setState(() => _selectedProvisional = prov);
    } catch (_) {
      // Non-fatal — search remains visible.
    }
  }

  // ── Date section ────────────────────────────────────────────────

  Widget _buildDateSection() {
    final label = _isLoan ? 'WHEN WAS IT LOANED?' : 'WHEN WAS IT GIVEN?';
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 20, 16, 0),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          _sectionLabel(label),
          const SizedBox(height: 10),
          _buildDarkDateField(
            date: _completedDate,
            placeholder: 'Select a date',
            onTap: () => _showPastDatePicker(
              initial: _completedDate,
              onPicked: (d) => setState(() {
                _completedDate = d;
                if (_returnedDate != null && _returnedDate!.isBefore(d)) {
                  _returnedDate = null;
                }
              }),
            ),
          ),
        ],
      ),
    );
  }

  // ── Return section (loans only) ─────────────────────────────────

  Widget _buildReturnSection() {
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 20, 16, 0),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          _buildReturnToggleRow(),
          if (_alreadyReturned) ...[
            const SizedBox(height: 16),
            _sectionLabel('WHEN WAS IT RETURNED?'),
            const SizedBox(height: 10),
            _buildDarkDateField(
              date: _returnedDate,
              placeholder: 'Select return date',
              onTap: () => _showPastDatePicker(
                initial: _returnedDate ?? _completedDate,
                onPicked: (d) => setState(() => _returnedDate = d),
              ),
            ),
          ],
        ],
      ),
    );
  }

  Widget _buildReturnToggleRow() {
    return Toggle(
      semanticsLabel: context.l10n.a11yGearToggleAlreadyReturned,
      selected: _alreadyReturned,
      onTap: () => setState(() {
        _alreadyReturned = !_alreadyReturned;
        if (!_alreadyReturned) _returnedDate = null;
      }),
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
        decoration: BoxDecoration(
          color: CompletionColors.tileBackground(
            context,
            included: _alreadyReturned,
          ),
          borderRadius: BorderRadius.circular(12),
          border: Border.all(
            color: CompletionColors.tileBorder(
              context,
              included: _alreadyReturned,
            ),
          ),
        ),
        child: Row(
          children: [
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    'Already returned',
                    style: TextStyle(
                      fontSize: 14,
                      color: CompletionColors.tileText(
                        context,
                        included: _alreadyReturned,
                      ),
                    ),
                  ),
                  Text(
                    'Log the return date too',
                    style: TextStyle(
                      fontSize: 10,
                      color: CompletionColors.textDim(context),
                    ),
                  ),
                ],
              ),
            ),
            DarkCheckCircle(checked: _alreadyReturned),
          ],
        ),
      ),
    );
  }

  // ── Shared helpers ──────────────────────────────────────────────

  Widget _sectionLabel(String label) {
    return Text(
      label,
      style: TextStyle(
        fontSize: 10,
        fontWeight: FontWeight.w600,
        letterSpacing: 1.2,
        color: CompletionColors.sectionLabel(context),
      ),
    );
  }

  Widget _buildDarkDateField({
    required DateTime? date,
    required String placeholder,
    required VoidCallback onTap,
  }) {
    final isSet = date != null;
    return Tappable(
      semanticsLabel: context.l10n.a11yGearPickDate,
      onTap: onTap,
      child: AnimatedContainer(
        duration: accessibleDuration(
          context,
          const Duration(milliseconds: 200),
        ),
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 13),
        decoration: BoxDecoration(
          color: CompletionColors.tileBackground(context, included: isSet),
          borderRadius: BorderRadius.circular(12),
          border: Border.all(
            color: CompletionColors.tileBorder(context, included: isSet),
          ),
        ),
        child: Row(
          children: [
            Icon(
              Icons.calendar_today_outlined,
              size: 15,
              color: isSet
                  ? kCompletionAccentLight
                  : CompletionColors.textDim(context),
            ),
            const SizedBox(width: 10),
            Expanded(
              child: Text(
                isSet
                    ? DateFormat('EEEE, MMMM d, yyyy').format(date)
                    : placeholder,
                style: TextStyle(
                  fontSize: 14,
                  color: isSet
                      ? CompletionColors.textPrimary(context)
                      : CompletionColors.sectionLabel(context),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildSubmitButton() {
    final label = _isLoan ? 'Log Loan' : 'Log Giveaway';
    return Container(
      padding: EdgeInsets.fromLTRB(
        16,
        12,
        16,
        MediaQuery.of(context).padding.bottom + 16,
      ),
      color: CompletionColors.bottomBar(context),
      child: SizedBox(
        width: double.infinity,
        child: ElevatedButton(
          onPressed: _canSubmit ? _submit : null,
          style: ElevatedButton.styleFrom(
            padding: const EdgeInsets.symmetric(vertical: 16),
            // Not `kCompletionGreen`: that green carries a check glyph well but
            // cannot carry a LABEL — white measures 3.93:1 on it and near-black
            // 4.12:1, so neither polarity clears 4.5. The glass primary pair
            // does, and matches the other completion-sheet CTAs.
            backgroundColor: GlassTokens.primary,
            disabledBackgroundColor: GlassTokens.primary.withValues(alpha: 0.4),
            foregroundColor: GlassTokens.onPrimary,
            shape: const StadiumBorder(),
            elevation: 0,
          ),
          child: _isSubmitting
              ? const Row(
                  mainAxisAlignment: MainAxisAlignment.center,
                  children: [
                    SizedBox(
                      width: 16,
                      height: 16,
                      child: CircularProgressIndicator(
                        strokeWidth: 2,
                        color: GlassTokens.textPrimary,
                      ),
                    ),
                    SizedBox(width: 10),
                    Text(
                      'Crafting your story\u2026',
                      style: TextStyle(
                        fontSize: 15,
                        fontWeight: FontWeight.w600,
                      ),
                    ),
                  ],
                )
              : Text(
                  label,
                  style: const TextStyle(
                    fontSize: 16,
                    fontWeight: FontWeight.w600,
                  ),
                ),
        ),
      ),
    );
  }
}
