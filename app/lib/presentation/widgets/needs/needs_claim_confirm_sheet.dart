import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/data/gen/ripls/api/gear_service.pb.dart' show GearItem;
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/gear/gear_pill.dart' show GearLink;
import 'package:ripls/presentation/widgets/modal/glass/glass_sheet.dart';
import 'package:ripls/presentation/widgets/needs/needs_claim_gear_picker.dart';
import 'package:ripls/presentation/widgets/needs/needs_edit_atoms.dart';
import 'package:ripls/presentation/widgets/needs/needs_lend_give_choice.dart';
import 'package:ripls/presentation/widgets/user_avatar.dart';

/// Result returned by [NeedsClaimConfirmSheet]. `confirmed=false`
/// means the user dismissed without claiming; `confirmed=true` means
/// fire the claim, optionally with [note] / [gearId]; [editRequested]
/// signals that the caller should dispatch the need-edit flow instead.
class NeedsClaimConfirmResult {
  const NeedsClaimConfirmResult({
    required this.confirmed,
    this.note,
    this.gearId,
    this.editRequested = false,
    this.unclaimRequested = false,
    this.quantity = 1,
    this.give = false,
  });
  final bool confirmed;
  final String? note;

  /// Optional gear ID captured by the "Link gear" disclosure. Null
  /// means the helper didn't link any gear — the claim is recorded as
  /// a bare commitment.
  final String? gearId;

  /// The helper's Lend/Give choice for a gear-backed offer (#2702):
  /// false = lend (default), true = give. Only meaningful when
  /// [gearId] is set and the sheet was shown with the lend/give
  /// choice enabled (Request scope).
  final bool give;

  /// Set when the user tapped the "Edit" pencil pill on the need
  /// title. The caller dismisses the sheet and dispatches the
  /// edit-row flow instead of firing the claim RPC.
  final bool editRequested;

  /// Set when an already-claimed viewer tapped "Remove my claim" on
  /// the manage variant of the sheet. The caller dispatches the
  /// unclaim RPC instead of the claim RPC.
  final bool unclaimRequested;

  /// How many slots the helper is pledging. Caller loops the claim
  /// RPC this many times (1..N), passing the same note / gearId on
  /// every call so each resulting contribution carries the same
  /// metadata. Always ≥ 1.
  final int quantity;
}

/// Editorial claim sheet opened when a Request-scope user taps an
/// uncovered need on the Volunteer surface. Renders the parent's
/// title in a small uppercase eyebrow, the need name in the serif
/// heading font, an "Asked by …" attribution, a dashed-border card
/// with the viewer's avatar + the inline "{You} · I'll bring this"
/// label, a note text field, an optional "Link gear" disclosure, and
/// a sage "I'll bring this" CTA at the bottom. An optional pencil
/// pill next to the title routes the viewer into the need-edit flow.
class NeedsClaimConfirmSheet extends StatefulWidget {
  const NeedsClaimConfirmSheet({
    super.key,
    required this.needName,
    required this.needNote,
    required this.proposerId,
    required this.proposerName,
    required this.currentUserId,
    required this.currentUser,
    required this.communityId,
    required this.canEditNeed,
    required this.initialGearId,
    required this.slotsNeeded,
    this.isAlreadyClaimed = false,
    this.initialQuantity = 1,
    this.showLendGiveChoice = false,
  });

  final String needName;
  final String? needNote;

  /// True when linking gear should also surface the Lend/Give choice
  /// (#2702) — the Request-scope claim flow, where confirming a
  /// gear-backed claim starts a real loan or giveaway offer to the
  /// requester. Experience scope keeps the plain display-only gear link.
  final bool showLendGiveChoice;

  /// Total slots needed on this row (the proposer's intended count).
  /// Read-only on the claim sheet — surfaced via
  /// [NeedsEditQuantityStepper] in its read-only mode so the helper
  /// sees how many people / items the proposer asked for without an
  /// affordance to edit (that lives on the NK4 edit modal).
  final int slotsNeeded;

