import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart' show TransferRequest;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/viewmodels/gear_booking_view_model.dart';
import 'package:ripls/presentation/viewmodels/gear_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/sharing/item_share_sheet.dart';
import 'package:ripls/presentation/widgets/user_avatar.dart';

/// GearOwnerReserveDock is the owner's "Who's it for?" chooser shown in the
/// who's-using calendar for an open day (gear-owner-who-using-reserve.html). The
/// owner can't borrow from themselves, so picking open days lets them reserve
/// for a friend, mint an accept-link for someone else, or block the days for
/// their own use. Each path commits through [GearBookingNotifier.claim].
class GearOwnerReserveDock extends ConsumerStatefulWidget {
  final String gearId;
  final String communityId;
  final DateTime start;
  final DateTime end;
  final String rangeLabel;

  const GearOwnerReserveDock({
    super.key,
    required this.gearId,
    required this.communityId,
    required this.start,
    required this.end,
    required this.rangeLabel,
  });

  @override
  ConsumerState<GearOwnerReserveDock> createState() =>
      _GearOwnerReserveDockState();
}

const _kSelf = '__self__';
const _kLink = '__link__';

class _GearOwnerReserveDockState extends ConsumerState<GearOwnerReserveDock> {
  static const _mint = Color(0xFFA7C59E);
  static const _mintInk = Color(0xFF23351C);
  static const _gold = Color(0xFFD8A13A);

