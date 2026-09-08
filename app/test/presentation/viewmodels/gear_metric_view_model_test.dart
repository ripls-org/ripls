import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart';
import 'package:ripls/presentation/viewmodels/gear_metric_view_model.dart';

/// Builds a [QualityTimeAttributes] proto with all fields set to the given values.
QualityTimeAttributes _attrs({
  double durationMinutes = 15,
  SocialModality modality = SocialModality.SOCIAL_MODALITY_IN_PERSON_BRIEF,
  int groupSize = 2,
  SocialTieStrength tieStrength = SocialTieStrength.SOCIAL_TIE_STRENGTH_NEW,
  SocialReciprocity reciprocity = SocialReciprocity.SOCIAL_RECIPROCITY_GIVING,
  SocialNovelty novelty = SocialNovelty.SOCIAL_NOVELTY_NOVEL,
  SocialVulnerabilityLevel vulnerability =
      SocialVulnerabilityLevel.SOCIAL_VULNERABILITY_LEVEL_MEDIUM,
}) {
  return QualityTimeAttributes(
    estimatedDurationMinutes: durationMinutes,
    modality: modality,
    groupSize: groupSize,
    tieStrength: tieStrength,
    reciprocity: reciprocity,
    novelty: novelty,
    vulnerability: vulnerability,
  );
}

