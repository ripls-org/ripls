import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/paper_tokens.gen.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart' as pb_common;
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart';
import 'package:ripls/presentation/viewmodels/impact_draft_notifier.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/impact/completion/impact_edit_chip.dart';
import 'package:ripls/presentation/widgets/impact/completion/impact_modal_shared.dart';

const Duration _kAutoSaveDebounce = Duration(milliseconds: 250);

/// ValueDetailModal shows the Money Saved calculation for a draft impact estimate.
///
/// The Calculation tab displays the computed total. The Inputs tab exposes the
/// hire-equivalent value input so users can override the server default.
class ValueDetailModal extends ConsumerStatefulWidget {
  const ValueDetailModal({
    super.key,
    required this.targetId,
    required this.isExperience,
  });

  final String targetId;
  final bool isExperience;

  static Future<void> show(
    BuildContext context,
    String targetId, {
    required bool isExperience,
  }) {
    return showAccessibleModal<void>(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      builder: (_) => FractionallySizedBox(
        heightFactor: 0.85,
        child: ValueDetailModal(targetId: targetId, isExperience: isExperience),
      ),
    );
  }

  @override
  ConsumerState<ValueDetailModal> createState() => _ValueDetailModalState();
}

class _ValueDetailModalState extends ConsumerState<ValueDetailModal>
    with SingleTickerProviderStateMixin {
  late TabController _tabController;
  String _activeTab = 'Calculation';

  // Local editable state (initialized from draft on first build).
  double _hireEquivalent = 0;
  bool _initialized = false;
  int _resetEpoch = 0;
  Timer? _autoSaveTimer;

  @override
  void initState() {
    super.initState();
    _tabController = TabController(length: 2, vsync: this);
  }

  @override
  void dispose() {
    _autoSaveTimer?.cancel();
    _tabController.dispose();
    super.dispose();
  }

  void _initFromDraft(MoneySavings money) {
    if (_initialized) return;
    _initialized = true;
    _hireEquivalent = money.hasInputs() && money.inputs.hasHireEquivalentValue()
        ? money.inputs.hireEquivalentValue.mean
        : (money.hasValueUsd() ? money.valueUsd.mean : 0);
  }

  void _scheduleAutoSave() {
    _autoSaveTimer?.cancel();
    _autoSaveTimer = Timer(_kAutoSaveDebounce, () {
      if (!mounted) return;
      final overrides = MoneySavings(
        inputs: MoneySavingsInput(
          hireEquivalentValue: pb_common.Estimate(mean: _hireEquivalent),
        ),
      );
      ref
          .read(impactDraftProvider(widget.targetId).notifier)
          .applyMoneySavingsOverrides(
            overrides,
            isExperience: widget.isExperience,
          );
    });
  }

  Future<void> _handleReset() async {
    _autoSaveTimer?.cancel();
    // Await the redraft so state.draft has settled to the AI-drafted values
    // before we flip _initialized. Flipping it earlier lets the next rebuild
    // re-seed from the *current* (still-overridden) draft and lock the inputs
    // back to their override values for good.
    await ref
        .read(impactDraftProvider(widget.targetId).notifier)
        .resetOverrides(isExperience: widget.isExperience);
    if (!mounted) return;
    // Bumping the reset epoch forces a fresh ImpactEditChip via the keyed
    // rebuild below, which discards any TextField focus and re-runs the
    // controller seed from the AI-drafted value. Without this, an EditChip
    // that still has focus from the user's just-typed override ignores the
    // value-prop change in didUpdateWidget and the field stays stale.
    setState(() {
      _initialized = false;
      _resetEpoch++;
    });
  }

  @override
  Widget build(BuildContext context) {
    final draftState = ref.watch(impactDraftProvider(widget.targetId));
    final money = draftState.draft?.hasMoneySaved() ?? false
        ? draftState.draft!.moneySaved
        : null;

    if (money != null) _initFromDraft(money);

    final totalUsd = money?.hasValueUsd() ?? false ? money!.valueUsd.mean : 0.0;
    final confidence = money != null ? '70%' : '—';

    return _ModalShell(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          ImpactSegTabs(
            active: _activeTab,
            onChanged: (tab) => setState(() => _activeTab = tab),
          ),
          const SizedBox(height: 18),
          ImpactMetricHeadline(
            icon: Icons.attach_money_rounded,
            value: '\$${totalUsd.toStringAsFixed(0)}',
            label: 'Money Saved',
            color: ImpactModalColors.coral,
            bgColor: ImpactModalColors.coralBg,
          ),
          const SizedBox(height: 16),
          Text(
            _activeTab == 'Calculation'
                ? 'Money Saved estimates how much a borrower avoided spending by using a shared item instead of buying or renting.'
                : 'Tap any value to adjust. The Money Saved figure will update to match.',
            style: const TextStyle(
              fontSize: 14,
              color: ImpactModalColors.inkSoft,
              height: 1.45,
            ),
          ),
          const SizedBox(height: 14),
          ImpactCalcCard(
            label: _activeTab == 'Calculation' ? 'Calculation' : 'Inputs · tap to edit',
            children: [
              ImpactCalcRow(
                label: 'Hire equivalent value',
                child: _activeTab == 'Calculation'
                    ? Text('\$${_hireEquivalent.toStringAsFixed(0)}')
                    : ImpactEditChip(
                        key: ValueKey('hire-$_resetEpoch'),
                        value: _hireEquivalent,
                        onChanged: (v) {
                          setState(() => _hireEquivalent = v);
                          _scheduleAutoSave();
                        },
                        prefix: '\$',
                        width: 56,
                        min: 0,
                      ),
              ),
              ImpactCalcRow(
                label: 'Money Saved',
                divider: true,
                heavy: true,
                valueColor: ImpactModalColors.coral,
                child: Text('\$${totalUsd.toStringAsFixed(0)}'),
              ),
              ImpactCalcRow(
                label: 'Confidence',
                child: Text(confidence),
              ),
            ],
          ),
          if (_activeTab == 'Calculation')
            const ImpactMethodFooter(
              method: 'Hire Equivalent',
              source: 'Library of Things UK — hire cost equivalence survey data',
            )
          else ...[
            const ImpactHint(
              text: 'Your edits replace this item\'s defaults. Reset anytime to restore community values.',
              variant: ImpactHintVariant.coral,
            ),
            ImpactCTARow(
              onReset: _handleReset,
              isSaving: draftState.isRedrafting,
            ),
          ],
        ],
      ),
    );
  }
}

class _ModalShell extends StatelessWidget {
  const _ModalShell({required this.child});

  final Widget child;

  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: const BoxDecoration(
        color: ImpactModalColors.sheetBg,
        borderRadius: BorderRadius.vertical(top: Radius.circular(22)),
      ),
      child: Column(
        children: [
          // Drag handle
          Padding(
            padding: const EdgeInsets.symmetric(vertical: 10),
            child: Center(
              child: Container(
                width: 36,
                height: 4,
                decoration: BoxDecoration(
                  color: PaperTokens.borderStrong,
                  borderRadius: BorderRadius.circular(3),
                ),
              ),
            ),
          ),
          Expanded(
            child: SingleChildScrollView(
              padding: const EdgeInsets.fromLTRB(22, 4, 22, 32),
              child: child,
            ),
          ),
        ],
      ),
    );
  }
}
