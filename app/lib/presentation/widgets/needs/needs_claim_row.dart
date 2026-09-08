import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/presentation/viewmodels/needs_scope.dart';
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';
import 'package:ripls/presentation/widgets/accessibility/toggle.dart';
import 'package:ripls/presentation/widgets/gear/gear_pill.dart';
import 'package:ripls/presentation/widgets/needs/needs_copy.dart';

/// One contribution slot on a Need — a contributor plus their
/// optional linked gear. Aggregated per user.id when rendered on the
/// row so the contributors line reads like
/// `Alex (x2) [Chainsaw], Marcia, Lin (x3) [Tarp]` — the quantity
/// comes from counting slots for the same user, the gear chip from
/// any [gearId] populated on those slots.
typedef NeedsContributorSlot = ({User user, String? gearId});

/// NeedsClaimRow is the tap-to-claim row used in the Volunteer surface
/// (NV1 / NV2 / NO1) and, in read-only form, in the Archived sheet (NA).
///
/// Visual states (v2):
///
/// * **idle**: empty rounded-square checkbox + white name + voter stack on
///   the right (when anyone else has claimed). Tap claims.
/// * **claimed**: sage-filled checkbox with a dark check, sage tint on the
///   name + count line + voter stack. Tap unclaims. When `needed > 1` an
///   inline multi-qty stepper renders below the row body —
///   `BRINGING – | N | + of {needed} needed`.
/// * **edit-mode + canEdit**: row gets a sage outline + a trailing
///   pencil glyph (replacing the voter stack); whole row is tappable
///   and fires [onEditTap]. Claim/unclaim taps are suppressed.
/// * **edit-mode + !canEdit**: row dims to 0.45 opacity; inert.
///
/// Uses [Toggle] so the selectable state is announced by screen readers
/// via the semantic `selected` flag instead of being baked into the
/// label. The sage-fill settle animation is wrapped in
/// [accessibleDuration] so reduce-motion disables it.
class NeedsClaimRow extends StatelessWidget {
  /// Scope flavour — picks Experience vs. Request copy via [NeedsCopy].
  final NeedsScopeKind scopeKind;

  final String title;

  /// Optional inline note shown beneath the title (e.g. "Dry-treated
  /// preferred"). Renders unconditionally when non-null.
  final String? note;

  /// Optional "added by {name}" attribution. Renders muted, only when
  /// the proposer isn't the current user.
  final String? addedByName;

  /// Total slot capacity on the parent Need. Drives the count line
  /// ("{claimed} of {needed} claimed") and the inline multi-qty
  /// stepper. `needed == 1` rows show neither — the checkbox alone
  /// carries the binary intent.
  final int needed;

  /// How many slots are currently claimed in total (across all
  /// contributors). Drives the count line.
  final int claimed;

  /// Whether the current user has claimed at least one slot on this
  /// Need. Drives the visual claimed-state (sage fill, dark check).
  final bool claimedByMe;

  /// How many slots the current user holds. Drives the inline stepper
  /// when `claimedByMe && needed > 1`.
  final int myQty;

  /// Whether the row should accept new claims. False when the parent
  /// entity is terminal, the Need has zero remaining slots, or the
  /// caller has disabled interaction.
  final bool enabled;

  /// Tap callback for the row body. Called whether the row is idle
  /// (claim) or claimed (no-op or unclaim, caller's choice). Suppressed
  /// when [editMode] is true.
  final VoidCallback? onTap;

  /// Localized semantics label announced for the row itself. Should
  /// describe the *Need* — selected state is communicated separately by
  /// [Toggle]'s built-in `selected` flag.
  final String semanticsLabel;

  /// Every slot pledged on this Need. The line aggregates per
  /// user.id to render `Name (xN) [Gear]` inline — quantity comes
  /// from counting slots for the same user, gear chip(s) from any
  /// `gearId` populated on those slots.
  final List<NeedsContributorSlot> contributors;

  /// Current viewer's user id. Used to substitute "You" for the
  /// viewer's own name in the contributors line. Pass null on read-
  /// only surfaces where the viewer can never be a contributor.
  final String? currentUserId;

  /// Whether the whole screen is in edit mode. When true the row's
  /// primary tap target becomes "edit", not "claim/unclaim".
  final bool editMode;

