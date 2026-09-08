import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/viewmodels/needs_scope.dart';
import 'package:ripls/presentation/viewmodels/request_needs_view_model.dart';
import 'package:ripls/presentation/viewmodels/request_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass.dart';
import 'package:ripls/presentation/widgets/needs/need_claim_chip.dart';
import 'package:ripls/presentation/widgets/needs/needs_actions.dart';
import 'package:ripls/presentation/widgets/needs/needs_claim_gear_picker.dart';

/// RequestContributionComposerSheet is the request "what are you bringing"
/// composer — the request analog of `ExperienceRsvpComposerSheet`'s
/// contributions-only mode. It opens from the participation panel's "I'll
/// bring" action.
///
/// Each chip is a shared [NeedClaimChip]: tapping it opens the claim sheet for
/// that item (or "edit your contribution" when already bringing it). The
/// "From your gear" affordance opens [NeedsClaimGearPicker] to link a library
/// item (or snap/create one); "Add something else" opens the shared add-to-list
/// sheet. Contributions save inline as they're added, so the primary action
/// just closes the editor.
class RequestContributionComposerSheet extends ConsumerStatefulWidget {
  final String requestId;
  final Color accentColor;

  const RequestContributionComposerSheet({
    super.key,
    required this.requestId,
    required this.accentColor,
  });

  static Future<void> show(
    BuildContext context, {
    required String requestId,
    required Color accentColor,
  }) {
    return showAccessibleModal<void>(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (_) => RequestContributionComposerSheet(
        requestId: requestId,
        accentColor: accentColor,
      ),
    );
  }

  @override
  ConsumerState<RequestContributionComposerSheet> createState() =>
      _RequestContributionComposerSheetState();
}

