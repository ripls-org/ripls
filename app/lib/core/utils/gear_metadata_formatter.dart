import 'package:ripls/core/utils/savings_formatter.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart';
import 'package:ripls/data/gen/ripls/api/gear_service.pb.dart';
import 'package:ripls/presentation/widgets/item/item_metric_data.dart';

/// Utility class for formatting gear metadata from proto messages.
///
/// Pure utility with static methods (no state, no dependencies).
/// Formats AI-detected product metadata for display in ItemImpactCard.
class GearMetadataFormatter {
  static const String _notAvailable = '--';

  /// Returns true if a string field should be treated as absent.
  ///
  /// Filters empty strings, LLM sentinel values like `<UNKNOWN>`, `UNKNOWN`,
  /// `<unknown>`, and any angle-bracket-wrapped sentinel from AI detection.
  static bool isUnknownOrEmpty(String value) {
    if (value.isEmpty) return true;
    final trimmed = value.trim();
    if (trimmed.isEmpty) return true;
    final upper = trimmed.toUpperCase();
    if (upper == 'UNKNOWN') return true;
    // Matches any <...> angle-bracket sentinel (e.g. <UNKNOWN>, <N/A>).
    if (trimmed.startsWith('<') && trimmed.endsWith('>')) return true;
    return false;
  }

  /// Converts GearMetadata proto to a list of ItemMetricValue display models.
  ///
  /// Returns an empty list if metadata is null. Each metadata field becomes
  /// a separate metric that can be tapped to show details.
  ///
  /// The returned metrics are:
  /// - Category: Product category (e.g., "Power Tools")
  /// - Brand: Brand name (e.g., "DeWalt")
  /// - Model: Model identifier (e.g., "DCD771C2")
  /// - Material: Material composition (e.g., "Mixed Plastic-Metal")
  /// - Weight: Estimated weight with auto-scaling (e.g., "2.3 kg", "350 g")
  /// - Embodied Carbon: CO2e to manufacture (e.g., "18.4 kg CO₂")
  static List<ItemMetricValue> formatMetadata(GearMetadata? metadata) {
    if (metadata == null) return [];

    final metrics = <ItemMetricValue>[];

    // Category
    metrics.add(_buildCategoryMetric(metadata));

    // Brand
    metrics.add(_buildBrandMetric(metadata));

    // Model
    metrics.add(_buildModelMetric(metadata));

    // Material
    metrics.add(_buildMaterialMetric(metadata));

    // Weight
    metrics.add(_buildWeightMetric(metadata));

    // Embodied Carbon
    metrics.add(_buildEmbodiedCarbonMetric(metadata));

    return metrics;
  }

  static ItemMetricValue _buildCategoryMetric(GearMetadata metadata) {
    final tracked = metadata.category;
    final category = tracked.value;
    final p = tracked.hasProvenance() ? tracked.provenance : null;

    return ItemMetricValue(
      label: 'Category',
      displayValue: category.isEmpty ? _notAvailable : category,
      confidence: category.isEmpty ? null : _provenanceConfidence(p),
      methodName: category.isEmpty ? null : SavingsFormatter.provenanceDisplayName(p?.name ?? ''),
      reasoning: _provenanceReasoning(p),
      sources: _provenanceSources(p),
    );
  }

  static ItemMetricValue _buildBrandMetric(GearMetadata metadata) {
    final tracked = metadata.brand;
    final brand = tracked.value;
    final p = tracked.hasProvenance() ? tracked.provenance : null;

    return ItemMetricValue(
      label: 'Brand',
      displayValue: brand.isEmpty ? _notAvailable : brand,
      confidence: brand.isEmpty ? null : _provenanceConfidence(p),
      methodName: brand.isEmpty ? null : SavingsFormatter.provenanceDisplayName(p?.name ?? ''),
      reasoning: _provenanceReasoning(p),
      sources: _provenanceSources(p),
    );
  }

  static ItemMetricValue _buildModelMetric(GearMetadata metadata) {
    final tracked = metadata.model;
    final model = tracked.value;
    final p = tracked.hasProvenance() ? tracked.provenance : null;

    return ItemMetricValue(
      label: 'Model',
      displayValue: model.isEmpty ? _notAvailable : model,
      confidence: model.isEmpty ? null : _provenanceConfidence(p),
      methodName: model.isEmpty ? null : SavingsFormatter.provenanceDisplayName(p?.name ?? ''),
      reasoning: _provenanceReasoning(p),
      sources: _provenanceSources(p),
    );
  }

