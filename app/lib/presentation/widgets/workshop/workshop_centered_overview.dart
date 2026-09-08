import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/core/utils/community_display.dart';
import 'package:ripls/core/utils/date_time_formatter.dart';
import 'package:ripls/data/gen/ripls/api/item.pb.dart';
import 'package:ripls/presentation/viewmodels/workshop_glimpse_view_model.dart';
import 'package:ripls/presentation/viewmodels/workshop_identity_rollup_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/workshop/workshop_metrics_sentence.dart';
import 'package:ripls/presentation/widgets/workshop/workshop_overview_theme.dart';
import 'package:ripls/services/providers/auth_providers.dart';
import 'package:ripls/services/providers/profile_providers.dart';

/// The signed-in user's "known for" interests, used to seed the Workshop
/// zero-state's Common Ground before the community has any of its own. Returns
/// an empty list when signed out or the profile can't be loaded.
final _inviterInterestsProvider = FutureProvider.autoDispose<List<String>>(
  (ref) async {
    final userId = ref.watch(authStateProvider).user?.id ?? '';
    if (userId.isEmpty) return const <String>[];
    final profile = await ref.read(profileRepositoryProvider).getProfile(userId);
    return profile.knownFor.toList(growable: false);
  },
);

/// The Workshop overview body — a single, centered, editorial column over the
/// community photo (community-chooser-scaling design). Each block is tappable
/// to morph-open the expanded view that already exists:
///
/// - **members** (the serif name rollup) → roster
/// - **metrics** (the prose sentence's underlined numbers) → time / money / CO₂
/// - **latest message** → community conversation
/// - **on the calendar** → library calendar
/// - **newest in the library** → library map
///
/// The expanded views are owned by the screen; this widget only reports the
/// tapped block's footprint [Rect] so the morph can grow from it.
class WorkshopCenteredOverview extends ConsumerWidget {
  const WorkshopCenteredOverview({
    super.key,
    required this.communityId,
    required this.communityName,
    this.originItemName = '',
    required this.hours,
    required this.dollars,
    required this.co2Kg,
    required this.problems,
    required this.problemsPotential,
    required this.memberCount,
    required this.items,
    required this.specialties,
    required this.onTapMembers,
    required this.onTapTime,
    required this.onTapMoney,
    required this.onTapCo2,
    required this.onTapProblems,
    required this.onTapConversation,
    required this.onTapCalendar,
    required this.onTapCommonGround,
    required this.onInvite,
    this.onNameGroup,
  });

  /// Active community id (for the conversation glimpse). Empty when unresolved.
  final String communityId;

  /// Active community name — shown left of the member count in the eyebrow.
  final String communityName;

  /// For a nameless (per-item) community, the name of the item that spawned it.
  /// Used in the eyebrow ("FROM {item} · N MEMBERS") so even an unnamed group is
  /// identifiable. Empty for named communities (#2492).
  final String originItemName;
  final int hours;
  final int dollars;
  final double co2Kg;

  /// "Problems handled" fraction — [problems] is the numerator (X, handled),
  /// [problemsPotential] the denominator (Y, the universe of requests raised).
  final int problems;
  final int problemsPotential;
  final int memberCount;
  final List<Item> items;

  /// Category tags (the community's "Common Ground"). Tapping one opens the map
  /// filtered to that category.
  final List<String> specialties;

  final ValueChanged<Rect> onTapMembers;
  final ValueChanged<Rect> onTapTime;
  final ValueChanged<Rect> onTapMoney;
  final ValueChanged<Rect> onTapCo2;
  final ValueChanged<Rect> onTapProblems;
  final ValueChanged<Rect> onTapConversation;
  final ValueChanged<Rect> onTapCalendar;

  /// Opens the map filtered to a category — `null` category = "All".
  final void Function(String? category, Rect rect) onTapCommonGround;

  /// Opens the invite flow for this community — always shown as a header action
  /// pill (not just in the zero-state).
  final VoidCallback onInvite;

