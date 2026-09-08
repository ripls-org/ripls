// dart-line-count-allow: pushed over by needs-v2 additions
// (openArchivedSheet projection, NK4 edit-row flow, nudge dispatch).
// A clean split (extract the per-surface openers — openVolunteerSheet,
// openProposeSheet, openManageMenu, openArchivedSheet — into separate
// mixins applied to NeedsActions) is tracked in #2148. Done together
// with the rest of the v2 work would have made the diff harder to
// review; landing as-is so the surface contract is stable first.
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/errors/user_error.dart' show UserError;
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/data/gen/ripls/api/request.pb.dart'
    show RequestGearOffer;
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart'
    show TransferState, TransferType;
import 'package:ripls/data/repositories/experience_repository.dart'
    show RSVPIntention;
import 'package:ripls/presentation/screens/experience/widgets/experience_rsvp_composer_sheet.dart';
import 'package:ripls/presentation/screens/request/widgets/request_contribution_composer_sheet.dart';
import 'package:ripls/presentation/viewmodels/experience_needs_view_model.dart';
import 'package:ripls/presentation/viewmodels/needs_scope.dart';
import 'package:ripls/presentation/viewmodels/request_needs_view_model.dart';
import 'package:ripls/presentation/viewmodels/request_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/experience/compose/experience_compose_sheet.dart';
import 'package:ripls/presentation/widgets/experience/experience_needs_sheets.dart';
import 'package:ripls/presentation/widgets/needs/needs_add_to_list_sheet.dart';
import 'package:ripls/presentation/widgets/needs/needs_archived_sheet.dart';
import 'package:ripls/presentation/widgets/needs/needs_claim_confirm_sheet.dart';
import 'package:ripls/presentation/widgets/needs/needs_manage_menu_sheet.dart';
import 'package:ripls/presentation/widgets/needs/needs_picker_modal.dart';
import 'package:ripls/presentation/widgets/needs/needs_suggestion_grid.dart';
import 'package:ripls/presentation/widgets/needs/needs_volunteer_sheet.dart';
import 'package:ripls/presentation/widgets/request/compose/request_compose_sheet.dart';
import 'package:ripls/services/providers/auth_providers.dart';
import 'package:ripls/services/providers/gear_providers.dart';

final _log = Logger('NeedsActions');

/// Transfer states in which a gear-backed request offer (#2702) is live —
/// pre-handoff or on loan. A contribution whose offer is in one of these
/// states must not be escalated again.
const _liveOfferStates = {
  TransferState.TRANSFER_STATE_INTEREST_EXPRESSED,
  TransferState.TRANSFER_STATE_RECIPIENT_SELECTED,
  TransferState.TRANSFER_STATE_ACTIVE,
};

/// Shared action handler for needs and contributions across Experience and
/// Request scopes. Takes a [NeedsScope] at construction and dispatches to the
/// correct provider and sheets based on the scope at runtime.
///
/// Used by [NeedsSection] (Plan/Details tab) and the chat-pane pill handlers so
/// that tapping a pill always opens the same sheet as tapping the Plan tab card.
class NeedsActions {
  const NeedsActions({required this.scope});

  final NeedsScope scope;

  /// Opens the view sheet for a need. [need] must be [ExperienceNeedResponse]
  /// for [ExperienceNeedsScope] or [RequestNeedResponse] for [RequestNeedsScope].
  Future<void> onNeedTap(
    BuildContext context,
    WidgetRef ref,
    Object need,
  ) async {
    switch (scope) {
      case final ExperienceNeedsScope s:
        final n = need as ExperienceNeedResponse;
        final isProposer = n.proposer.id == s.currentUserId;
        await ViewNeedSheet.show(
          context,
          need: n,
          canClaim: !s.isTerminal && n.slotsRemaining > 0,
          canRemove: isProposer && !s.isTerminal,
          isRsvped: s.isRsvped,
          onClaim: () => claimNeed(context, ref, n),
          onRemove: () => removeNeed(context, ref, n.id),
        );
      case RequestNeedsScope():
        // Request scope migrated to the redesigned Volunteer/Archived flow
        // (#2175). Single-need taps from chat pills route through the same
        // state-aware dispatcher as the row tap on the first tab, so a
        // chat-mentioned need lands the viewer on the canonical list view.
        await openPlanTabDispatcher(context, ref);
    }
  }

  /// Opens the claim flow for a need. [need] must match the scope type.
  Future<void> claimNeed(
    BuildContext context,
    WidgetRef ref,
    Object need,
  ) async {
    switch (scope) {
      case final ExperienceNeedsScope s:
        final n = need as ExperienceNeedResponse;
        final isProposer = n.proposer.id == s.currentUserId;
        final result = await AddContributionSheet.show(
          context,
          needName: n.name,
          needNote: n.hasNote() && n.note.isNotEmpty ? n.note : null,
          isRsvped: s.isRsvped,
          isRsvpedMaybe: s.isRsvpedMaybe,
          experienceName: s.experienceName,
          proposerName: n.proposer.name.isNotEmpty ? n.proposer.name : null,
          canRemove: isProposer && !s.isTerminal,
        );
        if (result == null || !context.mounted) return;
        final expNotifier = ref.read(
          experienceNeedsProvider(s.experienceId).notifier,
        );
        if (result.remove) {
          await expNotifier.removeNeed(n.id);
          if (!context.mounted) return;
          final expState = ref.read(experienceNeedsProvider(s.experienceId));
          if (expState.mutationError != null) {
            ToastHelper.showError(
              context,
              RpcErrorHandler.localize(expState.mutationError!, context.l10n),
            );
          }
          return;
        }
        if (!s.isRsvped) {
          final intention = result.preferMaybe
              ? RSVPIntention.RSVP_INTENTION_MAYBE
              : RSVPIntention.RSVP_INTENTION_YES;
          await expNotifier.rsvpWithIntention(s.communityId, intention);
          if (!context.mounted) return;
          final rsvpState = ref.read(experienceNeedsProvider(s.experienceId));
          if (rsvpState.mutationError != null) {
            ToastHelper.showError(
              context,
              RpcErrorHandler.localize(rsvpState.mutationError!, context.l10n),
            );
            return;
          }
        }
        final note = result.description;
        await expNotifier.claimNeed(
          n.id,
          note: note?.isEmpty ?? false ? null : note,
        );
        if (!context.mounted) return;
        final expState = ref.read(experienceNeedsProvider(s.experienceId));
        if (expState.mutationError != null) {
          ToastHelper.showError(
            context,
            RpcErrorHandler.localize(expState.mutationError!, context.l10n),
          );
        }
      case RequestNeedsScope():
        // Request scope migrated to the redesigned Volunteer flow (#2175);
        // claim happens inline through tap-to-claim autosave rather than a
        // dedicated claim sheet.
        await openPlanTabDispatcher(context, ref);
    }
  }