  final String? proposerId;
  final String? proposerName;
  final String? currentUserId;
  final User? currentUser;

  /// Community context forwarded to [NeedsClaimGearPicker] so the
  /// suggested section can intersect with community-scoped gear search.
  /// When null/empty the "Link gear" disclosure is hidden.
  final String? communityId;

  /// True when the viewer is allowed to edit this need's name /
  /// quantity / note. Drives whether the "Edit" pencil pill renders on
  /// the title row.
  final bool canEditNeed;

  /// Gear ID already linked on the viewer's existing contribution for
  /// this need (when adjusting an existing claim). Seeds the "Link
  /// gear" disclosure so the picker opens with the current pick
  /// highlighted.
  final String? initialGearId;

  /// True when the viewer is already on this need. Swaps the eyebrow
  /// to "EDIT YOUR CONTRIBUTION", seeds the quantity stepper with
  /// [initialQuantity] (the helper's current claim count) and the
  /// CTA pair from "I'll bring this" to
  /// `[Remove my claim | Save changes]` so the viewer can adjust
  /// their pledge or back out without accidentally tap-toggling on
  /// the row.
  final bool isAlreadyClaimed;

  /// Initial value for the quantity stepper. Defaults to 1 for a
  /// fresh claim; callers seed this with the viewer's existing claim
  /// count when [isAlreadyClaimed] is true so the stepper reflects
  /// what they're actually bringing today.
  final int initialQuantity;

  static Future<NeedsClaimConfirmResult?> show(
    BuildContext context, {
    required String needName,
    String? needNote,
    String? proposerId,
    String? proposerName,
    String? currentUserId,
    User? currentUser,
    String? communityId,
    bool canEditNeed = false,
    String? initialGearId,
    required int slotsNeeded,
    bool isAlreadyClaimed = false,
    int initialQuantity = 1,
    bool showLendGiveChoice = false,
  }) {
    return showAccessibleModal<NeedsClaimConfirmResult>(
      context,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      isScrollControlled: true,
      builder: (_) => NeedsClaimConfirmSheet(
        needName: needName,
        needNote: needNote,
        proposerId: proposerId,
        proposerName: proposerName,
        currentUserId: currentUserId,
        currentUser: currentUser,
        communityId: communityId,
        canEditNeed: canEditNeed,
        initialGearId: initialGearId,
        slotsNeeded: slotsNeeded,
        isAlreadyClaimed: isAlreadyClaimed,
        initialQuantity: initialQuantity,
        showLendGiveChoice: showLendGiveChoice,
      ),
    );
  }

  @override
  State<NeedsClaimConfirmSheet> createState() =>
      _NeedsClaimConfirmSheetState();
}

class _NeedsClaimConfirmSheetState extends State<NeedsClaimConfirmSheet> {
  final _noteCtrl = TextEditingController();

  /// Gear ID currently linked to the draft claim. Seeded from
  /// [widget.initialGearId] so re-opens of the sheet preserve the
  /// helper's prior pick.
  String? _selectedGearId;

  /// Snapshot of the picked gear, used to render its name and
  /// thumbnail inline on the "Link gear" tile after the picker
  /// closes. Null when no gear is linked or when the sheet opened
  /// pre-seeded with [widget.initialGearId] alone (without a backing
  /// snapshot) — the tile then falls back to a generic "linked" pill.
  GearItem? _selectedGear;

  /// How many slots the helper is pledging on this claim. Seeded
  /// from [widget.initialQuantity] so the stepper opens at the
  /// helper's current claim count when editing.
  late int _quantity;

  /// The Lend/Give choice for a gear-backed offer (#2702). False =
  /// lend (the default); only surfaced when gear is linked and
  /// [NeedsClaimConfirmSheet.showLendGiveChoice] is on.
  bool _give = false;

  @override
  void initState() {
    super.initState();
    _selectedGearId = widget.initialGearId;
    _quantity = widget.initialQuantity < 1 ? 1 : widget.initialQuantity;
  }

