import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/content/content_edges_card.dart';
import 'package:ripls/presentation/widgets/user_avatar.dart';

/// SharedWithCard is the gear/request read-shell's audience surface — the
/// non-RSVP analog of the event's "Who's In" entry (#2492). It reuses the
/// read-shell [ContentEdgesCard] chrome so sharing reads the same across item
/// types: a "Shared with N people" header over a facepile of the actual people
/// behind that number (avatars up to [maxFaces], then a "+N" overflow disk),
/// tapping the card opens the access sheet ("who can see this"), and a Share
/// button opens the item share sheet.
///
/// The count base is the *sharee set* — everyone the item is shared with,
/// excluding the item's owner — the same base the who's-helping panel's
/// "Shared · no reply" section subtracts from, so the two surfaces reconcile
/// (#2724). Callers derive it from the server's distinct-member count via
/// [shareeCount].
///
/// Purely presentational: the parent wires [onTap] (access sheet) and
/// [onInvite] (share sheet) so the same card serves gear and requests.
class SharedWithCard extends StatelessWidget {
  /// The most invitee faces the facepile renders before folding the remainder
  /// into a "+N" overflow disk.
  static const int maxFaces = 5;

  /// Number of people the item is shared *with*: the server-computed
  /// deduplicated audience (GetGearResponse / Request
  /// `total_distinct_member_count`, which counts the owner as a member of the
  /// shared communities) minus the owner themself (#2724). Zero stays zero —
  /// an unshared item has no audience.
  static int shareeCount(int totalDistinctMemberCount) =>
      totalDistinctMemberCount > 0 ? totalDistinctMemberCount - 1 : 0;

  /// Deduplicated count of people the item is shared with (the owner
  /// excluded) — derive via [shareeCount].
  final int totalPeople;

  /// The people directly invited to the item's own ad-hoc community
  /// (`invited_individuals`, owner already excluded server-side), used for the
  /// facepile. May be empty when the item is only shared into named
  /// communities — the count still shows and the body collapses.
  final List<User> invitedIndividuals;

  /// The colour of this card and its sharing affordance.
  ///
  /// Sharing is an ACTION, not a content type, so it takes the on-glass action
  /// colour and looks the same on every item. This used to be a per-view
  /// item-type accent passed in by the caller, which meant the request hero
  /// tinted the whole card amber — a hue the Ink & Sage palette does not
  /// contain. The status ramp's warm end exists to say "warning" and "error";
  /// it is not a source of decorative accents.
  static const Color _accent = GlassTokens.primary;

  /// Opens the access sheet ("who can see this"). Available to everyone who can
  /// see the item.
  final VoidCallback onTap;

  /// Opens the item share sheet (QR + link + invite people / community). Any
  /// member may reshare the item's open link (#2630); the sheet itself hides
  /// the audience-management rows for non-owners. Null hides the Invite button
  /// entirely (e.g. when the viewer has no shareable community context).
  final VoidCallback? onInvite;

  const SharedWithCard({
    super.key,
    required this.totalPeople,
    required this.invitedIndividuals,
    required this.onTap,
    this.onInvite,
  });

  /// The body facepile: up to [maxFaces] overlapping invitee avatars, then a
  /// "+N" disk covering the rest of the counted audience (named-community
  /// members the server doesn't list individually). Null when nobody has been
  /// individually invited — the card collapses to its header instead of
  /// rendering an empty body. Decorative — the card carries the semantic
  /// label.
  Widget? _facepile() {
    if (invitedIndividuals.isEmpty) return null;
    const size = 28.0;
    const overlap = 9.0;
    final faces = invitedIndividuals.take(maxFaces).toList();
    final remainder = totalPeople - faces.length;
    final slots = faces.length + (remainder > 0 ? 1 : 0);
    final width = size + (slots - 1) * (size - overlap);
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 6),
      child: Align(
        alignment: Alignment.centerLeft,
        child: SizedBox(
          height: size,
          width: width,
          child: Stack(
            children: [
              for (int i = 0; i < faces.length; i++)
                Positioned(
                  left: i * (size - overlap),
                  child: _ringed(
                    UserAvatar(user: faces[i], radius: size / 2 - 1.5),
                  ),
                ),
              if (remainder > 0)
                Positioned(
                  left: faces.length * (size - overlap),
                  child: _ringed(_overflowDisk(remainder, size - 3)),
                ),
            ],
          ),
        ),
      ),
    );
  }

  /// Wraps a facepile disk in the dark separator ring so overlapped faces stay
  /// legible.
  Widget _ringed(Widget child) => Container(
        decoration: BoxDecoration(
          shape: BoxShape.circle,
          border: Border.all(color: AppColors.darkBackground, width: 1.5),
        ),
        child: child,
      );

  /// The "+N" disk closing the facepile — the counted people the server
  /// doesn't list individually (named-community members).
  Widget _overflowDisk(int n, double size) => Container(
        width: size,
        height: size,
        alignment: Alignment.center,
        decoration: BoxDecoration(
          shape: BoxShape.circle,
          color: _accent.withValues(alpha: 0.22),
        ),
        child: Text(
          '+$n',
          style: TextStyle(
            color: AppColors.onContentImage,
            fontSize: size * 0.34,
            fontWeight: FontWeight.w700,
          ),
        ),
      );

  Widget _shareButton(BuildContext context) {
    final l10n = context.l10n;
    return Align(
      alignment: Alignment.centerRight,
      child: Tappable(
        semanticsLabel: l10n.sharedWithCardShareAction,
        onTap: onInvite!,
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 7),
          decoration: BoxDecoration(
            color: _accent.withValues(alpha: 0.16),
            borderRadius: BorderRadius.circular(999),
            border: Border.all(color: _accent.withValues(alpha: 0.4)),
          ),
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Icon(Icons.ios_share_outlined, size: 15, color: _accent),
              const SizedBox(width: 6),
              Text(
                l10n.sharedWithCardShareAction,
                style: const TextStyle(
                  color: _accent,
                  fontSize: 12,
                  fontWeight: FontWeight.w700,
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    final facepile = _facepile();
    return ContentEdgesCard(
      accentColor: _accent,
      headerColor: AppColors.darkTextTertiary,
      headerLeading: Icon(
        Icons.group_outlined,
        size: 20,
        color: _accent.withValues(alpha: 0.7),
      ),
      headerLabel: l10n.sharedWithCardTitle(totalPeople),
      onTap: (_) => onTap(),
      semanticsLabel: l10n.a11ySharedWithCard(totalPeople),
      rows: [?facepile],
      footer: onInvite == null ? null : _shareButton(context),
    );
  }
}
