import 'package:intl/intl.dart';
import 'package:ripls/data/gen/ripls/api/portfolio.pb.dart';
import 'package:ripls/l10n/app_localizations.dart';

/// Locale-aware copy for Needs-you decision rows, resolved from the typed
/// fields on [HomeDecision] (`kind`, `transfer_action`, `reason`,
/// `due_at_unix_sec`, `message_preview`). The server no longer renders this
/// copy (#2827); every string a decision row shows comes from these resolvers
/// at widget build time.

/// homeDecisionAcceptLabel is the action-chip label for [d], derived from its
/// kind (and, for borrowing updates, the typed transfer action). Empty when
/// the kind is unknown — the row hides the chip.
String homeDecisionAcceptLabel(AppLocalizations l10n, HomeDecision d) {
  switch (d.kind) {
    case HomeDecisionKind.HOME_DECISION_KIND_LENDING_REQUEST:
      return l10n.homeAcceptLend;
    case HomeDecisionKind.HOME_DECISION_KIND_GIVEAWAY_REQUEST:
      return l10n.homeAcceptGive;
    case HomeDecisionKind.HOME_DECISION_KIND_ASK_CLAIM:
      return l10n.homeAcceptSayThanks;
    case HomeDecisionKind.HOME_DECISION_KIND_REPLY:
      return l10n.homeAcceptReply;
    case HomeDecisionKind.HOME_DECISION_KIND_MARK_DONE:
      return l10n.homeAcceptDone;
    case HomeDecisionKind.HOME_DECISION_KIND_TRANSFER_UPDATE:
      return d.transferAction ==
              HomeTransferAction.HOME_TRANSFER_ACTION_COMPLETE_LOAN
          ? l10n.homeAcceptMarkReturned
          : l10n.homeAcceptMarkPickedUp;
    default:
      return '';
  }
}

/// homeDecisionHeadline is the row's title: the composed "{name} wants your
/// {item}" line for lend/giveaway requests, the subject title (pure user
/// content) for every other kind.
String homeDecisionHeadline(AppLocalizations l10n, HomeDecision d) {
  if (d.kind == HomeDecisionKind.HOME_DECISION_KIND_LENDING_REQUEST ||
      d.kind == HomeDecisionKind.HOME_DECISION_KIND_GIVEAWAY_REQUEST) {
    final who = d.hasCounterparty()
        ? d.counterparty.displayName.split(' ').first
        : '';
    return l10n.homeDecisionWants(who, d.subjectTitle);
  }
  return d.subjectTitle;
}

/// homeDecisionWhy is the row's supporting context line, resolved from the
/// typed [HomeDecision.reason]. Message previews return the user's text
/// verbatim; dated reasons format [HomeDecision.dueAtUnixSec] in the device
/// locale. Empty when there is no context line.
String homeDecisionWhy(AppLocalizations l10n, HomeDecision d,
    {DateTime? now}) {
  final effectiveNow = now ?? DateTime.now();
  switch (d.reason) {
    case HomeDecisionReason.HOME_DECISION_REASON_MESSAGE_PREVIEW:
      return d.messagePreview;
    case HomeDecisionReason.HOME_DECISION_REASON_PICKUP_SCHEDULED:
      return l10n.homeWhyPickupDay(
          homeDayLabel(l10n, _dueDate(d), effectiveNow));
    case HomeDecisionReason.HOME_DECISION_REASON_READY_TO_PICK_UP:
      return l10n.homeWhyReadyToPickUp;
    case HomeDecisionReason.HOME_DECISION_REASON_RETURN_DUE:
      return l10n.homeWhyDueDay(homeDayLabel(l10n, _dueDate(d), effectiveNow));
    case HomeDecisionReason.HOME_DECISION_REASON_BORROWING:
      return l10n.homeWhyBorrowing;
    case HomeDecisionReason.HOME_DECISION_REASON_PAST_EVENT:
      return l10n.homeMarkDoneWhy(
          DateFormat.E(l10n.localeName).format(_dueDate(d)),
          _markDonePhrase(l10n, d.id, isEvent: true));
    case HomeDecisionReason.HOME_DECISION_REASON_PAST_REQUEST:
      return l10n.homeMarkDoneWhy(
          DateFormat.E(l10n.localeName).format(_dueDate(d)),
          _markDonePhrase(l10n, d.id, isEvent: false));
    default:
      return '';
  }
}

/// homeDecisionWhyIsQuote reports whether the context line is verbatim user
/// text that should render inside quotation marks, as opposed to composed
/// status copy.
bool homeDecisionWhyIsQuote(HomeDecision d) =>
    d.reason == HomeDecisionReason.HOME_DECISION_REASON_MESSAGE_PREVIEW;

/// homeDayLabel formats [target] as a short day label in the device locale:
/// "today" when it falls on the current day, an abbreviated weekday within
/// the next week ("Mon"), or a short date otherwise ("Jun 20").
String homeDayLabel(AppLocalizations l10n, DateTime target, DateTime now) {
  final targetDay = DateTime(target.year, target.month, target.day);
  final today = DateTime(now.year, now.month, now.day);
  final days = targetDay.difference(today).inDays;
  if (days == 0) return l10n.homeDayToday;
  if (days > 0 && days < 7) {
    return DateFormat.E(l10n.localeName).format(target);
  }
  return DateFormat.MMMd(l10n.localeName).format(target);
}

DateTime _dueDate(HomeDecision d) =>
    DateTime.fromMillisecondsSinceEpoch(d.dueAtUnixSec.toInt() * 1000);

/// Deterministic per-item wrap-up prompt so a long Mark-done list doesn't
/// read as identical rows while each item keeps its prompt across refreshes.
/// FNV-1a over the decision id, matching the pick the server used so a row's
/// prompt survives the copy migration unchanged.
String _markDonePhrase(AppLocalizations l10n, String id,
    {required bool isEvent}) {
  final phrases = isEvent
      ? [
          l10n.homeMarkDoneEventPhrase1,
          l10n.homeMarkDoneEventPhrase2,
          l10n.homeMarkDoneEventPhrase3,
          l10n.homeMarkDoneEventPhrase4,
        ]
      : [
          l10n.homeMarkDoneRequestPhrase1,
          l10n.homeMarkDoneRequestPhrase2,
          l10n.homeMarkDoneRequestPhrase3,
          l10n.homeMarkDoneRequestPhrase4,
        ];
  return phrases[_fnv1a32(id) % phrases.length];
}

/// 32-bit FNV-1a, kept within the JS-safe integer range so web and native
/// builds hash identically.
int _fnv1a32(String id) {
  var h = 0x811C9DC5;
  for (final unit in id.codeUnits) {
    h ^= unit;
    final hi = ((h >> 16) * 0x01000193) & 0xFFFF;
    final lo = (h & 0xFFFF) * 0x01000193;
    h = ((hi << 16) + lo) & 0xFFFFFFFF;
  }
  return h;
}
