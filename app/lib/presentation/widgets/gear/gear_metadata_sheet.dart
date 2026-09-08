import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/gear_metadata_formatter.dart'
    show GearMetadataFormatter;
import 'package:ripls/data/gen/ripls/api/common.pb.dart'
    show
        Estimate,
        MaterialCategory,
        TrackedEstimate,
        TrackedMaterialCategory,
        TrackedString;
import 'package:ripls/data/gen/ripls/api/gear_service.pb.dart'
    show DetectedGearItem, GearMetadata;
import 'package:ripls/data/gen/ripls/api/value.pb.dart' show ValueEstimate;
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/gear/metadata/brand_model_editor.dart';
import 'package:ripls/presentation/widgets/gear/metadata/category_material_editor.dart';
import 'package:ripls/presentation/widgets/gear/metadata/metric_tiles_editor.dart';
import 'package:ripls/presentation/widgets/gear/metadata/save_bar.dart';
import 'package:ripls/presentation/widgets/gear/metadata/url_editor.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass.dart';

export 'package:ripls/presentation/widgets/gear/metadata/url_editor.dart'
    show AutoFillApplied;

/// Result returned from the metadata editing sheet.
class GearMetadataEditResult {
  final GearMetadata metadata;
  final ValueEstimate? valueEstimate;

  /// Field names that the user changed from their initial values.
  ///
  /// Possible values: 'brand', 'model', 'value', 'weight', 'category', 'material'.
  final Set<String> editedFields;

  /// Source URL set by the user or populated via auto-fill.
  final String? sourceUrl;

  const GearMetadataEditResult({
    required this.metadata,
    this.valueEstimate,
    this.editedFields = const {},
    this.sourceUrl,
  });
}

/// Bottom sheet for viewing and editing gear metadata fields.
///
/// Accepts initial values from either [DetectedGearItem] (creation flow) or
/// [GearMetadata] + [ValueEstimate] (detail page). Returns a
/// [GearMetadataEditResult] when the user saves.
///
/// When [readOnly] is true, all fields are disabled and the save bar is hidden.
/// Empty or unspecified fields are hidden in read-only mode. Optionally displays
/// a tappable [sourceUrl] row at the bottom.
///
/// The [onAutoFill] callback is called with the URL the user entered and should
/// return a [DetectedGearItem] with pre-filled fields. It must be wired through
/// the caller's ViewModel/Repository layer — the sheet itself never accesses
/// providers directly.
class GearMetadataSheet extends StatefulWidget {
  final GearMetadata? initialMetadata;
  final ValueEstimate? initialValueEstimate;
  final DetectedGearItem? detectedGear;
  final bool readOnly;
  final String? sourceUrl;
  final Future<DetectedGearItem?> Function(String url)? onAutoFill;

  const GearMetadataSheet({
    super.key,
    this.initialMetadata,
    this.initialValueEstimate,
    this.detectedGear,
    this.readOnly = false,
    this.sourceUrl,
    this.onAutoFill,
  });

  /// Shows the metadata sheet as a modal bottom sheet.
  ///
  /// Returns a [GearMetadataEditResult] if the user saves, or null if dismissed.
  /// When [readOnly] is true the sheet is view-only and always returns null.
  static Future<GearMetadataEditResult?> show(
    BuildContext context, {
    GearMetadata? metadata,
    ValueEstimate? valueEstimate,
    DetectedGearItem? detectedGear,
    bool readOnly = false,
    String? sourceUrl,
    Future<DetectedGearItem?> Function(String url)? onAutoFill,
  }) {
    return showAccessibleModal<GearMetadataEditResult>(context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (_) => GearMetadataSheet(
        initialMetadata: metadata,
        initialValueEstimate: valueEstimate,
        detectedGear: detectedGear,
        readOnly: readOnly,
        sourceUrl: sourceUrl,
        onAutoFill: onAutoFill,
      ),
    );
  }

  @override
  State<GearMetadataSheet> createState() => _GearMetadataSheetState();
}