  @override
  void dispose() {
    _noteCtrl.dispose();
    super.dispose();
  }

  void _confirm() {
    final raw = _noteCtrl.text.trim();
    Navigator.of(context).pop(NeedsClaimConfirmResult(
      confirmed: true,
      note: raw.isEmpty ? null : raw,
      gearId: _selectedGearId,
      quantity: _quantity,
      give: _give,
    ));
  }

  void _dismiss() {
    Navigator.of(context).pop(const NeedsClaimConfirmResult(confirmed: false));
  }

  void _requestEdit() {
    Navigator.of(context).pop(const NeedsClaimConfirmResult(
      confirmed: false,
      editRequested: true,
    ));
  }

  void _requestUnclaim() {
    Navigator.of(context).pop(const NeedsClaimConfirmResult(
      confirmed: false,
      unclaimRequested: true,
    ));
  }

  Future<void> _openGearPicker() async {
    final result = await NeedsClaimGearPicker.show(
      context,
      communityId: widget.communityId,
      initialQuery: widget.needName,
      initialGearId: _selectedGearId,
    );
    if (!mounted || result == null) return;
    setState(() {
      _selectedGearId = result.gearId;
      // Clear the cached snapshot when the user cleared the link;
      // otherwise adopt the picker's snapshot so we can render the
      // gear's name + thumbnail inline without an extra fetch.
      _selectedGear = result.gearId == null ? null : result.gear;
    });
  }

  /// Clears the linked gear directly from the tile — short-circuits
  /// the picker round-trip so unlinking is a one-tap action. The
  /// resulting state flows to the claim RPC via
  /// [NeedsClaimConfirmResult.gearId] = null.
  void _unlinkGear() {
    setState(() {
      _selectedGearId = null;
      _selectedGear = null;
    });
  }

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    final accent = AppColors.experienceSageGreen;

    // "Asked by You" when the proposer is the current viewer;
    // otherwise "Asked by {proposer name}". Fall back gracefully when
    // either id or name is missing.
    final isProposerMe = widget.proposerId != null &&
        widget.currentUserId != null &&
        widget.proposerId == widget.currentUserId;
    final askedBy = isProposerMe
        ? l10n.needsClaimSheetAskedByYou
        : (widget.proposerName != null && widget.proposerName!.isNotEmpty)
            ? l10n.needsClaimSheetAskedBy(widget.proposerName!)
            : null;

    // Reframe the surface as an edit-your-contribution screen when
    // the viewer is already on the row — copy mirrors the NK4 edit
    // sheet's intent.
    final eyebrowText = widget.isAlreadyClaimed
        ? l10n.needsClaimSheetEyebrowEdit.toUpperCase()
        : l10n.needsClaimSheetEyebrow.toUpperCase();

    final hasGearLinking =
        widget.communityId != null && widget.communityId!.isNotEmpty;

