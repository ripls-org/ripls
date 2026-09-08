import 'package:intl/intl.dart';
import 'package:ripls/data/gen/ripls/api/chat_service.pb.dart'
    show SystemMessage;
import 'package:ripls/l10n/app_localizations.dart';

/// Resolves a [SystemMessage]'s display text to a locale-appropriate
/// string.
///
/// Phase 4b of #1904: server emits a structured
/// [SystemMessage.templateKey] + [SystemMessage.templateParams] pair
/// so the client can render in the viewer's locale. When
/// [templateKey] is empty (legacy emit sites not yet migrated, or
/// historical rows pre-migration) the resolver falls back to the
/// literal [SystemMessage.description] emitted by the server.
///
/// Unknown template keys (server newer than client) also fall back
/// to the literal description so the user still sees something
/// readable during a staged rollout.
///
/// Resolution happens in widget `build()` against the current
/// [AppLocalizations] — viewmodels stay string-free per
/// `docs/client/i18n.md` § "Viewmodels never resolve strings".
String resolveSystemMessageText(
  SystemMessage sysMsg,
  AppLocalizations l10n,
) {
  if (!sysMsg.hasTemplateKey() || sysMsg.templateKey.isEmpty) {
    return sysMsg.description;
  }
  final params = sysMsg.templateParams;
  String p(String key) => params[key] ?? '';

  // Person-name params. The server sends the raw name, or nothing at all when
  // it could not resolve one (deleted account, member with no name set) —
  // rather than an English stand-in that would land untranslated inside an
  // otherwise-localized sentence (#2844).
  String person(String key) {
    final name = params[key];
    if (name == null || name.isEmpty) return l10n.serverChatSystemUnknownPerson;
    return name;
  }

  switch (sysMsg.templateKey) {
    // --- Transfer lifecycle ---
    case 'chat.transfer.approved':
      return l10n.serverChatSystemTransferApproved(person('recipientName'));
    case 'chat.transfer.cancelled':
      return l10n.serverChatSystemTransferCancelled;
    case 'chat.transfer.loan_started':
      return l10n.serverChatSystemTransferLoanStarted;
    case 'chat.transfer.completed':
      return l10n.serverChatSystemTransferCompleted;

    // --- Experience lifecycle ---
    case 'chat.experience.started':
      return l10n.serverChatSystemExperienceStarted(person('actorName'));
    case 'chat.experience.completed_by_actor':
      return l10n.serverChatSystemExperienceCompletedByActor(person('actorName'));
    case 'chat.experience.cancelled':
      return l10n.serverChatSystemExperienceCancelled(person('actorName'));

    // --- Transfer interest ---
    case 'chat.transfer.requested_to_borrow':
      return l10n.serverChatSystemTransferRequestedToBorrow(person('actorName'));
    // 'submitted_interest' is the retired key (#2724): messages written
    // before the raise-a-hand rename still carry it, so both resolve here.
    case 'chat.transfer.raised_hand':
    case 'chat.transfer.submitted_interest':
      return l10n.serverChatSystemTransferRaisedHand(person('actorName'));
    case 'chat.transfer.withdrew_interest':
      return l10n.serverChatSystemTransferWithdrewInterest(person('actorName'));

    // --- Undo (one variant per retracted action) ---
    case 'chat.undone.approved':
      return l10n.serverChatSystemUndoneApproved(person('actorName'));
    case 'chat.undone.completed':
      return l10n.serverChatSystemUndoneCompleted(person('actorName'));
    case 'chat.undone.fulfilled':
      return l10n.serverChatSystemUndoneFulfilled(person('actorName'));
    case 'chat.undone.started':
      return l10n.serverChatSystemUndoneStarted(person('actorName'));
    case 'chat.undone.cancelled':
      return l10n.serverChatSystemUndoneCancelled(person('actorName'));
    case 'chat.undone.generic':
      return l10n.serverChatSystemUndoneGeneric(person('actorName'));

    // --- Experience time/location/share/RSVP ---
    case 'chat.experience.time_poll_opened':
      return l10n.serverChatSystemExperienceTimePollOpened(person('actorName'));
    case 'chat.experience.time_confirmed':
      return l10n.serverChatSystemExperienceTimeConfirmed(
          person('actorName'), _eventTimeText(l10n, params));
    case 'chat.experience.time_confirmed_tbd':
      return l10n.serverChatSystemExperienceTimeConfirmedTbd(person('actorName'));
    case 'chat.experience.detail_changed_time':
      return l10n.serverChatSystemExperienceDetailChangedTime(
          person('actorName'), _eventTimeText(l10n, params));
    case 'chat.experience.detail_changed_time_tbd':
      return l10n.serverChatSystemExperienceDetailChangedTimeTbd(
          person('actorName'));
    case 'chat.experience.time_poll_cancelled':
      return l10n.serverChatSystemExperienceTimePollCancelled(person('actorName'));
    case 'chat.experience.location_poll_opened':
      return l10n.serverChatSystemExperienceLocationPollOpened(person('actorName'));
    case 'chat.experience.detail_changed_location':
      return l10n.serverChatSystemExperienceDetailChangedLocation(
          person('actorName'), p('locationName'));
    case 'chat.experience.detail_changed_location_unnamed':
      return l10n.serverChatSystemExperienceDetailChangedLocationUnnamed(
          person('actorName'));
    case 'chat.experience.location_poll_cancelled':
      return l10n.serverChatSystemExperienceLocationPollCancelled(
          person('actorName'));
    case 'chat.experience.created':
      return l10n.serverChatSystemExperienceCreated(
          person('actorName'), p('eventName'));
    case 'chat.experience.completion_summary':
      return l10n.serverChatSystemExperienceCompletionSummary(
          person('actorName'), p('summary'));
    case 'chat.experience.rsvp_yes':
      return l10n.serverChatSystemExperienceRsvpYes(person('actorName'));
    case 'chat.experience.rsvp_maybe':
      return l10n.serverChatSystemExperienceRsvpMaybe(person('actorName'));
    case 'chat.experience.rsvp_no':
      return l10n.serverChatSystemExperienceRsvpNo(person('actorName'));

    // --- Request lifecycle ---
    case 'chat.request.fulfilled':
      return l10n.serverChatSystemRequestFulfilled(person('actorName'));
    case 'chat.request.cancelled':
      return l10n.serverChatSystemRequestCancelled(person('actorName'));
    case 'chat.request.offered':
      return l10n.serverChatSystemRequestOffered(person('actorName'));
    case 'chat.request.withdrew_offer':
      return l10n.serverChatSystemRequestWithdrewOffer(person('actorName'));
    case 'chat.request.created':
      return l10n.serverChatSystemRequestCreated(
          person('actorName'), p('requestTitle'));

    // --- Community gear sharing ---
    case 'chat.community.gear_shared_giveaway':
      return l10n.serverChatSystemCommunityGearSharedGiveaway(
          person('actorName'), p('gearName'));
    case 'chat.community.gear_shared_loan':
      return l10n.serverChatSystemCommunityGearSharedLoan(
          person('actorName'), p('gearName'));

    // --- Planning needs/contributions ---
    case 'chat.planning.need_added':
      return l10n.serverChatSystemPlanningNeedAdded(
          person('actorName'), p('needName'));
    case 'chat.planning.need_added_with_note':
      return l10n.serverChatSystemPlanningNeedAddedWithNote(
          person('actorName'), p('needName'), p('noteSnippet'));
    case 'chat.planning.need_removed':
      return l10n.serverChatSystemPlanningNeedRemoved(
          person('actorName'), p('needName'));
    case 'chat.planning.need_claimed':
      return l10n.serverChatSystemPlanningNeedClaimed(
          person('actorName'), p('needName'));
    case 'chat.planning.need_claimed_with_note':
      return l10n.serverChatSystemPlanningNeedClaimedWithNote(
          person('actorName'), p('needName'), p('noteSnippet'));
    case 'chat.planning.contribution_added':
      return l10n.serverChatSystemPlanningContributionAdded(
          person('actorName'), p('title'));
    case 'chat.planning.contribution_added_with_description':
      return l10n.serverChatSystemPlanningContributionAddedWithDescription(
          person('actorName'), p('title'), p('descriptionSnippet'));
    case 'chat.planning.contribution_removed':
      return l10n.serverChatSystemPlanningContributionRemoved(
          person('actorName'), p('title'));

    default:
      // Unknown template_key — server is newer than client. Fall
      // back to the literal description so the user still sees
      // something readable.
      return sysMsg.description;
  }
}

/// Renders a time-bearing message's time in the viewer's locale. Current
/// servers send the instant plus the event timezone's UTC offset
/// (`timeUnixSec` / `timeUtcOffsetMin`), from which the event-local wall
/// time is reconstructed and formatted locale-aware. Older payloads (and
/// informal, user-typed descriptions) carry only the pre-rendered
/// `formattedTime`, returned verbatim.
String _eventTimeText(AppLocalizations l10n, Map<String, String> params) {
  final sec = int.tryParse(params['timeUnixSec'] ?? '');
  if (sec == null) return params['formattedTime'] ?? '';
  final offsetMin = int.tryParse(params['timeUtcOffsetMin'] ?? '') ?? 0;
  final wall = DateTime.fromMillisecondsSinceEpoch(sec * 1000, isUtc: true)
      .add(Duration(minutes: offsetMin));
  final locale = l10n.localeName;
  final day = DateFormat('EEE, MMM d', locale).format(wall);
  final clock = DateFormat.jm(locale).format(wall);
  return '$day · $clock';
}