  // Pick: a borrower's user id, [_kSelf], [_kLink], or null (nothing chosen).
  String? _pick;

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    final gear = ref.watch(gearProvider(widget.gearId));
    final ownerId = gear.gearDetails?.owner.id ?? '';
    final interested = <User>[
      for (final r
          in gear.transferContext?.pendingRequests ??
              const <TransferRequest>[])
        if (r.hasBorrower() && r.borrower.id != ownerId) r.borrower,
    ];
    final saving = ref.watch(gearBookingProvider(widget.gearId)).isMutating;

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        _hero(l10n.gearCalendarOpenLabel, widget.rangeLabel),
        const SizedBox(height: 18),
        Text(
          l10n.gearReserveWhosFor.toUpperCase(),
          style: TextStyle(
            color: Colors.white.withAlpha(115),
            fontSize: 10,
            fontWeight: FontWeight.w800,
            letterSpacing: 0.8,
          ),
        ),
        const SizedBox(height: 11),
        _peopleRow(context, interested),
        const SizedBox(height: 12),
        _explain(context),
        const SizedBox(height: 14),
        _confirm(context, ownerId, saving: saving),
      ],
    );
  }

  Widget _hero(String label, String title) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          label.toUpperCase(),
          style: const TextStyle(
            color: _mint,
            fontSize: 10,
            fontWeight: FontWeight.w800,
            letterSpacing: 1,
          ),
        ),
        const SizedBox(height: 4),
        Text(
          title,
          style: const TextStyle(
            fontFamily: AppTheme.headingFont,
            fontSize: 27,
            fontWeight: FontWeight.w600,
            color: Colors.white,
            height: 1.02,
          ),
        ),
      ],
    );
  }

  Widget _peopleRow(BuildContext context, List<User> interested) {
    return SizedBox(
      height: 74,
      child: ListView(
        scrollDirection: Axis.horizontal,
        children: [
          for (final u in interested)
            _personPick(context, u),
          if (interested.isNotEmpty)
            Container(
              width: 1,
              height: 42,
              margin: const EdgeInsets.symmetric(horizontal: 8, vertical: 11),
              color: Colors.white.withAlpha(41),
            ),
          _choicePick(
            context,
            selected: _pick == _kLink,
            label: context.l10n.gearReserveSomeoneElse,
            semanticsLabel: context.l10n.a11yGearReserveSomeoneElse,
            onTap: () => _setPick(_kLink),
            avatar: _dashedAvatar(const Icon(Icons.add_rounded,
                size: 22, color: Colors.white70)),
          ),
          _choicePick(
            context,
            selected: _pick == _kSelf,
            label: context.l10n.gearReserveJustMe,
            semanticsLabel: context.l10n.a11yGearReserveJustMe,
            onTap: () => _setPick(_kSelf),
            avatar: Container(
              width: 46,
              height: 46,
              alignment: Alignment.center,
              decoration: BoxDecoration(
                shape: BoxShape.circle,
                color: _mint.withValues(alpha: 0.18),
                border: Border.all(color: _mint.withValues(alpha: 0.5), width: 2),
              ),
              child: Text(
                context.l10n.gearReserveYou,
                style: const TextStyle(
                  color: _mint,
                  fontSize: 13,
                  fontWeight: FontWeight.w800,
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _personPick(BuildContext context, User user) {
    final selected = _pick == user.id;
    return _pickColumn(
      label: user.name.split(' ').first,
      selected: selected,
      semanticsLabel: context.l10n.a11yGearReservePerson(user.name),
      onTap: () => _setPick(user.id),
      avatar: Container(
        decoration: BoxDecoration(
          shape: BoxShape.circle,
          border: Border.all(
            color: selected ? Colors.white : Colors.transparent,
            width: 2.5,
          ),
        ),
        child: UserAvatar(user: user, radius: 22),
      ),
    );
  }

  Widget _choicePick(
    BuildContext context, {
    required bool selected,
    required String label,
    required String semanticsLabel,
    required VoidCallback onTap,
    required Widget avatar,
  }) {
    return _pickColumn(
      label: label,
      selected: selected,
      semanticsLabel: semanticsLabel,
      onTap: onTap,
      avatar: avatar,
    );
  }

  Widget _pickColumn({
    required String label,
    required bool selected,
    required String semanticsLabel,
    required VoidCallback onTap,
    required Widget avatar,
  }) {
    return Tappable(
      semanticsLabel: semanticsLabel,
      onTap: onTap,
      child: SizedBox(
        width: 62,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            DecoratedBox(
              decoration: BoxDecoration(
                shape: BoxShape.circle,
                boxShadow: selected
                    ? [const BoxShadow(color: _mint, blurRadius: 0, spreadRadius: 2)]
                    : null,
              ),
              child: avatar,
            ),
            const SizedBox(height: 6),
            Text(
              label,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: TextStyle(
                color: selected ? Colors.white : Colors.white.withAlpha(191),
                fontSize: 11,
                fontWeight: FontWeight.w600,
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _dashedAvatar(Widget child) {
    return Container(
      width: 46,
      height: 46,
      alignment: Alignment.center,
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        color: Colors.white.withAlpha(20),
        border: Border.all(color: Colors.white.withAlpha(102)),
      ),
      child: child,
    );
  }

  Widget _explain(BuildContext context) {
    final l10n = context.l10n;
    final (IconData icon, String text) = switch (_pick) {
      _kSelf => (Icons.lock_outline_rounded, l10n.gearReserveExplainSelf),
      _kLink => (Icons.link_rounded, l10n.gearReserveExplainLink),
      final p when p != null => (
          Icons.event_outlined,
          l10n.gearReserveExplainPerson,
        ),
      _ => (Icons.event_outlined, ''),
    };
    if (text.isEmpty) return const SizedBox(height: 20);
    return Row(
      children: [
        Icon(icon, size: 15, color: Colors.white.withAlpha(163)),
        const SizedBox(width: 7),
        Expanded(
          child: Text(
            text,
            style: TextStyle(
              color: Colors.white.withAlpha(163),
              fontSize: 12.5,
              height: 1.4,
            ),
          ),
        ),
      ],
    );
  }

  Widget _confirm(BuildContext context, String ownerId, {required bool saving}) {
    final l10n = context.l10n;
    final pick = _pick;
    final enabled = pick != null && !saving;

    final (String label, Color bg, Color fg) = switch (pick) {
      _kSelf => (l10n.gearReserveBlock, _mint, _mintInk),
      _kLink => (l10n.gearReserveCreateLink, _gold, _mintInk),
      final p when p != null => (
          l10n.gearReserveForName(_firstName(p)),
          _mint,
          _mintInk,
        ),
      _ => (l10n.gearReserveChoosePrompt, Colors.white.withAlpha(23), Colors.white54),
    };

    return Tappable(
      semanticsLabel: label,
      onTap: enabled ? () => _commit(ownerId) : () {},
      child: Container(
        padding: const EdgeInsets.symmetric(vertical: 16),
        decoration: BoxDecoration(
          color: bg,
          borderRadius: BorderRadius.circular(15),
        ),
        child: Row(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Text(
              label,
              style: TextStyle(
                color: fg,
                fontSize: 16,
                fontWeight: FontWeight.w800,
              ),
            ),
            if (enabled) ...[
              const SizedBox(width: 9),
              Icon(Icons.arrow_forward_rounded, size: 17, color: fg),
            ],
          ],
        ),
      ),
    );
  }

  String _firstName(String userId) {
    final reqs = ref.read(gearProvider(widget.gearId)).transferContext
            ?.pendingRequests ??
        const [];
    for (final r in reqs) {
      if (r.hasBorrower() && r.borrower.id == userId) {
        return r.borrower.name.split(' ').first;
      }
    }
    return '';
  }

  void _setPick(String value) {
    setState(() => _pick = _pick == value ? null : value);
  }

  Future<void> _commit(String ownerId) async {
    final pick = _pick;
    if (pick == null) return;
    final startUnix = widget.start.millisecondsSinceEpoch ~/ 1000;
    final endUnix = widget.end.millisecondsSinceEpoch ~/ 1000;
    final notifier = ref.read(gearBookingProvider(widget.gearId).notifier);

    final booking = await notifier.claim(
      startDateUnixSec: startUnix,
      endDateUnixSec: endUnix,
      recipientId: pick == _kLink
          ? null
          : (pick == _kSelf ? ownerId : pick),
      pending: pick == _kLink,
    );
    if (!mounted) return;
    if (booking == null) {
      final err = ref.read(gearBookingProvider(widget.gearId)).error;
      ToastHelper.showError(
        context,
        err != null
            ? RpcErrorHandler.localize(err, context.l10n)
            : context.l10n.gearCalendarGenericError,
      );
      return;
    }
    // A link reservation: share a link to the gear (QR + invite URL) so the
    // recipient can reach it and accept the held days.
    if (pick == _kLink) {
      await _openShareSheet();
    }
  }

  Future<void> _openShareSheet() async {
    final gear = ref.read(gearProvider(widget.gearId)).gearDetails;
    if (gear == null) return;
    await ItemShareSheet.show(
      context,
      itemType: ShareableItemType.gear,
      itemId: gear.id,
      itemName: gear.name,
    );
  }
}