    // Pledge quantity is uncapped on the claim sheet — a helper can
    // bring more than the proposer asked for. The stepper still bottoms
    // out at 1.
    final stepperQuantity = _quantity < 1 ? 1 : _quantity;
    final cardChildren = <Widget>[
      _MineBringingRow(
        currentUser: widget.currentUser,
        label: l10n
            .needsClaimSheetMineBringingLabel(l10n.needsRowContributorYou),
      ),
      const SizedBox(height: 12),
      _NoteField(
        controller: _noteCtrl,
        placeholder: l10n.needsClaimSheetNotePlaceholder,
      ),
      if (hasGearLinking) ...[
        const SizedBox(height: 10),
        _LinkGearTile(
          gear: _selectedGear,
          isLinked: _selectedGearId != null,
          title: l10n.needsClaimSheetLinkGearTitle,
          hintIdle: l10n.needsClaimSheetLinkGearHint,
          hintLinked: l10n.needsClaimSheetLinkGearPicked,
          semanticsLabel: l10n.a11yNeedsClaimSheetLinkGear,
          unlinkSemanticsLabel: l10n.a11yNeedsClaimSheetUnlinkGear,
          onTap: _openGearPicker,
          onUnlink: _unlinkGear,
        ),
        // Lend / Give choice (#2702): confirming a gear-backed claim on a
        // Request starts a real offer to the requester, so the helper picks
        // whether the item comes back.
        if (widget.showLendGiveChoice && _selectedGearId != null) ...[
          const SizedBox(height: 10),
          NeedsLendGiveChoice(
            give: _give,
            onChanged: (next) => setState(() => _give = next),
          ),
        ],
      ],
      // Quantity sits at the bottom of the card — the helper decides
      // how many they're bringing after they've set their note and
      // (optionally) linked gear. Always rendered so a helper can
      // pledge more than the proposer asked for, including bumping a
      // single-slot need to 2+ items.
      const SizedBox(height: 10),
      Align(
        alignment: Alignment.centerLeft,
        child: NeedsEditQuantityStepper(
          value: stepperQuantity,
          suffix: l10n.needsClaimSheetQuantitySuffix(stepperQuantity),
          decrementLabel: l10n.a11yNeedsDecrement,
          incrementLabel: l10n.a11yNeedsIncrement,
          max: null,
          onChanged: (next) => setState(
            () => _quantity = next < 1 ? 1 : next,
          ),
        ),
      ),
    ];

    return GlassSheet(
      padding: EdgeInsets.zero,
      showDragHandle: true,
      child: SafeArea(
        top: false,
        child: Padding(
          padding: EdgeInsets.fromLTRB(
            20,
            8,
            20,
            24 + MediaQuery.of(context).viewInsets.bottom,
          ),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              // Eyebrow + close X.
              Row(
                children: [
                  Expanded(
                    child: Text(
                      eyebrowText,
                      style: const TextStyle(
                        fontSize: 11,
                        fontWeight: FontWeight.w700,
                        letterSpacing: 1.6,
                        color: AppColors.modalTextSecondary,
                      ),
                    ),
                  ),
                  IconAction(
                    icon: Icons.close,
                    semanticsLabel: l10n.a11yNeedsClaimSheetClose,
                    color: AppColors.modalTextPrimary,
                    onPressed: _dismiss,
                  ),
                ],
              ),
              const SizedBox(height: 6),
              // Need name (serif heading) + optional Edit pencil pill.
              Row(
                crossAxisAlignment: CrossAxisAlignment.center,
                children: [
                  Expanded(
                    child: Text(
                      widget.needName,
                      style: const TextStyle(
                        fontFamily: AppTheme.headingFont,
                        fontSize: 32,
                        height: 1.05,
                        fontWeight: FontWeight.w600,
                        letterSpacing: -0.4,
                        color: AppColors.modalTextPrimary,
                      ),
                    ),
                  ),
                  if (widget.canEditNeed) ...[
                    const SizedBox(width: 8),
                    _EditNeedPill(
                      label: l10n.needsClaimSheetEditNeed,
                      semanticsLabel: l10n.a11yNeedsClaimSheetEditNeed,
                      onTap: _requestEdit,
                    ),
                  ],
                ],
              ),
              if (askedBy != null) ...[
                const SizedBox(height: 6),
                Text(
                  askedBy,
                  style: const TextStyle(
                    fontSize: 14,
                    color: AppColors.modalTextSecondary,
                  ),
                ),
              ],
              if (widget.needNote != null && widget.needNote!.isNotEmpty) ...[
                const SizedBox(height: 8),
                Text(
                  widget.needNote!,
                  style: const TextStyle(
                    fontSize: 13.5,
                    color: AppColors.modalTextSecondary,
                    height: 1.4,
                  ),
                ),
              ],
              const SizedBox(height: 18),
              // Dashed-border card holding the avatar+pledge row, the
              // editable quantity stepper, the note field, and the
              // optional gear-link disclosure.
              Container(
                padding: const EdgeInsets.all(14),
                decoration: ShapeDecoration(
                  shape: _ClaimCardBorder(
                    color: AppColors.modalTextPrimary.withValues(alpha: 0.22),
                    radius: const Radius.circular(16),
                  ),
                ),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  mainAxisSize: MainAxisSize.min,
                  children: cardChildren,
                ),
              ),
              const SizedBox(height: 18),
              if (widget.isAlreadyClaimed)
                // Manage variant: helper is already on the row, so
                // pair the sage Save with a red Remove so they can
                // back out intentionally.
                Row(
                  children: [
                    Expanded(
                      flex: 1,
                      child: _ClaimRemoveButton(
                        label: l10n.needsClaimSheetRemoveClaimCta,
                        semanticsLabel: l10n.a11yNeedsClaimSheetRemoveClaim,
                        onTap: _requestUnclaim,
                      ),
                    ),
                    const SizedBox(width: 10),
                    Expanded(
                      flex: 2,
                      child: _ClaimPrimaryButton(
                        label: l10n.needsClaimSheetSaveChangesCta,
                        accent: accent,
                        onTap: _confirm,
                      ),
                    ),
                  ],
                )
              else
                _ClaimPrimaryButton(
                  label: l10n.needsClaimSheetConfirmCta,
                  accent: accent,
                  onTap: _confirm,
                ),
            ],
          ),
        ),
      ),
    );
  }
}