  /// Opens the request-process claim sheet ([NeedsClaimConfirmSheet]) for an
  /// item identified by [name]. The sheet renders as "Claim a need" when the
  /// viewer isn't bringing the item yet and "Edit your contribution" when they
  /// already are — the exact pair the Request volunteer flow uses. Shared by
  /// the RSVP composer's chips and the pitching-in roster so both surfaces use
  /// the same modal and behavior.
  ///
  /// Pass [linkedNeedId] + [slotsNeeded] when the item is an open need so a
  /// fresh claim reserves a slot; otherwise it's recorded as a free-form
  /// contribution.
  Future<void> openClaimSheet(
    BuildContext context,
    WidgetRef ref, {
    required String name,
    String? note,
    String? proposerName,
    String? linkedNeedId,
    int slotsNeeded = 1,
  }) async {
    switch (scope) {
      case final ExperienceNeedsScope s:
        final notifier = ref.read(
          experienceNeedsProvider(s.experienceId).notifier,
        );
        // The viewer's existing contribution for this item, if any, decides
        // whether the sheet opens in claim ("Claim a need") or edit ("Edit
        // your contribution") mode.
        final lower = name.toLowerCase();
        ExperienceContributionResponse? mine;
        for (final c
            in ref
                .read(experienceNeedsProvider(s.experienceId))
                .contributions) {
          if (c.contributor.id == s.currentUserId &&
              c.title.toLowerCase() == lower) {
            mine = c;
            break;
          }
        }
        final alreadyMine = mine != null;

        // A gear-backed claim on an event escalates into a real loan or
        // giveaway to the host for the event's duration (#2708) — surface the
        // Lend/Give choice unless the viewer is the host (a self-loan is
        // meaningless) or this contribution already carries a live offer.
        final showLendGive = s.currentUserId != null &&
            s.currentUserId != s.ownerId &&
            (mine == null || !_experienceContributionHasLiveOffer(mine));

        final result = await NeedsClaimConfirmSheet.show(
          context,
          needName: name,
          needNote: note != null && note.isNotEmpty ? note : null,
          proposerName: proposerName != null && proposerName.isNotEmpty
              ? proposerName
              : null,
          currentUserId: s.currentUserId,
          currentUser: ref.read(authStateProvider).user,
          communityId: s.communityId,
          canEditNeed: false,
          initialGearId:
              mine != null && mine.hasGearId() && mine.gearId.isNotEmpty
              ? mine.gearId
              : null,
          slotsNeeded: slotsNeeded < 1 ? 1 : slotsNeeded,
          isAlreadyClaimed: alreadyMine,
          initialQuantity: 1,
          showLendGiveChoice: showLendGive,
        );
        if (result == null || !context.mounted) return;

        if (result.unclaimRequested) {
          if (mine != null) {
            if (mine.hasFromNeedId() && mine.fromNeedId.isNotEmpty) {
              await notifier.unclaimNeed(mine.id);
            } else {
              await notifier.removeContribution(mine.id);
            }
            if (!context.mounted) return;
            _surfaceMutationError(
              context,
              ref.read(experienceNeedsProvider(s.experienceId)).mutationError,
            );
          }
          return;
        }
        // `editRequested` only fires when `canEditNeed` is on (the need-edit
        // pencil), which this surface disables.
        if (result.editRequested || !result.confirmed) return;

        // A brand-new claim requires the viewer to be RSVPed (the server
        // rejects contributions otherwise). "I'll bring it" implies going.
        if (!s.isRsvped && !alreadyMine) {
          await notifier.rsvpWithIntention(
            s.communityId,
            RSVPIntention.RSVP_INTENTION_YES,
          );
          if (!context.mounted) return;
          final rsvpState = ref.read(experienceNeedsProvider(s.experienceId));
          if (rsvpState.mutationError != null) {
            _surfaceMutationError(context, rsvpState.mutationError);
            return;
          }
        }

        final desc = result.note == null || result.note!.isEmpty
            ? null
            : result.note;
        if (alreadyMine) {
          await notifier.editContribution(
            contributionId: mine.id,
            title: name,
            description: desc,
            gearId: result.gearId,
          );
        } else if (linkedNeedId != null && linkedNeedId.isNotEmpty) {
          final qty = result.quantity < 1 ? 1 : result.quantity;
          for (var i = 0; i < qty; i++) {
            await notifier.claimNeed(
              linkedNeedId,
              note: desc,
              gearId: result.gearId,
            );
            if (!context.mounted) return;
          }
        } else {
          await notifier.addContribution(
            title: name,
            description: desc,
            gearId: result.gearId,
          );
        }
        if (!context.mounted) return;
        final expPostState = ref.read(experienceNeedsProvider(s.experienceId));
        _surfaceMutationError(context, expPostState.mutationError);

        // Escalate the gear-backed claim into a live offer to the host
        // (#2708). Only after a clean mutation, and only when the Lend/Give
        // choice was shown (i.e. no live offer already exists).
        if (showLendGive &&
            expPostState.mutationError == null &&
            result.gearId != null &&
            result.gearId!.isNotEmpty) {
          await _escalateExperienceGearOffer(
            context,
            ref,
            s,
            gearId: result.gearId!,
            give: result.give,
            matchTitle: name,
            linkedNeedId: linkedNeedId,
          );
        }
      case final RequestNeedsScope s:
        final notifier = ref.read(requestNeedsProvider(s.requestId).notifier);
        final contribs =
            ref.read(requestNeedsProvider(s.requestId)).contributions;
        final lower = name.toLowerCase();
        final myIdx = contribs.indexWhere(
          (c) =>
              c.contributor.id == s.currentUserId &&
              c.title.toLowerCase() == lower,
        );
        final mine = myIdx >= 0 ? contribs[myIdx] : null;

        // A gear-backed claim on a request escalates into a real loan or
        // giveaway offer to the requester (#2702) — surface the Lend/Give
        // choice unless this contribution already carries a live offer.
        final showLendGive = !s.isOwner &&
            (mine == null ||
                _liveOfferForContribution(ref, s.requestId, mine.id) == null);

        final result = await NeedsClaimConfirmSheet.show(
          context,
          needName: name,
          needNote: note != null && note.isNotEmpty ? note : null,
          proposerName: proposerName != null && proposerName.isNotEmpty
              ? proposerName
              : null,
          currentUserId: s.currentUserId,
          currentUser: ref.read(authStateProvider).user,
          communityId: s.communityId,
          canEditNeed: false,
          initialGearId:
              mine != null && mine.hasGearId() && mine.gearId.isNotEmpty
              ? mine.gearId
              : null,
          slotsNeeded: slotsNeeded < 1 ? 1 : slotsNeeded,
          isAlreadyClaimed: mine != null,
          initialQuantity: 1,
          showLendGiveChoice: showLendGive,
        );
        if (result == null || !context.mounted) return;

        if (result.unclaimRequested) {
          if (mine != null) {
            if (mine.hasFromNeedId() && mine.fromNeedId.isNotEmpty) {
              await notifier.unclaimNeed(mine.id);
            } else {
              await notifier.removeContribution(mine.id);
            }
            if (!context.mounted) return;
            _surfaceMutationError(
              context,
              ref.read(requestNeedsProvider(s.requestId)).mutationError,
            );
          }
          return;
        }
        // `editRequested` only fires when `canEditNeed` is on, which this
        // surface disables.
        if (result.editRequested || !result.confirmed) return;

        final desc = result.note == null || result.note!.isEmpty
            ? null
            : result.note;
        if (mine != null) {
          await notifier.editContribution(
            contributionId: mine.id,
            title: name,
            description: desc,
            gearId: result.gearId,
          );
        } else if (linkedNeedId != null && linkedNeedId.isNotEmpty) {
          final qty = result.quantity < 1 ? 1 : result.quantity;
          for (var i = 0; i < qty; i++) {
            await notifier.claimNeed(
              linkedNeedId,
              communityId: s.communityId,
              note: desc,
              gearId: result.gearId,
            );
            if (!context.mounted) return;
          }
        } else {
          await notifier.addContribution(
            title: name,
            description: desc,
            gearId: result.gearId,
          );
        }
        if (!context.mounted) return;
        final postState = ref.read(requestNeedsProvider(s.requestId));
        _surfaceMutationError(context, postState.mutationError);

        // Escalate the gear-backed claim into a live offer (#2702). Only
        // after a clean mutation, and only when the Lend/Give choice was
        // shown (i.e. no live offer already exists for this contribution).
        if (showLendGive &&
            postState.mutationError == null &&
            result.gearId != null &&
            result.gearId!.isNotEmpty) {
          await _escalateRequestGearOffer(
            context,
            ref,
            s,
            gearId: result.gearId!,
            give: result.give,
            matchTitle: name,
            linkedNeedId: linkedNeedId,
          );
        }
    }
  }

  /// Returns the live gear-backed offer carried by [contributionId] on the
  /// request, or null when none exists (never offered, or the offer is
  /// terminal). Reads the already-loaded request state — no fetch.
  RequestGearOffer? _liveOfferForContribution(
    WidgetRef ref,
    String requestId,
    String contributionId,
  ) {
    final request = ref.read(requestProvider(requestId)).requestDetails;
    if (request == null) return null;
    for (final offer in request.gearOffers) {
      if (offer.contributionId == contributionId &&
          _liveOfferStates.contains(offer.state)) {
        return offer;
      }
    }
    return null;
  }