  static ItemMetricValue _buildMaterialMetric(GearMetadata metadata) {
    final tracked = metadata.materialCategory;
    final materialCategory = tracked.value;
    final displayValue = formatMaterialCategory(materialCategory);
    final isAvailable = materialCategory !=
        MaterialCategory.MATERIAL_CATEGORY_UNSPECIFIED;
    final p = tracked.hasProvenance() ? tracked.provenance : null;

    return ItemMetricValue(
      label: 'Material',
      displayValue: displayValue,
      confidence: isAvailable ? _provenanceConfidence(p) : null,
      methodName: isAvailable ? SavingsFormatter.provenanceDisplayName(p?.name ?? '') : null,
      reasoning: _provenanceReasoning(p),
      sources: _provenanceSources(p),
    );
  }

  static ItemMetricValue _buildWeightMetric(GearMetadata metadata) {
    final tracked = metadata.weightGrams;
    final weight = tracked.value;
    final displayValue = formatWeight(weight);
    final hasWeight = weight.hasMean() && weight.mean > 0;
    final p = tracked.hasProvenance() ? tracked.provenance : null;

    return ItemMetricValue(
      label: 'Weight',
      displayValue: displayValue,
      confidence: hasWeight ? _provenanceConfidence(p) ?? _stddevToConfidence(weight.mean, weight.stddev) : null,
      methodName: hasWeight ? SavingsFormatter.provenanceDisplayName(p?.name ?? '') : null,
      reasoning: _provenanceReasoning(p),
      sources: _provenanceSources(p),
    );
  }

  static ItemMetricValue _buildEmbodiedCarbonMetric(GearMetadata metadata) {
    final carbon = metadata.embodiedCarbon;
    final displayValue = formatEmbodiedCarbon(carbon);
    final hasCarbon = carbon.hasCo2eGrams() &&
        carbon.co2eGrams.hasMean() &&
        carbon.co2eGrams.mean > 0;
    final p = carbon.hasProvenance() ? carbon.provenance : null;

    return ItemMetricValue(
      label: 'Embodied Carbon',
      displayValue: displayValue,
      confidence: hasCarbon
          ? _provenanceConfidence(p) ?? _stddevToConfidence(carbon.co2eGrams.mean, carbon.co2eGrams.stddev)
          : null,
      methodName: hasCarbon ? SavingsFormatter.provenanceDisplayName(p?.name ?? '') : null,
      reasoning: _provenanceReasoning(p),
      sources: _provenanceSources(p),
      methodologyDocPath: hasCarbon ? 'embodied_carbon.md' : null,
    );
  }

  /// Formats a product category string.
  ///
  /// Returns the category as-is if present, otherwise returns "--".
  static String formatCategory(String? category) {
    if (category == null || category.isEmpty) return _notAvailable;
    return category;
  }

  /// Formats a brand name string.
  ///
  /// Returns the brand as-is if present, otherwise returns "--".
  static String formatBrand(String? brand) {
    if (brand == null || brand.isEmpty) return _notAvailable;
    return brand;
  }

  /// Formats a model identifier string.
  ///
  /// Returns the model as-is if present, otherwise returns "--".
  static String formatModel(String? model) {
    if (model == null || model.isEmpty) return _notAvailable;
    return model;
  }

