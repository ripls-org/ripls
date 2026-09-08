import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/widgets/profile_section/metric_tile_grid.dart'
    show MetricTileData;
import 'package:ripls/presentation/widgets/profile_section/profile_action_row.dart';
import 'package:ripls/presentation/widgets/profile_section/profile_chips.dart';
import 'package:ripls/presentation/widgets/profile_section/profile_face_stack.dart';
import 'package:ripls/presentation/widgets/profile_section/profile_hero.dart';
import 'package:ripls/presentation/widgets/profile_section/profile_hero_stat.dart';
import 'package:ripls/presentation/widgets/profile_section/profile_member_row.dart';
import 'package:ripls/presentation/widgets/profile_section/profile_metric_rows.dart';

import '../../../helpers/l10n_helpers.dart';

/// Regression guard for the `!semantics.parentDataDirty` assertion
/// (Flutter object.dart) on the profile layout: the presence and
/// member/metric providers resolve asynchronously, restructuring the
/// scroll column across consecutive frames while the semantics tree is
/// still dirty. The hero's expandable sentence uses a LayoutBuilder,
/// which cannot be measured intrinsically — an earlier layout that
/// wrapped it in IntrinsicHeight broke layout on device and cascaded
/// into semantics assertion spam. The harness mirrors the current v11
/// screen structure (backdrop stack, one scrolling column, no
/// intrinsics) exactly; reintroducing IntrinsicHeight around the hero
/// makes this test fail again.
Widget _screen({required bool loaded}) {
  return ProviderScope(
    child: localizedApp(
      Stack(
        fit: StackFit.expand,
        children: [
          const ColoredBox(color: Colors.black),
          SingleChildScrollView(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                const SizedBox(height: 118),
                const ProfileHero(
                  eyebrow: 'WHAT BETTY BRINGS',
                  title: 'Betty Ripls',
                  sentence: 'Climbing, cooking, and hiking.',
                ),
                const SizedBox(height: 8),
                if (loaded)
                  ProfileMemberRow(
                    faces: const [
                      FaceStackEntry(initial: 'A'),
                      FaceStackEntry(initial: 'B'),
                    ],
                    title: 'With you in Campus Crew +1',
                  ),
                if (loaded)
                  const Padding(
                    padding: EdgeInsets.fromLTRB(22, 6, 22, 0),
                    child: ProfileChips(
                      tags: ['Power tools', 'Trail days', 'Chili'],
                    ),
                  ),
                Padding(
                  padding: const EdgeInsets.fromLTRB(22, 16, 22, 0),
                  child: ProfileActionRow(actions: [
                    ProfileActionRowItem(
                      icon: Icons.chat_bubble_outline,
                      label: 'Message',
                      isPrimary: true,
                      onTapRect: (_) {},
                    ),
                  ]),
                ),
                const SizedBox(height: 18),
                if (loaded) ...[
                  const ProfileHeroStat(
                    kicker: 'TIME TOGETHER',
                    value: '214',
                    unit: 'hrs',
                    subtext: 'with people in your shared groups',
                    equivalence: 'A steady presence.',
                  ),
                  ProfileMetricRows(metrics: [
                    MetricTileData(
                        value: '14',
                        label: 'Problems solved',
                        subtitle: 'asks Betty answered'),
                    MetricTileData(
                        value: '9', label: 'Library', onTap: () {}),
                  ]),
                ] else
                  const SizedBox(height: 40),
                const SizedBox(height: 30),
              ],
            ),
          ),
          const Positioned(
              top: 0, left: 0, right: 0, child: SizedBox(height: 56)),
        ],
      ),
    ),
  );
}

void main() {
  testWidgets(
      'async providers resolving into the profile layout does not throw '
      'a semantics assertion', (tester) async {
    final handle = tester.ensureSemantics();

    await tester.pumpWidget(_screen(loaded: false));
    await tester.pumpAndSettle();

    // The presence + member + metric providers resolve, inserting the
    // member row, chips, hero stat, and ledger — the transition that
    // dirtied semantics parent data on device.
    await tester.pumpWidget(_screen(loaded: true));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 16));
    await tester.pumpAndSettle();

    // And back (a refresh restarting the providers).
    await tester.pumpWidget(_screen(loaded: false));
    await tester.pumpAndSettle();

    handle.dispose();
  });
}
