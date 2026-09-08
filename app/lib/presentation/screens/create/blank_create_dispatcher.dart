import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/config/feature_flags.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/utils/navigation_helpers.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/data/gen/ripls/api/unified_create_service.pb.dart'
    show DetectedContentType;
import 'package:ripls/data/gen/ripls/api/workshop_service.pb.dart'
    show GenerateWorkshopDraftResponse;
import 'package:ripls/presentation/screens/create/unified_create_modal.dart';
import 'package:ripls/presentation/screens/experience/experience_creation_modal.dart';
import 'package:ripls/presentation/screens/experience/experience_preview_modal.dart';
import 'package:ripls/presentation/screens/request/request_creation_modal.dart';
import 'package:ripls/presentation/viewmodels/gen_experience_view_model.dart'
    show genExperienceProvider;
import 'package:ripls/presentation/viewmodels/unified_create_save_actions.dart'
    show UnifiedSaveResult;
import 'package:ripls/presentation/widgets/sharing/item_share_sheet.dart'
    show ItemShareSheet, ShareableItemType;
import 'package:ripls/services/post_creation_service.dart';
import 'package:ripls/services/providers/workshop_providers.dart'
    show workshopRepositoryProvider;

/// Maps the unified-create detected type to the matching share-sheet item type.
/// Falls back to gear for any unexpected value (the only types the unified
/// flow produces are event / gear / request).
ShareableItemType shareableItemTypeForContent(DetectedContentType type) {
  switch (type) {
    case DetectedContentType.DETECTED_CONTENT_TYPE_EVENT:
      return ShareableItemType.experience;
    case DetectedContentType.DETECTED_CONTENT_TYPE_REQUEST:
      return ShareableItemType.request;
    case DetectedContentType.DETECTED_CONTENT_TYPE_GEAR:
    default:
      return ShareableItemType.gear;
  }
}

/// Runs the post-save flow shared by every primary create entry point:
/// refresh the feed, land the creator on the just-created entity's OWN
/// screen, auto-open the Share sheet over it, and confirm the creation with
/// a toast once the sheet is dismissed (CREATE-1, #2492; #2724).
///
/// Ordering matters: the entity screen is pushed BEFORE the Share sheet so
/// the sheet floats over the thing being shared — presenting it over the
/// feed put it over whatever the feed happened to show (a different item's
/// photo, a "Search nearby" void), which reads as sharing the wrong thing.
/// The toast fires after the sheet resolves because a snackbar shown while
/// a bottom sheet is up renders behind it (#2724); this way dismissing
/// the sheet lands on the new entity with its "Event created" confirmation.
/// All steps are guarded on the modal having popped (a non-null [result])
/// and the surrounding context still being mounted.
Future<void> runPostSaveAndShare(
  BuildContext context,
  WidgetRef ref,
  UnifiedSaveResult result,
) async {
  await ref.read(postCreationServiceProvider).handlePostCreation();
  if (!context.mounted) return;

  final shareType = shareableItemTypeForContent(result.type);
  final itemTypeName = switch (shareType) {
    ShareableItemType.experience => 'experience',
    ShareableItemType.gear => 'gear',
    ShareableItemType.request => 'request',
  };
  // pushToItemScreen's future completes when the user eventually POPS the
  // entity screen — deliberately not awaited before presenting the sheet.
  // The route mounts instantly (zero-duration transition), so the sheet
  // opens over the entity view.
  unawaited(NavigationHelpers.pushToItemScreen(
    context: context,
    itemId: result.entityId,
    itemType: itemTypeName,
  ));

  await ItemShareSheet.show(
    context,
    itemType: shareType,
    itemId: result.entityId,
    itemName: result.itemName,
  );
  if (!context.mounted) return;

  ToastHelper.showSuccess(context, _creationToast(context, shareType));
  // The share added the item to its communities only just now (after
  // handlePostCreation already refreshed the feed), so refetch feed + home
  // to surface it without a manual pull-to-refresh.
  ref.read(postCreationServiceProvider).refreshAfterShare();
}

