import 'package:flutter/foundation.dart' show kIsWeb;

/// Builds the URL to navigate to after a successful registration or
/// login completes. Shared by `RegisterScreen` and `PhoneAuthScreen`
/// so both auth paths land the user in the same place on web vs
/// mobile.
///
/// On Flutter Web, when the visitor came in from the
/// `WebExperienceScreen` guest entry point (`experienceId` + optional
/// `rsvpIntention` present), the destination becomes
/// `/event/{experienceId}?rsvp=…&code=…` so the
/// `WebEventRsvpHandoffNotifier` picks up the rsvp intention and
/// fires `RSVPToExperience` automatically.
///
/// The Flutter Web bundle is built with `<base href="/">`, so the
/// browser URL and the GoRouter path are the same: `/event/{id}`.
/// (Earlier iterations served the bundle under `<base href="/app/">`,
/// which required base-href stripping in GoRouter; the apex-domain
/// move dropped the prefix.)
///
/// On mobile (or on web without an item context), returns `/` — the
/// deep-link target is resolved from the share-link row after auth, not
/// carried as a query param.
String postRegistrationDestination({
  String? shortCode,
  String? gearId,
  String? requestId,
  String? experienceId,
  String? communityId,
  String? communityTab,
  String? rsvpIntention,
}) {
  if (kIsWeb && experienceId != null) {
    final query = <String>[];
    if (rsvpIntention != null) {
      query.add('rsvp=${Uri.encodeComponent(rsvpIntention)}');
    }
    if (shortCode != null) {
      query.add('code=${Uri.encodeComponent(shortCode)}');
    }
    final qs = query.isEmpty ? '' : '?${query.join('&')}';
    return '/event/${Uri.encodeComponent(experienceId)}$qs';
  }
  // Web gear / request guest flow (#2492, WEB-3/WEB-4): land on the web
  // item screen with the intent carrier so the WebGear/WebRequest screen
  // joins the ad-hoc community and auto-fires ExpressInterest /
  // OfferToFulfill. The intent value is implied by the item type.
  if (kIsWeb && gearId != null) {
    final query = <String>['intent=interest'];
    if (shortCode != null) {
      query.add('code=${Uri.encodeComponent(shortCode)}');
    }
    return '/item/${Uri.encodeComponent(gearId)}?${query.join('&')}';
  }
  if (kIsWeb && requestId != null) {
    final query = <String>['intent=offer'];
    if (shortCode != null) {
      query.add('code=${Uri.encodeComponent(shortCode)}');
    }
    return '/need/${Uri.encodeComponent(requestId)}?${query.join('&')}';
  }
  // Web community guest flow (#2875): land on the web community screen so it
  // joins the community and shows it. Checked last because a share link that
  // carries an item always also carries that item's community — the item is
  // the more specific destination.
  if (kIsWeb && communityId != null) {
    final query = <String>['intent=join'];
    if (shortCode != null) {
      query.add('code=${Uri.encodeComponent(shortCode)}');
    }
    // The destination hint survives the verify detour, so a guest who
    // registered on the way still lands on the surface the link promised
    // rather than the community's front page (#2876).
    if (communityTab != null && communityTab.isNotEmpty) {
      query.add('tab=${Uri.encodeComponent(communityTab)}');
    }
    return '/group/${Uri.encodeComponent(communityId)}?${query.join('&')}';
  }
  return '/';
}
