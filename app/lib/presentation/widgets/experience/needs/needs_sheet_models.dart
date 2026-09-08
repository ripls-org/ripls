// Public data-carrier types shared across experience needs sheets.

/// Result returned when the user confirms claiming a need.
///
/// When [remove] is true the user instead tapped the destructive
/// "Remove" affordance (only shown to the proposer of the need). The
/// caller should delete the need rather than create a contribution and
/// the other fields are unused.
class AddContributionResult {
  const AddContributionResult({
    required this.title,
    this.description,
    this.preferMaybe = false,
    this.remove = false,
  });

  final String title;
  final String? description;

  /// True when the user chose "make it a maybe" in the RSVP disclosure.
  final bool preferMaybe;

  /// True when the user tapped Remove. Caller routes to removeNeed.
  final bool remove;
}

enum EditAction { save, delete, unclaim }

/// Result returned when the user saves, deletes, or unclaims a contribution.
class EditContributionResult {
  const EditContributionResult({
    required this.action,
    this.title,
    this.description,
  });

  final EditAction action;
  final String? title;
  final String? description;
}

/// A single need item composed in the batch-add-need sheet.
class NeedDraft {
  const NeedDraft({required this.name, this.note, required this.slots});
  final String name;
  final String? note;
  final int slots;
}

/// A single contribution item composed in the batch-add-contribution sheet.
class ContributionDraft {
  const ContributionDraft({required this.title, this.description});
  final String title;
  final String? description;
}

/// Batch result from [BatchAddNeedSheet].
class BatchAddNeedResult {
  const BatchAddNeedResult(this.items);
  final List<NeedDraft> items;
}

/// Batch result from [BatchAddContributionSheet].
class BatchAddContributionResult {
  const BatchAddContributionResult(this.items, {this.preferMaybe = false});
  final List<ContributionDraft> items;
  final bool preferMaybe;
}
