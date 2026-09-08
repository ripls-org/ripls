import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/data/gen/ripls/api/media_service.pb.dart' show Attribution;
import 'package:ripls/presentation/screens/create/blank_create_dispatcher.dart';
import 'package:ripls/presentation/widgets/nudge/nudge_card_variants.dart';
import 'package:ripls/presentation/widgets/nudge/nudge_presentation.dart';
import 'package:ripls/services/feed_service.dart' show NudgePayload;
import 'package:ripls/services/providers.dart';

/// NudgeContentView renders a nudge card.
///
/// It resolves the nudge's background media (if any) via [mediaUrlProvider],
/// then dispatches to the appropriate [NudgeCardVariants] builder based on
/// [NudgePayload.nudgeVariant]. CTA taps open the matching creation flow
/// without consuming the nudge — the card remains visible behind the modal.
///
/// [onCtaTap] is an optional hook fired alongside the CTA navigation, used by
/// surfaces that consume the nudge on tap (e.g. the inbox optimistically
/// dismisses it). The feed leaves it null and relies on seen-expiry.
///
/// [presentation] tells the card whether it owns the screen. Hosts that bound
/// it to a small box pass [NudgePresentation.embedded]; the feed keeps the
/// full-screen default.
class NudgeContentView extends ConsumerWidget {
  final NudgePayload nudge;
  final VoidCallback? onCtaTap;
  final NudgePresentation presentation;

  const NudgeContentView({
    super.key,
    required this.nudge,
    this.onCtaTap,
    this.presentation = NudgePresentation.fullScreen,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final mediaId = nudge.mediaIds.firstOrNull;
    final hasMedia = mediaId != null && mediaId.isNotEmpty;

    final mediaUrl =
        hasMedia ? ref.watch(mediaUrlProvider(mediaId)).asData?.value : null;
    final attribution =
        hasMedia
            ? ref.watch(mediaAttributionProvider(mediaId)).asData?.value
            : null;

    return _buildVariant(context, ref, mediaUrl, mediaId, attribution);
  }

  Widget _buildVariant(
    BuildContext context,
    WidgetRef ref,
    String? mediaUrl,
    String? mediaId,
    Attribution? attribution,
  ) {
    void onCtaTap() {
      _openAction(context, ref, nudge.ctaAction);
      this.onCtaTap?.call();
    }

    switch (nudge.nudgeVariant) {
      case 2:
        return NudgeCardVariants.buildVariant2(
          context: context,
          nudge: nudge,
          mediaUrl: mediaUrl,
          mediaId: mediaId,
          onCtaTap: onCtaTap,
          attribution: attribution,
          presentation: presentation,
        );
      case 3:
        return NudgeCardVariants.buildVariant3(
          context: context,
          nudge: nudge,
          mediaUrl: mediaUrl,
          mediaId: mediaId,
          onCtaTap: onCtaTap,
          attribution: attribution,
          presentation: presentation,
        );
      default:
        return NudgeCardVariants.buildVariant1(
          context: context,
          nudge: nudge,
          mediaUrl: mediaUrl,
          mediaId: mediaId,
          onCtaTap: onCtaTap,
          attribution: attribution,
          presentation: presentation,
        );
    }
  }

  /// _openAction navigates to the creation flow matching [ctaAction].
  ///
  /// Workshop-surface CTAs (`schedule_repeat`, `revive_experience`,
  /// `seed_subhost`) reference an existing entity via
  /// [NudgePayload.contextId] and route into that entity's detail screen
  /// rather than opening an empty creation modal. Feed-surface CTAs
  /// continue to dispatch as before.
  ///
  /// Runs fire-and-forget (no await) — the nudge card stays visible behind
  /// the opened modal.
  void _openAction(BuildContext context, WidgetRef ref, String ctaAction) {
    switch (ctaAction) {
      case 'list_item':
        openBlankCreateGear(context, ref);
      case 'ask_for_help':
        openBlankCreateRequest(context, ref);

      // Workshop-surface CTAs that reference an existing experience. The
      // server pre-fills a draft from the prior instance and the host
      // lands directly in the preview modal to edit-and-publish. Shared
      // with the completed event's own "Schedule the next one" row.
      //
      // `propose_share` is deliberately absent (#2892): it pointed at gear,
      // so the draft lookup always missed and the host landed in a blank
      // modal. Any stale row still carrying it falls to `default:` — the
      // same blank modal, without the failing round trip first.
      case 'schedule_repeat':
      case 'revive_experience':
        unawaited(openRepeatDraft(
          context,
          ref,
          nudge.hasContextId() ? nudge.contextId : '',
        ));

      // Seed-the-catalyst lands on the catalyst-pull review screen via
      // the Crew strip overlay path; this branch is a defensive fallback
      // if a seed-catalyst nudge is ever rendered through the nudge
      // detail screen directly.
      case 'seed_subhost':
        unawaited(openBlankCreate(context, ref));

      // plan_experience is the default — also used for terminator nudges.
      default:
        unawaited(openBlankCreate(context, ref));
    }
  }

}
