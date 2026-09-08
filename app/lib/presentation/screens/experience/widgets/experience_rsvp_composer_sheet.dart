import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/data/gen/ripls/api/experience_service.pb.dart'
    show RSVPIntention;
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/viewmodels/experience_needs_view_model.dart';
import 'package:ripls/presentation/viewmodels/experience_view_model.dart';
import 'package:ripls/presentation/viewmodels/needs_scope.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass.dart';
import 'package:ripls/presentation/widgets/needs/need_claim_chip.dart';
import 'package:ripls/presentation/widgets/needs/needs_actions.dart';
import 'package:ripls/presentation/widgets/needs/needs_claim_gear_picker.dart';

/// ExperienceRsvpComposerSheet is the unified RSVP composer
/// (docs/issues/2278-experience-content-redesign.md): one modal that brings
/// together the RSVP intention (Going / Maybe / No) and the contributions the
/// viewer will bring (needs/contributions).
///
/// Each "Bringing" chip is a shared [NeedClaimChip]: tapping it opens the
/// claim-a-need sheet for that item — or the "Edit your contribution" variant
/// when the viewer is already bringing it (same widget and behavior as the
/// pitching-in roster). "Add something else" opens the shared add-to-list sheet
/// (request + "I'll bring it myself" toggle), and the gear affordance links a
/// library item. Send commits only the RSVP intention.
class ExperienceRsvpComposerSheet extends ConsumerStatefulWidget {
  final String experienceId;
  final Color accentColor;
  final RSVPIntention initialIntention;

  /// When true, the modal is a contribution-only editor: the RSVP intention
  /// toggle is hidden, the header reads "Your Contribution", the footer summary
  /// line is dropped, and the primary action is "Save" (it commits nothing
  /// extra — contributions are saved inline as they're added — it just closes).
  /// Used by the expanded pitching-in panel, where RSVP lives in its own
  /// control, so this surface is purely "what you're bringing."
  final bool contributionsOnly;

  const ExperienceRsvpComposerSheet({
    super.key,
    required this.experienceId,
    required this.accentColor,
    required this.initialIntention,
    this.contributionsOnly = false,
  });

  /// Opens the composer as an accessible bottom-sheet modal.
  static Future<void> show(
    BuildContext context, {
    required String experienceId,
    required Color accentColor,
    required RSVPIntention initialIntention,
    bool contributionsOnly = false,
  }) {
    return showAccessibleModal<void>(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (_) => ExperienceRsvpComposerSheet(
        experienceId: experienceId,
        accentColor: accentColor,
        initialIntention: initialIntention,
        contributionsOnly: contributionsOnly,
      ),
    );
  }

  @override
  ConsumerState<ExperienceRsvpComposerSheet> createState() =>
      _ExperienceRsvpComposerSheetState();
}