  /// Escalates a freshly written gear-backed claim on a request into a real
  /// loan or giveaway offer targeted at the requester (#2702): finds the
  /// contribution the mutation just wrote, calls OfferTransfer, refreshes the
  /// request, and confirms with an undoable toast (undo cancels the offer,
  /// which unwinds the claim server-side). A failure leaves the claim as a
  /// plain display-only gear link and surfaces a retry hint — re-confirming
  /// the claim retries the escalation.
  Future<void> _escalateRequestGearOffer(
    BuildContext context,
    WidgetRef ref,
    RequestNeedsScope s, {
    required String gearId,
    required bool give,
    required String matchTitle,
    String? linkedNeedId,
  }) async {
    final contribs = ref.read(requestNeedsProvider(s.requestId)).contributions;
    final lower = matchTitle.toLowerCase();
    RequestContributionResponse? target;
    for (final c in contribs) {
      if (c.contributor.id != s.currentUserId) continue;
      if (!(c.hasGearId() && c.gearId == gearId)) continue;
      final matches = linkedNeedId != null && linkedNeedId.isNotEmpty
          ? (c.hasFromNeedId() && c.fromNeedId == linkedNeedId)
          : c.title.toLowerCase() == lower;
      if (matches) {
        target = c;
        break;
      }
    }
    if (target == null) {
      _log.warning('gear claim written but contribution not found; '
          'skipping offer escalation');
      return;
    }
    if (_liveOfferForContribution(ref, s.requestId, target.id) != null) {
      return;
    }

    final l10n = context.l10n;
    try {
      final transfer =
          await ref.read(transferRepositoryProvider).offerTransfer(
                gearId: gearId,
                transferType: give
                    ? TransferType.TRANSFER_TYPE_GIVEAWAY
                    : TransferType.TRANSFER_TYPE_LOAN,
                recipientUserId: s.requestOwnerId,
                communityId: s.communityId,
                originRequestId: s.requestId,
                contributionId: target.id,
              );
      if (!context.mounted) return;
      await ref.read(requestProvider(s.requestId).notifier).refreshRequest();
      if (!context.mounted) return;
      ToastHelper.showUndo(
        context: context,
        message: give
            ? l10n.needsGearOfferSentGive(transfer.gearName)
            : l10n.needsGearOfferSentLend(transfer.gearName),
        undoLabel: l10n.commonUndo,
        onUndo: () async {
          try {
            await ref
                .read(transferRepositoryProvider)
                .cancelTransfer(transferId: transfer.id);
            await ref
                .read(requestProvider(s.requestId).notifier)
                .refreshRequest();
          } catch (e) {
            _log.warning('failed to cancel gear offer from undo toast: $e');
            if (context.mounted) {
              ToastHelper.showError(context, l10n.needsGearOfferUndoFailed);
            }
          }
        },
      );
    } catch (e) {
      _log.warning('gear offer escalation failed: $e');
      if (!context.mounted) return;
      ToastHelper.showError(context, l10n.needsGearOfferFailed);
    }
  }

  /// Whether an experience contribution carries a live (pre-handoff or on-loan)
  /// gear offer (#2708) — read straight off the contribution's own transfer
  /// fields, unlike requests which read a separate gear_offers array on the
  /// Request message.
  bool _experienceContributionHasLiveOffer(ExperienceContributionResponse c) {
    return c.hasTransferId() &&
        c.transferId.isNotEmpty &&
        c.hasTransferState() &&
        _liveOfferStates.contains(c.transferState);
  }

  /// Escalates a freshly written gear-backed claim on an event into a real
  /// loan or giveaway to the host for the event's duration (#2708): finds the
  /// contribution the mutation just wrote, calls OfferExperienceTransfer,
  /// refreshes the needs list, and confirms with an undoable toast (undo
  /// cancels the offer, which reopens the need slot server-side). A failure
  /// leaves the claim as a plain display-only gear link and surfaces a retry
  /// hint — re-confirming the claim retries the escalation.
  Future<void> _escalateExperienceGearOffer(
    BuildContext context,
    WidgetRef ref,
    ExperienceNeedsScope s, {
    required String gearId,
    required bool give,
    required String matchTitle,
    String? linkedNeedId,
  }) async {
    final contribs =
        ref.read(experienceNeedsProvider(s.experienceId)).contributions;
    final lower = matchTitle.toLowerCase();
    ExperienceContributionResponse? target;
    for (final c in contribs) {
      if (c.contributor.id != s.currentUserId) continue;
      if (!(c.hasGearId() && c.gearId == gearId)) continue;
      final matches = linkedNeedId != null && linkedNeedId.isNotEmpty
          ? (c.hasFromNeedId() && c.fromNeedId == linkedNeedId)
          : c.title.toLowerCase() == lower;
      if (matches) {
        target = c;
        break;
      }
    }
    if (target == null) {
      _log.warning('gear claim written but experience contribution not found; '
          'skipping offer escalation');
      return;
    }
    if (_experienceContributionHasLiveOffer(target)) {
      return;
    }

    final l10n = context.l10n;
    try {
      final transfer = await ref
          .read(transferRepositoryProvider)
          .offerExperienceTransfer(
            gearId: gearId,
            transferType: give
                ? TransferType.TRANSFER_TYPE_GIVEAWAY
                : TransferType.TRANSFER_TYPE_LOAN,
            communityId: s.communityId,
            originExperienceId: s.experienceId,
            contributionId: target.id,
          );
      if (!context.mounted) return;
      await ref
          .read(experienceNeedsProvider(s.experienceId).notifier)
          .refresh();
      if (!context.mounted) return;
      ToastHelper.showUndo(
        context: context,
        message: give
            ? l10n.needsGearOfferSentGive(transfer.gearName)
            : l10n.needsGearOfferSentLend(transfer.gearName),
        undoLabel: l10n.commonUndo,
        onUndo: () async {
          try {
            await ref
                .read(transferRepositoryProvider)
                .cancelTransfer(transferId: transfer.id);
            await ref
                .read(experienceNeedsProvider(s.experienceId).notifier)
                .refresh();
          } catch (e) {
            _log.warning('failed to cancel event gear offer from undo: $e');
            if (context.mounted) {
              ToastHelper.showError(context, l10n.needsGearOfferUndoFailed);
            }
          }
        },
      );
    } catch (e) {
      _log.warning('event gear offer escalation failed: $e');
      if (!context.mounted) return;
      ToastHelper.showError(context, l10n.needsGearOfferFailed);
    }
  }

  /// Removes a need (proposer only).
  Future<void> removeNeed(
    BuildContext context,
    WidgetRef ref,
    String needId,
  ) async {
    switch (scope) {
      case final ExperienceNeedsScope s:
        final notifier = ref.read(
          experienceNeedsProvider(s.experienceId).notifier,
        );
        await notifier.removeNeed(needId);
        if (!context.mounted) return;
        final newState = ref.read(experienceNeedsProvider(s.experienceId));
        if (newState.mutationError != null) {
          ToastHelper.showError(
            context,
            RpcErrorHandler.localize(newState.mutationError!, context.l10n),
          );
        }
      case final RequestNeedsScope s:
        final notifier = ref.read(requestNeedsProvider(s.requestId).notifier);
        await notifier.removeNeed(needId);
        if (!context.mounted) return;
        final newState = ref.read(requestNeedsProvider(s.requestId));
        if (newState.mutationError != null) {
          ToastHelper.showError(
            context,
            RpcErrorHandler.localize(newState.mutationError!, context.l10n),
          );
        }
    }
  }

  /// Opens the contribution sheet. [contribution] must match the scope type.
  Future<void> onContributionTap(
    BuildContext context,
    WidgetRef ref,
    Object contribution,
  ) async {
    switch (scope) {
      case final ExperienceNeedsScope s:
        final c = contribution as ExperienceContributionResponse;
        // All contribution taps — own and not-own — open the same
        // ViewContributionSheet. The modal's title is the item name
        // and the body lists every person bringing it. "I'll bring it
        // too" only appears when the current user isn't already in the
        // list (gated inside the sheet).
        final allContribs = ref
            .read(experienceNeedsProvider(s.experienceId))
            .contributions;
        final lower = c.title.toLowerCase();
        final group = [
          for (final x in allContribs)
            if (x.title.toLowerCase() == lower) x,
        ];
        await ViewContributionSheet.show(
          context,
          title: c.title,
          contributions: group,
          currentUserId: s.currentUserId ?? '',
          experienceOwnerId: s.ownerId,
          onAddMe: s.isTerminal
              ? null
              : () => _addContributionForCurrentUser(
                  context,
                  ref,
                  scope: s,
                  title: c.title,
                ),
          onRemoveMine: s.isTerminal
              ? null
              : () => _removeMyContributions(
                  context,
                  ref,
                  experienceId: s.experienceId,
                  currentUserId: s.currentUserId ?? '',
                  group: group,
                ),
        );
      case RequestNeedsScope():
        // Request scope migrated to the redesigned Volunteer flow (#2175);
        // contribution edits happen inline from the Volunteer sheet.
        await openPlanTabDispatcher(context, ref);
    }
  }