  /// Opens the promote (name + photo + description) flow for a nameless ad-hoc
  /// community. Non-null only when the viewer can name it (a nameless community
  /// they own); when set, a "Name this group" header action pill is shown
  /// alongside Invite (#2492).
  final VoidCallback? onNameGroup;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = context.l10n;
    final rollup = ref.watch(workshopIdentityRollupProvider).asData?.value;
    final names = rollup != null && rollup.firstFewNames.isNotEmpty
        ? formatNameList(l10n, rollup.firstFewNames, rollup.otherCount)
        : null;

    // The soonest upcoming experience drives the calendar block.
    final event = items
        .where((i) => i.kind == ItemKind.ITEM_KIND_EXPERIENCE)
        .firstOrNull;

    final name = communityName.trim();
    final memberLabel = l10n.workshopChooserMembersCount(memberCount);
    // A named community shows its name; a nameless (per-item) one identifies
    // itself by what spawned it ("FROM Dinner at Este · 2 MEMBERS"). The eyebrow
    // is uppercased below, so the lowercase "from {item}" reads as "FROM …".
    final eyebrowName = name.isNotEmpty
        ? name
        : (originItemName.trim().isEmpty
            ? ''
            : l10n.workshopGroupFromItem(originItemName.trim()));
    final eyebrowText =
        eyebrowName.isEmpty ? memberLabel : '$eyebrowName · $memberLabel';

    // "Real data" gate: the celebratory impact sentence is the emotional
    // payoff, so it's withheld until at least one number is non-zero. Before
    // then the screen shows an invitation instead of a demoralizing string of
    // zeros.
    final hasImpact = problems > 0 ||
        problemsPotential > 0 ||
        dollars > 0 ||
        hours > 0 ||
        co2Kg >= 1;

    // Seed Common Ground from the inviter's own interests when the community
    // has none of its own yet, so the section doesn't feel barren in the
    // zero-state.
    final seededInterests = (!hasImpact && specialties.isEmpty)
        ? (ref.watch(_inviterInterestsProvider).asData?.value ??
            const <String>[])
        : const <String>[];
    final commonGround = specialties.isNotEmpty ? specialties : seededInterests;

    final blocks = <Widget>[
      if (memberCount > 0)
        Text(
          eyebrowText.toUpperCase(),
          textAlign: TextAlign.center,
          style: _eyebrow,
        ),
      if (names != null) ...[
        const SizedBox(height: 9),
        _MembersHero(names: names, onTap: onTapMembers),
      ],
      // Header action pills, just below the member names: always an Invite pill
      // (so it's not lost once a group has activity), plus a "Name this group"
      // pill for a nameless community the viewer owns (#2492).
      const SizedBox(height: 14),
      _HeaderActions(
        communityName: name,
        onInvite: onInvite,
        onNameGroup: onNameGroup,
      ),
      const SizedBox(height: 14),
      if (hasImpact)
        WorkshopMetricsSentence(
          hours: hours,
          dollars: dollars,
          co2Kg: co2Kg,
          problems: problems,
          problemsPotential: problemsPotential,
          memberCount: memberCount,
          onTapTime: onTapTime,
          onTapMoney: onTapMoney,
          onTapCo2: onTapCo2,
          onTapProblems: onTapProblems,
        )
      else
        const _CrewInvitation(),
      _rule(),
      _LatestMessage(
        communityId: communityId,
        onTap: onTapConversation,
        // Zero-state nudges a first message instead of the generic empty copy.
        emptyText: hasImpact ? null : l10n.workshopCrewSayHi,
      ),
      // The calendar and Common-Ground sections always render — the calendar
      // with a zero-state prompt when there's nothing yet.
      _rule(),
      _CalendarBlock(event: event, onTap: onTapCalendar),
      _rule(),
      _CommonGroundBlock(
        specialties: commonGround,
        onTapCategory: onTapCommonGround,
      ),
    ];

