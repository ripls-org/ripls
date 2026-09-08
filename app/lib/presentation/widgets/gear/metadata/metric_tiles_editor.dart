import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';

/// Weight display unit for gear metadata.
enum WeightUnit { g, kg, lbs }

/// Displays or edits the estimated value and weight fields side by side.
class MetricTilesEditor extends StatelessWidget {
  final TextEditingController valueController;
  final TextEditingController weightController;
  final WeightUnit weightUnit;
  final bool readOnly;
  final TextStyle sectionLabelStyle;
  final ValueChanged<WeightUnit> onWeightUnitChanged;

  const MetricTilesEditor({
    super.key,
    required this.valueController,
    required this.weightController,
    required this.weightUnit,
    required this.readOnly,
    required this.sectionLabelStyle,
    required this.onWeightUnitChanged,
  });

  @override
  Widget build(BuildContext context) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Expanded(
          child: _ValueTile(
            controller: valueController,
            readOnly: readOnly,
            sectionLabelStyle: sectionLabelStyle,
          ),
        ),
        Expanded(
          child: _WeightTile(
            controller: weightController,
            weightUnit: weightUnit,
            readOnly: readOnly,
            sectionLabelStyle: sectionLabelStyle,
            onWeightUnitChanged: onWeightUnitChanged,
          ),
        ),
      ],
    );
  }
}

class _ValueTile extends StatelessWidget {
  final TextEditingController controller;
  final bool readOnly;
  final TextStyle sectionLabelStyle;

  const _ValueTile({
    required this.controller,
    required this.readOnly,
    required this.sectionLabelStyle,
  });

  @override
  Widget build(BuildContext context) {
    final hasValue = controller.text.isNotEmpty;
    final valueStyle = TextStyle(
      fontSize: 16,
      fontWeight: FontWeight.w700,
      color: AppColors.modalTextPrimary,
    );
    final prefixStyle = valueStyle.copyWith(
      fontSize: 16,
      color: AppColors.modalTextMuted,
    );

    final Widget valueContent;
    if (readOnly) {
      valueContent = Row(
        crossAxisAlignment: CrossAxisAlignment.baseline,
        textBaseline: TextBaseline.alphabetic,
        children: [
          Text('\$', style: prefixStyle),
          Text(hasValue ? controller.text : '--', style: valueStyle),
        ],
      );
    } else {
      valueContent = Row(
        crossAxisAlignment: CrossAxisAlignment.baseline,
        textBaseline: TextBaseline.alphabetic,
        children: [
          Text('\$', style: prefixStyle),
          Expanded(
            child: TextField(
              controller: controller,
              keyboardType:
                  const TextInputType.numberWithOptions(decimal: true),
              style: valueStyle,
              textInputAction: TextInputAction.next,
              decoration: InputDecoration(
                hintText: '0',
                hintStyle: valueStyle.copyWith(
                  color: GlassTokens.textFaint,
                ),
                filled: true,
                fillColor: Colors.transparent,
                border: UnderlineInputBorder(
                  borderSide: BorderSide(color: AppColors.modalInsetCardBorder),
                ),
                enabledBorder: UnderlineInputBorder(
                  borderSide: BorderSide(color: AppColors.modalInsetCardBorder),
                ),
                focusedBorder: UnderlineInputBorder(
                  borderSide:
                      BorderSide(color: AppColors.primary(context), width: 2),
                ),
                isDense: true,
                contentPadding: const EdgeInsets.only(bottom: 4),
              ),
            ),
          ),
        ],
      );
    }

    return Container(
      padding: const EdgeInsets.fromLTRB(20, 20, 16, 20),
      decoration: BoxDecoration(
        border: Border(
          right: BorderSide(color: AppColors.transferBorderLight(context)),
          bottom: BorderSide(color: AppColors.transferBorderLight(context)),
        ),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text('EST. VALUE', style: sectionLabelStyle),
          const SizedBox(height: 8),
          valueContent,
        ],
      ),
    );
  }
}

class _WeightTile extends StatelessWidget {
  final TextEditingController controller;
  final WeightUnit weightUnit;
  final bool readOnly;
  final TextStyle sectionLabelStyle;
  final ValueChanged<WeightUnit> onWeightUnitChanged;

  const _WeightTile({
    required this.controller,
    required this.weightUnit,
    required this.readOnly,
    required this.sectionLabelStyle,
    required this.onWeightUnitChanged,
  });

  String get _unitLabel => switch (weightUnit) {
        WeightUnit.kg => 'kg',
        WeightUnit.lbs => 'lbs',
        WeightUnit.g => 'g',
      };

