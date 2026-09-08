import 'package:ripls/data/gen/ripls/api/feed_service.pb.dart' show StoryPayload;
import 'package:ripls/l10n/app_localizations.dart';

/// Resolves a [StoryPayload]'s display title and description to
/// locale-appropriate strings.
///
/// Phase 4b of #1904: server emits a structured [StoryPayload.templateKey]
/// + [StoryPayload.templateParams] pair so the client can render in
/// the viewer's locale. When [templateKey] is empty (AI-generated
/// prose or pre-migration historical rows) the resolver falls back
/// to the literal [StoryPayload.title] / [StoryPayload.description]
/// emitted by the server.
///
/// Unknown template keys (server newer than client) also fall back
/// to the literal text so the user still sees something readable
/// during a deploy where server ramps ahead of client.
///
/// Resolution happens in widget `build()` against the current
/// [AppLocalizations] — viewmodels stay string-free per
/// `docs/client/i18n.md` § "Viewmodels never resolve strings".
({String title, String description}) resolveStoryText(
  StoryPayload story,
  AppLocalizations l10n,
) {
  if (!story.hasTemplateKey() || story.templateKey.isEmpty) {
    return (title: story.title, description: story.description);
  }
  final params = story.templateParams;
  String p(String key) => params[key] ?? '';

  switch (story.templateKey) {
    case 'story.loan_completed':
      return (
        title: l10n.serverStoryLoanCompletedTitle(p('gearName')),
        description: l10n.serverStoryLoanCompleted(
          p('borrowerName'),
          p('gearName'),
          p('lenderName'),
        ),
      );
    case 'story.loan_completed_simple':
      return (
        title: l10n.serverStoryLoanCompletedSimpleTitle,
        description: l10n.serverStoryLoanCompletedSimple(p('gearName')),
      );
    case 'story.giveaway_completed':
      return (
        title: l10n.serverStoryGiveawayCompletedTitle,
        description: l10n.serverStoryGiveawayCompleted(
          p('giverName'),
          p('gearName'),
          p('receiverName'),
        ),
      );
    case 'story.giveaway_completed_simple':
      return (
        title: l10n.serverStoryGiveawayCompletedSimpleTitle,
        description: l10n.serverStoryGiveawayCompletedSimple(p('gearName')),
      );
    case 'story.experience_concluded':
      return (
        title: l10n.serverStoryExperienceConcludedTitle,
        description: l10n.serverStoryExperienceConcluded(
          p('hostName'),
          p('eventName'),
          p('participantCount'),
        ),
      );
    case 'story.experience_concluded_simple':
      return (
        title: l10n.serverStoryExperienceConcludedSimpleTitle,
        description: l10n.serverStoryExperienceConcludedSimple(p('eventName')),
      );
    case 'story.request_fulfilled':
      return (
        title: l10n.serverStoryRequestFulfilledTitle,
        description: l10n.serverStoryRequestFulfilled(
          p('requesterName'),
          p('requestTitle'),
          p('fulfillerName'),
        ),
      );
    case 'story.request_fulfilled_simple':
      return (
        title: l10n.serverStoryRequestFulfilledSimpleTitle,
        description: l10n.serverStoryRequestFulfilledSimple(p('requestTitle')),
      );
    case 'story.new_member_welcome':
      return (
        title: l10n.serverStoryNewMemberWelcomeTitle,
        description: l10n.serverStoryNewMemberWelcome(
          p('memberName'),
          p('communityName'),
        ),
      );
    case 'story.new_member_welcome_simple':
      return (
        title: l10n.serverStoryNewMemberWelcomeSimpleTitle,
        description: l10n.serverStoryNewMemberWelcomeSimple(p('communityName')),
      );
    default:
      // Unknown template_key — server is newer than client. Fall
      // back to the literal text so the user still sees something.
      return (title: story.title, description: story.description);
  }
}