    // Center the column vertically within the actual available viewport
    // (below the floating header), scrolling when the content overflows —
    // matching the design's centered editorial feel. Using the layout
    // constraints rather than a fraction of the full screen height is what
    // makes the centering land correctly.
    return LayoutBuilder(
      builder: (context, constraints) => SingleChildScrollView(
        // No AlwaysScrollable: when the centered content fits the viewport the
        // page shouldn't scroll, so the vertical drag pages between communities
        // (the workbench is a vertical PageView). It scrolls only when a
        // community's content actually overflows.
        child: ConstrainedBox(
          constraints: BoxConstraints(minHeight: constraints.maxHeight),
          child: Padding(
            padding: const EdgeInsets.fromLTRB(28, 8, 28, 40),
            child: Column(
              mainAxisAlignment: MainAxisAlignment.center,
              crossAxisAlignment: CrossAxisAlignment.center,
              children: blocks,
            ),
          ),
        ),
      ),
    );
  }

  // ── Shared text styles (centered editorial over the photo) ──
  static const TextStyle _eyebrow = TextStyle(
    fontSize: 10,
    fontWeight: FontWeight.w600,
    letterSpacing: 1.5,
    color: WorkshopOverviewPalette.onPhotoDim,
  );

  static const TextStyle _shead = TextStyle(
    fontSize: 10,
    fontWeight: FontWeight.w600,
    letterSpacing: 1.5,
    color: WorkshopOverviewPalette.onPhotoDim,
  );

  static Widget _rule() => Container(
        width: 64,
        height: 0.5,
        margin: const EdgeInsets.symmetric(vertical: 18),
        color: const Color(0x33FFFFFF),
      );
}

/// The serif member-name rollup ("Thomas, Alfred & 10 others"), tappable to
/// open the roster.
class _MembersHero extends StatelessWidget {
  final String names;
  final ValueChanged<Rect> onTap;
  const _MembersHero({required this.names, required this.onTap});

  @override
  Widget build(BuildContext context) {
    return Builder(
      builder: (ctx) => Tappable(
        semanticsLabel: context.l10n.a11yWorkshopOpenRoster,
        onTap: () => onTap(workshopRectOf(ctx)),
        child: Text(
          names,
          textAlign: TextAlign.center,
          style: const TextStyle(
            fontFamily: AppTheme.headingFont,
            fontSize: 29,
            height: 1.12,
            fontWeight: FontWeight.w500,
            color: WorkshopOverviewPalette.onPhoto,
          ),
        ),
      ),
    );
  }
}

/// The header action pills shown just below the member names: an Invite pill
/// (always, so the CTA persists once a group has activity — #2492) plus a "Name
/// this group" pill when the viewer can promote a nameless community.
class _HeaderActions extends StatelessWidget {
  final String communityName;
  final VoidCallback onInvite;
  final VoidCallback? onNameGroup;

  const _HeaderActions({
    required this.communityName,
    required this.onInvite,
    this.onNameGroup,
  });

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    final name = communityName.trim();
    final inviteLabel = name.isEmpty
        ? l10n.workshopCrewInviteButtonGeneric
        : l10n.workshopCrewInviteButton(name);
    return Wrap(
      alignment: WrapAlignment.center,
      spacing: 10,
      runSpacing: 10,
      children: [
        if (onNameGroup != null)
          _ActionPill(
            label: l10n.workshopNameGroupPrompt,
            onTap: onNameGroup!,
            semanticsIdentifier: 'overview-name-group-pill',
          ),
        _ActionPill(label: inviteLabel, onTap: onInvite),
      ],
    );
  }
}

/// A filled, pill-shaped CTA over the community photo (Invite / Name this group).
class _ActionPill extends StatelessWidget {
  final String label;
  final VoidCallback onTap;
  final String? semanticsIdentifier;

  const _ActionPill({
    required this.label,
    required this.onTap,
    this.semanticsIdentifier,
  });

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: label,
      semanticsIdentifier: semanticsIdentifier,
      onTap: onTap,
      inkBorderRadius: BorderRadius.circular(999),
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 22, vertical: 11),
        decoration: BoxDecoration(
          color: WorkshopOverviewPalette.accent,
          borderRadius: BorderRadius.circular(999),
        ),
        child: Text(
          label,
          style: const TextStyle(
            fontSize: 14,
            fontWeight: FontWeight.w600,
            color: WorkshopOverviewPalette.onPhoto,
          ),
        ),
      ),
    );
  }
}

