import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';

/// Displays or edits the brand and model fields of a gear item.
///
/// In edit mode renders underline TextFields; in read-only mode renders plain
/// Text widgets. The model row is hidden when read-only and empty.
class BrandModelEditor extends StatelessWidget {
  final TextEditingController brandController;
  final TextEditingController modelController;
  final bool readOnly;
  final TextStyle sectionLabelStyle;

  const BrandModelEditor({
    super.key,
    required this.brandController,
    required this.modelController,
    required this.readOnly,
    required this.sectionLabelStyle,
  });

  @override
  Widget build(BuildContext context) {
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.fromLTRB(20, 16, 20, 20),
      decoration: BoxDecoration(
        border: Border(
          bottom: BorderSide(color: AppColors.transferBorderLight(context)),
        ),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text('BRAND', style: sectionLabelStyle),
          const SizedBox(height: 8),
          _BrandField(controller: brandController, readOnly: readOnly),
          const SizedBox(height: 16),
          Text('MODEL', style: sectionLabelStyle),
          const SizedBox(height: 8),
          _ModelField(controller: modelController, readOnly: readOnly),
        ],
      ),
    );
  }
}

class _BrandField extends StatelessWidget {
  final TextEditingController controller;
  final bool readOnly;

  const _BrandField({required this.controller, required this.readOnly});

  @override
  Widget build(BuildContext context) {
    final valueStyle = TextStyle(
      fontSize: 34,
      fontWeight: FontWeight.w700,
      color: AppColors.modalTextPrimary,
      height: 1.1,
    );

    if (readOnly) {
      final text = controller.text.isNotEmpty ? controller.text : '--';
      return Text(text, style: valueStyle);
    }

    return TextField(
      controller: controller,
      style: valueStyle,
      textInputAction: TextInputAction.next,
      decoration: InputDecoration(
        hintText: 'Brand name',
        hintStyle: valueStyle.copyWith(
          color: AppColors.modalTextMuted.withValues(alpha: 0.4),
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
          borderSide: BorderSide(color: AppColors.primary(context), width: 2),
        ),
        isDense: true,
        contentPadding: const EdgeInsets.only(bottom: 6),
      ),
    );
  }
}

class _ModelField extends StatelessWidget {
  final TextEditingController controller;
  final bool readOnly;

  const _ModelField({required this.controller, required this.readOnly});

  @override
  Widget build(BuildContext context) {
    final valueStyle = TextStyle(
      fontSize: 16,
      fontWeight: FontWeight.w700,
      color: AppColors.modalTextPrimary,
    );

    if (readOnly) {
      if (controller.text.isEmpty) return const SizedBox.shrink();
      return Text(controller.text, style: valueStyle);
    }

    return TextField(
      controller: controller,
      style: valueStyle,
      textInputAction: TextInputAction.next,
      decoration: InputDecoration(
        hintText: 'Model number',
        hintStyle: valueStyle.copyWith(
          color: AppColors.modalTextMuted.withValues(alpha: 0.4),
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
          borderSide: BorderSide(color: AppColors.primary(context), width: 2),
        ),
        isDense: true,
        contentPadding: const EdgeInsets.only(bottom: 6),
      ),
    );
  }
}