void main() {
  group('mapSocialAttributes', () {
    test('returns null for null input', () {
      expect(mapSocialAttributes(null), isNull);
    });

    group('duration formatting', () {
      const cases = [
        (0.0, '—'),
        (15.0, '15 min'),
        (45.0, '45 min'),
        (59.0, '59 min'),
        (60.0, '1 hrs'),
        (90.0, '1.5 hrs'),
        (120.0, '2 hrs'),
        (150.0, '2.5 hrs'),
      ];

      for (final (minutes, expected) in cases) {
        test('$minutes min → "$expected"', () {
          final result = mapSocialAttributes(_attrs(durationMinutes: minutes));
          expect(result!.duration, expected);
        });
      }
    });

    group('modality display strings', () {
      const cases = [
        (SocialModality.SOCIAL_MODALITY_IN_PERSON_SHARED, 'In-person'),
        (SocialModality.SOCIAL_MODALITY_IN_PERSON_BRIEF, 'In-person (brief)'),
        (SocialModality.SOCIAL_MODALITY_VIDEO, 'Video'),
        (SocialModality.SOCIAL_MODALITY_PHONE, 'Phone'),
        (SocialModality.SOCIAL_MODALITY_TEXT, 'Text'),
        (SocialModality.SOCIAL_MODALITY_UNSPECIFIED, '—'),
      ];

      for (final (modality, expected) in cases) {
        test('${modality.name} → "$expected"', () {
          final result = mapSocialAttributes(_attrs(modality: modality));
          expect(result!.modality, expected);
        });
      }
    });

    group('group size formatting', () {
      test('1 person uses singular', () {
        final result = mapSocialAttributes(_attrs(groupSize: 1));
        expect(result!.groupSize, '1 person');
      });

      test('2 people uses plural', () {
        final result = mapSocialAttributes(_attrs(groupSize: 2));
        expect(result!.groupSize, '2 people');
      });

      test('10 people', () {
        final result = mapSocialAttributes(_attrs(groupSize: 10));
        expect(result!.groupSize, '10 people');
      });
    });

    group('tie strength display strings', () {
      const cases = [
        (SocialTieStrength.SOCIAL_TIE_STRENGTH_NEW, 'New contact'),
        (SocialTieStrength.SOCIAL_TIE_STRENGTH_ACQUAINTANCE, 'Acquaintance'),
        (SocialTieStrength.SOCIAL_TIE_STRENGTH_ACTIVE, 'Active tie'),
        (SocialTieStrength.SOCIAL_TIE_STRENGTH_CLOSE, 'Close tie'),
        (SocialTieStrength.SOCIAL_TIE_STRENGTH_UNSPECIFIED, '—'),
      ];

      for (final (tier, expected) in cases) {
        test('${tier.name} → "$expected"', () {
          final result = mapSocialAttributes(_attrs(tieStrength: tier));
          expect(result!.connection, expected);
        });
      }
    });

    group('reciprocity display strings', () {
      const cases = [
        (SocialReciprocity.SOCIAL_RECIPROCITY_GIVING, 'Giving'),
        (SocialReciprocity.SOCIAL_RECIPROCITY_RECEIVING, 'Receiving'),
        (SocialReciprocity.SOCIAL_RECIPROCITY_MUTUAL, 'Mutual'),
        (SocialReciprocity.SOCIAL_RECIPROCITY_UNSPECIFIED, '—'),
      ];

      for (final (role, expected) in cases) {
        test('${role.name} → "$expected"', () {
          final result = mapSocialAttributes(_attrs(reciprocity: role));
          expect(result!.reciprocity, expected);
        });
      }
    });

    group('novelty display strings', () {
      const cases = [
        (SocialNovelty.SOCIAL_NOVELTY_NOVEL, 'Novel'),
        (SocialNovelty.SOCIAL_NOVELTY_INFREQUENT, 'Infrequent'),
        (SocialNovelty.SOCIAL_NOVELTY_ROUTINE, 'Routine'),
        (SocialNovelty.SOCIAL_NOVELTY_UNSPECIFIED, '—'),
      ];

      for (final (level, expected) in cases) {
        test('${level.name} → "$expected"', () {
          final result = mapSocialAttributes(_attrs(novelty: level));
          expect(result!.novelty, expected);
        });
      }
    });

    group('vulnerability display strings', () {
      const cases = [
        (SocialVulnerabilityLevel.SOCIAL_VULNERABILITY_LEVEL_LOW, 'Low'),
        (SocialVulnerabilityLevel.SOCIAL_VULNERABILITY_LEVEL_MEDIUM, 'Medium'),
        (SocialVulnerabilityLevel.SOCIAL_VULNERABILITY_LEVEL_HIGH, 'High'),
        (SocialVulnerabilityLevel.SOCIAL_VULNERABILITY_LEVEL_UNSPECIFIED, '—'),
      ];

      for (final (level, expected) in cases) {
        test('${level.name} → "$expected"', () {
          final result = mapSocialAttributes(_attrs(vulnerability: level));
          expect(result!.vulnerability, expected);
        });
      }
    });

    test('maps a complete loan baseline correctly', () {
      final attrs = _attrs(
        durationMinutes: 15,
        modality: SocialModality.SOCIAL_MODALITY_IN_PERSON_BRIEF,
        groupSize: 2,
        tieStrength: SocialTieStrength.SOCIAL_TIE_STRENGTH_ACQUAINTANCE,
        reciprocity: SocialReciprocity.SOCIAL_RECIPROCITY_GIVING,
        novelty: SocialNovelty.SOCIAL_NOVELTY_NOVEL,
        vulnerability: SocialVulnerabilityLevel.SOCIAL_VULNERABILITY_LEVEL_MEDIUM,
      );

      final result = mapSocialAttributes(attrs)!;

      expect(result.duration, '15 min');
      expect(result.modality, 'In-person (brief)');
      expect(result.groupSize, '2 people');
      expect(result.connection, 'Acquaintance');
      expect(result.reciprocity, 'Giving');
      expect(result.novelty, 'Novel');
      expect(result.vulnerability, 'Medium');
    });

    test('maps an experience baseline correctly', () {
      final attrs = _attrs(
        durationMinutes: 120,
        modality: SocialModality.SOCIAL_MODALITY_IN_PERSON_SHARED,
        groupSize: 8,
        tieStrength: SocialTieStrength.SOCIAL_TIE_STRENGTH_ACTIVE,
        reciprocity: SocialReciprocity.SOCIAL_RECIPROCITY_MUTUAL,
        novelty: SocialNovelty.SOCIAL_NOVELTY_INFREQUENT,
        vulnerability: SocialVulnerabilityLevel.SOCIAL_VULNERABILITY_LEVEL_HIGH,
      );

      final result = mapSocialAttributes(attrs)!;

      expect(result.duration, '2 hrs');
      expect(result.modality, 'In-person');
      expect(result.groupSize, '8 people');
      expect(result.connection, 'Active tie');
      expect(result.reciprocity, 'Mutual');
      expect(result.novelty, 'Infrequent');
      expect(result.vulnerability, 'High');
    });
  });
}
