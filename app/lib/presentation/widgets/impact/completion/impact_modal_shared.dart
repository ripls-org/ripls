import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';
import 'package:ripls/presentation/widgets/accessibility/toggle.dart';

/// Segmented two-tab pill control matching the calculation-modal design.
class ImpactSegTabs extends StatelessWidget {
  const ImpactSegTabs({
    super.key,
    required this.active,
    required this.onChanged,
    this.tabs = const ['Calculation', 'Inputs'],
  });

  final String active;
  final ValueChanged<String> onChanged;
  final List<String> tabs;

  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: BoxDecoration(
        color: ImpactModalColors.chipBg,
        borderRadius: BorderRadius.circular(11),
      ),
      padding: const EdgeInsets.all(3),
      child: Row(
        children: tabs.map((tab) {
          final isActive = tab == active;
          return Expanded(
            child: Toggle(
              semanticsLabel: tab,
              selected: isActive,
              onTap: () => onChanged(tab),
              child: AnimatedContainer(
                duration: accessibleDuration(context, const Duration(milliseconds: 150)),
                padding: const EdgeInsets.symmetric(vertical: 8),
                decoration: BoxDecoration(
                  color: isActive ? Colors.white : Colors.transparent,
                  borderRadius: BorderRadius.circular(9),
                  boxShadow: isActive
                      ? [
                          BoxShadow(
                            color: Colors.black.withValues(alpha: 0.06),
                            blurRadius: 2,
                            offset: const Offset(0, 1),
                          ),
                        ]
                      : null,
                ),
                child: Text(
                  tab,
                  textAlign: TextAlign.center,
                  style: TextStyle(
                    fontSize: 13,
                    fontWeight: isActive ? FontWeight.w600 : FontWeight.w500,
                    color: isActive ? ImpactModalColors.ink : ImpactModalColors.inkSoft,
                  ),
                ),
              ),
            ),
          );
        }).toList(),
      ),
    );
  }
}

/// CalcCard is the warm parchment-toned card containing calculation rows.
class ImpactCalcCard extends StatelessWidget {
  const ImpactCalcCard({
    super.key,
    required this.label,
    required this.children,
  });

  final String label;
  final List<Widget> children;

  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: BoxDecoration(
        color: ImpactModalColors.calcBg,
        borderRadius: BorderRadius.circular(14),
      ),
      padding: const EdgeInsets.fromLTRB(16, 16, 16, 14),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Text(
            label.toUpperCase(),
            style: const TextStyle(
              fontSize: 11,
              letterSpacing: 1.2,
              fontWeight: FontWeight.w600,
              color: ImpactModalColors.inkFade,
            ),
          ),
          const SizedBox(height: 12),
          ...children,
        ],
      ),
    );
  }
}

/// CalcRow is a single label/value row inside an ImpactCalcCard.
class ImpactCalcRow extends StatelessWidget {
  const ImpactCalcRow({
    super.key,
    required this.label,
    required this.child,
    this.divider = false,
    this.heavy = false,
    this.valueColor,
  });

  final String label;
  final Widget child;
  final bool divider;
  final bool heavy;
  final Color? valueColor;

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        if (divider) ...[
          const SizedBox(height: 4),
          Divider(color: ImpactModalColors.calcLine, height: 1, thickness: 1),
          const SizedBox(height: 8),
        ],
        Padding(
          padding: EdgeInsets.symmetric(vertical: divider ? 0 : 8),
          child: Row(
            mainAxisAlignment: MainAxisAlignment.spaceBetween,
            crossAxisAlignment: CrossAxisAlignment.center,
            children: [
              Text(
                label,
                style: TextStyle(
                  fontSize: 14,
                  color: heavy ? ImpactModalColors.ink : ImpactModalColors.inkSoft,
                  fontWeight: heavy ? FontWeight.w700 : FontWeight.w400,
                ),
              ),
              DefaultTextStyle(
                style: TextStyle(
                  fontSize: 14,
                  fontWeight: heavy || valueColor != null ? FontWeight.w700 : FontWeight.w500,
                  color: valueColor ?? ImpactModalColors.ink,
                ),
                child: child,
              ),
            ],
          ),
        ),
      ],
    );
  }
}

/// MethodFooter shows the estimation method and data source below a CalcCard.
class ImpactMethodFooter extends StatelessWidget {
  const ImpactMethodFooter({
    super.key,
    required this.method,
    required this.source,
  });