/// Zero-state companion to the metrics sentence: an invitation prompt + a
/// one-line teaser hinting at the impact story that appears once there's real
/// data. The invite CTA itself is the persistent header pill (_HeaderActions).
class _CrewInvitation extends StatelessWidget {
  const _CrewInvitation();

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        Text(
          l10n.workshopCrewInvitePrompt,
          textAlign: TextAlign.center,
          style: const TextStyle(
            fontFamily: AppTheme.headingFont,
            fontSize: 18,
            height: 1.4,
            color: WorkshopOverviewPalette.onPhoto,
          ),
        ),
        const SizedBox(height: 16),
        Text(
          l10n.workshopCrewTeaser,
          textAlign: TextAlign.center,
          style: const TextStyle(
            fontSize: 12.5,
            height: 1.4,
            color: WorkshopOverviewPalette.onPhotoFaint,
          ),
        ),
      ],
    );
  }
}

/// "Latest message" block — the most recent chat quote + author/time, tappable
/// to open the community conversation. Falls back to a "start the
/// conversation" placeholder when there are no messages.
class _LatestMessage extends ConsumerWidget {
  final String communityId;
  final ValueChanged<Rect> onTap;

  /// Overrides the empty-state copy (e.g. the crew zero-state's "Say hi to your
  /// crew"). Falls back to the generic "no messages yet" prompt when null.
  final String? emptyText;
  const _LatestMessage({
    required this.communityId,
    required this.onTap,
    this.emptyText,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = context.l10n;
    final quote = communityId.isEmpty
        ? null
        : ref
            .watch(workshopGlimpseProvider(communityId))
            .asData
            ?.value
            .firstOrNull;

    final body = quote == null
        ? Text(
            emptyText ?? l10n.communityChatEmpty,
            textAlign: TextAlign.center,
            style: const TextStyle(
              fontFamily: AppTheme.headingFont,
              fontStyle: FontStyle.italic,
              fontSize: 18,
              height: 1.3,
              color: WorkshopOverviewPalette.onPhotoDim,
            ),
          )
        : Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Text(
                '“${quote.text}”',
                textAlign: TextAlign.center,
                style: const TextStyle(
                  fontFamily: AppTheme.headingFont,
                  fontStyle: FontStyle.italic,
                  fontSize: 18,
                  height: 1.3,
                  color: WorkshopOverviewPalette.onPhoto,
                ),
              ),
              const SizedBox(height: 6),
              Text(
                _attr(quote),
                textAlign: TextAlign.center,
                style: const TextStyle(
                  fontSize: 11.5,
                  color: WorkshopOverviewPalette.onPhotoFaint,
                ),
              ),
            ],
          );

    return _LabeledBlock(
      label: l10n.workshopSectionLatestMessage,
      semantics: l10n.a11yContentOpenConversation,
      onTap: onTap,
      child: body,
    );
  }

  String _attr(WorkshopGlimpseQuote quote) {
    final ago = DateTimeFormatter.formatTimeAgo(quote.atUnixSec);
    return ago == null || ago.isEmpty
        ? quote.author
        : '${quote.author} · $ago';
  }
}

/// "On the calendar" block — the soonest upcoming experience, or a zero-state
/// prompt when there's nothing scheduled. Tappable to open the library
/// calendar.
class _CalendarBlock extends StatelessWidget {
  final Item? event;
  final ValueChanged<Rect> onTap;
  const _CalendarBlock({required this.event, required this.onTap});

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    final ev = event;