/// Localized creation confirmation per item type ("Event created" /
/// "Request posted" / "Item listed").
String _creationToast(BuildContext context, ShareableItemType type) {
  final l10n = context.l10n;
  return switch (type) {
    ShareableItemType.experience => l10n.createToastEventCreated,
    ShareableItemType.request => l10n.createToastRequestPosted,
    ShareableItemType.gear => l10n.createToastItemListed,
  };
}

/// openBlankCreate opens the unified create modal when the feature flag is on,
/// or the legacy experience-creation modal when the flag is off. On a
/// successful save it runs the post-creation flow and auto-opens the Share
/// sheet.
///
/// Use this at any "blank create" affordance (events, general create CTAs)
/// where no pre-filled draft is available. For gear-specific CTAs use
/// [openBlankCreateGear]; for request-specific CTAs use [openBlankCreateRequest].
Future<void> openBlankCreate(BuildContext context, WidgetRef ref) async {
  if (ref.read(unifiedCreateEnabledProvider)) {
    final result = await UnifiedCreateModal.show(context, ref);
    if (result != null && context.mounted) {
      await runPostSaveAndShare(context, ref, result);
    }
  } else {
    await ExperienceCreationModal.show(context);
  }
}

/// openSuggestedCreate opens the create flow **pre-filled** from a calendar
/// open-day suggestion: it seeds [prompt] and starts server generation so the
/// user lands on the streamed preview to accept or tweak, rather than an empty
/// composer. Falls back to a blank create when unified create is disabled.
///
/// Returns true when the user actually saved the generated item, so the caller
/// can refresh and surface the new item. On a successful save it also runs the
/// post-creation flow and auto-opens the Share sheet.
Future<bool> openSuggestedCreate(
  BuildContext context,
  WidgetRef ref,
  String prompt,
  int startUnixSec,
) async {
  if (ref.read(unifiedCreateEnabledProvider)) {
    final result = await UnifiedCreateModal.show(
      context,
      ref,
      initialPrompt: prompt,
      initialStartUnixSec: startUnixSec,
    );
    if (result == null) return false;
    if (context.mounted) {
      await runPostSaveAndShare(context, ref, result);
    }
    return true;
  }
  await openBlankCreate(context, ref);
  return false;
}

/// openBlankCreateGear opens the unified create modal for gear capture.
/// Used by every "share gear" entry point in the app. On a successful save it
/// runs the post-creation flow and auto-opens the Share sheet.
///
/// Seeds the gear type, so the composer opens on the camera tab and the
/// classifier is skipped — the caller already knows this is an item (#2936).
Future<void> openBlankCreateGear(BuildContext context, WidgetRef ref) async {
  final result = await UnifiedCreateModal.show(
    context,
    ref,
    targetType: DetectedContentType.DETECTED_CONTENT_TYPE_GEAR,
  );
  if (result != null && context.mounted) {
    await runPostSaveAndShare(context, ref, result);
  }
}

/// openBlankCreateExperience opens the unified create modal for planning an
/// event. Seeds the experience type, so the composer opens on the text tab
/// with the event hint — an event is not a thing you can photograph (#2936).
Future<void> openBlankCreateExperience(
  BuildContext context,
  WidgetRef ref,
) async {
  if (!ref.read(unifiedCreateEnabledProvider)) {
    await ExperienceCreationModal.show(context);
    return;
  }
  final result = await UnifiedCreateModal.show(
    context,
    ref,
    targetType: DetectedContentType.DETECTED_CONTENT_TYPE_EVENT,
  );
  if (result != null && context.mounted) {
    await runPostSaveAndShare(context, ref, result);
  }
}