  /// Creates a new contribution for the current user with the given
  /// [title] — used by the ViewContributionSheet's "I'll bring it too"
  /// button so multiple people can bring the same item without
  /// becoming separate-looking entries on the Plan tab.
  ///
  /// Mirrors the RSVP-then-claim path the existing claim flow uses:
  /// AddExperienceContribution requires the caller to be RSVPed,
  /// otherwise the server returns 403. When the current user isn't
  /// already RSVPed we mark them YES first.
  Future<void> _addContributionForCurrentUser(
    BuildContext context,
    WidgetRef ref, {
    required ExperienceNeedsScope scope,
    required String title,
  }) async {
    final notifier = ref.read(
      experienceNeedsProvider(scope.experienceId).notifier,
    );
    if (!scope.isRsvped) {
      await notifier.rsvpWithIntention(
        scope.communityId,
        RSVPIntention.RSVP_INTENTION_YES,
      );
      if (!context.mounted) return;
      final rsvpState = ref.read(experienceNeedsProvider(scope.experienceId));
      if (rsvpState.mutationError != null) {
        ToastHelper.showError(
          context,
          RpcErrorHandler.localize(rsvpState.mutationError!, context.l10n),
        );
        return;
      }
    }
    await notifier.addContribution(title: title);
    if (!context.mounted) return;
    final state = ref.read(experienceNeedsProvider(scope.experienceId));
    if (state.mutationError != null) {
      ToastHelper.showError(
        context,
        RpcErrorHandler.localize(state.mutationError!, context.l10n),
      );
    }
  }

  /// Removes every current-user contribution in [group] — used by the
  /// ViewContributionSheet's "Remove mine" affordance. Linked
  /// contributions (claims) route through `unclaimNeed` so the slot is
  /// returned to the need; freeform offers go through
  /// `removeContribution`.
  Future<void> _removeMyContributions(
    BuildContext context,
    WidgetRef ref, {
    required String experienceId,
    required String currentUserId,
    required List<ExperienceContributionResponse> group,
  }) async {
    final notifier = ref.read(experienceNeedsProvider(experienceId).notifier);
    final mine = [
      for (final c in group)
        if (c.contributor.id == currentUserId) c,
    ];
    for (final c in mine) {
      if (c.hasFromNeedId() && c.fromNeedId.isNotEmpty) {
        await notifier.unclaimNeed(c.id);
      } else {
        await notifier.removeContribution(c.id);
      }
      if (!context.mounted) return;
      final state = ref.read(experienceNeedsProvider(experienceId));
      if (state.mutationError != null) {
        ToastHelper.showError(
          context,
          RpcErrorHandler.localize(state.mutationError!, context.l10n),
        );
        return;
      }
    }
  }

  // ── Redesigned-flow entry points ───────────────────────────────────────────
  //
  // These open the Volunteer / Propose / Manage / Finalized sheets from the
  // redesign (issue #2148). The existing `onNeedTap` / `claimNeed` /
  // `onContributionTap` methods above remain wired to the legacy detail
  // sheets so the plan-tab card behaviour is unchanged until the call
  // sites migrate. See `docs/client/needs.md` for the lifecycle.

  /// State-aware dispatcher for the Plan-tab card tap. Mirrors the
  /// poll-state dispatcher in `experience_content_view.dart`. When the
  /// parent entity is terminal the read-only Archived sheet (NA)
  /// opens; otherwise the tap-to-claim Volunteer sheet opens.
  Future<void> openPlanTabDispatcher(
    BuildContext context,
    WidgetRef ref,
  ) async {
    switch (scope) {
      case final ExperienceNeedsScope s:
        if (s.isTerminal) {
          await openArchivedSheet(context, ref);
        } else {
          await openVolunteerSheet(context, ref);
        }
      case final RequestNeedsScope s:
        if (s.isTerminal) {
          await openArchivedSheet(context, ref);
        } else {
          await openVolunteerSheet(context, ref);
        }
    }
  }

