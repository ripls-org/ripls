import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/data/gen/ripls/api/experience_service.pb.dart'
    show RSVPIntention;
import 'package:ripls/presentation/viewmodels/experience_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass.dart';
import 'package:ripls/services/providers.dart';

/// WhosInManageSheet — host-only controls reached by tapping a Who's In entry
/// (#2492). An individual can be moved between invited / going / maybe / not
/// going, or removed; a community can be removed (unshared).
class WhosInManageSheet {
  WhosInManageSheet._();

  static Future<void> showForMember(
    BuildContext context, {
    required String experienceId,
    required String memberUserId,
    required String memberName,
    required RosterStatus currentStatus,
  }) {
    return showAccessibleModal<void>(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (_) => _MemberManageBody(
        experienceId: experienceId,
        memberUserId: memberUserId,
        memberName: memberName,
        currentStatus: currentStatus,
      ),
    );
  }

  static Future<void> showForCommunity(
    BuildContext context, {
    required String experienceId,
    required String communityId,
    required String communityName,
  }) {
    return showAccessibleModal<void>(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (_) => _CommunityManageBody(
        experienceId: experienceId,
        communityId: communityId,
        communityName: communityName,
      ),
    );
  }
}

/// Shared option row: a label with an optional leading check (when selected)
/// and optional destructive styling.
class _OptionRow extends StatelessWidget {
  const _OptionRow({
    required this.label,
    required this.onTap,
    this.selected = false,
    this.destructive = false,
    this.enabled = true,
  });

  final String label;
  final VoidCallback onTap;
  final bool selected;
  final bool destructive;
  final bool enabled;

  @override
  Widget build(BuildContext context) {
    final color = destructive
        ? AppColors.statusErrorOnDark
        : AppColors.modalTextPrimary;
    return Tappable(
      semanticsLabel: label,
      onTap: enabled ? onTap : () {},
      child: Opacity(
        opacity: enabled ? 1 : 0.5,
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 20, vertical: 14),
          child: Row(
            children: [
              SizedBox(
                width: 24,
                child: selected
                    ? Icon(Icons.check, size: 18, color: AppColors.primary(context))
                    : null,
              ),
              const SizedBox(width: 8),
              Text(
                label,
                style: TextStyle(
                  color: color,
                  fontSize: 16,
                  fontWeight: selected ? FontWeight.w600 : FontWeight.w400,
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _MemberManageBody extends ConsumerStatefulWidget {
  const _MemberManageBody({
    required this.experienceId,
    required this.memberUserId,
    required this.memberName,
    required this.currentStatus,
  });

  final String experienceId;
  final String memberUserId;
  final String memberName;
  final RosterStatus currentStatus;

  @override
  ConsumerState<_MemberManageBody> createState() => _MemberManageBodyState();
}

class _MemberManageBodyState extends ConsumerState<_MemberManageBody> {
  bool _busy = false;

  Future<void> _run(Future<void> Function() action) async {
    if (_busy) return;
    setState(() => _busy = true);
    try {
      await action();
      if (!mounted) return;
      unawaited(ref
          .read(experienceProvider(widget.experienceId).notifier)
          .refreshExperienceDetails());
      Navigator.of(context).pop();
    } catch (e) {
      if (!mounted) return;
      setState(() => _busy = false);
      ToastHelper.showError(context, "Couldn't update: $e");
    }
  }

  Future<void> _setRsvp(RSVPIntention intention) => _run(() => ref
      .read(experienceRepositoryProvider)
      .setMemberRsvp(
        experienceId: widget.experienceId,
        memberUserId: widget.memberUserId,
        intention: intention,
      ));

  Future<void> _remove() => _run(() => ref
      .read(experienceRepositoryProvider)
      .removeMember(
        experienceId: widget.experienceId,
        memberUserId: widget.memberUserId,
      ));

  @override
  Widget build(BuildContext context) {
    final s = widget.currentStatus;
    return GlassSheet(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Padding(
            padding: const EdgeInsets.fromLTRB(20, 4, 20, 8),
            child: Text(
              widget.memberName,
              style: TextStyle(
                color: AppColors.modalTextPrimary,
                fontSize: 18,
                fontWeight: FontWeight.w700,
              ),
            ),
          ),
          _OptionRow(
            label: context.l10n.experienceGoing,
            selected: s == RosterStatus.going,
            enabled: !_busy,
            onTap: () => _setRsvp(RSVPIntention.RSVP_INTENTION_YES),
          ),
          _OptionRow(
            label: context.l10n.experienceRsvpMaybe,
            selected: s == RosterStatus.maybe,
            enabled: !_busy,
            onTap: () => _setRsvp(RSVPIntention.RSVP_INTENTION_MAYBE),
          ),
          _OptionRow(
            label: context.l10n.pitchingInNotGoing,
            selected: s == RosterStatus.notGoing,
            enabled: !_busy,
            onTap: () => _setRsvp(RSVPIntention.RSVP_INTENTION_NO),
          ),
          _OptionRow(
            label: 'Invited',
            selected: s == RosterStatus.noReply,
            enabled: !_busy,
            onTap: () => _setRsvp(RSVPIntention.RSVP_INTENTION_UNSPECIFIED),
          ),
          Divider(color: AppColors.border(context), height: 8),
          _OptionRow(
            label: 'Remove from event',
            destructive: true,
            enabled: !_busy,
            onTap: _remove,
          ),
          const SizedBox(height: 8),
        ],
      ),
    );
  }
}

class _CommunityManageBody extends ConsumerStatefulWidget {
  const _CommunityManageBody({
    required this.experienceId,
    required this.communityId,
    required this.communityName,
  });

  final String experienceId;
  final String communityId;
  final String communityName;

  @override
  ConsumerState<_CommunityManageBody> createState() =>
      _CommunityManageBodyState();
}

class _CommunityManageBodyState extends ConsumerState<_CommunityManageBody> {
  bool _busy = false;

  Future<void> _remove() async {
    if (_busy) return;
    setState(() => _busy = true);
    try {
      await ref.read(experienceRepositoryProvider).unshareExperience(
            experienceId: widget.experienceId,
            communityId: widget.communityId,
          );
      if (!mounted) return;
      unawaited(ref
          .read(experienceProvider(widget.experienceId).notifier)
          .refreshExperienceDetails());
      Navigator.of(context).pop();
    } catch (e) {
      if (!mounted) return;
      setState(() => _busy = false);
      ToastHelper.showError(context, "Couldn't remove: $e");
    }
  }

  @override
  Widget build(BuildContext context) {
    return GlassSheet(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Padding(
            padding: const EdgeInsets.fromLTRB(20, 4, 20, 8),
            child: Text(
              widget.communityName,
              style: TextStyle(
                color: AppColors.modalTextPrimary,
                fontSize: 18,
                fontWeight: FontWeight.w700,
              ),
            ),
          ),
          _OptionRow(
            label: 'Invited',
            selected: true,
            enabled: !_busy,
            onTap: () {},
          ),
          Divider(color: AppColors.border(context), height: 8),
          _OptionRow(
            label: 'Remove from event',
            destructive: true,
            enabled: !_busy,
            onTap: _remove,
          ),
          const SizedBox(height: 8),
        ],
      ),
    );
  }
}
