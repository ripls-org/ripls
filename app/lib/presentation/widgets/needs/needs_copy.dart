import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/viewmodels/needs_scope.dart';

/// Scope-aware copy resolver for the Needs / Contributions modal stack.
///
/// Every user-visible string on those sheets has two flavours: the
/// Experience-scope phrasing (chipping in to help with an event) and
/// the Request-scope phrasing (breaking down a request into pieces). The
/// ARB carries both — `needs*` for Experience and a parallel
/// `needsReq*` for Request. This class hides that branch from the
/// widgets so sheet code stays focused on layout.
///
/// Construct one per build: `final copy = NeedsCopy(context.l10n,
/// scope.kind);` and call the typed getters below.
class NeedsCopy {
  const NeedsCopy(this._l10n, this._kind);

  final AppLocalizations _l10n;
  final NeedsScopeKind _kind;

  bool get _isRequest => _kind == NeedsScopeKind.request;

  // ── Propose sheet ─────────────────────────────────────────────────────────

  
  
  
  
  
  
  
  
  
  
  
  
  
  
  
  
  // ── Picker modal ──────────────────────────────────────────────────────────

  String get pickerTitle =>
      _isRequest ? _l10n.needsReqPickerTitle : _l10n.needsPickerTitle;

  String get pickerHeadline =>
      _isRequest ? _l10n.needsReqPickerHeadline : _l10n.needsPickerHeadline;

  String get pickerSubtitle =>
      _isRequest ? _l10n.needsReqPickerSubtitle : _l10n.needsPickerSubtitle;

  String get pickerSearchHint => _isRequest
      ? _l10n.needsReqPickerSearchHint
      : _l10n.needsPickerSearchHint;

  String get pickerSuggestionsHeader => _isRequest
      ? _l10n.needsReqPickerSuggestionsHeader
      : _l10n.needsPickerSuggestionsHeader;

  String pickerSuggestionsForCategory(String category) =>
      _l10n.needsPickerSuggestionsForCategory(category);

  String get pickerTakenSubtitle => _isRequest
      ? _l10n.needsReqPickerTakenSubtitle
      : _l10n.needsPickerTakenSubtitle;

  String get pickerBackToPick => _l10n.needsPickerBackToPick;

  String get pickerAddToList => _isRequest
      ? _l10n.needsReqPickerAddToList
      : _l10n.needsPickerAddToList;

  String get pickerEditEyebrow => _isRequest
      ? _l10n.needsReqPickerEditEyebrow
      : _l10n.needsPickerEditEyebrow;

  String get pickerEditRemove => _l10n.needsPickerEditRemove;

  String get pickerEditSave => _l10n.needsPickerEditSave;

  String get pickerEditAlreadyClaimed => _isRequest
      ? _l10n.needsReqPickerEditAlreadyClaimed
      : _l10n.needsPickerEditAlreadyClaimed;

  String get slotsLabel =>
      _isRequest ? _l10n.needsReqSlotsLabel : _l10n.experienceNeedsSlotsLabel;

  String get noteLabel => _l10n.experienceNeedsNoteLabel;

  // ── Volunteer sheet ───────────────────────────────────────────────────────

  String get volunteerEyebrow => _isRequest
      ? _l10n.needsReqVolunteerEyebrow
      : _l10n.needsVolunteerEyebrow;

  String get volunteerHeadlineOrganizer => _isRequest
      ? _l10n.needsReqVolunteerHeadlineOrganizer
      : _l10n.needsVolunteerHeadlineOrganizer;

  String get volunteerHeadlineHelper => _isRequest
      ? _l10n.needsReqVolunteerHeadlineHelper
      : _l10n.needsVolunteerHeadlineHelper;

  
  String get volunteerHeaderYourein => _isRequest
      ? _l10n.needsReqVolunteerHeaderYourein
      : _l10n.needsVolunteerHeaderYourein;

  String get volunteerEyebrowYourein => _isRequest
      ? _l10n.needsReqVolunteerEyebrowYourein
      : _l10n.needsVolunteerEyebrowYourein;

  
  String volunteerCoverage(int claimed, int total) => _isRequest
      ? _l10n.needsReqVolunteerCoverage(claimed, total)
      : _l10n.needsVolunteerCoverage(claimed, total);

