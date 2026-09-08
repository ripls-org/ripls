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

/// Co2DetailModal shows the CO₂ Avoided breakdown for a draft impact estimate.
///
/// The Calculation tab shows manufacture-avoided and waste-reduced carbon
/// components. The Inputs tab lets users override travel-avoided and repair-credit
/// carbon inputs.
class Co2DetailModal extends ConsumerStatefulWidget {
  const Co2DetailModal({
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
        child: Co2DetailModal(targetId: targetId, isExperience: isExperience),
      ),
    );
  }

  @override
  ConsumerState<Co2DetailModal> createState() => _Co2DetailModalState();
}

class _Co2DetailModalState extends ConsumerState<Co2DetailModal>
    with SingleTickerProviderStateMixin {
  late TabController _tabController;
  String _activeTab = 'Calculation';

  double _travelAvoidedCarbon = 0;
  double _repairCreditCarbon = 0;
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

  void _initFromDraft(PreventedEmissions emissions) {
    if (_initialized) return;
    _initialized = true;
    if (emissions.hasInputs()) {
      _travelAvoidedCarbon = emissions.inputs.hasTravelAvoidedCarbon()
          ? emissions.inputs.travelAvoidedCarbon.mean
          : 0;
      _repairCreditCarbon = emissions.inputs.hasRepairCreditCarbon()
          ? emissions.inputs.repairCreditCarbon.mean
          : 0;
    }
  }

  double _totalCo2Grams(PreventedEmissions? e) {
    if (e == null) return 0;
    double total = 0;
    if (e.hasManufactureAvoidedCarbon() && e.manufactureAvoidedCarbon.hasCo2eGrams()) {
      total += e.manufactureAvoidedCarbon.co2eGrams.mean;
    }
    if (e.hasWasteReducedCarbon() && e.wasteReducedCarbon.hasCo2eGrams()) {
      total += e.wasteReducedCarbon.co2eGrams.mean;
    }
    return total;
  }

  String _formatGrams(double grams) {
    if (grams >= 1000) {
      return '${(grams / 1000).toStringAsFixed(1)} kg';
    }
    return '${grams.toStringAsFixed(0)} g';
  }

  void _scheduleAutoSave() {
    _autoSaveTimer?.cancel();
    _autoSaveTimer = Timer(_kAutoSaveDebounce, () {
      if (!mounted) return;
      final overrides = PreventedEmissions(
        inputs: PreventedEmissionsInput(
          travelAvoidedCarbon: pb_common.Estimate(mean: _travelAvoidedCarbon),
          repairCreditCarbon: pb_common.Estimate(mean: _repairCreditCarbon),
        ),
      );
      ref
          .read(impactDraftProvider(widget.targetId).notifier)
          .applyEmissionsOverrides(
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
    // Bumping the reset epoch forces fresh ImpactEditChips via the keyed
    // rebuild below, which discards any TextField focus and re-runs the
    // controller seed from the AI-drafted value.
    setState(() {
      _initialized = false;
      _resetEpoch++;
    });
  }

  @override
  Widget build(BuildContext context) {
    final draftState = ref.watch(impactDraftProvider(widget.targetId));
    final emissions = draftState.draft?.hasEmissionsPrevented() ?? false
        ? draftState.draft!.emissionsPrevented
        : null;

    if (emissions != null) _initFromDraft(emissions);

    final totalGrams = _totalCo2Grams(emissions);
    final manufactureGrams = (emissions?.hasManufactureAvoidedCarbon() ?? false) &&
            emissions!.manufactureAvoidedCarbon.hasCo2eGrams()
        ? emissions.manufactureAvoidedCarbon.co2eGrams.mean
        : 0.0;
    final wasteGrams = (emissions?.hasWasteReducedCarbon() ?? false) &&
            emissions!.wasteReducedCarbon.hasCo2eGrams()
        ? emissions.wasteReducedCarbon.co2eGrams.mean
        : 0.0;

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
            icon: Icons.eco_rounded,
            value: _formatGrams(totalGrams),
            label: 'CO₂ Avoided',
            color: ImpactModalColors.green,
            bgColor: ImpactModalColors.greenBg,
          ),
          const SizedBox(height: 16),
          Text(
            _activeTab == 'Calculation'
                ? 'CO₂ Avoided estimates the carbon emissions prevented by sharing an item instead of manufacturing a new one.'
                : 'Tap any value to adjust. The CO₂ estimate will update based on your inputs.',
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
              if (_activeTab == 'Calculation') ...[
                ImpactCalcRow(
                  label: 'Manufacture avoided',
                  child: Text(_formatGrams(manufactureGrams)),
                ),
                ImpactCalcRow(
                  label: 'Waste reduced',
                  child: Text(_formatGrams(wasteGrams)),
                ),
                ImpactCalcRow(
                  label: 'CO₂ Avoided',
                  divider: true,
                  heavy: true,
                  valueColor: ImpactModalColors.green,
                  child: Text(_formatGrams(totalGrams)),
                ),
              ] else ...[
                ImpactCalcRow(
                  label: 'Travel avoided carbon',
                  child: ImpactEditChip(
                    key: ValueKey('travel-$_resetEpoch'),
                    value: _travelAvoidedCarbon,
                    onChanged: (v) {
                      setState(() => _travelAvoidedCarbon = v);
                      _scheduleAutoSave();
                    },
                    suffix: 'g',
                    width: 60,
                    min: 0,
                  ),
                ),
                ImpactCalcRow(
                  label: 'Repair credit carbon',
                  child: ImpactEditChip(
                    key: ValueKey('repair-$_resetEpoch'),
                    value: _repairCreditCarbon,
                    onChanged: (v) {
                      setState(() => _repairCreditCarbon = v);
                      _scheduleAutoSave();
                    },
                    suffix: 'g',
                    width: 60,
                    min: 0,
                  ),
                ),
                ImpactCalcRow(
                  label: 'CO₂ Avoided',
                  divider: true,
                  heavy: true,
                  valueColor: ImpactModalColors.green,
                  child: Text(_formatGrams(totalGrams)),
                ),
              ],
            ],
          ),
          if (_activeTab == 'Calculation')
            const ImpactMethodFooter(
              method: 'Embodied Carbon + Waste Diversion',
              source: 'Edinburgh Tool Library / ICE Database — material emission factors',
            )
          else ...[
            const ImpactHint(
              text: 'Inputs replace the AI-estimated defaults for this item. Reset anytime to restore community values.',
              variant: ImpactHintVariant.green,
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
