import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart';
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart';
import 'package:ripls/presentation/widgets/content/content_metric_sheet.dart';

import '../../../helpers/l10n_helpers.dart';

/// Tests the audit-trail-link gating on the metric detail sheet.
///
/// Per the Decisions section of issue #1249, the audit trail link must only
/// appear once the experience or request is completed (so per-attribute
/// provenance has been stamped). Pre-completion sheets must hide it.
void main() {
  group('ContentMetricSheet gating', () {
    ImpactEstimate makeImpact() {
      return ImpactEstimate(
        moneySaved: MoneySavings(valueUsd: Estimate(mean: 50)),
        emissionsPrevented: PreventedEmissions(
          manufactureAvoidedCarbon: CarbonEstimate(
            co2eGrams: Estimate(mean: 600),
          ),
        ),
        qualityTime: QualityTimeEstimate(
          qualityTimeMinutes: Estimate(mean: 45),
          attributes: QualityTimeAttributes(
            estimatedDurationMinutes: 30,
            modality: SocialModality.SOCIAL_MODALITY_IN_PERSON_SHARED,
            tieStrength: SocialTieStrength.SOCIAL_TIE_STRENGTH_ACTIVE,
            reciprocity: SocialReciprocity.SOCIAL_RECIPROCITY_MUTUAL,
            novelty: SocialNovelty.SOCIAL_NOVELTY_INFREQUENT,
            vulnerability:
                SocialVulnerabilityLevel.SOCIAL_VULNERABILITY_LEVEL_MEDIUM,
          ),
        ),
      );
    }

    Future<void> openSheet(
      WidgetTester tester, {
      required bool isCompleted,
    }) async {
      tester.view.physicalSize = const Size(1080, 2400);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);

      late BuildContext capturedContext;
      await tester.pumpWidget(
        localizedApp(
          Builder(
            builder: (ctx) {
              capturedContext = ctx;
              return const Scaffold(body: SizedBox.shrink());
            },
          ),
        ),
      );

      unawaited(ContentMetricSheet.show(
        capturedContext,
        metricId: 'savings',
        impact: makeImpact(),
        isCompleted: isCompleted,
      ));
      await tester.pumpAndSettle();
    }

    testWidgets('audit trail link shown when isCompleted=true', (tester) async {
      await openSheet(tester, isCompleted: true);

      // The audit trail link uses the manage_search icon plus a localized title.
      expect(find.byIcon(Icons.manage_search), findsOneWidget);
    });

    testWidgets('audit trail link hidden when isCompleted=false', (
      tester,
    ) async {
      await openSheet(tester, isCompleted: false);

      // Pre-completion, no audit trail affordance should render.
      expect(find.byIcon(Icons.manage_search), findsNothing);
    });
  });
}