  /// Opens the Volunteer sheet (NV1/NV2/NO1). The sheet itself is a
  /// dumb presentation widget; this method wraps it in a [Consumer] so
  /// the entries re-project from the scope's provider after each
  /// autosave-claim — without the wrapper the modal would render off
  /// the snapshot taken at open time and never reflect the new state.
  Future<void> openVolunteerSheet(
    BuildContext context,
    WidgetRef ref, {
    bool initialEditMode = false,
  }) async {
    await showAccessibleModal<void>(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (_) => Consumer(
        builder: (ctx, modalRef, _) {
          switch (scope) {
            case final ExperienceNeedsScope s:
              final state = modalRef.watch(
                experienceNeedsProvider(s.experienceId),
              );
              final myId = s.currentUserId ?? '';
              final isOrganizer = s.ownerId == myId;
              // Contributions either project onto a live Need's voter
              // stack (when `fromNeedId` matches an entry in
              // `state.needs`) or render as their own row. Orphan
              // need-linked contributions — `fromNeedId` set but the
              // parent Need has since been removed — fall into the
              // "render as own row" bucket too, otherwise we silently
              // drop them and the list looks empty.
              final liveNeedIds = {for (final n in state.needs) n.id};
              final entries = <NeedsVolunteerEntry>[
                for (final n in state.needs)
                  NeedsVolunteerEntry(
                    id: n.id,
                    name: n.name,
                    note: n.hasNote() && n.note.isNotEmpty ? n.note : null,
                    addedByName:
                        (n.proposer.id != myId && n.proposer.name.isNotEmpty)
                        ? n.proposer.name
                        : null,
                    slotsTotal: n.slots,
                    slotsRemaining: n.slotsRemaining,
                    claimedByMe: _countContributionsForNeed(
                      state.contributions,
                      n.id,
                      myId,
                    ),
                    contributors: [
                      for (final c in state.contributions)
                        if (c.hasFromNeedId() && c.fromNeedId == n.id)
                          (
                            user: c.contributor,
                            gearId: c.hasGearId() && c.gearId.isNotEmpty
                                ? c.gearId
                                : null,
                          ),
                    ],
                    canEdit: isOrganizer || n.proposer.id == myId,
                    gearId: _gearIdForNeed(state.contributions, n.id, myId),
                  ),
                for (final c in state.contributions)
                  if (!c.hasFromNeedId() ||
                      c.fromNeedId.isEmpty ||
                      !liveNeedIds.contains(c.fromNeedId))
                    NeedsVolunteerEntry(
                      id: c.id,
                      name:
                          c.hasOriginalNeedName() &&
                              c.originalNeedName.isNotEmpty
                          ? c.originalNeedName
                          : c.title,
                      note: c.hasDescription() && c.description.isNotEmpty
                          ? c.description
                          : (c.hasOriginalNeedNote() &&
                                    c.originalNeedNote.isNotEmpty
                                ? c.originalNeedNote
                                : null),
                      addedByName:
                          (c.contributor.id != myId &&
                              c.contributor.name.isNotEmpty)
                          ? c.contributor.name
                          : null,
                      slotsTotal: 1,
                      slotsRemaining: 0,
                      // Freestanding contributions are always "fully met"
                      // by their single contributor — claimedByMe tracks
                      // whether the current viewer is that contributor.
                      claimedByMe: c.contributor.id == myId ? 1 : 0,
                      contributors: [
                        (
                          user: c.contributor,
                          gearId: c.hasGearId() && c.gearId.isNotEmpty
                              ? c.gearId
                              : null,
                        ),
                      ],
                      isFreestanding: true,
                      canEdit: isOrganizer || c.contributor.id == myId,
                      gearId: c.hasGearId() ? c.gearId : null,
                    ),
              ];
              final myUser = modalRef.watch(authStateProvider).user;
              return NeedsVolunteerSheet(
                scopeKind: scope.kind,
                entries: entries,
                isTerminal: s.isTerminal,
                canClaim: !s.isTerminal,
                onClaim: (id, {note, gearId}) => _claimAutosave(
                  ctx,
                  modalRef,
                  id,
                  note: note,
                  gearId: gearId,
                ),
                onUnclaim: (id) => _unclaimAutosave(ctx, modalRef, id),
                onManageMenu: isOrganizer
                    ? () => openManageMenu(ctx, modalRef)
                    : null,
                onAddOption: s.isTerminal
                    ? null
                    : () => openSingleAddPicker(ctx, modalRef),
                onEditRow: (id) => _openEditRow(ctx, modalRef, id),
                initialEditMode: initialEditMode,
                currentUserId: s.currentUserId,
                currentUser: myUser,
                communityId: s.communityId,
              );
            case final RequestNeedsScope s:
              final state = modalRef.watch(requestNeedsProvider(s.requestId));
              final myId = s.currentUserId ?? '';
              final liveNeedIds = {for (final n in state.needs) n.id};
              final entries = <NeedsVolunteerEntry>[
                for (final n in state.needs)
                  NeedsVolunteerEntry(
                    id: n.id,
                    name: n.name,
                    note: n.hasNote() && n.note.isNotEmpty ? n.note : null,
                    addedByName:
                        (n.proposer.id != myId && n.proposer.name.isNotEmpty)
                        ? n.proposer.name
                        : null,
                    slotsTotal: n.slots,
                    slotsRemaining: n.slotsRemaining,
                    claimedByMe: _countRequestContributionsForNeed(
                      state.contributions,
                      n.id,
                      myId,
                    ),
                    contributors: [
                      for (final c in state.contributions)
                        if (c.hasFromNeedId() && c.fromNeedId == n.id)
                          (
                            user: c.contributor,
                            gearId: c.hasGearId() && c.gearId.isNotEmpty
                                ? c.gearId
                                : null,
                          ),
                    ],
                    canEdit: s.isOwner || n.proposer.id == myId,
                    gearId: _gearIdForRequestNeed(
                      state.contributions,
                      n.id,
                      myId,
                    ),
                    proposerId: n.proposer.id,
                    proposerName: n.proposer.name.isNotEmpty
                        ? n.proposer.name
                        : null,
                  ),
                for (final c in state.contributions)
                  if (!c.hasFromNeedId() ||
                      c.fromNeedId.isEmpty ||
                      !liveNeedIds.contains(c.fromNeedId))
                    NeedsVolunteerEntry(
                      id: c.id,
                      name:
                          c.hasOriginalNeedName() &&
                              c.originalNeedName.isNotEmpty
                          ? c.originalNeedName
                          : c.title,
                      note: c.hasDescription() && c.description.isNotEmpty
                          ? c.description
                          : (c.hasOriginalNeedNote() &&
                                    c.originalNeedNote.isNotEmpty
                                ? c.originalNeedNote
                                : null),
                      addedByName:
                          (c.contributor.id != myId &&
                              c.contributor.name.isNotEmpty)
                          ? c.contributor.name
                          : null,
                      slotsTotal: 1,
                      slotsRemaining: 0,
                      // Freestanding contributions are always "fully met"
                      // by their single contributor — claimedByMe tracks
                      // whether the current viewer is that contributor.
                      claimedByMe: c.contributor.id == myId ? 1 : 0,
                      contributors: [
                        (
                          user: c.contributor,
                          gearId: c.hasGearId() && c.gearId.isNotEmpty
                              ? c.gearId
                              : null,
                        ),
                      ],
                      gearId: c.hasGearId() ? c.gearId : null,
                      isFreestanding: true,
                      canEdit: s.isOwner || c.contributor.id == myId,
                    ),
              ];
              final myUser = modalRef.watch(authStateProvider).user;
              return NeedsVolunteerSheet(
                scopeKind: scope.kind,
                entries: entries,
                isTerminal: s.isTerminal,
                canClaim: !s.isTerminal,
                onClaim: (id, {note, gearId}) => _claimAutosave(
                  ctx,
                  modalRef,
                  id,
                  note: note,
                  gearId: gearId,
                ),
                onUnclaim: (id) => _unclaimAutosave(ctx, modalRef, id),
                onManageMenu: s.isOwner
                    ? () => openManageMenu(ctx, modalRef)
                    : null,
                onAddOption: s.isTerminal
                    ? null
                    : () => openSingleAddPicker(ctx, modalRef),
                onEditRow: (id) => _openEditRow(ctx, modalRef, id),
                initialEditMode: initialEditMode,
                currentUserId: s.currentUserId,
                currentUser: myUser,
                communityId: s.communityId,
              );
          }
        },
      ),
    );
  }

  
  /// Opens the experience batch "add needs" sheet (the organizer posts one or
  /// more things the group needs) and dispatches the batch add. Experience
  /// scope only — a no-op for requests.
  /// Opens the experience "compose needs" sheet — the same modal the event's
  /// "What's Needed" row / "We need" action opens (chips, paste-list,
  /// inline-add, pre-claim). On a request this is the unified
  /// [RequestComposeSheet] — the request analog of [ExperienceComposeSheet].
  Future<void> openAddNeed(BuildContext context, WidgetRef ref) async {
    switch (scope) {
      case final ExperienceNeedsScope s:
        await ExperienceComposeSheet.show(
          context,
          experienceId: s.experienceId,
          communityId: s.communityId,
          needsRsvp: !s.isRsvped,
        );
      case final RequestNeedsScope s:
        await RequestComposeSheet.show(
          context,
          requestId: s.requestId,
          communityId: s.communityId,
        );
    }
  }

  /// Opens the "what are you bringing" composer. On an experience this is the
  /// RSVP-aware contribution composer (with the gear-library link); on a
  /// request it's [RequestContributionComposerSheet] — the request analog, with
  /// the same chip + gear-link + add-to-list flow.
  Future<void> openAddContribution(
    BuildContext context,
    WidgetRef ref, {
    required Color accentColor,
  }) async {
    switch (scope) {
      case final ExperienceNeedsScope s:
        final initialIntention = s.isRsvpedMaybe
            ? RSVPIntention.RSVP_INTENTION_MAYBE
            : (s.isRsvped
                  ? RSVPIntention.RSVP_INTENTION_YES
                  : RSVPIntention.RSVP_INTENTION_UNSPECIFIED);
        await ExperienceRsvpComposerSheet.show(
          context,
          experienceId: s.experienceId,
          accentColor: accentColor,
          initialIntention: initialIntention,
          contributionsOnly: true,
        );
      case final RequestNeedsScope s:
        await RequestContributionComposerSheet.show(
          context,
          requestId: s.requestId,
          accentColor: accentColor,
        );
    }
  }

  /// Reads suggestion strings + category hint from the scope's notifier
  /// state and maps them into [NeedsSuggestion] tiles for the Picker.
  /// The mock's category tints (gear / food / help / personal) are a
  /// follow-up — for now every tile uses the generic `gear` slot.
  (List<NeedsSuggestion>, String?) _proposeSuggestionsForScope(WidgetRef ref) {
    switch (scope) {
      case final ExperienceNeedsScope s:
        final st = ref.read(experienceNeedsProvider(s.experienceId));
        return (
          [
            for (final name in st.suggestions)
              NeedsSuggestion(name: name, category: NeedsCategory.gear),
          ],
          st.categoryHint,
        );
      case final RequestNeedsScope s:
        final st = ref.read(requestNeedsProvider(s.requestId));
        return (
          [
            for (final ask in st.additionalAsks)
              NeedsSuggestion(name: ask, category: NeedsCategory.gear),
          ],
          null,
        );
    }
  }