/// openBlankCreateRequest opens the request creation flow and returns a
/// non-null value on success, null on cancellation.
///
/// When the flag is on, returns a non-null sentinel string
/// (`'unified-create:shared'`) if the user saved via the unified modal so
/// callers that branch on null vs. non-null work identically under both flag
/// states; it also runs the post-creation flow and auto-opens the Share sheet.
/// When the flag is off, returns the requestId string from the legacy modal
/// directly.
Future<String?> openBlankCreateRequest(
  BuildContext context,
  WidgetRef ref,
) async {
  if (ref.read(unifiedCreateEnabledProvider)) {
    final result = await UnifiedCreateModal.show(
      context,
      ref,
      targetType: DetectedContentType.DETECTED_CONTENT_TYPE_REQUEST,
    );
    if (result == null) return null;
    if (context.mounted) {
      await runPostSaveAndShare(context, ref, result);
    }
    return 'unified-create:shared';
  }
  return RequestCreationModal.show(context);
}

/// openRepeatDraft opens the experience preview **pre-filled from a prior
/// instance** of [experienceId]: `GenerateWorkshopDraft` returns that event's
/// name, description, location, media, the crew who actually came, and the
/// next weekly slot, and the host edits-and-publishes from the preview rather
/// than retyping an event they already ran.
///
/// The input and generating steps are skipped deliberately — there is nothing
/// to AI-generate when the server has already produced the draft.
///
/// Falls back to [openBlankCreate] whenever the draft can't be used, so no
/// entry point dead-ends: an empty [experienceId], an RPC/storage failure, or
/// a draft with no name.
///
/// Every caller must pass an **experience** id. The fallback is for a draft
/// that can't be built, not a licence to pass an id of another kind: the
/// retired `propose_share` lever passed a gear id, which missed the
/// experience lookup on every single tap and burned a round trip to reach
/// the same blank modal it would have opened anyway (#2892).
///
/// Shared by every "do this again" affordance: the completed event's own
/// "Schedule the next one" chip and the workshop-surface nudge CTAs
/// (`schedule_repeat`, `revive_experience`).
///
/// **The caller may vanish mid-flight.** The Home inbox dismisses its nudge
/// optimistically the moment the CTA is tapped, which unmounts the very widget
/// that owns the [WidgetRef] and [BuildContext] passed in here — and Riverpod
/// *throws* on a ref used after its widget is gone, so a read on the far side
/// of the `generateDraft` round trip is a crash, not a stale value. Everything
/// the success path needs — the repository, the viewmodel to seed, the
/// navigator to present on — is therefore resolved BEFORE the first await.
///
/// The blank-create fallbacks still need a live host (they reach for several
/// more providers), so they are guarded on the caller still being mounted.
/// That is the right split in practice: the surface that dismisses itself is
/// the one whose draft always resolves, while the completed event's own chip —
/// where a fallback actually matters — stays on screen throughout.
Future<void> openRepeatDraft(
  BuildContext context,
  WidgetRef ref,
  String experienceId,
) async {
  if (experienceId.isEmpty) {
    await openBlankCreate(context, ref);
    return;
  }
  final navigator = Navigator.of(context, rootNavigator: true);
  final repository = ref.read(workshopRepositoryProvider);
  final genNotifier = ref.read(genExperienceProvider.notifier);

  GenerateWorkshopDraftResponse draft;
  try {
    draft = await repository.generateDraft(experienceId: experienceId);
  } catch (_) {
    if (!context.mounted) return;
    await openBlankCreate(context, ref);
    return;
  }
  if (draft.name.isEmpty) {
    if (!context.mounted) return;
    await openBlankCreate(context, ref);
    return;
  }
  if (!navigator.mounted) return;

  genNotifier.seedFromWorkshopDraft(
    name: draft.name,
    description: draft.description,
    timeUnixSec: draft.hasTimeUnixSec() ? draft.timeUnixSec.toInt() : null,
    locationId: draft.hasLocationId() ? draft.locationId : null,
    mediaIds: draft.mediaIds.toList(),
    participantIds: draft.participantIds.toList(),
  );
  await showDialog<Object?>(
    context: navigator.context,
    barrierDismissible: false,
    builder: (_) => const ExperiencePreviewModal(),
  );
}
