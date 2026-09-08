import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/presentation/viewmodels/needs_scope.dart';
import 'package:ripls/presentation/widgets/accessibility/semantic_announcer.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass_sheet.dart';
import 'package:ripls/presentation/widgets/needs/needs_claim_confirm_sheet.dart';
import 'package:ripls/presentation/widgets/needs/needs_claim_row.dart';
import 'package:ripls/presentation/widgets/needs/needs_copy.dart';
import 'package:ripls/presentation/widgets/needs/needs_volunteer_atoms.dart';

/// One Need row fed to [NeedsVolunteerSheet]'s list.
///
/// v2 collapses the previous Needs + Contributions two-section split
/// into a single unified list — Need-linked contributions surface as
/// avatars in the parent Need's [contributors] list, and freestanding
/// contributions surface as `isFreestanding: true` synthetic entries
/// rendered in claimed state.
class NeedsVolunteerEntry {
  /// Stable identifier. For real Needs this is the Need's `id`; for
  /// synthetic freestanding-contribution rows this is the
  /// Contribution's `id` (so the claim/unclaim callbacks know which
  /// row was tapped, even though they're inert in v2).
  final String id;
  final String name;

  /// Optional inline note from the proposer.
  final String? note;

  /// "added by {name}" attribution shown only when the proposer
  /// isn't the current user. Caller is responsible for the
  /// hide-on-self check.
  final String? addedByName;

  /// Total slots on the Need (1 for a freestanding contribution).
  final int slotsTotal;

  /// How many slots are still open. 0 for a freestanding
  /// contribution.
  final int slotsRemaining;

  /// How many slots the current user holds on this Need.
  final int claimedByMe;

  /// Every slot pledged on this Need, in order of contribution.
  /// Multi-slot claims by the same user appear as repeated entries
  /// — the row aggregates per user.id to render
  /// `Name (xN) [Gear]` inline.
  final List<NeedsContributorSlot> contributors;

  /// True when the row represents a freestanding contribution rather
  /// than an open Need. Freestanding rows are read-only — there is no
  /// slot to claim against; they render in their permanently-claimed
  /// state to communicate the offer.
  final bool isFreestanding;

  /// Whether the current user is allowed to edit this row in edit
  /// mode. Caller computes this: organizers see `true` on every row;
  /// members see `true` only on rows they proposed or claimed.
  final bool canEdit;

  /// The current viewer's linked gear on this Need (the `gearId` of
  /// their own contribution, if any). Seeds the claim modal's
  /// "Link gear" disclosure with the viewer's previous pick. Other
  /// contributors' gear is surfaced inline via
  /// [contributors] — each slot carries its own optional gearId.
  final String? gearId;

  /// User id of the need's proposer, used by the claim sheet to render
  /// "Asked by You" vs "Asked by Alex." Distinct from [addedByName],
  /// which is the row-level "added by …" attribution that only fires
  /// when the proposer isn't the current viewer.
  final String? proposerId;

  /// Display name of the proposer (mirror of [proposerId]). Set even
  /// when the proposer is the current viewer so the claim sheet can
  /// fall back to it on a missing [proposerId].
  final String? proposerName;

  const NeedsVolunteerEntry({
    required this.id,
    required this.name,
    required this.slotsTotal,
    required this.slotsRemaining,
    required this.claimedByMe,
    this.note,
    this.addedByName,
    this.contributors = const [],
    this.isFreestanding = false,
    this.canEdit = false,
    this.gearId,
    this.proposerId,
    this.proposerName,
  });

  bool get isFullyCovered => slotsRemaining == 0;
  bool get isClaimedByMe => claimedByMe > 0;
}