  /// Opens the organizer Manage menu (NO2 in v2). The chosen action
  /// is dispatched in-place. `cancelNeeds` loops `removeNeed`
  /// one-by-one because there is no bulk-cancel RPC yet; tracked in
  /// #2148.
  ///
  /// v2 dropped the previous `addThing` row (now lives as the inline
  /// AddOptionGhost) and the `markReady` row (the list is alive until
  /// the parent terminates). `editList` flips the Volunteer sheet
  /// into edit mode; `nudgeUnclaimed` dispatches the v2
  /// `NudgeUncoveredNeedClaimers` RPC.
  Future<void> openManageMenu(BuildContext context, WidgetRef ref) async {
    final isTerminal = switch (scope) {
      ExperienceNeedsScope(:final isTerminal) => isTerminal,
      RequestNeedsScope(:final isTerminal) => isTerminal,
    };
    // Cancel is a no-op when there are no needs to remove; hide the row
    // in that case so the requester doesn't tap a silently-dead action.
    final hasAnyNeed = switch (scope) {
      final ExperienceNeedsScope s =>
        ref.read(experienceNeedsProvider(s.experienceId)).needs.isNotEmpty,
      final RequestNeedsScope s =>
        ref.read(requestNeedsProvider(s.requestId)).needs.isNotEmpty,
    };
    _log.info(
      'Manage menu opening (scope=${scope.kind}, '
      'isTerminal=$isTerminal, hasAnyNeed=$hasAnyNeed)',
    );
    final action = await showNeedsManageMenu(
      context,
      scopeKind: scope.kind,
      showEditList: !isTerminal,
      showNudgeUnclaimed: !isTerminal,
      showCancel: !isTerminal && hasAnyNeed,
    );
    _log.info('Manage menu action: $action');
    if (action == null || !context.mounted) return;
    switch (action) {
      case NeedsManageAction.editList:
        // The Volunteer sheet is already open above; "Edit list" flips
        // its `_editMode` flag. Pop the parent Volunteer sheet and
        // re-open it directly into edit mode.
        Navigator.of(context).pop();
        if (!context.mounted) return;
        await openVolunteerSheet(context, ref, initialEditMode: true);
      case NeedsManageAction.nudgeUnclaimed:
        await _nudgeUncovered(context, ref);
      case NeedsManageAction.cancelNeeds:
        await _cancelAllNeeds(context, ref);
    }
  }

  Future<void> _nudgeUncovered(BuildContext context, WidgetRef ref) async {
    int? count;
    switch (scope) {
      case final ExperienceNeedsScope s:
        final notifier = ref.read(
          experienceNeedsProvider(s.experienceId).notifier,
        );
        count = await notifier.nudgeUncoveredNeedClaimers();
      case final RequestNeedsScope s:
        final notifier = ref.read(requestNeedsProvider(s.requestId).notifier);
        count = await notifier.nudgeUncoveredNeedClaimers();
    }
    if (!context.mounted) return;
    if (count != null) {
      ToastHelper.showSuccess(context, context.l10n.needsNudgeToastSent(count));
    } else {
      switch (scope) {
        case final ExperienceNeedsScope s:
          _surfaceMutationError(
            context,
            ref.read(experienceNeedsProvider(s.experienceId)).mutationError,
          );
        case final RequestNeedsScope s:
          _surfaceMutationError(
            context,
            ref.read(requestNeedsProvider(s.requestId)).mutationError,
          );
      }
    }
  }

  /// Opens the Archived sheet (NA in v2). Read-only; uses the same
  /// `state.needs` + `state.contributions` data as Volunteer but
  /// renders rows as read-only [NeedsClaimRow]s with uncovered rows
  /// dimmed. The Save CTA archives the list to the user's records
  /// (v2 ships as a toast-only no-op — persistence is deferred).
  Future<void> openArchivedSheet(BuildContext context, WidgetRef ref) async {
    switch (scope) {
      case final ExperienceNeedsScope s:
        final state = ref.read(experienceNeedsProvider(s.experienceId));
        final myId = s.currentUserId ?? '';
        final liveNeedIds = {for (final n in state.needs) n.id};
        final entries = <NeedsVolunteerEntry>[
          for (final n in state.needs)
            NeedsVolunteerEntry(
              id: n.id,
              name: n.name,
              note: n.hasNote() && n.note.isNotEmpty ? n.note : null,
              addedByName: (n.proposer.id != myId && n.proposer.name.isNotEmpty)
                  ? n.proposer.name
                  : null,
              slotsTotal: n.slots,
              slotsRemaining: n.slotsRemaining,
              claimedByMe: 0,
              contributors: [
                for (final c in state.contributions)
                  if (c.hasFromNeedId() && c.fromNeedId == n.id)
                    (
                      user: c.contributor,
                      gearId: c.hasGearId() && c.gearId.isNotEmpty
                          ? c.gearId
                          : null,
                    ),
              ],
              gearId: _firstGearIdOnNeed(state.contributions, n.id),
            ),
          for (final c in state.contributions)
            if (!c.hasFromNeedId() ||
                c.fromNeedId.isEmpty ||
                !liveNeedIds.contains(c.fromNeedId))
              NeedsVolunteerEntry(
                id: c.id,
                name: c.hasOriginalNeedName() && c.originalNeedName.isNotEmpty
                    ? c.originalNeedName
                    : c.title,
                slotsTotal: 1,
                slotsRemaining: 0,
                claimedByMe: c.contributor.id == myId ? 1 : 0,
                contributors: [
                  (
                    user: c.contributor,
                    gearId: c.hasGearId() && c.gearId.isNotEmpty
                        ? c.gearId
                        : null,
                  ),
                ],
                isFreestanding: true,
                gearId: c.hasGearId() ? c.gearId : null,
              ),
        ];
        await NeedsArchivedSheet.show(
          context,
          scopeKind: scope.kind,
          entries: entries,
          onSave: () {
            Navigator.of(context).pop();
            showNeedsArchivedSavedToast(context);
          },
        );
      case final RequestNeedsScope s:
        final state = ref.read(requestNeedsProvider(s.requestId));
        final myId = s.currentUserId ?? '';
        final liveNeedIds = {for (final n in state.needs) n.id};
        final entries = <NeedsVolunteerEntry>[
          for (final n in state.needs)
            NeedsVolunteerEntry(
              id: n.id,
              name: n.name,
              note: n.hasNote() && n.note.isNotEmpty ? n.note : null,
              addedByName: (n.proposer.id != myId && n.proposer.name.isNotEmpty)
                  ? n.proposer.name
                  : null,
              slotsTotal: n.slots,
              slotsRemaining: n.slotsRemaining,
              claimedByMe: 0,
              contributors: [
                for (final c in state.contributions)
                  if (c.hasFromNeedId() && c.fromNeedId == n.id)
                    (
                      user: c.contributor,
                      gearId: c.hasGearId() && c.gearId.isNotEmpty
                          ? c.gearId
                          : null,
                    ),
              ],
              gearId: _firstGearIdOnRequestNeed(state.contributions, n.id),
            ),
          for (final c in state.contributions)
            if (!c.hasFromNeedId() ||
                c.fromNeedId.isEmpty ||
                !liveNeedIds.contains(c.fromNeedId))
              NeedsVolunteerEntry(
                id: c.id,
                name: c.hasOriginalNeedName() && c.originalNeedName.isNotEmpty
                    ? c.originalNeedName
                    : c.title,
                slotsTotal: 1,
                slotsRemaining: 0,
                claimedByMe: c.contributor.id == myId ? 1 : 0,
                contributors: [
                  (
                    user: c.contributor,
                    gearId: c.hasGearId() && c.gearId.isNotEmpty
                        ? c.gearId
                        : null,
                  ),
                ],
                isFreestanding: true,
                gearId: c.hasGearId() ? c.gearId : null,
              ),
        ];
        await NeedsArchivedSheet.show(
          context,
          scopeKind: scope.kind,
          entries: entries,
          onSave: () {
            Navigator.of(context).pop();
            showNeedsArchivedSavedToast(context);
          },
        );
    }
  }

  // ── Internal helpers for the redesigned flow ───────────────────────────────