class _GearMetadataSheetState extends State<GearMetadataSheet> {
  late final TextEditingController _brandController;
  late final TextEditingController _modelController;
  late final TextEditingController _valueController;
  late final TextEditingController _weightController;
  late final TextEditingController _categoryController;
  late final TextEditingController _urlController;
  late MaterialCategory _materialCategory;
  late WeightUnit _weightUnit;

  // Listener shared across all text controllers so it can be removed in dispose.
  late final VoidCallback _textListener;

  bool _isSaved = false;

  // Initial value snapshots for change detection.
  late final String _initialBrand;
  late final String _initialModel;
  late final String _initialValue;
  late final String _initialCategory;
  late final MaterialCategory _initialMaterial;
  late final double _initialWeightGrams;
  late final String _initialUrl;

  @override
  void initState() {
    super.initState();
    _initializeFields();
    _initialBrand = _brandController.text;
    _initialModel = _modelController.text;
    _initialValue = _valueController.text;
    _initialCategory = _categoryController.text;
    _initialMaterial = _materialCategory;
    _initialWeightGrams = _parseWeightGrams();
    _initialUrl = _urlController.text;

    _textListener = () {
      if (mounted) setState(() {});
    };
    _brandController.addListener(_textListener);
    _modelController.addListener(_textListener);
    _valueController.addListener(_textListener);
    _weightController.addListener(_textListener);
    _categoryController.addListener(_textListener);
    _urlController.addListener(_textListener);
  }

  void _initializeFields() {
    if (widget.detectedGear != null) {
      _initFromDetectedGear(widget.detectedGear!);
    } else {
      _initFromMetadata(widget.initialMetadata, widget.initialValueEstimate);
    }
  }

  /// Returns true when the user has made changes that differ from the initial values.
  bool get _hasEdits {
    if (_isSaved) return false;
    if (_brandController.text.trim() != _initialBrand) return true;
    if (_modelController.text.trim() != _initialModel) return true;
    if (_valueController.text.trim() != _initialValue) return true;
    if (_categoryController.text.trim() != _initialCategory) return true;
    if (_materialCategory != _initialMaterial) return true;
    final currentWeightGrams = _parseWeightGrams();
    if ((currentWeightGrams - _initialWeightGrams).abs() > 0.01) return true;
    if (_urlController.text.trim() != _initialUrl) return true;
    return false;
  }

  /// Parses the current weight controller text back to grams for comparison.
  double _parseWeightGrams() {
    final text = _weightController.text.trim();
    if (text.isEmpty) return 0;
    final parsed = double.tryParse(text);
    if (parsed == null || parsed <= 0) return 0;
    switch (_weightUnit) {
      case WeightUnit.kg:
        return parsed * 1000;
      case WeightUnit.lbs:
        return parsed * 453.592;
      case WeightUnit.g:
        return parsed;
    }
  }

  void _initFromDetectedGear(DetectedGearItem gear) {
    _brandController = TextEditingController(
      text:
          GearMetadataFormatter.isUnknownOrEmpty(gear.brand) ? '' : gear.brand,
    );
    _modelController = TextEditingController(
      text:
          GearMetadataFormatter.isUnknownOrEmpty(gear.model) ? '' : gear.model,
    );
    _categoryController = TextEditingController(
      text: GearMetadataFormatter.isUnknownOrEmpty(gear.category)
          ? ''
          : gear.category,
    );
    _materialCategory = gear.materialCategory;
    _urlController = TextEditingController(text: widget.sourceUrl ?? '');

    if (gear.hasValueEstimate() && gear.valueEstimate.estimatedValueUsd > 0) {
      _valueController = TextEditingController(
        text: gear.valueEstimate.estimatedValueUsd.toStringAsFixed(2),
      );
    } else {
      _valueController = TextEditingController();
    }

    if (gear.hasWeightGrams() &&
        gear.weightGrams.hasMean() &&
        gear.weightGrams.mean > 0) {
      final grams = gear.weightGrams.mean;
      _weightUnit = grams >= 1000 ? WeightUnit.kg : WeightUnit.g;
      _weightController = TextEditingController(
        text: _weightUnit == WeightUnit.kg
            ? (grams / 1000).toStringAsFixed(1)
            : grams.toStringAsFixed(0),
      );
    } else {
      _weightUnit = WeightUnit.kg;
      _weightController = TextEditingController();
    }
  }