/// Volunteer surface — NV1 (no claims) / NV2 ("You're in") for
/// helpers, NO1 (organizer) for organizers. Tap a row to claim it;
/// tap again to release. Autosaves on each tap via the [onClaim] /
/// [onUnclaim] callbacks owned by the caller. There is no Save /
/// Submit button.
///
/// v2 chrome:
/// - Eyebrow + Title state-aware: "What can you bring?" idle,
///   "You're in." once the current user has at least one claim,
///   "Who's bringing what?" for the organizer.
/// - [NeedsClaimCounter] pill under the title.
/// - Single scrollable list of [NeedsClaimRow]s (uncovered-first
///   sort), then a [NeedsAddOptionGhost] dashed row.
/// - Bottom action row, 52px tall: `[Edit/Done | SaveIndicator]` for
///   members, `[Manage | SaveIndicator]` for organizers.
/// - Edit mode (organizer or member): sage banner + per-row pencil
///   on editable rows + dim on locked rows. Tapping an editable row
///   fires [onEditRow] with the entry id — the caller opens NK4.
class NeedsVolunteerSheet extends StatefulWidget {
  /// Scope flavour — picks Experience vs. Request copy via [NeedsCopy].
  final NeedsScopeKind scopeKind;

  final List<NeedsVolunteerEntry> entries;

  /// True when the parent entity is in a terminal state (COMPLETED /
  /// CANCELLED for Experiences, FULFILLED / CANCELLED for Requests).
  /// Disables claim and edit affordances.
  final bool isTerminal;

  /// Whether the current user can claim at all (parent not terminal,
  /// caller has membership, etc.).
  final bool canClaim;

  /// Called when the user confirms a claim. The optional [note] is
  /// captured by the Request-scope claim sheet and forwarded to
  /// `ClaimRequestNeed` as the contribution's description. The optional
  /// [gearId] is captured by the inline "Link gear" disclosure inside
  /// the claim sheet and forwarded to the claim RPC so the resulting
  /// contribution is linked to a specific item from the helper's
  /// library.
  final Future<void> Function(
    String entryId, {
    String? note,
    String? gearId,
  }) onClaim;

  /// Called when the user taps a row they've already claimed.
  final Future<void> Function(String entryId) onUnclaim;

  /// Organizer-only Manage trigger. When null the bottom-left slot
  /// renders the Edit/Done toggle instead — that's how the volunteer
  /// sheet differentiates the organizer NO1 layout from the member
  /// NV1/NV2 layout.
  final VoidCallback? onManageMenu;

  /// Called when the user taps the dashed "Add something we're
  /// missing" ghost row. The caller opens the Picker for a
  /// single-item add.
  final VoidCallback? onAddOption;

  /// Called when an editable row is tapped while edit mode is on.
  /// Caller opens NK4 picker pre-populated with that row's data.
  final void Function(String entryId)? onEditRow;

  /// Whether edit mode should start active. Lets the caller open the
  /// sheet directly into edit mode from the organizer's "Edit list"
  /// menu action.
  final bool initialEditMode;

  /// Current viewer's user id. Passed through to each row so the
  /// contributors line can substitute "You" for the viewer's own
  /// name, and used to gate the confirm-claim modal on Request scope.
  final String? currentUserId;

  /// Current viewer's full [User]. Required by the claim sheet's
  /// avatar circle. Null when unavailable; the sheet falls back to a
  /// generic glyph.
  final User? currentUser;

  const NeedsVolunteerSheet({
    super.key,
    required this.scopeKind,
    required this.entries,
    required this.isTerminal,
    required this.canClaim,
    required this.onClaim,
    required this.onUnclaim,
    this.onManageMenu,
    this.onAddOption,
    this.onEditRow,
    this.initialEditMode = false,
    this.currentUserId,
    this.currentUser,
    this.communityId,
  });

  /// Community scope used by the claim sheet's "Link gear" disclosure
  /// to filter gear suggestions. Null disables gear linking from the
  /// claim sheet (the dashed card renders without the disclosure).
  final String? communityId;