class _RequestContributionComposerSheetState
    extends ConsumerState<RequestContributionComposerSheet> {
  RequestNeedsNotifier get _needsNotifier =>
      ref.read(requestNeedsProvider(widget.requestId).notifier);

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    final state = ref.watch(requestProvider(widget.requestId));
    final needs = ref.watch(requestNeedsProvider(widget.requestId));
    if (state.requestDetails == null) return const SizedBox.shrink();

    // The viewer's current contributions, grouped by title, so a chip reflects
    // its selected state, quantity (×N), and attached note.
    final myContrib = <String, List<RequestContributionResponse>>{};
    for (final c in needs.contributions) {
      if (c.contributor.id == state.currentUserId && c.title.isNotEmpty) {
        (myContrib[c.title] ??= []).add(c);
      }
    }

    // Chip set: the viewer's contributions (selected) + server suggestions +
    // the request's open needs. Order-stable, de-duplicated.
    final chips = <String>[];
    void addChip(String t) {
      if (t.isNotEmpty && !chips.contains(t)) chips.add(t);
    }

    for (final t in myContrib.keys) {
      addChip(t);
    }
    for (final s in needs.additionalAsks) {
      addChip(s);
    }
    final needByTitle = <String, RequestNeedResponse>{};
    for (final n in needs.needs) {
      addChip(n.name);
      if (n.name.isNotEmpty) needByTitle[n.name.toLowerCase()] = n;
    }

    final actions = _needsActions(state);

    return GlassSheet(
      padding: EdgeInsets.zero,
      scrim: AppColors.modalContentScrim,
      child: SafeArea(
        top: false,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            _header(l10n),
            Flexible(
              child: SingleChildScrollView(
                padding: const EdgeInsets.fromLTRB(20, 4, 20, 0),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    _bringingLabel(l10n),
                    const SizedBox(height: 10),
                    Wrap(
                      spacing: 8,
                      runSpacing: 8,
                      children: [
                        for (final title in chips)
                          _chip(title, myContrib, needByTitle, actions),
                      ],
                    ),
                    const SizedBox(height: 10),
                    GlassInlineAction(
                      text: l10n.rsvpComposerAddSomething,
                      semanticsLabel: l10n.a11yRsvpComposerAdd,
                      icon: Icons.add_rounded,
                      onTap: () => actions.openSingleAddPicker(context, ref),
                    ),
                  ],
                ),
              ),
            ),
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 0, 20, 4),
              child: GlassFooterButtons(
                primaryLabel: l10n.contributionComposerSave,
                primarySemanticsLabel: l10n.a11yContributionComposerSave,
                primaryEnabled: true,
                onPrimary: () => Navigator.of(context).pop(),
                showSecondary: false,
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _header(AppLocalizations l10n) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(20, 4, 12, 0),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: [
          Row(
            children: [
              Expanded(
                child: Text(
                  l10n.contributionComposerKicker.toUpperCase(),
                  style: TextStyle(
                    color: widget.accentColor,
                    fontSize: 11,
                    fontWeight: FontWeight.w700,
                    letterSpacing: 1.6,
                  ),
                ),
              ),
              IconAction(
                icon: Icons.close_rounded,
                semanticsLabel: l10n.a11yClose,
                color: AppColors.modalTextSecondary,
                onPressed: () => Navigator.of(context).pop(),
              ),
            ],
          ),
          Padding(
            padding: const EdgeInsets.only(right: 8),
            child: Semantics(
              header: true,
              child: Text(
                l10n.contributionComposerTitle,
                style: const TextStyle(
                  color: AppColors.modalTextPrimary,
                  fontSize: 28,
                  fontWeight: FontWeight.w800,
                  height: 1.1,
                  letterSpacing: -0.4,
                ),
              ),
            ),
          ),
          const SizedBox(height: 6),
          Padding(
            padding: const EdgeInsets.only(right: 8, bottom: 16),
            child: Text(
              l10n.contributionComposerSubtitle,
              style: const TextStyle(
                color: AppColors.modalTextSecondary,
                fontSize: 14,
                height: 1.4,
              ),
            ),
          ),
        ],
      ),
    );
  }

  /// The "Bringing" field label with a trailing affordance to link an item
  /// from the viewer's gear library.
  Widget _bringingLabel(AppLocalizations l10n) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(4, 0, 4, 0),
      child: Row(
        children: [
          Text(
            l10n.rsvpComposerBringing.toUpperCase(),
            style: TextStyle(
              color: widget.accentColor,
              fontSize: 11,
              fontWeight: FontWeight.w700,
              letterSpacing: 1.2,
            ),
          ),
          const Spacer(),
          Tappable(
            semanticsLabel: l10n.a11yRsvpComposerFromGear,
            onTap: _pickFromGear,
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(Icons.add_rounded, size: 13, color: widget.accentColor),
                const SizedBox(width: 3),
                Text(
                  l10n.rsvpComposerFromGear.toUpperCase(),
                  style: TextStyle(
                    color: widget.accentColor,
                    fontSize: 10,
                    fontWeight: FontWeight.w700,
                    letterSpacing: 1,
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Widget _chip(
    String title,
    Map<String, List<RequestContributionResponse>> myContrib,
    Map<String, RequestNeedResponse> needByTitle,
    NeedsActions actions,
  ) {
    final mine = myContrib[title];
    final need = needByTitle[title.toLowerCase()];
    String? note;
    if (mine != null) {
      for (final c in mine) {
        if (c.description.trim().isNotEmpty) {
          note = c.description;
          break;
        }
      }
    }
    note ??= (need != null && need.note.trim().isNotEmpty) ? need.note : null;
    return NeedClaimChip(
      actions: actions,
      title: title,
      selected: mine != null,
      note: note,
      proposerName: need?.proposer.name,
      linkedNeedId: need?.id,
      slotsNeeded: need?.slots ?? 1,
      quantity: mine != null ? mine.length : (need?.slots ?? 1),
    );
  }

  NeedsActions _needsActions(RequestState state) {
    final request = state.requestDetails!;
    return NeedsActions(
      scope: NeedsScope.request(
        requestId: widget.requestId,
        currentUserId: state.currentUserId,
        // The composer only opens for active requests, so never terminal here.
        isTerminal: false,
        communityId: state.communityId ?? '',
        isOwner: state.isOwner,
        requestOwnerId: request.requester.id,
        requestName: request.title,
      ),
    );
  }

  /// Opens the gear picker so the viewer can link an item from their library
  /// (or snap/create one). The pick is added as a gear-linked contribution.
  Future<void> _pickFromGear() async {
    final communityId =
        ref.read(requestProvider(widget.requestId)).communityId;
    // Widget async exception: the gear picker is a modal.
    final result = await NeedsClaimGearPicker.show(
      context,
      communityId: communityId,
    );
    if (!mounted || result?.gearId == null) return;
    final name = result!.gear?.name.trim() ?? '';
    if (name.isEmpty) return;
    await _needsNotifier.addContribution(title: name, gearId: result.gearId);
  }
}
