import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';

/// What a non-RSVP guest is here to do, which selects the phone-entry
/// heading's verb ("claim" / "help with" / "join"). An event RSVP is not in
/// this enum — it has its own earlier branch keyed on `isRsvpFlow`.
enum GuestFlowKind {
  /// Borrowing or claiming shared gear.
  gear,

  /// Offering to help with a request.
  request,

  /// Joining a community from a plain invite (#2875).
  community,
}

/// The top-of-screen header for the phone-auth flow, split out of
/// `phone_auth_screen.dart` to keep that file under the size limit. Pure
/// presentation: the screen resolves its flow flags and step and passes them as
/// booleans (so this widget doesn't depend on the screen's private enums).
class PhoneAuthHeader extends StatelessWidget {
  const PhoneAuthHeader({
    super.key,
    required this.isAttach,
    required this.isPhoneInputStep,
    required this.isNameInputStep,
    required this.isRsvpFlow,
    required this.isGuestActionFlow,
    this.eventName,
    this.guestFlowKind = GuestFlowKind.gear,
  });

  /// Attaching a phone to the signed-in account (`PhoneAuthMode.attach`).
  final bool isAttach;

  /// The phone-entry step (vs OTP / name).
  final bool isPhoneInputStep;

  /// The name-entry step (register only).
  final bool isNameInputStep;

  /// Reached from an event invite (phone-first "Confirm your phone to RSVP").
  final bool isRsvpFlow;

  /// Any phone-first guest action flow (event RSVP / gear / request).
  final bool isGuestActionFlow;

  /// Item/event/group name for the guest-flow framing; null when unavailable.
  final String? eventName;

  /// What the non-RSVP guest flow points at, selecting the heading's verb.
  final GuestFlowKind guestFlowKind;

  @override
  Widget build(BuildContext context) {
    // Phone-first RSVP framing on the entry step: confirming the phone IS the
    // RSVP, so lead with that rather than a generic "Phone Verification".
    if (isRsvpFlow && isPhoneInputStep) {
      final name = eventName;
      final title = (name != null && name.isNotEmpty)
          ? context.l10n.phoneAuthRsvpTitleNamed(name)
          : context.l10n.phoneAuthRsvpTitle;
      return Text(
        title,
        style: Theme.of(
          context,
        ).textTheme.headlineSmall?.copyWith(fontWeight: FontWeight.bold),
        textAlign: TextAlign.center,
      );
    }
    // Gear / request / community guest flow: same phone-first framing as
    // RSVP, with the destination's verb ("…to claim {item}" / "…to help with
    // {item}" / "…to join {group}").
    if (isGuestActionFlow && isPhoneInputStep) {
      final name = eventName;
      final String title;
      if (name != null && name.isNotEmpty) {
        // Gear and request titles are common nouns sitting mid-sentence
        // ("…to help with an extension ladder"), so their first letter is
        // lowercased. A group name is a proper noun — "…to join Ferndale Tool
        // Library" — so it keeps its capital.
        title = switch (guestFlowKind) {
          GuestFlowKind.request => context.l10n.phoneAuthHelpTitleNamed(
            _itemNameMidSentence(name),
          ),
          GuestFlowKind.gear => context.l10n.phoneAuthClaimTitleNamed(
            _itemNameMidSentence(name),
          ),
          GuestFlowKind.community => context.l10n.phoneAuthJoinTitleNamed(name),
        };
      } else {
        title = switch (guestFlowKind) {
          GuestFlowKind.request => context.l10n.phoneAuthHelpTitle,
          GuestFlowKind.gear => context.l10n.phoneAuthClaimTitle,
          GuestFlowKind.community => context.l10n.phoneAuthJoinTitle,
        };
      }
      return Text(
        title,
        style: Theme.of(
          context,
        ).textTheme.headlineSmall?.copyWith(fontWeight: FontWeight.bold),
        textAlign: TextAlign.center,
      );
    }
    // Attach flow (adding a phone to the signed-in account): lead with the
    // benefit rather than a generic "Phone Verification".
    if (isAttach) {
      return Column(
        children: [
          Icon(
            Icons.phone_android,
            size: 48,
            color: Theme.of(context).colorScheme.primary,
          ),
          const SizedBox(height: 16),
          Text(
            context.l10n.phoneAuthAddTitle,
            style: Theme.of(
              context,
            ).textTheme.headlineSmall?.copyWith(fontWeight: FontWeight.bold),
            textAlign: TextAlign.center,
          ),
          const SizedBox(height: 8),
          Text(
            context.l10n.phoneAuthAddSubtitle,
            style: Theme.of(context).textTheme.bodyMedium?.copyWith(
              color: Theme.of(context).colorScheme.onSurfaceVariant,
            ),
            textAlign: TextAlign.center,
          ),
        ],
      );
    }
    // Name step: verification is already done, so confirm it with a check +
    // "Phone Verified" rather than repeating the phone icon / "Phone Verification".
    if (isNameInputStep) {
      return Column(
        children: [
          Icon(
            Icons.check_circle,
            size: 48,
            color: Theme.of(context).colorScheme.primary,
          ),
          const SizedBox(height: 16),
          Text(
            context.l10n.phoneAuthVerifiedTitle,
            style: Theme.of(
              context,
            ).textTheme.headlineSmall?.copyWith(fontWeight: FontWeight.bold),
            textAlign: TextAlign.center,
          ),
        ],
      );
    }
    return Column(
      children: [
        Icon(
          Icons.phone_android,
          size: 48,
          color: Theme.of(context).colorScheme.primary,
        ),
        const SizedBox(height: 16),
        Text(
          context.l10n.phoneAuthTitle,
          style: Theme.of(
            context,
          ).textTheme.headlineSmall?.copyWith(fontWeight: FontWeight.bold),
          textAlign: TextAlign.center,
        ),
      ],
    );
  }

  String _itemNameMidSentence(String name) {
    if (name.isEmpty) return name;
    return name[0].toLowerCase() + name.substring(1);
  }
}