  String get volunteerCoverageIncludingYou =>
      _l10n.needsVolunteerCoverageIncludingYou;

  
  /// Short coverage clause for the merged eyebrow row, e.g. "1 of 2
  /// covered". Falls back to the long "X of Y pieces covered" key on
  /// the Experience scope where the short variant isn't defined yet.
  String volunteerCoverageShort(int covered, int total) => _isRequest
      ? _l10n.needsReqVolunteerCoverageShort(covered, total)
      : _l10n.needsVolunteerCoverageThingsCovered(covered, total);

  
  String get volunteerManageButton => _l10n.needsVolunteerManageButton;

  String get volunteerEmpty =>
      _isRequest ? _l10n.needsReqVolunteerEmpty : _l10n.needsVolunteerEmpty;

  String get volunteerSavedIdle => _isRequest
      ? _l10n.needsReqVolunteerSavedIdle
      : _l10n.needsVolunteerSavedIdle;

  String volunteerSavedDone(int claimed, int total) => _isRequest
      ? _l10n.needsReqVolunteerSavedDone(claimed, total)
      : _l10n.needsVolunteerSavedDone(claimed, total);

  String get addOptionGhost =>
      _isRequest ? _l10n.needsReqAddOptionGhost : _l10n.needsAddOptionGhost;

  
  // ── Manage menu ───────────────────────────────────────────────────────────

  String get manageEditList =>
      _isRequest ? _l10n.needsReqManageEditList : _l10n.needsManageEditList;

  String get manageEditListDescription => _isRequest
      ? _l10n.needsReqManageEditListDescription
      : _l10n.needsManageEditListDescription;

  String get manageNudgeUnclaimed => _isRequest
      ? _l10n.needsReqManageNudgeUnclaimed
      : _l10n.needsManageNudgeUnclaimed;

  String get manageNudgeUnclaimedDescription => _isRequest
      ? _l10n.needsReqManageNudgeUnclaimedDescription
      : _l10n.needsManageNudgeUnclaimedDescription;

  String get manageCancel =>
      _isRequest ? _l10n.needsReqManageCancel : _l10n.needsManageCancel;

  String get manageCancelDescription => _isRequest
      ? _l10n.needsReqManageCancelDescription
      : _l10n.needsManageCancelDescription;

  // ── Edit-mode atoms ───────────────────────────────────────────────────────

  
  
  String get editBannerOrganizer => _isRequest
      ? _l10n.needsReqEditBannerOrganizer
      : _l10n.needsEditBannerOrganizer;

  String get editBannerMember => _isRequest
      ? _l10n.needsReqEditBannerMember
      : _l10n.needsEditBannerMember;

  // ── Claim row ─────────────────────────────────────────────────────────────

  
  
  
  // ── Archived sheet ────────────────────────────────────────────────────────

  String get archivedEyebrow => _isRequest
      ? _l10n.needsReqArchivedEyebrow
      : _l10n.needsArchivedEyebrow;

  String get archivedTitle =>
      _isRequest ? _l10n.needsReqArchivedTitle : _l10n.needsArchivedTitle;

  String get archivedSave => _l10n.needsArchivedSave;

  
  // ── Add to the list sheet ────────────────────────────────────────────────
  //
  // Most of the sheet's copy is shared across scopes (eyebrow,
  // section labels, toggle title, CTA). The three strings below
  // change tone — Experience leans collaborative-event ("Add
  // something the group can bring"), Request leans single-help-request
  // ("Add a need anyone can pick up").

  String get addToListInputHint => _isRequest
      ? _l10n.needsReqAddToListInputHint
      : _l10n.needsAddToListInputHint;

  String get addToListHelperText => _isRequest
      ? _l10n.needsReqAddToListHelperText
      : _l10n.needsAddToListHelperText;

  String get addToListNoteHint => _isRequest
      ? _l10n.needsReqAddToListNoteHint
      : _l10n.needsAddToListNoteHint;
}
