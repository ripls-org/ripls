import 'package:intl/intl.dart';
import 'package:ripls/data/gen/ripls/api/portfolio.pb.dart'
    show HomeUpNextEntry, HomeUpNextStatus;
import 'package:ripls/l10n/app_localizations.dart';

/// Locale-aware copy for calendar / up-next entries, resolved from the typed
/// fields on [HomeUpNextEntry] (`status`, `going_count`, `all_day`,
/// `is_giveaway`, `gear_name`, `counterparty_first_name`). The server no
/// longer renders this copy (#2827); every string a calendar row shows comes
/// from these resolvers at widget build time.

/// homeUpNextTitle is the entry's row title: for gear obligations a composed
/// line ("Pick up Drill from Mike") built from the typed parts, for every
/// other kind the item's own name carried in `title` (verbatim user content).
String homeUpNextTitle(AppLocalizations l10n, HomeUpNextEntry e) {
  if (!e.hasGearName()) return e.title;
  final item = e.gearName;
  final name = e.hasCounterpartyFirstName() ? e.counterpartyFirstName : '';
  switch (e.status) {
    case HomeUpNextStatus.HOME_UP_NEXT_STATUS_OBLIGATION_PICKUP:
      return l10n.homeCalPickUpFrom(item, name);
    case HomeUpNextStatus.HOME_UP_NEXT_STATUS_OBLIGATION_HANDOFF:
      return l10n.homeCalHandOffTo(item, name);
    case HomeUpNextStatus.HOME_UP_NEXT_STATUS_OBLIGATION_DUE_BACK:
      return l10n.homeCalDueBackFrom(item, name);
    case HomeUpNextStatus.HOME_UP_NEXT_STATUS_OBLIGATION_RETURN:
      return l10n.homeCalReturnTo(item, name);
    default:
      return e.title;
  }
}

/// homeUpNextStatusLabel is the entry's role/state line. Empty when the entry
/// has no stance to show.
String homeUpNextStatusLabel(AppLocalizations l10n, HomeUpNextEntry e) {
  switch (e.status) {
    case HomeUpNextStatus.HOME_UP_NEXT_STATUS_HOSTING:
      return e.goingCount > 0
          ? l10n.homeCalStatusHostingGoing(e.goingCount)
          : l10n.homeCalStatusHosting;
    case HomeUpNextStatus.HOME_UP_NEXT_STATUS_GOING:
      return l10n.homeCalStatusGoing;
    case HomeUpNextStatus.HOME_UP_NEXT_STATUS_MAYBE:
      return l10n.homeCalStatusMaybe;
    case HomeUpNextStatus.HOME_UP_NEXT_STATUS_INVITED:
      return l10n.homeCalStatusInvited;
    case HomeUpNextStatus.HOME_UP_NEXT_STATUS_OTHERS_GOING:
      return l10n.homeCalStatusOthersGoing(e.goingCount);
    case HomeUpNextStatus.HOME_UP_NEXT_STATUS_YOUR_REQUEST_DUE:
      return l10n.homeCalStatusYourRequestDue;
    case HomeUpNextStatus.HOME_UP_NEXT_STATUS_REQUEST_DUE:
      return l10n.homeCalStatusRequestDue;
    case HomeUpNextStatus.HOME_UP_NEXT_STATUS_YOUR_REQUEST:
      return l10n.homeCalStatusYourRequest;
    case HomeUpNextStatus.HOME_UP_NEXT_STATUS_REQUEST_SHARED:
      return l10n.homeCalStatusRequestShared;
    case HomeUpNextStatus.HOME_UP_NEXT_STATUS_ADDED_TO_LIBRARY:
      return l10n.homeCalStatusAddedToLibrary;
    case HomeUpNextStatus.HOME_UP_NEXT_STATUS_SHARED_WITH_YOU:
      return l10n.homeCalStatusSharedWithYou;
    case HomeUpNextStatus.HOME_UP_NEXT_STATUS_OBLIGATION_PICKUP:
      return e.isGiveaway
          ? l10n.homeCalStatusReceiving
          : l10n.homeCalStatusBorrowing;
    case HomeUpNextStatus.HOME_UP_NEXT_STATUS_OBLIGATION_HANDOFF:
      return e.isGiveaway
          ? l10n.homeCalStatusGivingAway
          : l10n.homeCalStatusLending;
    case HomeUpNextStatus.HOME_UP_NEXT_STATUS_OBLIGATION_DUE_BACK:
      return l10n.homeCalStatusLent;
    case HomeUpNextStatus.HOME_UP_NEXT_STATUS_OBLIGATION_RETURN:
      return l10n.homeCalStatusBorrowing;
    default:
      return '';
  }
}

/// homeUpNextTimeLabel is the entry's time line: a localized all-day label
/// for date-only entries, else the entry's clock time in the device locale.
String homeUpNextTimeLabel(AppLocalizations l10n, HomeUpNextEntry e) {
  if (e.allDay) return l10n.homeCalAllDay;
  final when =
      DateTime.fromMillisecondsSinceEpoch(e.timeUnixSec.toInt() * 1000);
  return DateFormat.jm(l10n.localeName).format(when);
}
