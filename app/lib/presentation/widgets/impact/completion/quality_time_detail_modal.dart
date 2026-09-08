import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/paper_tokens.gen.dart';
import 'package:ripls/core/utils/savings_formatter.dart';
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart';
import 'package:ripls/presentation/viewmodels/impact_draft_notifier.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/impact/completion/impact_edit_chip.dart';
import 'package:ripls/presentation/widgets/impact/completion/impact_modal_shared.dart';
import 'package:ripls/presentation/widgets/impact/completion/impact_select_chip.dart';

const Duration _kAutoSaveDebounce = Duration(milliseconds: 250);

/// QualityTimeDetailModal shows the Quality Time breakdown for a draft impact estimate.
///
/// The Calculation tab displays the computed total and resolved attribute tiers.
/// The Inputs tab lets users override each social attribute and base duration.
class QualityTimeDetailModal extends ConsumerStatefulWidget {
  const QualityTimeDetailModal({
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
        heightFactor: 0.92,
        child: QualityTimeDetailModal(
          targetId: targetId,
          isExperience: isExperience,
        ),
      ),
    );
  }

  @override
  ConsumerState<QualityTimeDetailModal> createState() =>
      _QualityTimeDetailModalState();
}

class _QualityTimeDetailModalState extends ConsumerState<QualityTimeDetailModal>
    with SingleTickerProviderStateMixin {
  late TabController _tabController;
  String _activeTab = 'Calculation';

  double _durationMinutes = 15;
  int _groupSize = 2;
  SocialModality _modality = SocialModality.SOCIAL_MODALITY_IN_PERSON_BRIEF;
  SocialTieStrength _tieStrength = SocialTieStrength.SOCIAL_TIE_STRENGTH_ACQUAINTANCE;
  SocialReciprocity _reciprocity = SocialReciprocity.SOCIAL_RECIPROCITY_MUTUAL;
  SocialNovelty _novelty = SocialNovelty.SOCIAL_NOVELTY_INFREQUENT;
  SocialVulnerabilityLevel _vulnerability =
      SocialVulnerabilityLevel.SOCIAL_VULNERABILITY_LEVEL_MEDIUM;
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

  void _initFromDraft(QualityTimeEstimate qt) {
    if (_initialized) return;
    _initialized = true;
    if (!qt.hasAttributes()) return;
    final a = qt.attributes;
    if (a.estimatedDurationMinutes > 0) _durationMinutes = a.estimatedDurationMinutes;
    if (a.groupSize > 0) _groupSize = a.groupSize;
    if (a.modality != SocialModality.SOCIAL_MODALITY_UNSPECIFIED) {
      _modality = a.modality;
    }
    if (a.tieStrength != SocialTieStrength.SOCIAL_TIE_STRENGTH_UNSPECIFIED) {
      _tieStrength = a.tieStrength;
    }
    if (a.reciprocity != SocialReciprocity.SOCIAL_RECIPROCITY_UNSPECIFIED) {
      _reciprocity = a.reciprocity;
    }
    if (a.novelty != SocialNovelty.SOCIAL_NOVELTY_UNSPECIFIED) {
      _novelty = a.novelty;
    }
    if (a.vulnerability != SocialVulnerabilityLevel.SOCIAL_VULNERABILITY_LEVEL_UNSPECIFIED) {
      _vulnerability = a.vulnerability;
    }
  }

  void _scheduleAutoSave() {
    _autoSaveTimer?.cancel();
    _autoSaveTimer = Timer(_kAutoSaveDebounce, () {
      if (!mounted) return;
      final overrides = QualityTimeAttributes(
        estimatedDurationMinutes: _durationMinutes,
        groupSize: _groupSize,
        modality: _modality,
        tieStrength: _tieStrength,
        reciprocity: _reciprocity,
        novelty: _novelty,
        vulnerability: _vulnerability,
      );
      ref
          .read(impactDraftProvider(widget.targetId).notifier)
          .applyQualityTimeOverrides(
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
    // Bumping the reset epoch forces fresh input chips via the keyed
    // rebuild below — discards any TextField focus and re-seeds controllers
    // from the AI-drafted values.
    setState(() {
      _initialized = false;
      _resetEpoch++;
    });
  }


  String _modalityLabel(SocialModality m) => switch (m) {
        SocialModality.SOCIAL_MODALITY_IN_PERSON_SHARED => 'Side-by-side',
        SocialModality.SOCIAL_MODALITY_IN_PERSON_BRIEF => 'Brief chat',
        SocialModality.SOCIAL_MODALITY_VIDEO => 'Video call',
        SocialModality.SOCIAL_MODALITY_PHONE => 'Phone call',
        SocialModality.SOCIAL_MODALITY_TEXT => 'Text / doorstep',
        _ => 'Unknown',
      };

  String _tieLabel(SocialTieStrength t) => switch (t) {
        SocialTieStrength.SOCIAL_TIE_STRENGTH_NEW => 'Stranger',
        SocialTieStrength.SOCIAL_TIE_STRENGTH_ACQUAINTANCE => 'Acquaintance',
        SocialTieStrength.SOCIAL_TIE_STRENGTH_ACTIVE => 'Friend',
        SocialTieStrength.SOCIAL_TIE_STRENGTH_CLOSE => 'Close friend',
        _ => 'Unknown',
      };

  String _reciprocityLabel(SocialReciprocity r) => switch (r) {
        SocialReciprocity.SOCIAL_RECIPROCITY_GIVING => 'One-way (giving)',
        SocialReciprocity.SOCIAL_RECIPROCITY_RECEIVING => 'One-way (receiving)',
        SocialReciprocity.SOCIAL_RECIPROCITY_MUTUAL => 'Mutual',
        _ => 'Unknown',
      };

  String _noveltyLabel(SocialNovelty n) => switch (n) {
        SocialNovelty.SOCIAL_NOVELTY_NOVEL => 'First time',
        SocialNovelty.SOCIAL_NOVELTY_INFREQUENT => 'Occasional',
        SocialNovelty.SOCIAL_NOVELTY_ROUTINE => 'Routine',
        _ => 'Unknown',
      };

  String _vulnerabilityLabel(SocialVulnerabilityLevel v) => switch (v) {
        SocialVulnerabilityLevel.SOCIAL_VULNERABILITY_LEVEL_LOW => 'Low',
        SocialVulnerabilityLevel.SOCIAL_VULNERABILITY_LEVEL_MEDIUM => 'Moderate',
        SocialVulnerabilityLevel.SOCIAL_VULNERABILITY_LEVEL_HIGH => 'High',
        _ => 'Unknown',
      };

  // Weight tables mirror server config so the Calculation tab makes the math
  // explicit. Source: server/impact_metrics/estimator/config.textproto.

  String _formatMultiplier(double w) => '× ${w.toStringAsFixed(1)}';

  double _modalityWeight(SocialModality m) => switch (m) {
        SocialModality.SOCIAL_MODALITY_IN_PERSON_SHARED => 1.0,
        SocialModality.SOCIAL_MODALITY_IN_PERSON_BRIEF => 0.6,
        SocialModality.SOCIAL_MODALITY_VIDEO => 0.4,
        SocialModality.SOCIAL_MODALITY_PHONE => 0.3,
        SocialModality.SOCIAL_MODALITY_TEXT => 0.1,
        _ => 1.0,
      };

  // Group-size tiers: dyadic (≤2), small (3–5), medium (6–15), large (16+).
  double _groupSizeWeight(int size) {
    if (size <= 2) return 1;
    if (size <= 5) return 1.2;
    if (size <= 15) return 1.3;
    return 1.1;
  }

  double _tieWeight(SocialTieStrength t) => switch (t) {
        SocialTieStrength.SOCIAL_TIE_STRENGTH_NEW => 0.8,
        SocialTieStrength.SOCIAL_TIE_STRENGTH_ACQUAINTANCE => 1.0,
        SocialTieStrength.SOCIAL_TIE_STRENGTH_ACTIVE => 1.1,
        SocialTieStrength.SOCIAL_TIE_STRENGTH_CLOSE => 1.2,
        _ => 1.0,
      };

  double _reciprocityWeight(SocialReciprocity r) => switch (r) {
        SocialReciprocity.SOCIAL_RECIPROCITY_GIVING => 1.0,
        SocialReciprocity.SOCIAL_RECIPROCITY_RECEIVING => 0.7,
        SocialReciprocity.SOCIAL_RECIPROCITY_MUTUAL => 1.0,
        _ => 1.0,
      };

  double _noveltyWeight(SocialNovelty n) => switch (n) {
        SocialNovelty.SOCIAL_NOVELTY_NOVEL => 1.3,
        SocialNovelty.SOCIAL_NOVELTY_INFREQUENT => 1.0,
        SocialNovelty.SOCIAL_NOVELTY_ROUTINE => 0.8,
        _ => 1.0,
      };

  double _vulnerabilityWeight(SocialVulnerabilityLevel v) => switch (v) {
        SocialVulnerabilityLevel.SOCIAL_VULNERABILITY_LEVEL_LOW => 0.8,
        SocialVulnerabilityLevel.SOCIAL_VULNERABILITY_LEVEL_MEDIUM => 1.0,
        SocialVulnerabilityLevel.SOCIAL_VULNERABILITY_LEVEL_HIGH => 1.3,
        _ => 1.0,
      };

  @override
  Widget build(BuildContext context) {
    final draftState = ref.watch(impactDraftProvider(widget.targetId));
    final qt = draftState.draft?.hasQualityTime() ?? false
        ? draftState.draft!.qualityTime
        : null;

    if (qt != null) _initFromDraft(qt);

    final totalMins = qt?.hasQualityTimeMinutes() ?? false
        ? qt!.qualityTimeMinutes.mean
        : 0.0;
    final attrs = qt?.hasAttributes() ?? false ? qt!.attributes : null;

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
            icon: Icons.access_time_rounded,
            value: SavingsFormatter.formatQualityTime(totalMins),
            label: 'Quality Time',
            color: ImpactModalColors.green,
            bgColor: ImpactModalColors.greenBg,
          ),
          const SizedBox(height: 16),
          Text(
            _activeTab == 'Calculation'
                ? 'Quality Time measures the depth of social connection when people get together, adjusted for group dynamics, relationship strength, and novelty.'
                : 'Tap any input to adjust how this interaction is weighted. Quality Time updates live.',
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
                label: 'Base duration',
                child: _activeTab == 'Calculation'
                    ? Text('${(attrs?.estimatedDurationMinutes ?? _durationMinutes).toStringAsFixed(0)} min')
                    : ImpactEditChip(
                        key: ValueKey('duration-$_resetEpoch'),
                        value: _durationMinutes,
                        onChanged: (v) {
                          setState(() => _durationMinutes = v);
                          _scheduleAutoSave();
                        },
                        suffix: 'min',
                        width: 44,
                        min: 1,
                      ),
              ),
              ImpactCalcRow(
                label: 'Modality',
                child: _activeTab == 'Calculation'
                    ? Text(_formatMultiplier(_modalityWeight(attrs?.modality ?? _modality)))
                    : ImpactSelectChip<SocialModality>(
                        key: ValueKey('modality-$_resetEpoch'),
                        value: _modality,
                        onChanged: (v) {
                          setState(() => _modality = v);
                          _scheduleAutoSave();
                        },
                        items: [
                          SocialModality.SOCIAL_MODALITY_TEXT,
                          SocialModality.SOCIAL_MODALITY_PHONE,
                          SocialModality.SOCIAL_MODALITY_VIDEO,
                          SocialModality.SOCIAL_MODALITY_IN_PERSON_BRIEF,
                          SocialModality.SOCIAL_MODALITY_IN_PERSON_SHARED,
                        ]
                            .map((m) => ImpactSelectItem(value: m, label: _modalityLabel(m)))
                            .toList(),
                      ),
              ),
              ImpactCalcRow(
                label: 'Group size',
                child: _activeTab == 'Calculation'
                    ? Text(_formatMultiplier(_groupSizeWeight(attrs?.groupSize ?? _groupSize)))
                    : ImpactEditChip(
                        key: ValueKey('group-$_resetEpoch'),
                        value: _groupSize.toDouble(),
                        onChanged: (v) {
                          setState(() => _groupSize = v.round().clamp(1, 99));
                          _scheduleAutoSave();
                        },
                        suffix: 'ppl',
                        width: 36,
                        min: 1,
                        max: 99,
                      ),
              ),
              ImpactCalcRow(
                label: 'Tie strength',
                child: _activeTab == 'Calculation'
                    ? Text(_formatMultiplier(_tieWeight(attrs?.tieStrength ?? _tieStrength)))
                    : ImpactSelectChip<SocialTieStrength>(
                        key: ValueKey('tie-$_resetEpoch'),
                        value: _tieStrength,
                        onChanged: (v) {
                          setState(() => _tieStrength = v);
                          _scheduleAutoSave();
                        },
                        items: [
                          SocialTieStrength.SOCIAL_TIE_STRENGTH_NEW,
                          SocialTieStrength.SOCIAL_TIE_STRENGTH_ACQUAINTANCE,
                          SocialTieStrength.SOCIAL_TIE_STRENGTH_ACTIVE,
                          SocialTieStrength.SOCIAL_TIE_STRENGTH_CLOSE,
                        ]
                            .map((t) => ImpactSelectItem(value: t, label: _tieLabel(t)))
                            .toList(),
                      ),
              ),
              ImpactCalcRow(
                label: 'Reciprocity',
                child: _activeTab == 'Calculation'
                    ? Text(_formatMultiplier(_reciprocityWeight(attrs?.reciprocity ?? _reciprocity)))
                    : ImpactSelectChip<SocialReciprocity>(
                        key: ValueKey('reciprocity-$_resetEpoch'),
                        value: _reciprocity,
                        onChanged: (v) {
                          setState(() => _reciprocity = v);
                          _scheduleAutoSave();
                        },
                        items: [
                          SocialReciprocity.SOCIAL_RECIPROCITY_GIVING,
                          SocialReciprocity.SOCIAL_RECIPROCITY_RECEIVING,
                          SocialReciprocity.SOCIAL_RECIPROCITY_MUTUAL,
                        ]
                            .map((r) => ImpactSelectItem(value: r, label: _reciprocityLabel(r)))
                            .toList(),
                      ),
              ),
              ImpactCalcRow(
                label: 'Novelty',
                child: _activeTab == 'Calculation'
                    ? Text(_formatMultiplier(_noveltyWeight(attrs?.novelty ?? _novelty)))
                    : ImpactSelectChip<SocialNovelty>(
                        key: ValueKey('novelty-$_resetEpoch'),
                        value: _novelty,
                        onChanged: (v) {
                          setState(() => _novelty = v);
                          _scheduleAutoSave();
                        },
                        items: [
                          SocialNovelty.SOCIAL_NOVELTY_ROUTINE,
                          SocialNovelty.SOCIAL_NOVELTY_INFREQUENT,
                          SocialNovelty.SOCIAL_NOVELTY_NOVEL,
                        ]
                            .map((n) => ImpactSelectItem(value: n, label: _noveltyLabel(n)))
                            .toList(),
                      ),
              ),
              ImpactCalcRow(
                label: 'Vulnerability',
                child: _activeTab == 'Calculation'
                    ? Text(_formatMultiplier(_vulnerabilityWeight(attrs?.vulnerability ?? _vulnerability)))
                    : ImpactSelectChip<SocialVulnerabilityLevel>(
                        key: ValueKey('vulnerability-$_resetEpoch'),
                        value: _vulnerability,
                        onChanged: (v) {
                          setState(() => _vulnerability = v);
                          _scheduleAutoSave();
                        },
                        items: [
                          SocialVulnerabilityLevel.SOCIAL_VULNERABILITY_LEVEL_LOW,
                          SocialVulnerabilityLevel.SOCIAL_VULNERABILITY_LEVEL_MEDIUM,
                          SocialVulnerabilityLevel.SOCIAL_VULNERABILITY_LEVEL_HIGH,
                        ]
                            .map((v) => ImpactSelectItem(
                                  value: v,
                                  label: _vulnerabilityLabel(v),
                                ))
                            .toList(),
                      ),
              ),
              ImpactCalcRow(
                label: 'Quality Time',
                divider: true,
                heavy: true,
                valueColor: ImpactModalColors.green,
                child: Text(SavingsFormatter.formatQualityTime(totalMins)),
              ),
            ],
          ),
          if (_activeTab == 'Calculation')
            const ImpactMethodFooter(
              method: 'Quality Time (Config Defaults)',
              source: 'social_connection_metrics.md — attribute weights and research references',
            )
          else ...[
            const ImpactHint(
              text: 'Multipliers pull from the community\'s weighting config. Changes here apply to this interaction only.',
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
