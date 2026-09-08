import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/presentation/viewmodels/content_expanded_provider.dart';
import 'package:ripls/presentation/widgets/content/content_morph_panel_launcher.dart';

/// Experience-flavored wrapper over the shared [openContentMorphPanel]. Binds
/// the launcher to [experienceContentExpandedProvider] so experience call
/// sites ("Who's pitching in?", conversation expand) don't have to name the
/// flag themselves. New surfaces should call [openContentMorphPanel] directly
/// with their own expanded provider.
Future<void> openExperienceContentPanel({
  required BuildContext context,
  required WidgetRef ref,
  required String experienceId,
  required Rect sourceRect,
  required Widget screen,
  required String routeName,
}) {
  return openContentMorphPanel(
    context: context,
    ref: ref,
    expandedProvider: experienceContentExpandedProvider(experienceId),
    sourceRect: sourceRect,
    screen: screen,
    routeName: routeName,
  );
}
