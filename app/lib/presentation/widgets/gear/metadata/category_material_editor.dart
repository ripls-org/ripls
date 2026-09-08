import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/gear_metadata_formatter.dart'
    show GearMetadataFormatter;
import 'package:ripls/data/gen/ripls/api/common.pb.dart'
    show MaterialCategory;
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass.dart';

/// Displays or edits the category and material fields side by side.
///
/// In edit mode each tile is tappable and opens a picker sheet. In read-only
/// mode tiles with no value are hidden.
class CategoryMaterialEditor extends StatelessWidget {
  final TextEditingController categoryController;
  final MaterialCategory materialCategory;
  final bool readOnly;
  final TextStyle sectionLabelStyle;
  final ValueChanged<String> onCategoryChanged;
  final ValueChanged<MaterialCategory> onMaterialChanged;

  const CategoryMaterialEditor({
    super.key,
    required this.categoryController,
    required this.materialCategory,
    required this.readOnly,
    required this.sectionLabelStyle,
    required this.onCategoryChanged,
    required this.onMaterialChanged,
  });

  @override
  Widget build(BuildContext context) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Expanded(
          child: _CategoryTile(
            controller: categoryController,
            readOnly: readOnly,
            sectionLabelStyle: sectionLabelStyle,
            onCategoryChanged: onCategoryChanged,
          ),
        ),
        Expanded(
          child: _MaterialTile(
            materialCategory: materialCategory,
            readOnly: readOnly,
            sectionLabelStyle: sectionLabelStyle,
            onMaterialChanged: onMaterialChanged,
          ),
        ),
      ],
    );
  }
}

class _CategoryTile extends StatelessWidget {
  final TextEditingController controller;
  final bool readOnly;
  final TextStyle sectionLabelStyle;
  final ValueChanged<String> onCategoryChanged;

  const _CategoryTile({
    required this.controller,
    required this.readOnly,
    required this.sectionLabelStyle,
    required this.onCategoryChanged,
  });

  static const _categories = [
    'Power Tools',
    'Hand Tools',
    'Outdoor & Garden',
    'Kitchen',
    'Sports & Fitness',
    'Electronics',
    'Furniture',
    'Camping',
    'Cleaning',
    'Other',
  ];

  void _showPicker(BuildContext context) {
    showAccessibleModal<void>(context,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      isScrollControlled: true,
      builder: (ctx) => GlassSheet(
        applyMaxHeight: false,
        child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Padding(
                padding: const EdgeInsets.fromLTRB(20, 0, 20, 8),
                child: Text(
                  'Category',
                  style: TextStyle(
                    fontSize: 17,
                    fontWeight: FontWeight.w600,
                    color: AppColors.modalTextPrimary,
                  ),
                ),
              ),
              Flexible(
                child: ListView(
                  shrinkWrap: true,
                  children: _categories
                      .map(
                        (cat) => ListTile(
                          title: Text(
                            cat,
                            style: TextStyle(
                              fontSize: 15,
                              color: AppColors.modalTextPrimary,
                              fontWeight: controller.text == cat
                                  ? FontWeight.w600
                                  : FontWeight.normal,
                            ),
                          ),
                          trailing: controller.text == cat
                              ? Icon(
                                  Icons.check,
                                  color: AppColors.modalPrimaryButtonBackground,
                                  size: 18,
                                )
                              : null,
                          onTap: () {
                            onCategoryChanged(cat);
                            Navigator.of(ctx).pop();
                          },
                        ),
                      )
                      .toList(),
                ),
              ),
              const SizedBox(height: 8),
            ],
          ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final hasCategory = controller.text.isNotEmpty;
    if (readOnly && !hasCategory) return const SizedBox.shrink();

    final valueStyle = TextStyle(
      fontSize: 16,
      fontWeight: FontWeight.w700,
      color: AppColors.modalTextPrimary,
    );
    final caretStyle = TextStyle(
      fontSize: 12,
      color: AppColors.modalTextMuted,
    );

    final content = Container(
      padding: const EdgeInsets.fromLTRB(20, 16, 16, 16),
      decoration: BoxDecoration(
        border: Border(
          right: BorderSide(color: AppColors.modalInsetCardBorder),
          bottom: BorderSide(color: AppColors.transferBorderLight(context)),
        ),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text('CATEGORY', style: sectionLabelStyle),
          const SizedBox(height: 8),
          Row(
            children: [
              Expanded(
                child: Text(
                  hasCategory ? controller.text : '--',
                  style: valueStyle,
                ),
              ),
              if (!readOnly) Text(' ↓', style: caretStyle),
            ],
          ),
        ],
      ),
    );

    if (readOnly) return content;
    return Tappable(
      semanticsLabel: context.l10n.a11yGearPickCategory,
      onTap: () => _showPicker(context),
      child: content,
    );
  }
}