    final Widget child;
    if (ev == null) {
      child = Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          _zeroStatePrompt(l10n.workshopCalendarEmptyPrompt),
          const SizedBox(height: 4),
          Text(
            l10n.workshopCalendarEmptySubprompt,
            textAlign: TextAlign.center,
            style: const TextStyle(
              fontSize: 12,
              height: 1.3,
              color: WorkshopOverviewPalette.onPhotoFaint,
            ),
          ),
        ],
      );
    } else {
      // The event's line is when it's happening (the date label), not an owner.
      final date = ev.hasMiniLabel() ? ev.miniLabel : '';
      final subtitle =
          date.isNotEmpty ? date : (ev.hasSubtitle() ? ev.subtitle : '');
      child = Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(
            ev.title,
            textAlign: TextAlign.center,
            style: const TextStyle(
              fontFamily: AppTheme.headingFont,
              fontSize: 20,
              height: 1.18,
              fontWeight: FontWeight.w500,
              color: WorkshopOverviewPalette.onPhoto,
            ),
          ),
          if (subtitle.isNotEmpty) ...[
            const SizedBox(height: 5),
            Text(
              subtitle,
              textAlign: TextAlign.center,
              style: const TextStyle(
                fontSize: 11.5,
                color: WorkshopOverviewPalette.onPhotoFaint,
              ),
            ),
          ],
        ],
      );
    }

    return _LabeledBlock(
      label: l10n.workshopSectionOnCalendar,
      semantics: l10n.a11yWorkshopOpenLibraryCalendar,
      onTap: onTap,
      child: child,
    );
  }
}

/// Serif-italic, muted zero-state prompt — the same shape the conversation
/// glimpse uses for its empty state.
Widget _zeroStatePrompt(String text) {
  return Text(
    text,
    textAlign: TextAlign.center,
    style: const TextStyle(
      fontFamily: AppTheme.headingFont,
      fontStyle: FontStyle.italic,
      fontSize: 16,
      height: 1.35,
      color: WorkshopOverviewPalette.onPhotoDim,
    ),
  );
}

/// "Common ground" block — the community's category tags as tappable chips
/// (with a leading "All"). Each opens the library map filtered to that
/// category; "All" opens the unfiltered map. Capped at two rows when collapsed,
/// with a tappable "+N" chip that reveals the rest (the count is computed by
/// measuring chip widths and simulating the Wrap layout, like `KnownForChips`).
class _CommonGroundBlock extends StatefulWidget {
  final List<String> specialties;
  final void Function(String? category, Rect rect) onTapCategory;

  const _CommonGroundBlock({
    required this.specialties,
    required this.onTapCategory,
  });

  @override
  State<_CommonGroundBlock> createState() => _CommonGroundBlockState();
}

class _CommonGroundBlockState extends State<_CommonGroundBlock> {
  static const int _maxRowsCollapsed = 2;
  static const double _spacing = 8;
  static const double _runSpacing = 8;
  static const double _fontSize = 12.5;
  // Horizontal padding (14) + 1px border each side — keep in sync with _pill.
  static const double _chipExtraWidth = 14 * 2 + 2;