class _ExperienceRsvpComposerSheetState
    extends ConsumerState<ExperienceRsvpComposerSheet> {
  late RSVPIntention _intention;
  bool _saving = false;

  @override
  void initState() {
    super.initState();
    _intention =
        widget.initialIntention == RSVPIntention.RSVP_INTENTION_UNSPECIFIED
        ? RSVPIntention.RSVP_INTENTION_YES
        : widget.initialIntention;
  }

  ExperienceNeedsNotifier get _needsNotifier =>
      ref.read(experienceNeedsProvider(widget.experienceId).notifier);

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    final state = ref.watch(experienceProvider(widget.experienceId));
    final needs = ref.watch(experienceNeedsProvider(widget.experienceId));
    final exp = state.experienceDetails?.experience;
    if (exp == null) return const SizedBox.shrink();

    // The viewer's current contributions (live), grouped by title, so a chip
    // reflects its selected state, quantity (×N), and attached comment, and the
    // footer summary reflects what's committed.
    final myContrib = <String, List<ExperienceContributionResponse>>{};
    for (final c in needs.contributions) {
      if (c.contributor.id == state.currentUserId && c.title.isNotEmpty) {
        (myContrib[c.title] ??= []).add(c);
      }
    }

    // Chip set: the viewer's contributions (selected) + server suggestions +
    // the event's open needs. Order-stable, de-duplicated.
    final chips = <String>[];
    void addChip(String t) {
      if (t.isNotEmpty && !chips.contains(t)) chips.add(t);
    }

    for (final t in myContrib.keys) {
      addChip(t);
    }
    for (final s in needs.suggestions) {
      addChip(s);
    }
    // Open needs carry a note + proposer that the claim sheet surfaces, so map
    // them by (lowercased) name for the shared chip below.
    final needByTitle = <String, ExperienceNeedResponse>{};
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
                    if (!widget.contributionsOnly) _intentionToggle(l10n),
                    if (widget.contributionsOnly ||
                        _intention != RSVPIntention.RSVP_INTENTION_NO) ...[
                      if (!widget.contributionsOnly)
                        const SizedBox(height: 18),
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
                        onTap: _openAddToList,
                      ),
                    ],
                  ],
                ),
              ),
            ),
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 0, 20, 4),
              child: _footer(l10n, myContrib.keys.toList()),
            ),
          ],
        ),
      ),
    );
  }

  String _title(AppLocalizations l10n) {
    if (widget.contributionsOnly) return l10n.contributionComposerTitle;
    switch (_intention) {
      case RSVPIntention.RSVP_INTENTION_YES:
        return l10n.rsvpComposerTitleGoing;
      case RSVPIntention.RSVP_INTENTION_NO:
        return l10n.rsvpComposerTitleNo;
      default:
        return l10n.rsvpComposerTitleMaybe;
    }
  }

  /// Eyebrow + close, then the (intention-dependent) title and subtitle —
  /// matching the eyebrow/title/subtitle header of the time and location
  /// poll modals.
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
                  (widget.contributionsOnly
                          ? l10n.contributionComposerKicker
                          : l10n.rsvpComposerKicker)
                      .toUpperCase(),
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
                _title(l10n),
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
              widget.contributionsOnly
                  ? l10n.contributionComposerSubtitle
                  : l10n.rsvpComposerSubtitle,
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

  Widget _intentionToggle(AppLocalizations l10n) {
    return Row(
      children: [
        Expanded(
          child: GlassChip(
            primary: l10n.experienceGoing,
            semanticsLabel: l10n.a11yExpRsvpGoing,
            selected: _intention == RSVPIntention.RSVP_INTENTION_YES,
            onTap: () =>
                setState(() => _intention = RSVPIntention.RSVP_INTENTION_YES),
          ),
        ),
        const SizedBox(width: 8),
        Expanded(
          child: GlassChip(
            primary: l10n.experienceRsvpMaybe,
            semanticsLabel: l10n.a11yExpRsvpMaybe,
            selected: _intention == RSVPIntention.RSVP_INTENTION_MAYBE,
            onTap: () =>
                setState(() => _intention = RSVPIntention.RSVP_INTENTION_MAYBE),
          ),
        ),
        const SizedBox(width: 8),
        Expanded(
          child: GlassChip(
            primary: l10n.experienceRsvpNo,
            semanticsLabel: l10n.a11yExpRsvpNotGoing,
            selected: _intention == RSVPIntention.RSVP_INTENTION_NO,
            onTap: () =>
                setState(() => _intention = RSVPIntention.RSVP_INTENTION_NO),
          ),
        ),
      ],
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

  /// Builds a contribution chip for [title]. When the viewer is already
  /// bringing the item the chip is selected and shows their quantity (×N) and
  /// a comment glyph if they left a note; otherwise it carries the matching
  /// open need's slots/note (if any) so the claim sheet is pre-filled.
  Widget _chip(
    String title,
    Map<String, List<ExperienceContributionResponse>> myContrib,
    Map<String, ExperienceNeedResponse> needByTitle,
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

  NeedsActions _needsActions(ExperienceState state) {
    final exp = state.experienceDetails!.experience;
    final intention = state.currentUserIntention;
    return NeedsActions(
      scope: NeedsScope.experience(
        experienceId: widget.experienceId,
        currentUserId: state.currentUserId,
        // The composer only opens for active events, so never terminal here.
        isTerminal: false,
        communityId: state.communityId ?? '',
        isRsvped:
            intention == RSVPIntention.RSVP_INTENTION_YES ||
            intention == RSVPIntention.RSVP_INTENTION_MAYBE,
        isRsvpedMaybe: intention == RSVPIntention.RSVP_INTENTION_MAYBE,
        ownerId: exp.owner.id,
        experienceName: exp.name,
      ),
    );
  }

  /// Opens the shared "add to list" sheet (request + "I'll bring it myself"
  /// toggle), reusing the needs flow rather than a bespoke dialog.
  Future<void> _openAddToList() async {
    final state = ref.read(experienceProvider(widget.experienceId));
    if (state.experienceDetails?.experience == null) return;
    await _needsActions(state).openSingleAddPicker(context, ref);
  }

  /// Opens the gear picker so the viewer can link an item from their library
  /// (reuses the shared needs gear-picker / claim-gear-linking flow, #2240).
  /// The pick is added as a gear-linked contribution immediately.
  Future<void> _pickFromGear() async {
    final communityId = ref
        .read(experienceProvider(widget.experienceId))
        .communityId;
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

  /// A confirmation summary line above the standard glass primary action. In
  /// contribution-only mode the summary is dropped and the action reads "Save".
  Widget _footer(AppLocalizations l10n, List<String> bringing) {
    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        if (!widget.contributionsOnly)
          Text(
            _summary(l10n, bringing),
            textAlign: TextAlign.center,
            style: const TextStyle(
              color: AppColors.modalTextSecondary,
              fontSize: 13,
              height: 1.3,
            ),
          ),
        GlassFooterButtons(
          primaryLabel: widget.contributionsOnly
              ? l10n.contributionComposerSave
              : l10n.rsvpComposerSend,
          primarySemanticsLabel: widget.contributionsOnly
              ? l10n.a11yContributionComposerSave
              : l10n.a11yRsvpComposerSend,
          primaryEnabled: !_saving,
          onPrimary: _send,
          showSecondary: false,
        ),
      ],
    );
  }

  String _summary(AppLocalizations l10n, List<String> bringing) {
    final String label;
    switch (_intention) {
      case RSVPIntention.RSVP_INTENTION_YES:
        label = l10n.experienceGoing;
      case RSVPIntention.RSVP_INTENTION_NO:
        label = l10n.experienceRsvpNo;
      default:
        label = l10n.experienceRsvpMaybe;
    }
    if (_intention == RSVPIntention.RSVP_INTENTION_NO || bringing.isEmpty) {
      return label;
    }
    return '$label · ${bringing.join(', ')}';
  }

  Future<void> _send() async {
    // Contribution-only mode commits nothing extra: each contribution is saved
    // inline as it's added (chip claim, add-to-list, gear link), so "Save" just
    // closes the editor. RSVP is owned by the panel's own control here.
    if (widget.contributionsOnly) {
      Navigator.of(context).pop();
      return;
    }
    setState(() => _saving = true);
    try {
      await ref
          .read(experienceProvider(widget.experienceId).notifier)
          .updateRSVP(_intention);
    } finally {
      if (mounted) Navigator.of(context).pop();
    }
  }
}