  static Future<void> show(
    BuildContext context, {
    required NeedsScopeKind scopeKind,
    required List<NeedsVolunteerEntry> entries,
    required bool isTerminal,
    required bool canClaim,
    required Future<void> Function(
      String entryId, {
      String? note,
      String? gearId,
    }) onClaim,
    required Future<void> Function(String entryId) onUnclaim,
    VoidCallback? onManageMenu,
    VoidCallback? onAddOption,
    void Function(String entryId)? onEditRow,
    bool initialEditMode = false,
    String? currentUserId,
    User? currentUser,
    String? communityId,
  }) {
    return showAccessibleModal<void>(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (_) => NeedsVolunteerSheet(
        scopeKind: scopeKind,
        entries: entries,
        isTerminal: isTerminal,
        canClaim: canClaim,
        onClaim: onClaim,
        onUnclaim: onUnclaim,
        onManageMenu: onManageMenu,
        onAddOption: onAddOption,
        onEditRow: onEditRow,
        initialEditMode: initialEditMode,
        currentUserId: currentUserId,
        currentUser: currentUser,
        communityId: communityId,
      ),
    );
  }

  @override
  State<NeedsVolunteerSheet> createState() => _NeedsVolunteerSheetState();
}

class _NeedsVolunteerSheetState extends State<NeedsVolunteerSheet> {
  late bool _editMode;

  @override
  void initState() {
    super.initState();
    _editMode = widget.initialEditMode;
  }

  Future<void> _onTapEntry(NeedsVolunteerEntry e) async {
    if (!widget.canClaim || widget.isTerminal || e.isFreestanding) return;
    final l10n = context.l10n;
    // Fully-covered rows stay tappable — the server allows
    // over-claiming so a helper can intentionally bring more than the
    // proposer asked for. The modal handles confirmation; we don't
    // need a tap-level gate on `slotsRemaining`.

    // Both scopes route through the same editorial claim sheet — taps
    // open the modal so the viewer can confirm a new claim, edit the
    // note / linked gear on an existing one, or remove themselves
    // with intent. Copy differs by scope via [NeedsCopy] inside the
    // sheet; the modal stack itself is identical.
    final result = await NeedsClaimConfirmSheet.show(
      context,
      needName: e.name,
      needNote: e.note,
      proposerId: e.proposerId,
      proposerName: e.proposerName,
      currentUserId: widget.currentUserId,
      currentUser: widget.currentUser,
      communityId: widget.communityId,
      canEditNeed: e.canEdit && widget.onEditRow != null,
      initialGearId: e.gearId,
      slotsNeeded: e.slotsTotal,
      isAlreadyClaimed: e.isClaimedByMe,
      initialQuantity: e.isClaimedByMe ? e.claimedByMe : 1,
    );
    if (!mounted || result == null) return;
    if (result.editRequested) {
      widget.onEditRow?.call(e.id);
      return;
    }
    if (result.unclaimRequested) {
      await widget.onUnclaim(e.id);
      if (!mounted) return;
      await SemanticAnnouncer.announce(context, l10n.a11yNeedsUnclaimed);
      return;
    }
    if (!result.confirmed) return;
    // Treat the modal's quantity as the helper's *new total* on the
    // row. On a fresh claim the existing count is 0, so the delta is
    // the full quantity; on an edit it's the change against the
    // current claim count. Positive delta → add claims; negative →
    // remove the surplus. The claim/unclaim RPCs are one-slot-at-a-
    // time so we loop. Same note + gear link rides on every new
    // claim.
    final targetQuantity = result.quantity < 1 ? 1 : result.quantity;
    final existing = e.isClaimedByMe ? e.claimedByMe : 0;
    final delta = targetQuantity - existing;
    if (delta > 0) {
      for (var i = 0; i < delta; i++) {
        await widget.onClaim(e.id, note: result.note, gearId: result.gearId);
        if (!mounted) return;
      }
      await SemanticAnnouncer.announce(context, l10n.a11yNeedsClaimed);
    } else if (delta < 0) {
      for (var i = 0; i < -delta; i++) {
        await widget.onUnclaim(e.id);
        if (!mounted) return;
      }
      await SemanticAnnouncer.announce(context, l10n.a11yNeedsUnclaimed);
    }
    // delta == 0 → nothing to dispatch. Note / gear-link edits aren't
    // wired through to existing contributions yet (tracked in #2148).
  }

  void _toggleEditMode() {
    setState(() => _editMode = !_editMode);
  }

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    final copy = NeedsCopy(l10n, widget.scopeKind);
    final isOrganizer = widget.onManageMenu != null;