  /// Whether THIS row is editable in edit mode. Organizers see
  /// `canEdit: true` on every row; non-organizers see `canEdit: true`
  /// only on rows they added or claimed.
  final bool canEdit;

  /// Tap callback fired when the row is tapped in edit mode and
  /// [canEdit] is true. Caller opens NK4 picker pre-populated.
  final VoidCallback? onEditTap;

  /// When true the row is read-only — taps do nothing, the leading
  /// checkbox is rendered as plain text-style and no stepper appears.
  /// Used by the Archived sheet (NA).
  final bool readOnly;

  /// Optional dim opacity override applied to the entire row. Used by
  /// the Archived sheet to sink uncovered items.
  final double opacity;

  /// Optional Gear item the *current user's* contribution on this Need
  /// is linked to. When non-null, renders an inline [GearPill] below
  /// the title (between the note and the meta line). Single-link per
  /// contribution — multi-link is intentionally not supported.
  const NeedsClaimRow({
    super.key,
    required this.scopeKind,
    required this.title,
    required this.semanticsLabel,
    this.note,
    this.addedByName,
    this.needed = 1,
    this.claimed = 0,
    this.claimedByMe = false,
    this.myQty = 0,
    this.enabled = true,
    this.onTap,
    this.contributors = const [],
    this.currentUserId,
    this.editMode = false,
    this.canEdit = false,
    this.onEditTap,
    this.readOnly = false,
    this.opacity = 1,
  });

  bool get isFullyCovered => needed > 0 && claimed >= needed;

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    final accent = AppColors.experienceSageGreen;

    final editable = editMode && canEdit;
    final editLocked = editMode && !canEdit;

    // Heuristic: 1) if it's being met, check the box; 2) when current user
    // is contributing, use the sage-tinted "me" style; 3) when only others
    // are contributing, render the row in the normal white style with a
    // checked white box. The previous "fully covered" branch dimmed the
    // background and left the checkbox empty — both wrong per the design.
    final claimedByOther = !claimedByMe && isFullyCovered;
    Color bg;
    Color borderColor;
    if (claimedByMe) {
      bg = accent.withValues(alpha: 0.18);
      borderColor = accent;
    } else {
      bg = AppColors.modalInlineActionBackground;
      borderColor = AppColors.modalInlineActionBorder;
    }
    if (editable) borderColor = accent;

    final rowOpacity =
        opacity * (editLocked ? 0.45 : 1.0);

    // Right-justified `claimed/needed` counter (e.g. "0/1", "1/3").
    // Replaces the previous "{claimed} of {needed} claimed" meta line
    // that lived under the title — the value reads more naturally as
    // a trailing fraction. Hidden in edit mode so the pencil glyph
    // owns the right edge instead.
    final showCountChip = !editMode && needed > 0;

