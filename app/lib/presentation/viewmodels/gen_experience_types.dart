import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/data/repositories/provisional_user_repository.dart'
    show ProvisionalUser;

/// Result returned from the experience creation flow.
///
/// When [isPastEvent] is true, the caller should open [MarkCompletedModal]
/// with the [preTaggedUsers] and [preProvisionalUsers] after closing the creation
/// modal stack so the completion modal appears over the feed.
class ExperienceCreationResult {
  const ExperienceCreationResult({
    required this.experienceId,
    required this.isPastEvent,
    this.preTaggedUsers = const [],
    this.preProvisionalUsers = const [],
  });

  final String experienceId;
  final bool isPastEvent;
  final List<User> preTaggedUsers;
  final List<ProvisionalUser> preProvisionalUsers;
}

/// Resolved participants from AI-mentioned names.
///
/// [users] are registered community member User objects (may include multiple
/// matches per name).
/// [provisionalUsers] are newly created provisional users for unresolved names.
class ResolvedParticipants {
  const ResolvedParticipants({
    required this.users,
    required this.provisionalUsers,
  });

  final List<User> users;
  final List<ProvisionalUser> provisionalUsers;
}
