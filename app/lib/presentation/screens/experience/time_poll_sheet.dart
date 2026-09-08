import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/screens/experience/time_poll_finalized_modal.dart';
import 'package:ripls/presentation/screens/experience/time_poll_propose_modal.dart';
import 'package:ripls/presentation/screens/experience/time_poll_vote_modal.dart';
import 'package:ripls/presentation/viewmodels/time_modal_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass.dart';

/// Single morphing entry sheet for the time-poll flow.
///
/// Replaces the previous pattern of dispatching to one of three separate
/// pushed modals. One [GlassSheet] stays open and its body cross-fades
/// between the TBD-first compose view, the floating/voting view, and the
/// finalized "It's Set" view as the experience's poll state changes — so
/// "Ask the group" or "Set the final time" morph the sheet in place instead
/// of closing it and pushing a sibling sheet.
///
/// Each status body is the corresponding modal rendered in `embedded` mode
/// (chrome-less, no self-dismiss on state transitions); those modals keep
/// their standalone `show()` entry points for any non-morphing callers.
class TimePollSheet extends ConsumerWidget {
  const TimePollSheet({super.key, required this.experienceId});

  final String experienceId;

  /// Opens the morphing time-poll sheet.
  static Future<void> show(BuildContext context, String experienceId) async {
    await showAccessibleModal(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (_) => TimePollSheet(experienceId: experienceId),
    );
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    // Drive the morph off the coarse poll status only, so votes and other
    // intra-state changes don't restart the cross-fade (the body widgets
    // watch the provider themselves for their own content).
    final status = ref.watch(
      timeModalProvider(experienceId).select((s) => s.value?.pollStatus),
    );

    final Widget body = switch (status) {
      'poll' => TimePollVoteModal(experienceId: experienceId, embedded: true),
      'set' =>
        TimePollFinalizedModal(experienceId: experienceId, embedded: true),
      _ => TimePollProposeModal(experienceId: experienceId, embedded: true),
    };

    return GlassSheet(
      padding: EdgeInsets.zero,
      child: AnimatedSwitcher(
        duration: accessibleDuration(context, const Duration(milliseconds: 220)),
        child: KeyedSubtree(
          key: ValueKey(status ?? 'loading'),
          child: body,
        ),
      ),
    );
  }
}