    // Main tappable row content. Multi-qty adjustments moved to the
    // claim modal; the row itself stays a single-tap surface and
    // doesn't surface a stepper anymore regardless of `needed`.
    final rowContent = Row(
      crossAxisAlignment: CrossAxisAlignment.center,
      children: [
        _LeadingCheckbox(
          claimedByMe: claimedByMe,
          claimedByOther: claimedByOther,
          accent: accent,
        ),
        const SizedBox(width: 11),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            mainAxisSize: MainAxisSize.min,
            children: [
              Text(
                title,
                style: const TextStyle(
                  fontSize: 15.5,
                  fontWeight: FontWeight.w700,
                  // Foreground text stays white on the frosted-
                  // glass surface regardless of claim state —
                  // sage doesn't have enough contrast against
                  // the modal backdrop. The claim state is
                  // signalled by the sage row tint + checkbox.
                  color: AppColors.modalTextPrimary,
                  height: 1.2,
                ),
              ),
              if (note != null && note!.isNotEmpty) ...[
                const SizedBox(height: 2),
                Text(
                  note!,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(
                    fontSize: 12.5,
                    color: AppColors.modalTextSecondary,
                  ),
                ),
              ],
              if (!editMode && contributors.isNotEmpty) ...[
                const SizedBox(height: 3),
                _ContributorsLine(
                  contributors: contributors,
                  currentUserId: currentUserId,
                  youLabel: l10n.needsRowContributorYou,
                ),
              ],
            ],
          ),
        ),
        if (showCountChip) ...[
          const SizedBox(width: 8),
          _CountChip(
            claimed: claimed,
            needed: needed,
            isFullyCovered: isFullyCovered,
            accent: accent,
          ),
        ],
        if (editable) ...[
          const SizedBox(width: 8),
          const Icon(
            Icons.edit,
            size: 14,
            color: AppColors.modalTextPrimary,
          ),
        ],
      ],
    );

    // The stepper is kept structurally outside the Toggle so taps on
    // its blank chrome don't bubble to the row's tap-to-unclaim
    // handler. It has its own +/- IconActions for actual interaction.
    Widget buildCard(Widget tappableArea) {
      return AnimatedContainer(
        duration:
            accessibleDuration(context, const Duration(milliseconds: 180)),
        curve: Curves.easeOut,
        margin: const EdgeInsets.only(bottom: 9),
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
        decoration: BoxDecoration(
          color: bg,
          borderRadius: BorderRadius.circular(16),
          border: Border.all(color: borderColor, width: editable ? 1.5 : 1),
        ),
        child: tappableArea,
      );
    }

    if (readOnly) {
      return Semantics(
        label: semanticsLabel,
        container: true,
        child: ExcludeSemantics(
          child: Opacity(
            opacity: rowOpacity,
            child: buildCard(rowContent),
          ),
        ),
      );
    }

    final VoidCallback? rowOnTap = (editable && onEditTap != null)
        ? onEditTap
        : ((enabled && !editMode) ? onTap : null);
    final bool rowSelected = (editable && onEditTap != null)
        ? false
        : claimedByMe;

    return Opacity(
      opacity: rowOpacity,
      child: buildCard(
        Toggle(
          semanticsLabel: semanticsLabel,
          selected: rowSelected,
          onTap: rowOnTap,
          child: rowContent,
        ),
      ),
    );
  }
}

/// Rounded-square checkbox on the leading edge. Three visual states:
///
///   - idle (nobody contributing): white outline, empty
///   - me (current user is contributing): sage-filled, dark check
///   - other (only others contributing): white outline, white check
///
/// Both "claimed" states are mutually exclusive — [claimedByMe] wins
/// when both flags are set (defensive; shouldn't happen in practice).
class _LeadingCheckbox extends StatelessWidget {
  final bool claimedByMe;
  final bool claimedByOther;
  final Color accent;
  const _LeadingCheckbox({
    required this.claimedByMe,
    required this.claimedByOther,
    required this.accent,
  });

  @override
  Widget build(BuildContext context) {
    const outlineColor = GlassTokens.borderActive;
    return AnimatedContainer(
      duration:
          accessibleDuration(context, const Duration(milliseconds: 180)),
      width: 24,
      height: 24,
      decoration: BoxDecoration(
        color: claimedByMe ? accent : Colors.transparent,
        borderRadius: BorderRadius.circular(6),
        border: claimedByMe
            ? null
            : Border.all(color: outlineColor, width: 2),
      ),
      child: claimedByMe
          ? const Icon(Icons.check, size: 13, color: Color(0xFF0F1A14))
          : claimedByOther
              ? Icon(Icons.check, size: 13, color: outlineColor)
              : null,
    );
  }
}

/// Right-justified "{claimed}/{needed}" counter on each row (e.g.
/// "0/1", "1/3", "3/3"). When the row is fully covered the chip uses
/// a sage tint; otherwise it sits muted so it doesn't compete with
/// the title.
class _CountChip extends StatelessWidget {
  const _CountChip({
    required this.claimed,
    required this.needed,
    required this.isFullyCovered,
    required this.accent,
  });

  final int claimed;
  final int needed;
  final bool isFullyCovered;
  final Color accent;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
      decoration: BoxDecoration(
        color: isFullyCovered
            ? accent.withValues(alpha: 0.20)
            : GlassTokens.fillSubtle,
        borderRadius: BorderRadius.circular(999),
      ),
      child: Text(
        '$claimed/$needed',
        style: TextStyle(
          fontSize: 11.5,
          fontWeight: FontWeight.w700,
          letterSpacing: 0.2,
          color: isFullyCovered
              ? AppColors.modalTextPrimary
              : AppColors.modalTextMuted,
        ),
      ),
    );
  }
}

