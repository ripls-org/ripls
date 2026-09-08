import 'package:ripls/core/utils/community_display.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart' show SharedCommunity;
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/services/providers.dart' show CommunitiesState;

/// CommunityHelper provides shared utility functions for community-related operations.
class CommunityHelper {
  /// Feed-header action text for a community-created feed item.
  ///
  /// [communityName] distinguishes real, named communities from the
  /// nameless per-item ad-hoc communities that sharing an item provisions
  /// (#2492): labelling those "Created community" misdescribes what the
  /// actor did — they shared a request/item, not founded a community
  /// (#2724) — so ad-hoc rows read "Started a group" instead.
  static String getFeedActionText(
    AppLocalizations l10n, {
    required String communityName,
  }) {
    return communityName.trim().isEmpty
        ? l10n.feedActionStartedGroup
        : l10n.feedActionCreatedCommunity;
  }

  /// communitiesTitle returns a localized header title for the user's
  /// communities. When the user belongs to exactly one community it
  /// returns that community's name; when multiple it returns
  /// "{N} Communities"; when none it returns the localized
  /// "No communities" string.
  static String communitiesTitle(
    CommunitiesState state,
    AppLocalizations l10n,
  ) {
    final communities = state.communities;
    if (communities.isEmpty) return l10n.commonNoCommunities;
    // Derived, not `.name` — a viewer whose single membership is a nameless
    // ad-hoc group would otherwise get a blank title (#2937).
    if (communities.length == 1) {
      return communityDisplayName(communities.first, l10n);
    }
    return l10n.commonCommunitiesCount(communities.length);
  }

  /// invitableSharedCommunities returns the communities the current user may
  /// invite into for an item. The item GET responses already scope
  /// `shared_communities` to communities the caller is an active member of
  /// (the server builders take `viewerIDs ∩ callerSet`), so this returns
  /// [shared] as-is — only members can invite, and the server already
  /// guarantees that (#1666).
  ///
  /// It deliberately does NOT re-filter against the client community list
  /// ([state]): that list lags a freshly-created per-item ad-hoc community, so
  /// re-filtering would wrongly drop the item's own community and leave the
  /// invite/access "Invite someone" flow with nothing to share into. [state] is
  /// retained for call-site compatibility.
  static List<SharedCommunity> invitableSharedCommunities(
    Iterable<SharedCommunity> shared,
    CommunitiesState state,
  ) {
    return shared.toList();
  }
}
