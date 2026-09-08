import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/viewmodels/unified_create_state.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/toggle.dart';

export 'package:ripls/presentation/viewmodels/unified_create_state.dart' show ItemDetailsValue;

const List<String> _weightUnits = ['g', 'kg', 'lbs'];

String _localizedWeightUnit(AppLocalizations l10n, String unit) {
  switch (unit) {
    case 'g':
      return l10n.unifiedCreateWeightUnitG;
    case 'lbs':
      return l10n.unifiedCreateWeightUnitLbs;
    case 'kg':
    default:
      return l10n.unifiedCreateWeightUnitKg;
  }
}

/// Glass push-sheet for gear item details (brand, model, est. value,
/// material, weight). Pushed from the preview card's item-details row
/// when type=Item. The Lend/Give toggle is not in this sheet — it lives
/// in the preview card itself (P1.7).
///
/// See `docs/design/unified-create.md` § UX overview.
class UnifiedItemDetailsSheet extends StatefulWidget {
  const UnifiedItemDetailsSheet({super.key, this.initial});

  final ItemDetailsValue? initial;

  static Future<ItemDetailsValue?> show(
    BuildContext context, {
    ItemDetailsValue? initial,
  }) {
    return showAccessibleModal<ItemDetailsValue>(
      context,
      isScrollControlled: true,
      builder: (_) => UnifiedItemDetailsSheet(initial: initial),
    );
  }

  @override
  State<UnifiedItemDetailsSheet> createState() => _UnifiedItemDetailsSheetState();
}

class _UnifiedItemDetailsSheetState extends State<UnifiedItemDetailsSheet> {
  late final TextEditingController _brand;
  late final TextEditingController _model;
  late final TextEditingController _value;
  late final TextEditingController _material;
  late final TextEditingController _weight;
  late String _weightUnit;

  @override
  void initState() {
    super.initState();
    final init = widget.initial;
    _brand = TextEditingController(text: init?.brand ?? '');
    _model = TextEditingController(text: init?.model ?? '');
    _value = TextEditingController(text: init?.estValueUsd ?? '');
    _material = TextEditingController(text: init?.material ?? '');
    _weight = TextEditingController(text: init?.weight ?? '');
    final initialUnit = init?.weightUnit;
    _weightUnit = (initialUnit != null && _weightUnits.contains(initialUnit))
        ? initialUnit
        : 'kg';
  }

  @override
  void dispose() {
    _brand.dispose();
    _model.dispose();
    _value.dispose();
    _material.dispose();
    _weight.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: EdgeInsets.only(
        left: 24,
        right: 24,
        top: 24,
        bottom: MediaQuery.of(context).viewInsets.bottom + 24,
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Text(context.l10n.unifiedCreateItemDetails,
              style:
                  const TextStyle(fontSize: 18, fontWeight: FontWeight.bold)),
          const SizedBox(height: 16),
          _row(context.l10n.unifiedCreateItemBrand, _brand),
          _row(context.l10n.unifiedCreateItemModel, _model),
          _row(context.l10n.unifiedCreateItemValue, _value),
          _row(context.l10n.unifiedCreateItemMaterial, _material),
          _weightRow(context),
          const SizedBox(height: 16),
          FilledButton(
            onPressed: () {
              Navigator.of(context).pop(ItemDetailsValue(
                brand: _brand.text,
                model: _model.text,
                estValueUsd: _value.text,
                material: _material.text,
                weight: _weight.text,
                weightUnit: _weightUnit,
              ));
            },
            child: Text(context.l10n.commonDone),
          ),
        ],
      ),
    );
  }

  Widget _row(String label, TextEditingController c) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 6),
      child: Row(
        children: [
          SizedBox(
              width: 90,
              child: Text(label,
                  style: const TextStyle(fontWeight: FontWeight.w500))),
          Expanded(
            child: TextField(
              controller: c,
              textCapitalization: TextCapitalization.sentences,
              decoration: const InputDecoration(hintText: '—', isDense: true),
            ),
          ),
        ],
      ),
    );
  }

  Widget _weightRow(BuildContext context) {
    final l10n = context.l10n;
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 6),
      child: Row(
        children: [
          SizedBox(
              width: 90,
              child: Text(l10n.unifiedCreateItemWeight,
                  style: const TextStyle(fontWeight: FontWeight.w500))),
          Expanded(
            child: TextField(
              controller: _weight,
              keyboardType:
                  const TextInputType.numberWithOptions(decimal: true),
              decoration: const InputDecoration(hintText: '—', isDense: true),
            ),
          ),
          const SizedBox(width: 8),
          _UnitToggleGroup(
            units: _weightUnits,
            selected: _weightUnit,
            onSelected: (u) => setState(() => _weightUnit = u),
          ),
        ],
      ),
    );
  }
}

class _UnitToggleGroup extends StatelessWidget {
  final List<String> units;
  final String selected;
  final ValueChanged<String> onSelected;

  const _UnitToggleGroup({
    required this.units,
    required this.selected,
    required this.onSelected,
  });

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        for (final unit in units) ...[
          Toggle(
            semanticsLabel: _localizedWeightUnit(l10n, unit),
            selected: unit == selected,
            onTap: () => onSelected(unit),
            inkBorderRadius: BorderRadius.circular(8),
            child: Container(
              padding:
                  const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
              decoration: BoxDecoration(
                borderRadius: BorderRadius.circular(8),
                color: unit == selected
                    ? Theme.of(context).colorScheme.primary.withValues(alpha: 0.15)
                    : Colors.transparent,
              ),
              child: Text(
                _localizedWeightUnit(l10n, unit),
                style: TextStyle(
                  fontSize: 12,
                  fontWeight: unit == selected
                      ? FontWeight.w700
                      : FontWeight.w400,
                ),
              ),
            ),
          ),
          if (unit != units.last) const SizedBox(width: 2),
        ],
      ],
    );
  }
}