/// Inline contributors line shown below a [NeedsClaimRow]'s title.
///
/// Aggregates per user.id — counts repeat slots so multi-slot claims
/// surface as "(xN)" and collects distinct gear ids so each
/// contributor's linked gear renders as an inline chip next to their
/// name. Result reads
/// `Alex (x2) [Chainsaw], Marcia, Lin (x3) [Tarp]`. The viewer's own
/// slot uses the localized "You" label and sorts to the front.
class _ContributorsLine extends StatelessWidget {
  const _ContributorsLine({
    required this.contributors,
    required this.currentUserId,
    required this.youLabel,
  });

  final List<NeedsContributorSlot> contributors;
  final String? currentUserId;
  final String youLabel;

  @override
  Widget build(BuildContext context) {
    final ordered = <String>[];
    final counts = <String, int>{};
    final namesById = <String, String>{};
    final gearIdsById = <String, List<String>>{};
    String? currentUserKey;
    for (final slot in contributors) {
      final u = slot.user;
      if (u.id.isEmpty) continue;
      final isMe = currentUserId != null && u.id == currentUserId;
      final label = isMe
          ? youLabel
          : (u.name.isNotEmpty ? u.name : '');
      if (label.isEmpty) continue;
      if (!counts.containsKey(u.id)) {
        ordered.add(u.id);
        counts[u.id] = 0;
        namesById[u.id] = label;
        gearIdsById[u.id] = <String>[];
        if (isMe) currentUserKey = u.id;
      }
      counts[u.id] = counts[u.id]! + 1;
      final gid = slot.gearId;
      if (gid != null && gid.isNotEmpty && !gearIdsById[u.id]!.contains(gid)) {
        gearIdsById[u.id]!.add(gid);
      }
    }
    if (ordered.isEmpty) return const SizedBox.shrink();
    if (currentUserKey != null) {
      ordered.remove(currentUserKey);
      ordered.insert(0, currentUserKey);
    }

    final l10n = context.l10n;
    const textStyle = TextStyle(
      fontSize: 12.5,
      color: AppColors.modalTextSecondary,
    );

    // Each contributor renders as its own Row so the name + count
    // decoration + "bringing X" stays glued together; Rows are
    // interleaved with ", " separators inside a Wrap so a wide row
    // flows to the next line at contributor boundaries. LayoutBuilder
    // caps each segment to the parent's max width so a segment that's
    // still wider than the run ellipsizes its name / gear name
    // instead of overflowing.
    return LayoutBuilder(
      builder: (context, constraints) {
        final maxSegmentWidth = constraints.maxWidth.isFinite
            ? constraints.maxWidth
            : double.infinity;
        final segments = <Widget>[];
        for (var i = 0; i < ordered.length; i++) {
          final id = ordered[i];
          final count = counts[id]!;
          final name = namesById[id]!;
          final gids = gearIdsById[id]!;
          // Prose for the leading portion of each contributor segment.
          // Counts of 1 stay bare ("Marcia"); higher counts use the
          // localized "·xN·" decoration ("You ·x2·"). When the
          // contributor linked gear we tack on a " bringing " prefix
          // before the underlined gear name(s) — multiple gear ids
          // join with " · " so multi-link contributions stay legible.
          final prose = count > 1
              ? '$name ${l10n.needsRowContributorCount(count)}'
              : name;
          final hasGear = gids.isNotEmpty;
          segments.add(ConstrainedBox(
            constraints: BoxConstraints(maxWidth: maxSegmentWidth),
            child: Row(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.center,
              children: [
                Flexible(
                  child: Text(
                    prose,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: textStyle,
                  ),
                ),
                if (hasGear) ...[
                  Text(
                    ' ${l10n.needsRowContributorBringing} ',
                    style: textStyle,
                  ),
                  for (var g = 0; g < gids.length; g++) ...[
                    if (g > 0)
                      const Text(' · ', style: textStyle),
                    Flexible(
                      child: GearLink(
                        gearId: gids[g],
                        style: textStyle,
                      ),
                    ),
                  ],
                ],
                if (i < ordered.length - 1)
                  const Text(', ', style: textStyle),
              ],
            ),
          ));
        }

        return Wrap(
          spacing: 0,
          runSpacing: 4,
          crossAxisAlignment: WrapCrossAlignment.center,
          children: segments,
        );
      },
    );
  }
}
