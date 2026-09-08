import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/utils/gear_metadata_formatter.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart';
import 'package:ripls/data/gen/ripls/api/gear_service.pb.dart';

void main() {
  group('GearMetadataFormatter', () {
    group('formatMetadata', () {
      test('returns empty list when metadata is null', () {
        final metrics = GearMetadataFormatter.formatMetadata(null);
        expect(metrics, isEmpty);
      });

      test('returns all six metrics when metadata is fully populated', () {
        final metadata = GearMetadata(
          category: TrackedString(value: 'Power Tools'),
          brand: TrackedString(value: 'DeWalt'),
          model: TrackedString(value: 'DCD771C2'),
          materialCategory: TrackedMaterialCategory(value: MaterialCategory.MATERIAL_CATEGORY_CORDLESS_POWER_TOOL),
          weightGrams: TrackedEstimate(value: Estimate(mean: 2300, stddev: 200)),
          embodiedCarbon: CarbonEstimate(
            co2eGrams: Estimate(mean: 18400, stddev: 2000),
            provenance: Provenance(
              source: ProvenanceSource.PROVENANCE_SOURCE_FORMULA,
              name: 'weight_material_carbon',
            ),
          ),
        );

        final metrics = GearMetadataFormatter.formatMetadata(metadata);

        expect(metrics, hasLength(6));
        expect(metrics[0].label, 'Category');
        expect(metrics[1].label, 'Brand');
        expect(metrics[2].label, 'Model');
        expect(metrics[3].label, 'Material');
        expect(metrics[4].label, 'Weight');
        expect(metrics[5].label, 'Embodied Carbon');
      });

      test('shows -- for empty string fields', () {
        final metadata = GearMetadata();

        final metrics = GearMetadataFormatter.formatMetadata(metadata);

        expect(metrics[0].displayValue, '--'); // Category
        expect(metrics[1].displayValue, '--'); // Brand
        expect(metrics[2].displayValue, '--'); // Model
      });
    });

    group('provenance extraction', () {
      test('category metric reads provenance from TrackedString', () {
        final metadata = GearMetadata(
          category: TrackedString(
            value: 'Power Tools',
            provenance: Provenance(
              source: ProvenanceSource.PROVENANCE_SOURCE_LLM,
              name: 'genai_time_estimate',
              confidence: 0.85,
              reasoning: 'Detected from product description.',
              sources: ['product-catalog-v2'],
            ),
          ),
        );
        final metrics = GearMetadataFormatter.formatMetadata(metadata);
        final cat = metrics[0];

        expect(cat.confidence, 0.85);
        expect(cat.methodName, 'AI Estimated');
        expect(cat.reasoning, 'Detected from product description.');
        expect(cat.sources, ['product-catalog-v2']);
      });

      test('brand metric reads provenance from TrackedString', () {
        final metadata = GearMetadata(
          brand: TrackedString(
            value: 'DeWalt',
            provenance: Provenance(
              source: ProvenanceSource.PROVENANCE_SOURCE_LLM,
              name: 'genai_time_estimate',
              reasoning: 'Brand identified from logo.',
            ),
          ),
        );
        final metrics = GearMetadataFormatter.formatMetadata(metadata);
        final brand = metrics[1];

        expect(brand.reasoning, 'Brand identified from logo.');
        expect(brand.methodName, 'AI Estimated');
      });

      test('model metric reads provenance from TrackedString', () {
        final metadata = GearMetadata(
          model: TrackedString(
            value: 'DCD771C2',
            provenance: Provenance(
              source: ProvenanceSource.PROVENANCE_SOURCE_LLM,
              name: 'genai_time_estimate',
              reasoning: 'Model number extracted from product page.',
              sources: ['https://dewalt.com/dcd771c2'],
            ),
          ),
        );
        final metrics = GearMetadataFormatter.formatMetadata(metadata);
        final model = metrics[2];

        expect(model.reasoning, 'Model number extracted from product page.');
        expect(model.sources, ['https://dewalt.com/dcd771c2']);
      });

      test('material metric reads provenance from TrackedMaterialCategory', () {
        final metadata = GearMetadata(
          materialCategory: TrackedMaterialCategory(
            value: MaterialCategory.MATERIAL_CATEGORY_CORDLESS_POWER_TOOL,
            provenance: Provenance(
              source: ProvenanceSource.PROVENANCE_SOURCE_LLM,
              name: 'genai_time_estimate',
              confidence: 0.9,
              reasoning: 'Classified as cordless power tool from visual analysis.',
            ),
          ),
        );
        final metrics = GearMetadataFormatter.formatMetadata(metadata);
        final material = metrics[3];

        expect(material.confidence, 0.9);
        expect(material.reasoning, 'Classified as cordless power tool from visual analysis.');
      });

      test('weight metric reads provenance from TrackedEstimate', () {
        final metadata = GearMetadata(
          weightGrams: TrackedEstimate(
            value: Estimate(mean: 2300, stddev: 200),
            provenance: Provenance(
              source: ProvenanceSource.PROVENANCE_SOURCE_LLM,
              name: 'genai_time_estimate',
              confidence: 0.75,
              reasoning: 'Weight estimated from product specs.',
            ),
          ),
        );
        final metrics = GearMetadataFormatter.formatMetadata(metadata);
        final weight = metrics[4];

        expect(weight.confidence, 0.75);
        expect(weight.reasoning, 'Weight estimated from product specs.');
      });

      test('weight metric falls back to stddev-based confidence when provenance has none', () {
        final metadata = GearMetadata(
          weightGrams: TrackedEstimate(
            value: Estimate(mean: 2300, stddev: 200),
            provenance: Provenance(
              source: ProvenanceSource.PROVENANCE_SOURCE_LLM,
              name: 'genai_time_estimate',
            ),
          ),
        );
        final metrics = GearMetadataFormatter.formatMetadata(metadata);
        final weight = metrics[4];

        // Should fall back to stddev-based: 1.0 - (200/2300) ≈ 0.913
        expect(weight.confidence, isNotNull);
        expect(weight.confidence!, closeTo(0.913, 0.01));
      });

      test('embodied carbon metric reads provenance', () {
        final metadata = GearMetadata(
          embodiedCarbon: CarbonEstimate(
            co2eGrams: Estimate(mean: 18400, stddev: 2000),
            provenance: Provenance(
              source: ProvenanceSource.PROVENANCE_SOURCE_FORMULA,
              name: 'weight_material_carbon',
              reasoning: 'Computed from weight and material emission factor.',
              sources: ['EPA emission factors'],
            ),
          ),
        );
        final metrics = GearMetadataFormatter.formatMetadata(metadata);
        final carbon = metrics[5];

        expect(carbon.methodName, 'Weight × Material Factor');
        expect(carbon.methodologyDocPath, 'embodied_carbon.md');
        expect(carbon.reasoning, 'Computed from weight and material emission factor.');
        expect(carbon.sources, ['EPA emission factors']);
      });

      test('metrics have null provenance fields when no provenance set', () {
        final metadata = GearMetadata(
          category: TrackedString(value: 'Power Tools'),
          brand: TrackedString(value: 'DeWalt'),
        );
        final metrics = GearMetadataFormatter.formatMetadata(metadata);

        // Category has value but no provenance
        expect(metrics[0].displayValue, 'Power Tools');
        expect(metrics[0].confidence, isNull);
        expect(metrics[0].reasoning, isNull);
        expect(metrics[0].sources, isNull);

        // Brand has value but no provenance
        expect(metrics[1].displayValue, 'DeWalt');
        expect(metrics[1].confidence, isNull);
        expect(metrics[1].reasoning, isNull);
      });

      test('empty fields have null detail fields regardless of provenance', () {
        final metadata = GearMetadata(
          category: TrackedString(value: ''),
          materialCategory: TrackedMaterialCategory(
            value: MaterialCategory.MATERIAL_CATEGORY_UNSPECIFIED,
          ),
        );
        final metrics = GearMetadataFormatter.formatMetadata(metadata);

        expect(metrics[0].displayValue, '--');
        expect(metrics[0].confidence, isNull);
        expect(metrics[0].methodName, isNull);

        expect(metrics[3].displayValue, '--');
        expect(metrics[3].confidence, isNull);
        expect(metrics[3].methodName, isNull);
      });
    });

    group('formatCategory', () {
      test('returns category when present', () {
        expect(GearMetadataFormatter.formatCategory('Power Tools'), 'Power Tools');
      });

      test('returns -- when null', () {
        expect(GearMetadataFormatter.formatCategory(null), '--');
      });

      test('returns -- when empty', () {
        expect(GearMetadataFormatter.formatCategory(''), '--');
      });
    });

    group('formatBrand', () {
      test('returns brand when present', () {
        expect(GearMetadataFormatter.formatBrand('DeWalt'), 'DeWalt');
      });

      test('returns -- when null', () {
        expect(GearMetadataFormatter.formatBrand(null), '--');
      });

      test('returns -- when empty', () {
        expect(GearMetadataFormatter.formatBrand(''), '--');
      });
    });

    group('formatModel', () {
      test('returns model when present', () {
        expect(GearMetadataFormatter.formatModel('DCD771C2'), 'DCD771C2');
      });

      test('returns -- when null', () {
        expect(GearMetadataFormatter.formatModel(null), '--');
      });

      test('returns -- when empty', () {
        expect(GearMetadataFormatter.formatModel(''), '--');
      });
    });

    group('formatMaterialCategory', () {
      test('formats solid metal', () {
        expect(
          GearMetadataFormatter.formatMaterialCategory(
            MaterialCategory.MATERIAL_CATEGORY_SOLID_METAL,
          ),
          'Solid Metal',
        );
      });

      test('formats mixed plastic-metal', () {
        expect(
          GearMetadataFormatter.formatMaterialCategory(
            MaterialCategory.MATERIAL_CATEGORY_MIXED_PLASTIC_METAL,
          ),
          'Mixed Plastic-Metal',
        );
      });

      test('formats cordless power tool', () {
        expect(
          GearMetadataFormatter.formatMaterialCategory(
            MaterialCategory.MATERIAL_CATEGORY_CORDLESS_POWER_TOOL,
          ),
          'Cordless Power Tool',
        );
      });

      test('formats wood', () {
        expect(
          GearMetadataFormatter.formatMaterialCategory(
            MaterialCategory.MATERIAL_CATEGORY_WOOD,
          ),
          'Solid Wood',
        );
      });

      test('returns -- for unspecified', () {
        expect(
          GearMetadataFormatter.formatMaterialCategory(
            MaterialCategory.MATERIAL_CATEGORY_UNSPECIFIED,
          ),
          '--',
        );
      });
    });

    group('formatWeight', () {
      test('formats weight in grams when less than 1kg', () {
        final weight = Estimate(mean: 350);
        expect(GearMetadataFormatter.formatWeight(weight), '350 g');
      });

      test('formats weight in kg when 1kg or more', () {
        final weight = Estimate(mean: 2300);
        expect(GearMetadataFormatter.formatWeight(weight), '2.3 kg');
      });

      test('formats large weight without decimal', () {
        final weight = Estimate(mean: 12500);
        expect(GearMetadataFormatter.formatWeight(weight), '13 kg');
      });

      test('returns -- when null', () {
        expect(GearMetadataFormatter.formatWeight(null), '--');
      });

      test('returns -- when mean is zero', () {
        final weight = Estimate(mean: 0);
        expect(GearMetadataFormatter.formatWeight(weight), '--');
      });

      test('returns -- when mean is negative', () {
        final weight = Estimate(mean: -100);
        expect(GearMetadataFormatter.formatWeight(weight), '--');
      });
    });

    group('formatEmbodiedCarbon', () {
      test('formats carbon in grams when less than 1kg', () {
        final carbon = CarbonEstimate(
          co2eGrams: Estimate(mean: 500),
        );
        expect(GearMetadataFormatter.formatEmbodiedCarbon(carbon), '500g CO₂');
      });

      test('formats carbon in kg when 1kg or more', () {
        final carbon = CarbonEstimate(
          co2eGrams: Estimate(mean: 18400),
        );
        expect(GearMetadataFormatter.formatEmbodiedCarbon(carbon), '18kg CO₂');
      });

      test('formats large carbon values in kg', () {
        final carbon = CarbonEstimate(
          co2eGrams: Estimate(mean: 125000),
        );
        expect(GearMetadataFormatter.formatEmbodiedCarbon(carbon), '125kg CO₂');
      });

      test('returns -- when null', () {
        expect(GearMetadataFormatter.formatEmbodiedCarbon(null), '--');
      });

      test('returns -- when co2eGrams is missing', () {
        final carbon = CarbonEstimate();
        expect(GearMetadataFormatter.formatEmbodiedCarbon(carbon), '--');
      });

      test('returns -- when mean is zero', () {
        final carbon = CarbonEstimate(
          co2eGrams: Estimate(mean: 0),
        );
        expect(GearMetadataFormatter.formatEmbodiedCarbon(carbon), '--');
      });
    });
  });
}