  void _initFromMetadata(GearMetadata? metadata, ValueEstimate? ve) {
    _brandController = TextEditingController(
      text: GearMetadataFormatter.isUnknownOrEmpty(
              metadata?.brand.value ?? '')
          ? ''
          : (metadata?.brand.value ?? ''),
    );
    _modelController = TextEditingController(
      text: GearMetadataFormatter.isUnknownOrEmpty(
              metadata?.model.value ?? '')
          ? ''
          : (metadata?.model.value ?? ''),
    );
    _categoryController = TextEditingController(
      text: GearMetadataFormatter.isUnknownOrEmpty(
              metadata?.category.value ?? '')
          ? ''
          : (metadata?.category.value ?? ''),
    );
    _materialCategory = metadata?.materialCategory.value ??
        MaterialCategory.MATERIAL_CATEGORY_UNSPECIFIED;
    _urlController = TextEditingController(text: widget.sourceUrl ?? '');

    if (ve != null && ve.estimatedValueUsd > 0) {
      _valueController = TextEditingController(
        text: ve.estimatedValueUsd.toStringAsFixed(2),
      );
    } else {
      _valueController = TextEditingController();
    }

    if (metadata != null &&
        metadata.hasWeightGrams() &&
        metadata.weightGrams.value.hasMean() &&
        metadata.weightGrams.value.mean > 0) {
      final grams = metadata.weightGrams.value.mean;
      _weightUnit = grams >= 1000 ? WeightUnit.kg : WeightUnit.g;
      _weightController = TextEditingController(
        text: _weightUnit == WeightUnit.kg
            ? (grams / 1000).toStringAsFixed(1)
            : grams.toStringAsFixed(0),
      );
    } else {
      _weightUnit = WeightUnit.kg;
      _weightController = TextEditingController();
    }
  }

  @override
  void dispose() {
    _brandController.removeListener(_textListener);
    _modelController.removeListener(_textListener);
    _valueController.removeListener(_textListener);
    _weightController.removeListener(_textListener);
    _categoryController.removeListener(_textListener);
    _urlController.removeListener(_textListener);
    _brandController.dispose();
    _modelController.dispose();
    _valueController.dispose();
    _weightController.dispose();
    _categoryController.dispose();
    _urlController.dispose();
    super.dispose();
  }

  GearMetadataEditResult _buildResult() {
    final metadata = GearMetadata(
      brand: TrackedString(value: _brandController.text.trim()),
      model: TrackedString(value: _modelController.text.trim()),
      category: TrackedString(value: _categoryController.text.trim()),
      materialCategory: TrackedMaterialCategory(value: _materialCategory),
    );

    final weightText = _weightController.text.trim();
    if (weightText.isNotEmpty) {
      final parsed = double.tryParse(weightText);
      if (parsed != null && parsed > 0) {
        final double grams;
        switch (_weightUnit) {
          case WeightUnit.kg:
            grams = parsed * 1000;
          case WeightUnit.lbs:
            grams = parsed * 453.592;
          case WeightUnit.g:
            grams = parsed;
        }
        metadata.weightGrams = TrackedEstimate(value: Estimate(mean: grams));
      }
    }

    ValueEstimate? valueEstimate;
    final valueText = _valueController.text.trim();
    if (valueText.isNotEmpty) {
      final parsed = double.tryParse(valueText);
      if (parsed != null && parsed > 0) {
        valueEstimate = ValueEstimate(estimatedValueUsd: parsed);
      }
    }

    final editedFields = <String>{};
    if (_brandController.text.trim() != _initialBrand) {
      editedFields.add('brand');
    }
    if (_modelController.text.trim() != _initialModel) {
      editedFields.add('model');
    }
    if (_valueController.text.trim() != _initialValue) {
      editedFields.add('value');
    }
    if (_categoryController.text.trim() != _initialCategory) {
      editedFields.add('category');
    }
    if (_materialCategory != _initialMaterial) editedFields.add('material');
    if ((_parseWeightGrams() - _initialWeightGrams).abs() > 0.01) {
      editedFields.add('weight');
    }

    final urlText = _urlController.text.trim();
    return GearMetadataEditResult(
      metadata: metadata,
      valueEstimate: valueEstimate,
      editedFields: editedFields,
      sourceUrl: urlText.isNotEmpty ? urlText : null,
    );
  }