  /// Formats a MaterialCategory enum to human-readable string.
  ///
  /// Examples:
  /// - MATERIAL_CATEGORY_MIXED_PLASTIC_METAL → "Mixed Plastic-Metal"
  /// - MATERIAL_CATEGORY_SOLID_WOOD → "Solid Wood"
  /// - MATERIAL_CATEGORY_UNSPECIFIED → "--"
  static String formatMaterialCategory(MaterialCategory category) {
    switch (category) {
      case MaterialCategory.MATERIAL_CATEGORY_SOLID_METAL:
        return 'Solid Metal';
      case MaterialCategory.MATERIAL_CATEGORY_SOLID_PLASTIC:
        return 'Solid Plastic';
      case MaterialCategory.MATERIAL_CATEGORY_MIXED_PLASTIC_METAL:
        return 'Mixed Plastic-Metal';
      case MaterialCategory.MATERIAL_CATEGORY_MIXED_WOOD_METAL:
        return 'Mixed Wood-Metal';
      case MaterialCategory.MATERIAL_CATEGORY_MIXED_WOOD_PLASTIC:
        return 'Mixed Wood-Plastic';
      case MaterialCategory.MATERIAL_CATEGORY_WOOD:
        return 'Solid Wood';
      case MaterialCategory.MATERIAL_CATEGORY_ALUMINUM:
        return 'Aluminum';
      case MaterialCategory.MATERIAL_CATEGORY_FABRIC:
        return 'Fabric';
      case MaterialCategory.MATERIAL_CATEGORY_CORDLESS_POWER_TOOL:
        return 'Cordless Power Tool';
      case MaterialCategory.MATERIAL_CATEGORY_CORDED_POWER_TOOL:
        return 'Corded Power Tool';
      case MaterialCategory.MATERIAL_CATEGORY_PETROL_TOOL:
        return 'Petrol Tool';
      case MaterialCategory.MATERIAL_CATEGORY_ELECTRONICS_SMALL:
        return 'Small Electronics';
      default:
        return _notAvailable;
    }
  }

  /// Formats weight from Estimate proto to display string with auto-scaling.
  ///
  /// Automatically chooses between grams and kilograms for readability.
  /// Examples:
  /// - 350 g (mean < 1000) → "350 g"
  /// - 2300 g (mean >= 1000) → "2.3 kg"
  /// - null → "--"
  static String formatWeight(Estimate? weight) {
    if (weight == null || !weight.hasMean() || weight.mean <= 0) {
      return _notAvailable;
    }
    return _formatWeightValue(weight.mean);
  }

  /// Formats a weight value in grams to display string with auto-scaling.
  static String _formatWeightValue(double grams) {
    if (grams >= 1000) {
      final kg = grams / 1000;
      return '${kg.toStringAsFixed(kg < 10 ? 1 : 0)} kg';
    }
    return '${grams.toStringAsFixed(0)} g';
  }

  /// Formats embodied carbon from CarbonEstimate proto to display string.
  ///
  /// Uses grams for values < 1kg, kilograms otherwise (no space before unit).
  /// Examples:
  /// - 500 g CO2e → "500g CO₂"
  /// - 18400 g CO2e → "18kg CO₂"
  /// - null → "--"
  static String formatEmbodiedCarbon(CarbonEstimate? carbon) {
    if (carbon == null ||
        !carbon.hasCo2eGrams() ||
        !carbon.co2eGrams.hasMean() ||
        carbon.co2eGrams.mean <= 0) {
      return _notAvailable;
    }
    final grams = carbon.co2eGrams.mean;
    final co2Display = _formatCO2Value(grams);
    return '$co2Display CO₂';
  }

  /// Formats a CO2 value in grams to display string with auto-scaling.
  static String _formatCO2Value(double grams) {
    if (grams >= 1000) {
      final kg = grams / 1000;
      return '${kg.toStringAsFixed(kg < 10 ? 1 : 0)}kg';
    }
    return '${grams.toStringAsFixed(0)}g';
  }

  /// Converts stddev/mean ratio to a 0.0-1.0 confidence value.
  ///
  /// confidence = 1.0 - (stddev / mean), clamped to [0.1, 0.99].
  /// Higher confidence means lower relative uncertainty.
  static double? _stddevToConfidence(double mean, double stddev) {
    if (mean <= 0) return null;
    if (stddev <= 0) return 0.99;
    final confidence = 1.0 - (stddev / mean);
    return confidence.clamp(0.1, 0.99);
  }

  /// Extracts confidence from provenance, returning null if not set.
  static double? _provenanceConfidence(Provenance? p) {
    if (p == null || !p.hasConfidence()) return null;
    return p.confidence;
  }

  /// Extracts reasoning from provenance, returning null if empty.
  static String? _provenanceReasoning(Provenance? p) {
    if (p == null || p.reasoning.isEmpty) return null;
    return p.reasoning;
  }

  /// Extracts sources from provenance, returning null if empty.
  static List<String>? _provenanceSources(Provenance? p) {
    if (p == null || p.sources.isEmpty) return null;
    return p.sources.toList();
  }
}