  bool _expanded = false;

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    // "All" (null category) leads, then each specialty.
    final entries = <({String label, String? category})>[
      (label: l10n.workshopCommonGroundAll, category: null),
      for (final c in widget.specialties) (label: c, category: c),
    ];
    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        Text(
          l10n.workshopSectionCommonGround.toUpperCase(),
          textAlign: TextAlign.center,
          style: WorkshopCenteredOverview._shead,
        ),
        const SizedBox(height: 10),
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 8),
          child: _expanded
              ? _wrap([for (final e in entries) _chip(e)])
              : LayoutBuilder(
                  builder: (ctx, c) => _buildCollapsed(entries, c.maxWidth),
                ),
        ),
      ],
    );
  }

  Widget _wrap(List<Widget> children) => Wrap(
        alignment: WrapAlignment.center,
        spacing: _spacing,
        runSpacing: _runSpacing,
        children: children,
      );

  Widget _buildCollapsed(
    List<({String label, String? category})> entries,
    double maxWidth,
  ) {
    final widths = [for (final e in entries) _measure(e.label)];
    if (_fitsAll(widths, maxWidth)) {
      return _wrap([for (final e in entries) _chip(e)]);
    }
    // Find the most entries (always keeping "All") that fit in two rows
    // alongside the trailing "+N" chip.
    var bestN = 1;
    for (var n = entries.length - 1; n >= 1; n--) {
      final actionWidth = _measure('+${entries.length - n}');
      if (_fitsWithTrailing(widths, n, actionWidth, maxWidth)) {
        bestN = n;
        break;
      }
    }
    final hidden = entries.length - bestN;
    return _wrap([
      for (var i = 0; i < bestN; i++) _chip(entries[i]),
      Tappable(
        semanticsLabel: context.l10n.a11yProfileShowMoreChips(hidden),
        onTap: () => setState(() => _expanded = true),
        inkBorderRadius: BorderRadius.circular(999),
        child: _pill('+$hidden', isAction: true),
      ),
    ]);
  }

  /// A category chip — reports its footprint so the map can morph-grow from it.
  Widget _chip(({String label, String? category}) e) {
    return Builder(
      builder: (ctx) => Tappable(
        semanticsLabel: e.label,
        onTap: () => widget.onTapCategory(e.category, workshopRectOf(ctx)),
        inkBorderRadius: BorderRadius.circular(999),
        child: _pill(e.label),
      ),
    );
  }

  Widget _pill(String label, {bool isAction = false}) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 7),
      decoration: BoxDecoration(
        color: WorkshopOverviewPalette.chipFill,
        borderRadius: BorderRadius.circular(999),
        border: Border.all(color: WorkshopOverviewPalette.cardBorder),
      ),
      child: Text(
        label,
        style: TextStyle(
          fontSize: _fontSize,
          fontWeight: isAction ? FontWeight.w700 : FontWeight.w400,
          color: isAction
              ? WorkshopOverviewPalette.accentSoft
              : WorkshopOverviewPalette.onPhoto,
        ),
      ),
    );
  }

  double _measure(String label) {
    final painter = TextPainter(
      text: TextSpan(text: label, style: const TextStyle(fontSize: _fontSize)),
      textDirection: TextDirection.ltr,
    )..layout();
    return painter.width + _chipExtraWidth;
  }

  bool _fitsAll(List<double> widths, double maxWidth) {
    var row = 0;
    var rowWidth = 0.0;
    for (final w in widths) {
      final added = rowWidth == 0 ? w : _spacing + w;
      if (rowWidth + added > maxWidth) {
        row++;
        if (row >= _maxRowsCollapsed) return false;
        rowWidth = w;
      } else {
        rowWidth += added;
      }
    }
    return true;
  }

  bool _fitsWithTrailing(
    List<double> widths,
    int n,
    double trailingWidth,
    double maxWidth,
  ) {
    var row = 0;
    var rowWidth = 0.0;
    for (var i = 0; i < n; i++) {
      final w = widths[i];
      final added = rowWidth == 0 ? w : _spacing + w;
      if (rowWidth + added > maxWidth) {
        row++;
        if (row >= _maxRowsCollapsed) return false;
        rowWidth = w;
      } else {
        rowWidth += added;
      }
    }
    final added = rowWidth == 0 ? trailingWidth : _spacing + trailingWidth;
    if (rowWidth + added <= maxWidth) return true;
    row++;
    return row < _maxRowsCollapsed;
  }
}

/// A labeled, centered, tappable block: an uppercase section head over [child],
/// reporting its footprint [Rect] on tap so a destination can morph from it.
class _LabeledBlock extends StatelessWidget {
  final String label;
  final String semantics;
  final ValueChanged<Rect> onTap;
  final Widget child;

  const _LabeledBlock({
    required this.label,
    required this.semantics,
    required this.onTap,
    required this.child,
  });

  @override
  Widget build(BuildContext context) {
    return Builder(
      builder: (ctx) => Tappable(
        semanticsLabel: semantics,
        onTap: () => onTap(workshopRectOf(ctx)),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(
              label.toUpperCase(),
              textAlign: TextAlign.center,
              style: WorkshopCenteredOverview._shead,
            ),
            const SizedBox(height: 8),
            child,
          ],
        ),
      ),
    );
  }
}
