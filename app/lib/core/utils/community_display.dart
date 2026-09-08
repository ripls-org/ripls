import 'package:ripls/data/gen/ripls/api/community_service.pb.dart'
    show CommunityItem;
import 'package:ripls/l10n/app_localizations.dart';

/// Display name for a community in lists and pickers.
///
/// Named communities show their [CommunityItem.name]. Nameless (ad-hoc /
/// per-item) communities render like a group chat from the viewer's
/// perspective as an Oxford-comma list — "You and Alex", "You, Alex, and Sam",
/// "You, Alex, Sam, Bo, and 2 others" — so even a two-person group reads as a
/// group. Host-only nameless communities are dropped server-side
/// (ListCommunities) and never reach here.
String communityDisplayName(CommunityItem community, AppLocalizations l10n) =>
    communityLabel(
      name: community.name,
      memberCount: community.memberCount,
      memberPreviewFirstNames: community.memberPreviewFirstNames,
      l10n: l10n,
    );

/// Group-text display name for a community a call site knows only by id +
/// a fallback name + member count (e.g. an `AccessGroup`, or the item header's
/// `SharedCommunity` — neither carries member-preview names). Resolves the
/// member-preview first names from the loaded [communities] list (populated for
/// communities the viewer is a member of) so a nameless community renders
/// group-text style ("You and Ada, Sam + 2 others") rather than blank.
///
/// Fallback order: a non-empty [fallbackName] wins; otherwise the resolved
/// [CommunityItem]'s group-text label; otherwise a plain member count — so the
/// label is never empty.
String communityDisplayNameFromList({
  required String communityId,
  required String fallbackName,
  required int fallbackMemberCount,
  required List<CommunityItem> communities,
  required AppLocalizations l10n,
}) {
  if (fallbackName.trim().isNotEmpty) return fallbackName.trim();
  for (final c in communities) {
    if (c.id == communityId) return communityDisplayName(c, l10n);
  }
  return l10n.workshopChooserMembersCount(fallbackMemberCount);
}

/// Public, non-identifying label for a nameless community — used on any surface
/// whose text leaves the group.
///
/// [communityDisplayName] renders a nameless community as its members ("You,
/// Alex, and Sam"). That is correct for the viewer, who is one of them, and
/// wrong for anyone else: an invite composed with it would carry members' first
/// names to a non-member. The server draws the same line for the public `/go/`
/// landing (`communityDisplayName` in `server/services/web/service.go`
/// substitutes a generic label there); this is the client half of it.
///
/// Named communities are unaffected — a name the owner chose is already public.
String communityPublicName(CommunityItem community, AppLocalizations l10n) {
  final trimmed = community.name.trim();
  return trimmed.isNotEmpty ? trimmed : l10n.communityGenericGroupLabel;
}

/// Secondary line for a community row: the description when the community has
/// one, otherwise — for a nameless community — the item that spawned it, and
/// failing that a plain member count.
///
/// Kept here rather than in a widget so the branching is unit-testable without a
/// widget tree, and so every community row derives its subtitle the same way.
/// Returns null when there is nothing worth showing.
String? communitySubtitle(CommunityItem community, AppLocalizations l10n) {
  final description = community.description.trim();
  if (description.isNotEmpty) return description;

  // Named communities without a description get no second line — inventing a
  // member count for them would be new information on an existing surface.
  if (community.name.trim().isNotEmpty) return null;

  final origin = community.originItemName.trim();
  if (origin.isNotEmpty) return l10n.workshopGroupFromItem(origin);

  // The member count is only worth a second line when the first one is the
  // member *names*. With no preview names [communityDisplayName] has already
  // fallen back to the count itself, and repeating it renders the row as
  // "4 members / 4 members".
  if (community.memberPreviewFirstNames.isEmpty) return null;
  if (community.memberCount > 0) {
    return l10n.workshopChooserMembersCount(community.memberCount);
  }
  return null;
}

/// Primitive form of [communityDisplayName] for call sites that hold a
/// community's fields rather than a [CommunityItem].
String communityLabel({
  required String name,
  required int memberCount,
  required List<String> memberPreviewFirstNames,
  required AppLocalizations l10n,
}) {
  final trimmed = name.trim();
  if (trimmed.isNotEmpty) return trimmed;

  if (memberPreviewFirstNames.isEmpty) {
    // No usable member names (e.g. members with no name set) — fall back to a
    // plain member count rather than rendering a blank label.
    return l10n.workshopChooserMembersCount(memberCount);
  }
  // memberPreviewFirstNames excludes the viewer; memberCount includes them, so
  // the overflow is (total others) minus (names already shown). Render from the
  // viewer's perspective as a proper Oxford-comma list — "You", then the other
  // first names — so even a two-person group reads as a group.
  final otherCount = (memberCount - 1) - memberPreviewFirstNames.length;
  return formatNameList(
    l10n,
    [l10n.notificationSelfName, ...memberPreviewFirstNames],
    otherCount < 0 ? 0 : otherCount,
  );
}

/// Joins [names] into a localized, Oxford-comma people list with an optional
/// "+ N others" overflow tail:
///   - `["You", "Ada"]`, 0            -> "You and Ada"
///   - `["You", "Ada", "Jon"]`, 0     -> "You, Ada, and Jon"
///   - `["You", "Ada", "Jon"]`, 5     -> "You, Ada, Jon, and 5 others"
///
/// Callers decide whether the viewer leads the list ("You", for a community's
/// group-text name) or appears by their own first name (the Workshop subtitle).
/// [otherCount] is how many more people are not shown.
String formatNameList(
  AppLocalizations l10n,
  List<String> names,
  int otherCount,
) {
  final separator = l10n.userProfileSubtitleSeparator;
  if (otherCount > 0) {
    return l10n.nameListOverflow(names.join(separator), otherCount);
  }
  if (names.isEmpty) return '';
  if (names.length == 1) return names.first;
  if (names.length == 2) return l10n.nameListPair(names[0], names[1]);
  final head = names.sublist(0, names.length - 1).join(separator);
  return l10n.nameListSeries(head, names.last);
}