  /// Opens the NK1 single-item Picker ("What are we adding?") from the
  /// bottom-of-list AddOptionGhost row. On confirm dispatches `addNeed`
  /// through the scope's notifier — this is the single-add path, not
  /// the bulk Propose sheet (which is reached from "Break it down").
  Future<void> openSingleAddPicker(BuildContext context, WidgetRef ref) async {
    final (rawSuggestions, _) = _proposeSuggestionsForScope(ref);
    final takenNames = _takenNamesForScope(ref);
    // Drop anything already on the list — the picker should never show
    // a suggestion that's a no-op to tap. Then cap at six so the grid
    // stays compact (~3 rows × 2 cols) per the reference design.
    const maxSuggestions = 6;
    final suggestionNames = [
      for (final s in rawSuggestions)
        if (!takenNames.contains(s.name.trim().toLowerCase())) s.name,
    ].take(maxSuggestions).toList();
    final result = await NeedsAddToListSheet.show(
      context,
      scopeKind: scope.kind,
      suggestions: suggestionNames,
      takenNames: takenNames,
      communityId: _communityIdForScope(),
    );
    if (result == null || !context.mounted) return;
    switch (scope) {
      case final ExperienceNeedsScope s:
        final notifier = ref.read(
          experienceNeedsProvider(s.experienceId).notifier,
        );
        final newNeedId = await notifier.addNeed(
          name: result.name,
          note: result.note,
        );
        if (!context.mounted) return;
        final st = ref.read(experienceNeedsProvider(s.experienceId));
        if (st.mutationError != null || newNeedId == null) {
          _surfaceMutationError(context, st.mutationError);
          return;
        }
        if (result.preClaim) {
          // Pre-claim needs an RSVP first so the requester is on the
          // attendee list before the claim attaches.
          if (!s.isRsvped) {
            await notifier.rsvpWithIntention(
              s.communityId,
              RSVPIntention.RSVP_INTENTION_YES,
            );
            if (!context.mounted) return;
            final after = ref.read(experienceNeedsProvider(s.experienceId));
            if (after.mutationError != null) {
              _surfaceMutationError(context, after.mutationError);
              return;
            }
          }
          await notifier.claimNeed(newNeedId, gearId: result.gearId);
          if (!context.mounted) return;
          _surfaceMutationError(
            context,
            ref.read(experienceNeedsProvider(s.experienceId)).mutationError,
          );
        }
      case final RequestNeedsScope s:
        final notifier = ref.read(requestNeedsProvider(s.requestId).notifier);
        final newNeedId = await notifier.addNeed(
          name: result.name,
          note: result.note,
        );
        if (!context.mounted) return;
        final st = ref.read(requestNeedsProvider(s.requestId));
        if (st.mutationError != null || newNeedId == null) {
          _surfaceMutationError(context, st.mutationError);
          return;
        }
        if (result.preClaim) {
          await notifier.claimNeed(
            newNeedId,
            communityId: s.communityId,
            gearId: result.gearId,
          );
          if (!context.mounted) return;
          _surfaceMutationError(
            context,
            ref.read(requestNeedsProvider(s.requestId)).mutationError,
          );
        }
    }
  }

  /// Returns the case-insensitive set of need names already on the list.
  /// Used by the NK1 picker to dim suggestion tiles whose name is already
  /// in use ("on the list" / sage outline).
  Set<String> _takenNamesForScope(WidgetRef ref) {
    switch (scope) {
      case final ExperienceNeedsScope s:
        return {
          for (final n
              in ref.read(experienceNeedsProvider(s.experienceId)).needs)
            n.name.toLowerCase(),
        };
      case final RequestNeedsScope s:
        return {
          for (final n in ref.read(requestNeedsProvider(s.requestId)).needs)
            n.name.toLowerCase(),
        };
    }
  }

  /// Opens the NK4 edit picker for an existing Need row. Looks up the
  /// underlying Need in the current state, surfaces the picker with the
  /// values pre-populated, and on Save dispatches `updateNeed(...)` or
  /// on Remove dispatches `removeNeed(...)` through the scope's
  /// notifier. Freestanding contribution rows currently route through
  /// the legacy edit flow — that lives in `onContributionTap`.
  Future<void> _openEditRow(
    BuildContext context,
    WidgetRef ref,
    String entryId,
  ) async {
    switch (scope) {
      case final ExperienceNeedsScope s:
        final state = ref.read(experienceNeedsProvider(s.experienceId));
        final need = state.needs.where((n) => n.id == entryId).firstOrNull;
        if (need == null) return;
        final claimedSummary = _summarizeExistingClaims([
          for (final c in state.contributions)
            if (c.hasFromNeedId() && c.fromNeedId == need.id) c,
        ]);
        final result = await NeedsPickerModal.showEdit(
          context,
          scopeKind: scope.kind,
          name: need.name,
          slots: need.slots,
          note: need.hasNote() && need.note.isNotEmpty ? need.note : null,
          existingClaimsSummary: claimedSummary,
          communityId: _communityIdForScope(),
          initialGearId: _gearIdForNeed(
            state.contributions,
            need.id,
            s.currentUserId ?? '',
          ),
        );
        if (result == null || !context.mounted) return;
        final notifier = ref.read(
          experienceNeedsProvider(s.experienceId).notifier,
        );
        if (result.removed) {
          await notifier.removeNeed(need.id);
        } else {
          await notifier.updateNeed(
            need.id,
            name: result.name,
            note: result.note ?? '',
            slots: result.slots,
          );
        }
        if (!context.mounted) return;
        _surfaceMutationError(
          context,
          ref.read(experienceNeedsProvider(s.experienceId)).mutationError,
        );
      case final RequestNeedsScope s:
        final state = ref.read(requestNeedsProvider(s.requestId));
        final need = state.needs.where((n) => n.id == entryId).firstOrNull;
        if (need == null) return;
        final claimedSummary = _summarizeRequestExistingClaims([
          for (final c in state.contributions)
            if (c.hasFromNeedId() && c.fromNeedId == need.id) c,
        ]);
        final result = await NeedsPickerModal.showEdit(
          context,
          scopeKind: scope.kind,
          name: need.name,
          slots: need.slots,
          note: need.hasNote() && need.note.isNotEmpty ? need.note : null,
          existingClaimsSummary: claimedSummary,
          communityId: _communityIdForScope(),
          initialGearId: _gearIdForRequestNeed(
            state.contributions,
            need.id,
            s.currentUserId ?? '',
          ),
        );
        if (result == null || !context.mounted) return;
        final notifier = ref.read(requestNeedsProvider(s.requestId).notifier);
        if (result.removed) {
          await notifier.removeNeed(need.id);
        } else {
          await notifier.updateNeed(
            need.id,
            name: result.name,
            note: result.note ?? '',
            slots: result.slots,
          );
        }
        if (!context.mounted) return;
        _surfaceMutationError(
          context,
          ref.read(requestNeedsProvider(s.requestId)).mutationError,
        );
    }
  }

  String? _summarizeExistingClaims(
    List<ExperienceContributionResponse> claims,
  ) {
    if (claims.isEmpty) return null;
    final byContributor = <String, int>{};
    for (final c in claims) {
      final name = c.contributor.name.isNotEmpty
          ? c.contributor.name
          : c.contributor.id;
      byContributor[name] = (byContributor[name] ?? 0) + 1;
    }
    return byContributor.entries
        .map(
          (e) =>
              e.value > 1 ? '${e.key} brings ${e.value}' : '${e.key} brings 1',
        )
        .join(' · ');
  }

  String? _summarizeRequestExistingClaims(
    List<RequestContributionResponse> claims,
  ) {
    if (claims.isEmpty) return null;
    final byContributor = <String, int>{};
    for (final c in claims) {
      final name = c.contributor.name.isNotEmpty
          ? c.contributor.name
          : c.contributor.id;
      byContributor[name] = (byContributor[name] ?? 0) + 1;
    }
    return byContributor.entries
        .map(
          (e) =>
              e.value > 1 ? '${e.key} brings ${e.value}' : '${e.key} brings 1',
        )
        .join(' · ');
  }

  Future<void> _claimAutosave(
    BuildContext context,
    WidgetRef ref,
    String needId, {
    String? note,
    String? gearId,
  }) async {
    switch (scope) {
      case final ExperienceNeedsScope s:
        final notifier = ref.read(
          experienceNeedsProvider(s.experienceId).notifier,
        );
        if (!s.isRsvped) {
          await notifier.rsvpWithIntention(
            s.communityId,
            RSVPIntention.RSVP_INTENTION_YES,
          );
          if (!context.mounted) return;
          final st = ref.read(experienceNeedsProvider(s.experienceId));
          if (st.mutationError != null) {
            _surfaceMutationError(context, st.mutationError);
            return;
          }
        }
        await notifier.claimNeed(needId, note: note, gearId: gearId);
        if (!context.mounted) return;
        _surfaceMutationError(
          context,
          ref.read(experienceNeedsProvider(s.experienceId)).mutationError,
        );
      case final RequestNeedsScope s:
        final notifier = ref.read(requestNeedsProvider(s.requestId).notifier);
        await notifier.claimNeed(
          needId,
          communityId: s.communityId,
          note: note,
          gearId: gearId,
        );
        if (!context.mounted) return;
        _surfaceMutationError(
          context,
          ref.read(requestNeedsProvider(s.requestId)).mutationError,
        );
    }
  }