/// Sage primary CTA — used for both "I'll bring this" (fresh claim)
/// and "Save changes" (already-claimed manage variant). Leading check
/// icon mirrors the prototype's claim-sheet treatment.
class _ClaimPrimaryButton extends StatelessWidget {
  const _ClaimPrimaryButton({
    required this.label,
    required this.accent,
    required this.onTap,
  });
  final String label;
  final Color accent;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: label,
      onTap: onTap,
      inkBorderRadius: BorderRadius.circular(16),
      child: Container(
        width: double.infinity,
        height: 52,
        alignment: Alignment.center,
        decoration: BoxDecoration(
          color: accent,
          borderRadius: BorderRadius.circular(16),
          boxShadow: [
            BoxShadow(
              color: accent.withValues(alpha: 0.30),
              offset: const Offset(0, 8),
              blurRadius: 20,
            ),
          ],
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(Icons.check, size: 18, color: Color(0xFF0F1A14)),
            const SizedBox(width: 8),
            Text(
              label,
              style: const TextStyle(
                fontSize: 16,
                fontWeight: FontWeight.w700,
                letterSpacing: 0.1,
                color: Color(0xFF0F1A14),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

/// Red Remove button paired with [_ClaimPrimaryButton] on the
/// already-claimed variant of the claim sheet. Dispatches the unclaim
/// RPC via the [NeedsClaimConfirmResult.unclaimRequested] flag.
class _ClaimRemoveButton extends StatelessWidget {
  const _ClaimRemoveButton({
    required this.label,
    required this.semanticsLabel,
    required this.onTap,
  });
  final String label;
  final String semanticsLabel;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: semanticsLabel,
      onTap: onTap,
      inkBorderRadius: BorderRadius.circular(16),
      child: Container(
        height: 52,
        alignment: Alignment.center,
        decoration: BoxDecoration(
          color: AppColors.statusErrorOnDark,
          borderRadius: BorderRadius.circular(16),
        ),
        child: Row(
          mainAxisAlignment: MainAxisAlignment.center,
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(Icons.close, size: 14, color: Colors.white),
            const SizedBox(width: 6),
            Text(
              label,
              style: const TextStyle(
                fontSize: 14,
                fontWeight: FontWeight.w700,
                color: Colors.white,
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _MineBringingRow extends StatelessWidget {
  const _MineBringingRow({required this.currentUser, required this.label});
  final User? currentUser;
  final String label;

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        if (currentUser != null)
          UserAvatar(user: currentUser!, radius: 16)
        else
          Container(
            width: 32,
            height: 32,
            decoration: BoxDecoration(
              color: AppColors.experienceSageGreen.withValues(alpha: 0.30),
              shape: BoxShape.circle,
            ),
            alignment: Alignment.center,
            child: const Icon(Icons.person_outline,
                size: 16, color: AppColors.modalTextPrimary),
          ),
        const SizedBox(width: 12),
        Expanded(
          child: Text(
            label,
            style: const TextStyle(
              fontSize: 15,
              fontWeight: FontWeight.w600,
              color: AppColors.modalTextPrimary,
            ),
          ),
        ),
      ],
    );
  }
}

/// Pencil-pill rendered next to the need title on the claim sheet
/// when the viewer can edit the need (proposer or organizer). Tapping
/// dismisses the sheet so the caller can dispatch the existing
/// edit-row flow (NK4 picker pre-populated with the row's data).
class _EditNeedPill extends StatelessWidget {
  const _EditNeedPill({
    required this.label,
    required this.semanticsLabel,
    required this.onTap,
  });
  final String label;
  final String semanticsLabel;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: semanticsLabel,
      onTap: onTap,
      inkBorderRadius: BorderRadius.circular(999),
      child: Container(
        padding: const EdgeInsets.fromLTRB(10, 6, 12, 6),
        decoration: BoxDecoration(
          color: AppColors.modalTextPrimary.withValues(alpha: 0.08),
          borderRadius: BorderRadius.circular(999),
          border: Border.all(
            color: AppColors.modalTextPrimary.withValues(alpha: 0.18),
          ),
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(
              Icons.edit_outlined,
              size: 14,
              color: AppColors.modalTextPrimary.withValues(alpha: 0.78),
            ),
            const SizedBox(width: 6),
            Text(
              label,
              style: TextStyle(
                fontSize: 12.5,
                fontWeight: FontWeight.w600,
                color: AppColors.modalTextPrimary.withValues(alpha: 0.85),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

/// Inline disclosure tile rendered inside the dashed claim card
/// inviting the helper to link an item from their library to the
/// claim. Tap opens [NeedsClaimGearPicker]; the resulting gear ID is
/// captured by the parent state and forwarded with the claim RPC.
class _LinkGearTile extends StatelessWidget {
  const _LinkGearTile({
    required this.gear,
    required this.isLinked,
    required this.title,
    required this.hintIdle,
    required this.hintLinked,
    required this.semanticsLabel,
    required this.unlinkSemanticsLabel,
    required this.onTap,
    required this.onUnlink,
  });

  /// Snapshot of the linked gear, used to render the thumbnail + name
  /// pill once the picker closes. Null when no gear is linked, or when
  /// the sheet opened with [initialGearId] alone (no backing snapshot);
  /// in the latter case the tile falls back to the generic [hintLinked]
  /// pill.
  final GearItem? gear;
  final bool isLinked;
  final String title;
  final String hintIdle;
  final String hintLinked;
  final String semanticsLabel;
  final String unlinkSemanticsLabel;
  final VoidCallback onTap;

  /// Called when the helper taps the trailing × on the tile to clear
  /// the linked gear without opening the picker. Only rendered when
  /// [isLinked] is true.
  final VoidCallback onUnlink;

  @override
  Widget build(BuildContext context) {
    final accent = AppColors.experienceSageGreen;
    return Tappable(
      semanticsLabel: semanticsLabel,
      onTap: onTap,
      inkBorderRadius: BorderRadius.circular(14),
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
        decoration: BoxDecoration(
          color: isLinked
              ? accent.withValues(alpha: 0.10)
              : AppColors.modalTextPrimary.withValues(alpha: 0.04),
          borderRadius: BorderRadius.circular(14),
          border: Border.all(
            color: isLinked
                ? accent.withValues(alpha: 0.55)
                : AppColors.modalTextPrimary.withValues(alpha: 0.18),
          ),
        ),
        child: Row(
          children: [
            Text(
              title,
              style: const TextStyle(
                fontSize: 14,
                fontWeight: FontWeight.w600,
                color: AppColors.modalTextPrimary,
              ),
            ),
            const SizedBox(width: 10),
            Expanded(
              child: Align(
                alignment: Alignment.centerRight,
                child: isLinked && gear != null
                    ? GearLink(
                        gearId: gear!.id,
                        style: const TextStyle(
                          fontSize: 14,
                          color: AppColors.modalTextPrimary,
                        ),
                      )
                    : Text(
                        isLinked ? hintLinked : hintIdle,
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                        textAlign: TextAlign.right,
                        style: TextStyle(
                          fontSize: 12,
                          color: AppColors.modalTextPrimary
                              .withValues(alpha: isLinked ? 0.85 : 0.55),
                        ),
                      ),
              ),
            ),
            if (isLinked) ...[
              const SizedBox(width: 4),
              IconAction(
                icon: Icons.close,
                semanticsLabel: unlinkSemanticsLabel,
                color: AppColors.modalTextPrimary,
                iconSize: 16,
                padding: const EdgeInsets.all(4),
                constraints: const BoxConstraints(
                  minWidth: 28,
                  minHeight: 28,
                ),
                onPressed: onUnlink,
              ),
            ],
          ],
        ),
      ),
    );
  }
}


class _NoteField extends StatelessWidget {
  const _NoteField({required this.controller, required this.placeholder});
  final TextEditingController controller;
  final String placeholder;

  @override
  Widget build(BuildContext context) {
    return TextField(
      controller: controller,
      maxLines: 3,
      minLines: 2,
      style: const TextStyle(
        color: AppColors.modalTextPrimary,
        fontSize: 14,
        height: 1.4,
      ),
      decoration: InputDecoration(
        hintText: placeholder,
        hintStyle: TextStyle(
          color: AppColors.modalTextPrimary.withValues(alpha: 0.45),
          fontSize: 14,
          height: 1.4,
        ),
        filled: true,
        fillColor: AppColors.modalTextPrimary.withValues(alpha: 0.06),
        contentPadding:
            const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
        enabledBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(12),
          borderSide: BorderSide(
            color: AppColors.modalTextPrimary.withValues(alpha: 0.18),
          ),
        ),
        focusedBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(12),
          borderSide: BorderSide(color: AppColors.experienceSageGreen),
        ),
      ),
    );
  }
}

/// Dashed rounded-rectangle border used by the claim card. Compact
/// re-implementation kept private here so we don't depend on an
/// external dashed-border package.
class _ClaimCardBorder extends ShapeBorder {
  const _ClaimCardBorder({
    required this.color,
    required this.radius,
    this.dashWidth = 5,
    this.gap = 4,
    this.strokeWidth = 1.2,
  });
  final Color color;
  final Radius radius;
  final double dashWidth;
  final double gap;
  final double strokeWidth;

  @override
  EdgeInsetsGeometry get dimensions => EdgeInsets.all(strokeWidth);

  @override
  ShapeBorder scale(double t) => _ClaimCardBorder(
        color: color,
        radius: radius,
        dashWidth: dashWidth * t,
        gap: gap * t,
        strokeWidth: strokeWidth * t,
      );

  @override
  Path getInnerPath(Rect rect, {TextDirection? textDirection}) =>
      Path()..addRRect(RRect.fromRectAndRadius(rect.deflate(strokeWidth), radius));

  @override
  Path getOuterPath(Rect rect, {TextDirection? textDirection}) =>
      Path()..addRRect(RRect.fromRectAndRadius(rect, radius));

  @override
  void paint(Canvas canvas, Rect rect, {TextDirection? textDirection}) {
    final paint = Paint()
      ..color = color
      ..style = PaintingStyle.stroke
      ..strokeWidth = strokeWidth;
    final outline = Path()
      ..addRRect(RRect.fromRectAndRadius(
        rect.deflate(strokeWidth / 2),
        radius,
      ));
    final dashed = Path();
    for (final metric in outline.computeMetrics()) {
      var distance = 0.0;
      while (distance < metric.length) {
        final next = distance + dashWidth;
        dashed.addPath(
          metric.extractPath(distance, next.clamp(0, metric.length)),
          Offset.zero,
        );
        distance = next + gap;
      }
    }
    canvas.drawPath(dashed, paint);
  }
}