  final String method;
  final String source;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(top: 14),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              _InfoBadge(),
              const SizedBox(width: 8),
              Expanded(
                child: Text(
                  'Method: $method',
                  style: const TextStyle(
                    fontSize: 12,
                    color: ImpactModalColors.inkFade,
                  ),
                ),
              ),
            ],
          ),
          const SizedBox(height: 4),
          Padding(
            padding: const EdgeInsets.only(left: 22),
            child: Text(
              '· $source',
              style: const TextStyle(
                fontSize: 11.5,
                color: ImpactModalColors.inkFade,
                height: 1.5,
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class _InfoBadge extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    return Container(
      width: 14,
      height: 14,
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        border: Border.all(color: ImpactModalColors.inkFade),
      ),
      child: const Center(
        child: Text(
          'i',
          style: TextStyle(
            fontSize: 9,
            fontStyle: FontStyle.italic,
            color: ImpactModalColors.inkFade,
            height: 1,
          ),
        ),
      ),
    );
  }
}

/// ImpactHint is the dashed-border guidance box shown on the Inputs tab.
class ImpactHint extends StatelessWidget {
  const ImpactHint({
    super.key,
    required this.text,
    this.variant = ImpactHintVariant.coral,
  });

  final String text;
  final ImpactHintVariant variant;

  @override
  Widget build(BuildContext context) {
    final accentColor = variant == ImpactHintVariant.coral
        ? ImpactModalColors.coral
        : ImpactModalColors.green;
    return Container(
      margin: const EdgeInsets.only(top: 14),
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
      decoration: BoxDecoration(
        color: ImpactModalColors.hintBg,
        borderRadius: BorderRadius.circular(10),
        border: Border.all(
          color: ImpactModalColors.calcLine,
          style: BorderStyle.solid,
        ),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text('✎', style: TextStyle(color: accentColor, fontWeight: FontWeight.w700)),
          const SizedBox(width: 8),
          Expanded(
            child: Text(
              text,
              style: const TextStyle(
                fontSize: 12,
                color: ImpactModalColors.inkSoft,
                height: 1.45,
              ),
            ),
          ),
        ],
      ),
    );
  }
}

enum ImpactHintVariant { coral, green }

/// ImpactCTARow shows the Reset button at the bottom of the Inputs tab.
///
/// Inputs auto-save on change; only Reset remains as an explicit affordance
/// to revert any user overrides back to the AI-drafted defaults.
class ImpactCTARow extends StatelessWidget {
  const ImpactCTARow({
    super.key,
    required this.onReset,
    this.isSaving = false,
  });

  final VoidCallback onReset;

  /// When true, an inline progress indicator hints at the in-flight redraft
  /// caused by an auto-saved input. The Reset button stays interactive.
  final bool isSaving;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(top: 16),
      child: Row(
        children: [
          Expanded(
            child: TextButton(
              onPressed: onReset,
              style: TextButton.styleFrom(
                backgroundColor: ImpactModalColors.chipBg,
                foregroundColor: ImpactModalColors.ink,
                padding: const EdgeInsets.symmetric(vertical: 14),
                shape: RoundedRectangleBorder(
                  borderRadius: BorderRadius.circular(14),
                ),
              ),
              child: const Text(
                'Reset',
                style: TextStyle(fontSize: 15, fontWeight: FontWeight.w600),
              ),
            ),
          ),
          if (isSaving) ...[
            const SizedBox(width: 12),
            const SizedBox(
              width: 16,
              height: 16,
              child: CircularProgressIndicator(
                strokeWidth: 2,
                color: ImpactModalColors.inkSoft,
              ),
            ),
          ],
        ],
      ),
    );
  }
}

/// ImpactMetricHeadline shows the hero icon, large value, and label at the top of a modal.
class ImpactMetricHeadline extends StatelessWidget {
  const ImpactMetricHeadline({
    super.key,
    required this.icon,
    required this.value,
    required this.label,
    required this.color,
    required this.bgColor,
  });

  final IconData icon;
  final String value;
  final String label;
  final Color color;
  final Color bgColor;

  @override
  Widget build(BuildContext context) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Container(
          width: 44,
          height: 44,
          decoration: BoxDecoration(
            color: bgColor,
            borderRadius: BorderRadius.circular(10),
          ),
          child: Icon(icon, color: color, size: 22),
        ),
        const SizedBox(width: 14),
        Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              value,
              style: TextStyle(
                fontFamily: AppTheme.headingFont,
                fontSize: 34,
                fontWeight: FontWeight.w500,
                color: color,
                height: 1.05,
                letterSpacing: -0.01 * 34,
              ),
            ),
            const SizedBox(height: 4),
            Text(
              label,
              style: const TextStyle(
                fontSize: 15,
                fontWeight: FontWeight.w600,
                color: ImpactModalColors.ink,
              ),
            ),
          ],
        ),
      ],
    );
  }
}