class _MaterialTile extends StatelessWidget {
  final MaterialCategory materialCategory;
  final bool readOnly;
  final TextStyle sectionLabelStyle;
  final ValueChanged<MaterialCategory> onMaterialChanged;

  const _MaterialTile({
    required this.materialCategory,
    required this.readOnly,
    required this.sectionLabelStyle,
    required this.onMaterialChanged,
  });

  void _showPicker(BuildContext context) {
    final categories = MaterialCategory.values
        .where((c) => c != MaterialCategory.MATERIAL_CATEGORY_UNSPECIFIED)
        .toList();

    showAccessibleModal<void>(context,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      isScrollControlled: true,
      builder: (ctx) => GlassSheet(
        applyMaxHeight: false,
        child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Padding(
                padding: const EdgeInsets.fromLTRB(20, 0, 20, 8),
                child: Text(
                  'Material',
                  style: TextStyle(
                    fontSize: 17,
                    fontWeight: FontWeight.w600,
                    color: AppColors.modalTextPrimary,
                  ),
                ),
              ),
              Flexible(
                child: ListView(
                  shrinkWrap: true,
                  children: categories.map((cat) {
                    final label =
                        GearMetadataFormatter.formatMaterialCategory(cat);
                    return ListTile(
                      title: Text(
                        label,
                        style: TextStyle(
                          fontSize: 15,
                          color: AppColors.modalTextPrimary,
                          fontWeight: materialCategory == cat
                              ? FontWeight.w600
                              : FontWeight.normal,
                        ),
                      ),
                      trailing: materialCategory == cat
                          ? Icon(
                              Icons.check,
                              color: AppColors.modalPrimaryButtonBackground,
                              size: 18,
                            )
                          : null,
                      onTap: () {
                        onMaterialChanged(cat);
                        Navigator.of(ctx).pop();
                      },
                    );
                  }).toList(),
                ),
              ),
              const SizedBox(height: 8),
            ],
          ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final hasMaterial =
        materialCategory != MaterialCategory.MATERIAL_CATEGORY_UNSPECIFIED;
    if (readOnly && !hasMaterial) return const SizedBox.shrink();

    final valueStyle = TextStyle(
      fontSize: 16,
      fontWeight: FontWeight.w700,
      color: AppColors.modalTextPrimary,
    );
    final caretStyle = TextStyle(
      fontSize: 12,
      color: AppColors.modalTextMuted,
    );
    final materialLabel = hasMaterial
        ? GearMetadataFormatter.formatMaterialCategory(materialCategory)
        : '--';

    final content = Container(
      padding: const EdgeInsets.fromLTRB(20, 16, 20, 16),
      decoration: BoxDecoration(
        border: Border(
          bottom: BorderSide(color: AppColors.transferBorderLight(context)),
        ),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text('MATERIAL', style: sectionLabelStyle),
          const SizedBox(height: 8),
          Row(
            children: [
              Expanded(child: Text(materialLabel, style: valueStyle)),
              if (!readOnly) Text(' ↓', style: caretStyle),
            ],
          ),
        ],
      ),
    );

    if (readOnly) return content;
    return Tappable(
      semanticsLabel: context.l10n.a11yGearPickMaterial,
      onTap: () => _showPicker(context),
      child: content,
    );
  }
}