  @override
  Widget build(BuildContext context) {
    final hasWeight = controller.text.isNotEmpty;
    final valueStyle = TextStyle(
      fontSize: 16,
      fontWeight: FontWeight.w700,
      color: AppColors.modalTextPrimary,
    );
    final suffixStyle = valueStyle.copyWith(
      fontSize: 16,
      color: AppColors.modalTextMuted,
    );

    final Widget valueContent;
    if (readOnly) {
      valueContent = Row(
        crossAxisAlignment: CrossAxisAlignment.baseline,
        textBaseline: TextBaseline.alphabetic,
        children: [
          Text(hasWeight ? controller.text : '--', style: valueStyle),
          if (hasWeight) ...[
            const SizedBox(width: 4),
            Text(_unitLabel, style: suffixStyle),
          ],
        ],
      );
    } else {
      valueContent = Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          TextField(
            controller: controller,
            keyboardType:
                const TextInputType.numberWithOptions(decimal: true),
            style: valueStyle,
            textInputAction: TextInputAction.done,
            decoration: InputDecoration(
              hintText: '0',
              hintStyle: valueStyle.copyWith(
                color: GlassTokens.textFaint,
              ),
              filled: true,
              fillColor: Colors.transparent,
              border: UnderlineInputBorder(
                borderSide: BorderSide(color: AppColors.modalInsetCardBorder),
              ),
              enabledBorder: UnderlineInputBorder(
                borderSide: BorderSide(color: AppColors.modalInsetCardBorder),
              ),
              focusedBorder: UnderlineInputBorder(
                borderSide:
                    BorderSide(color: AppColors.primary(context), width: 2),
              ),
              isDense: true,
              contentPadding: const EdgeInsets.only(bottom: 4),
            ),
          ),
          const SizedBox(height: 10),
          _WeightUnitToggle(
            weightUnit: weightUnit,
            controller: controller,
            onWeightUnitChanged: onWeightUnitChanged,
          ),
        ],
      );
    }

    return Container(
      padding: const EdgeInsets.fromLTRB(20, 20, 20, 20),
      decoration: BoxDecoration(
        border: Border(
          bottom: BorderSide(color: AppColors.transferBorderLight(context)),
        ),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text('WEIGHT', style: sectionLabelStyle),
          const SizedBox(height: 8),
          valueContent,
        ],
      ),
    );
  }
}

class _WeightUnitToggle extends StatelessWidget {
  final WeightUnit weightUnit;
  final TextEditingController controller;
  final ValueChanged<WeightUnit> onWeightUnitChanged;

  const _WeightUnitToggle({
    required this.weightUnit,
    required this.controller,
    required this.onWeightUnitChanged,
  });

  void _handleUnitChange(int index) {
    final newUnit = WeightUnit.values[index];
    if (newUnit == weightUnit) return;
    final currentText = controller.text.trim();
    final parsed = double.tryParse(currentText);
    if (parsed != null && parsed > 0) {
      final double grams;
      switch (weightUnit) {
        case WeightUnit.kg:
          grams = parsed * 1000;
        case WeightUnit.lbs:
          grams = parsed * 453.592;
        case WeightUnit.g:
          grams = parsed;
      }
      switch (newUnit) {
        case WeightUnit.kg:
          controller.text = (grams / 1000).toStringAsFixed(1);
        case WeightUnit.lbs:
          controller.text = (grams / 453.592).toStringAsFixed(1);
        case WeightUnit.g:
          controller.text = grams.toStringAsFixed(0);
      }
    }
    onWeightUnitChanged(newUnit);
  }

  @override
  Widget build(BuildContext context) {
    return ToggleButtons(
      isSelected: [
        weightUnit == WeightUnit.g,
        weightUnit == WeightUnit.kg,
        weightUnit == WeightUnit.lbs,
      ],
      onPressed: _handleUnitChange,
      borderRadius: BorderRadius.circular(8),
      constraints: const BoxConstraints(minWidth: 44, minHeight: 34),
      selectedColor: AppColors.modalTextPrimary,
      color: AppColors.modalTextMuted,
      fillColor: AppColors.modalInsetCardBg,
      borderColor: AppColors.border(context),
      selectedBorderColor: AppColors.transferBorder(context),
      children: [
        _UnitLabel(label: 'g', selected: weightUnit == WeightUnit.g),
        _UnitLabel(label: 'kg', selected: weightUnit == WeightUnit.kg),
        _UnitLabel(label: 'lbs', selected: weightUnit == WeightUnit.lbs),
      ],
    );
  }
}

class _UnitLabel extends StatelessWidget {
  final String label;
  final bool selected;

  const _UnitLabel({required this.label, required this.selected});

  @override
  Widget build(BuildContext context) {
    return Text(
      label,
      style: TextStyle(
        fontSize: 12,
        fontWeight: selected ? FontWeight.w700 : FontWeight.w400,
        color: selected
            ? AppColors.modalTextPrimary
            : AppColors.modalTextMuted,
      ),
    );
  }
}