  Future<void> _unclaimAutosave(
    BuildContext context,
    WidgetRef ref,
    String needId,
  ) async {
    switch (scope) {
      case final ExperienceNeedsScope s:
        final myId = s.currentUserId ?? '';
        final state = ref.read(experienceNeedsProvider(s.experienceId));
        final mine = _findOwnContributionForNeed(
          state.contributions,
          needId,
          myId,
        );
        if (mine == null) return;
        final notifier = ref.read(
          experienceNeedsProvider(s.experienceId).notifier,
        );
        await notifier.unclaimNeed(mine);
        if (!context.mounted) return;
        _surfaceMutationError(
          context,
          ref.read(experienceNeedsProvider(s.experienceId)).mutationError,
        );
      case final RequestNeedsScope s:
        final myId = s.currentUserId ?? '';
        final state = ref.read(requestNeedsProvider(s.requestId));
        final mine = _findOwnRequestContributionForNeed(
          state.contributions,
          needId,
          myId,
        );
        if (mine == null) return;
        final notifier = ref.read(requestNeedsProvider(s.requestId).notifier);
        await notifier.unclaimNeed(mine);
        if (!context.mounted) return;
        _surfaceMutationError(
          context,
          ref.read(requestNeedsProvider(s.requestId)).mutationError,
        );
    }
  }

  Future<void> _cancelAllNeeds(BuildContext context, WidgetRef ref) async {
    switch (scope) {
      case final ExperienceNeedsScope s:
        final notifier = ref.read(
          experienceNeedsProvider(s.experienceId).notifier,
        );
        final ids = [
          for (final n
              in ref.read(experienceNeedsProvider(s.experienceId)).needs)
            n.id,
        ];
        _log.info(
          'Cancel-all dispatching for Experience '
          '(experience_id=${s.experienceId}, need_count=${ids.length})',
        );
        if (ids.isEmpty) {
          ToastHelper.showError(
            context,
            context.l10n.needsManageNothingToCancel,
          );
          return;
        }
        for (final id in ids) {
          await notifier.removeNeed(id);
          if (!context.mounted) return;
          final st = ref.read(experienceNeedsProvider(s.experienceId));
          if (st.mutationError != null) {
            _surfaceMutationError(context, st.mutationError);
            return;
          }
        }
        // Pop the Volunteer sheet so the host lands back in the same
        // compose flow they'd see for a brand-new TBD experience —
        // chips, paste-list, inline-add — instead of the empty
        // "nothing requested yet" state.
        Navigator.of(context).pop();
        if (!context.mounted) return;
        await ExperienceComposeSheet.show(
          context,
          experienceId: s.experienceId,
          communityId: s.communityId,
          needsRsvp: !s.isRsvped,
        );
      case final RequestNeedsScope s:
        final notifier = ref.read(requestNeedsProvider(s.requestId).notifier);
        final ids = [
          for (final n in ref.read(requestNeedsProvider(s.requestId)).needs)
            n.id,
        ];
        _log.info(
          'Cancel-all dispatching for Request '
          '(target_request_id=${s.requestId}, need_count=${ids.length})',
        );
        if (ids.isEmpty) {
          ToastHelper.showError(
            context,
            context.l10n.needsManageNothingToCancel,
          );
          return;
        }
        for (final id in ids) {
          await notifier.removeNeed(id);
          if (!context.mounted) return;
          final st = ref.read(requestNeedsProvider(s.requestId));
          if (st.mutationError != null) {
            _surfaceMutationError(context, st.mutationError);
            return;
          }
        }
        // Pop the Volunteer sheet so the requester lands back in the same
        // compose flow they'd see for a brand-new TBD request —
        // chips, paste-list, inline-add — instead of the empty
        // "nothing requested yet" state.
        Navigator.of(context).pop();
        if (!context.mounted) return;
        await RequestComposeSheet.show(
          context,
          requestId: s.requestId,
          communityId: s.communityId,
        );
    }
  }

  /// Takes the error rather than the state it came off: the callers below
  /// hold half a dozen unrelated notifier-state types that share only a
  /// `mutationError` field, and reading it here would mean typing the
  /// parameter `dynamic` (#2794).
  void _surfaceMutationError(BuildContext context, UserError? err) {
    if (err == null) return;
    ToastHelper.showError(context, RpcErrorHandler.localize(err, context.l10n));
  }

  int _countContributionsForNeed(
    List<ExperienceContributionResponse> contributions,
    String needId,
    String userId,
  ) {
    var count = 0;
    for (final c in contributions) {
      if (c.hasFromNeedId() &&
          c.fromNeedId == needId &&
          c.contributor.id == userId) {
        count++;
      }
    }
    return count;
  }

  int _countRequestContributionsForNeed(
    List<RequestContributionResponse> contributions,
    String needId,
    String userId,
  ) {
    var count = 0;
    for (final c in contributions) {
      if (c.hasFromNeedId() &&
          c.fromNeedId == needId &&
          c.contributor.id == userId) {
        count++;
      }
    }
    return count;
  }

  /// Returns the gear link on the current user's contribution to the
  /// given Experience-scope Need, or null when the user has not
  /// linked any gear. With one-link-per-contribution semantics there
  /// is at most one match per (need, user) pair.
  String? _gearIdForNeed(
    List<ExperienceContributionResponse> contributions,
    String needId,
    String userId,
  ) {
    for (final c in contributions) {
      if (c.hasFromNeedId() &&
          c.fromNeedId == needId &&
          c.contributor.id == userId &&
          c.hasGearId() &&
          c.gearId.isNotEmpty) {
        return c.gearId;
      }
    }
    return null;
  }

  /// Request-scope counterpart of [_gearIdForNeed].
  String? _gearIdForRequestNeed(
    List<RequestContributionResponse> contributions,
    String needId,
    String userId,
  ) {
    for (final c in contributions) {
      if (c.hasFromNeedId() &&
          c.fromNeedId == needId &&
          c.contributor.id == userId &&
          c.hasGearId() &&
          c.gearId.isNotEmpty) {
        return c.gearId;
      }
    }
    return null;
  }

  /// Returns the community ID associated with the current scope.
  /// Forwarded to the gear-link picker so its search runs in the
  /// correct community context. Empty string when the scope has no
  /// community (shouldn't happen for Active scopes — both Experience
  /// and Request always carry a community).
  String _communityIdForScope() {
    switch (scope) {
      case final ExperienceNeedsScope s:
        return s.communityId;
      case final RequestNeedsScope s:
        return s.communityId;
    }
  }

  /// Returns the first non-empty gear link across all Experience-scope
  /// contributions on the given Need, regardless of contributor. Used
  /// by the read-only Archived sheet, where the row is a roll-up of
  /// all claims (we surface the gear once if anyone linked one).
  String? _firstGearIdOnNeed(
    List<ExperienceContributionResponse> contributions,
    String needId,
  ) {
    for (final c in contributions) {
      if (c.hasFromNeedId() &&
          c.fromNeedId == needId &&
          c.hasGearId() &&
          c.gearId.isNotEmpty) {
        return c.gearId;
      }
    }
    return null;
  }

  /// Request-scope counterpart of [_firstGearIdOnNeed].
  String? _firstGearIdOnRequestNeed(
    List<RequestContributionResponse> contributions,
    String needId,
  ) {
    for (final c in contributions) {
      if (c.hasFromNeedId() &&
          c.fromNeedId == needId &&
          c.hasGearId() &&
          c.gearId.isNotEmpty) {
        return c.gearId;
      }
    }
    return null;
  }

  String? _findOwnContributionForNeed(
    List<ExperienceContributionResponse> contributions,
    String needId,
    String userId,
  ) {
    for (final c in contributions) {
      if (c.hasFromNeedId() &&
          c.fromNeedId == needId &&
          c.contributor.id == userId) {
        return c.id;
      }
    }
    return null;
  }

  String? _findOwnRequestContributionForNeed(
    List<RequestContributionResponse> contributions,
    String needId,
    String userId,
  ) {
    for (final c in contributions) {
      if (c.hasFromNeedId() &&
          c.fromNeedId == needId &&
          c.contributor.id == userId) {
        return c.id;
      }
    }
    return null;
  }
}