    // Uncovered first (v2 rule).
    final sorted = [...widget.entries]..sort((a, b) {
        if (a.isFullyCovered == b.isFullyCovered) return 0;
        return a.isFullyCovered ? 1 : -1;
      });

    final totalClaimedByMe = widget.entries
        .fold<int>(0, (sum, e) => sum + e.claimedByMe);
    final hasOwnClaims = totalClaimedByMe > 0;

    // Slot-level totals for the SaveIndicator + ClaimCounter.
    var totalSlots = 0;
    var coveredSlots = 0;
    for (final e in widget.entries) {
      if (e.isFreestanding) continue;
      totalSlots += e.slotsTotal;
      coveredSlots += e.slotsTotal - e.slotsRemaining;
    }

    final headerEyebrow =
        hasOwnClaims ? copy.volunteerEyebrowYourein : copy.volunteerEyebrow;
    final headerTitle = isOrganizer
        ? copy.volunteerHeadlineOrganizer
        : hasOwnClaims
            ? copy.volunteerHeaderYourein
            : copy.volunteerHeadlineHelper;

    // Eyebrow rolls coverage into one row so the title can be a clean
    // "What's still needed" statement underneath. Pulse dot on the
    // left mirrors the prototype's "● YOU'RE ON IT · 1 OF 2 COVERED"
    // treatment.
    final eyebrowParts = <String>[headerEyebrow];
    if (totalSlots > 0) {
      eyebrowParts
          .add(copy.volunteerCoverageShort(coveredSlots, totalSlots));
    }
    final eyebrowText = eyebrowParts.join(' · ').toUpperCase();
    final showAddOptionRow =
        widget.onAddOption != null && !widget.isTerminal;

    return GlassSheet(
      padding: EdgeInsets.zero,
      child: SafeArea(
        top: false,
        child: ConstrainedBox(
          constraints: BoxConstraints(
            maxHeight: MediaQuery.of(context).size.height * 0.85,
          ),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Padding(
                padding: const EdgeInsets.fromLTRB(20, 4, 20, 0),
                child: _EyebrowRow(
                  text: eyebrowText,
                  pulse: hasOwnClaims,
                ),
              ),
              Padding(
                padding: const EdgeInsets.fromLTRB(20, 10, 20, 0),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Semantics(
                      header: true,
                      child: Text(
                        headerTitle,
                        style: const TextStyle(
                          fontFamily: AppTheme.headingFont,
                          fontSize: 30,
                          fontWeight: FontWeight.w600,
                          height: 1.08,
                          letterSpacing: -0.4,
                          color: AppColors.modalTextPrimary,
                        ),
                      ),
                    ),
                    if (_editMode) ...[
                      const SizedBox(height: 8),
                      NeedsEditModeBanner(
                        scopeKind: widget.scopeKind,
                        isOrganizer: isOrganizer,
                      ),
                    ],
                  ],
                ),
              ),
              Flexible(
                child: SingleChildScrollView(
                  padding: const EdgeInsets.fromLTRB(20, 16, 20, 16),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      if (widget.entries.isEmpty)
                        Padding(
                          padding: const EdgeInsets.symmetric(vertical: 24),
                          child: Text(
                            copy.volunteerEmpty,
                            textAlign: TextAlign.center,
                            style: const TextStyle(
                              fontSize: 13,
                              color: AppColors.modalTextMuted,
                            ),
                          ),
                        )
                      else
                        for (final e in sorted) _buildRow(context, e),
                      if (showAddOptionRow) ...[
                        const SizedBox(height: 4),
                        _BottomActionRow(
                          isOrganizer: isOrganizer,
                          editMode: _editMode,
                          isTerminal: widget.isTerminal,
                          onManage: widget.onManageMenu,
                          onToggleEdit: _toggleEditMode,
                          onAddOption: widget.onAddOption,
                          scopeKind: widget.scopeKind,
                          manageLabel: copy.volunteerManageButton,
                          manageSemanticsLabel: l10n.a11yNeedsManageMenu,
                        ),
                      ],
                    ],
                  ),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }

