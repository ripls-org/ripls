import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/paper_tokens.gen.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/accessibility/toggle.dart';

/// ImpactSelectChip renders an inline labelled selector styled to match the
/// CalcCard Inputs tab design. Tapping opens a bottom-drawer picker — no
/// platform DropdownButton so the appearance stays consistent.
class ImpactSelectChip<T> extends StatelessWidget {
  const ImpactSelectChip({
    super.key,
    required this.value,
    required this.onChanged,
    required this.items,
  });

  final T value;
  final ValueChanged<T> onChanged;
  final List<ImpactSelectItem<T>> items;

  String get _currentLabel =>
      items.firstWhere((i) => i.value == value, orElse: () => items.first).label;

  void _openPicker(BuildContext context) {
    showAccessibleModal<void>(
      context,
      backgroundColor: Colors.transparent,
      builder: (_) => _SelectPickerSheet<T>(
        items: items,
        selected: value,
        onSelected: (v) {
          Navigator.of(context).pop();
          onChanged(v);
        },
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: _currentLabel,
      onTap: () => _openPicker(context),
      child: Container(
        height: 36,
        decoration: BoxDecoration(
          color: ImpactModalColors.inputFieldBg,
          borderRadius: BorderRadius.circular(999),
          border: Border.all(color: ImpactModalColors.inputBorder),
        ),
        padding: const EdgeInsets.fromLTRB(14, 0, 10, 0),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(
              _currentLabel,
              style: const TextStyle(
                fontSize: 15,
                fontWeight: FontWeight.w700,
                color: ImpactModalColors.ink,
              ),
            ),
            const SizedBox(width: 4),
            const Icon(
              Icons.arrow_drop_down,
              size: 18,
              color: ImpactModalColors.inkSoft,
            ),
          ],
        ),
      ),
    );
  }
}

class _SelectPickerSheet<T> extends StatelessWidget {
  const _SelectPickerSheet({
    required this.items,
    required this.selected,
    required this.onSelected,
  });

  final List<ImpactSelectItem<T>> items;
  final T selected;
  final ValueChanged<T> onSelected;

  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: const BoxDecoration(
        color: ImpactModalColors.sheetBg,
        borderRadius: BorderRadius.vertical(top: Radius.circular(18)),
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
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
          ...items.map((item) {
            final isSelected = item.value == selected;
            return Toggle(
              semanticsLabel: item.label,
              selected: isSelected,
              onTap: () => onSelected(item.value),
              child: Padding(
                padding: const EdgeInsets.symmetric(horizontal: 22, vertical: 14),
                child: Row(
                  children: [
                    Expanded(
                      child: Text(
                        item.label,
                        style: TextStyle(
                          fontSize: 15,
                          fontWeight:
                              isSelected ? FontWeight.w600 : FontWeight.w400,
                          color: isSelected
                              ? ImpactModalColors.ink
                              : ImpactModalColors.inkSoft,
                        ),
                      ),
                    ),
                    if (isSelected)
                      const Icon(
                        Icons.check,
                        size: 18,
                        color: ImpactModalColors.green,
                      ),
                  ],
                ),
              ),
            );
          }),
          SizedBox(height: MediaQuery.of(context).padding.bottom + 8),
        ],
      ),
    );
  }
}

class ImpactSelectItem<T> {
  const ImpactSelectItem({required this.value, required this.label});

  final T value;
  final String label;
}