  void _handleSave() {
    setState(() => _isSaved = true);
    Future.delayed(const Duration(seconds: 2), () {
      if (!mounted) return;
      Navigator.of(context).pop(_buildResult());
    });
  }

  void _applyAutoFill({
    String? brand,
    String? model,
    String? category,
    MaterialCategory? materialCategory,
    double? estimatedValueUsd,
    double? weightGrams,
  }) {
    setState(() {
      if (brand != null) _brandController.text = brand;
      if (model != null) _modelController.text = model;
      if (category != null) _categoryController.text = category;
      if (materialCategory != null) _materialCategory = materialCategory;
      if (estimatedValueUsd != null) {
        _valueController.text = estimatedValueUsd.toStringAsFixed(2);
      }
      if (weightGrams != null) {
        _weightUnit = weightGrams >= 1000 ? WeightUnit.kg : WeightUnit.g;
        _weightController.text = _weightUnit == WeightUnit.kg
            ? (weightGrams / 1000).toStringAsFixed(1)
            : weightGrams.toStringAsFixed(0);
      }
    });
  }

  TextStyle _sectionLabelStyle() => TextStyle(
        fontSize: 10,
        fontWeight: FontWeight.w800,
        letterSpacing: 1.5,
        color: AppColors.modalTextMuted,
      );

  // ─── Build ───────────────────────────────────────────────────────────────────

  @override
  Widget build(BuildContext context) {
    final bottomInset = MediaQuery.of(context).viewInsets.bottom;
    final readOnly = widget.readOnly;
    final hasUrl = _urlController.text.trim().isNotEmpty;
    final labelStyle = _sectionLabelStyle();

    return GlassSheet(
      padding: EdgeInsets.zero,
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Flexible(
            child: SingleChildScrollView(
              padding: EdgeInsets.only(bottom: readOnly ? 24 : 48),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  BrandModelEditor(
                    brandController: _brandController,
                    modelController: _modelController,
                    readOnly: readOnly,
                    sectionLabelStyle: labelStyle,
                  ),
                  CategoryMaterialEditor(
                    categoryController: _categoryController,
                    materialCategory: _materialCategory,
                    readOnly: readOnly,
                    sectionLabelStyle: labelStyle,
                    onCategoryChanged: (cat) =>
                        setState(() => _categoryController.text = cat),
                    onMaterialChanged: (mat) =>
                        setState(() => _materialCategory = mat),
                  ),
                  MetricTilesEditor(
                    valueController: _valueController,
                    weightController: _weightController,
                    weightUnit: _weightUnit,
                    readOnly: readOnly,
                    sectionLabelStyle: labelStyle,
                    onWeightUnitChanged: (unit) =>
                        setState(() => _weightUnit = unit),
                  ),
                  if (!readOnly || hasUrl)
                    UrlEditor(
                      urlController: _urlController,
                      readOnly: readOnly,
                      onAutoFill: widget.onAutoFill,
                      onAutoFillApplied: _applyAutoFill,
                    ),
                ],
              ),
            ),
          ),
          if (!readOnly)
            MetadataSaveBar(
              hasEdits: _hasEdits,
              isSaved: _isSaved,
              bottomInset: bottomInset,
              onSave: _handleSave,
            ),
        ],
      ),
    );
  }
}