  Widget _buildRow(BuildContext context, NeedsVolunteerEntry e) {
    final l10n = context.l10n;
    final label = e.isFullyCovered
        ? l10n.a11yNeedsClaimRowCovered(e.name)
        : l10n.a11yNeedsClaimRow(e.name, e.slotsRemaining);
    final claimed = e.slotsTotal - e.slotsRemaining;
    return NeedsClaimRow(
      scopeKind: widget.scopeKind,
      title: e.name,
      note: e.note,
      addedByName: e.addedByName,
      needed: e.slotsTotal,
      claimed: claimed,
      claimedByMe: e.isClaimedByMe,
      myQty: e.claimedByMe,
      enabled: !e.isFreestanding && widget.canClaim && !widget.isTerminal,
      semanticsLabel: label,
      onTap: () => _onTapEntry(e),
      contributors: e.contributors,
      currentUserId: widget.currentUserId,
      editMode: _editMode,
      canEdit: e.canEdit && !e.isFreestanding && !widget.isTerminal,
      onEditTap: widget.onEditRow == null ? null : () => widget.onEditRow!(e.id),
    );
  }
}


/// Eyebrow row above the Volunteer sheet title. Renders an optional
/// sage pulse dot followed by the uppercased eyebrow text. The pulse
/// dot fires when the viewer has at least one own claim, mirroring
/// the prototype's "● YOU'RE ON IT · 1 OF 2 COVERED" treatment.
class _EyebrowRow extends StatelessWidget {
  const _EyebrowRow({required this.text, required this.pulse});

  final String text;
  final bool pulse;

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        if (pulse) ...[
          Container(
            width: 8,
            height: 8,
            decoration: BoxDecoration(
              color: AppColors.experienceSageGreen,
              shape: BoxShape.circle,
              boxShadow: [
                BoxShadow(
                  color: AppColors.experienceSageGreen.withValues(alpha: 0.35),
                  blurRadius: 6,
                  spreadRadius: 2,
                ),
              ],
            ),
          ),
          const SizedBox(width: 10),
        ],
        Flexible(
          child: Text(
            text,
            style: TextStyle(
              fontSize: 11,
              fontWeight: FontWeight.w700,
              letterSpacing: 1.6,
              color: pulse
                  ? AppColors.experienceSageGreen
                  : AppColors.modalTextSecondary,
            ),
          ),
        ),
      ],
    );
  }
}

/// Bottom-of-list action row replacing the old separate-toolbar
/// pattern. Helpers see the AddOptionGhost full-width; organizers see
/// a compact Manage / Done button on the LEFT followed by the
/// AddOptionGhost on the right, per the redesign.
class _BottomActionRow extends StatelessWidget {
  const _BottomActionRow({
    required this.isOrganizer,
    required this.editMode,
    required this.isTerminal,
    required this.onManage,
    required this.onToggleEdit,
    required this.onAddOption,
    required this.scopeKind,
    required this.manageLabel,
    required this.manageSemanticsLabel,
  });

  final bool isOrganizer;
  final bool editMode;
  final bool isTerminal;
  final VoidCallback? onManage;
  final VoidCallback onToggleEdit;
  final VoidCallback? onAddOption;
  final NeedsScopeKind scopeKind;
  final String manageLabel;
  final String manageSemanticsLabel;

  @override
  Widget build(BuildContext context) {
    final addCard = NeedsAddOptionGhost(
      scopeKind: scopeKind,
      onTap: onAddOption,
    );

    if (!isOrganizer || isTerminal) {
      return addCard;
    }

    // Organizer: while editing, the Manage slot becomes "Done"
    // (mirrors the legacy Edit/Done flip). Otherwise it's the regular
    // Manage pill. Either way, the action sits to the LEFT of the
    // Add-something card.
    final leading = editMode
        ? NeedsEditDoneButton(active: true, onTap: onToggleEdit)
        : NeedsManagePillButton(
            onTap: onManage!,
            label: manageLabel,
            semanticsLabel: manageSemanticsLabel,
          );

    return Row(
      crossAxisAlignment: CrossAxisAlignment.center,
      children: [
        leading,
        const SizedBox(width: 10),
        Expanded(child: addCard),
      ],
    );
  }
}
