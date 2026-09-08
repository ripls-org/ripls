import 'package:flutter/material.dart';
import 'package:ripls/core/config/environment.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:url_launcher/url_launcher.dart';

import '../../widgets/accessibility/tappable.dart';

/// What brought the guest to the phone step. Selects the consent
/// disclosure's lead-in — the categories of message the user is opting into
/// — so a help-request landing doesn't lead with "event invites". Only that
/// lead-in varies; the A2P compliance body is identical for every value.
enum PhoneConsentContext {
  /// An event invite (the SSR "I'm in" / "Maybe" path).
  event,

  /// A help-request landing (offering to help with a request).
  request,

  /// An item landing (borrowing or claiming shared gear).
  item,

  /// A plain community invite, login, or add-a-phone — no single item or
  /// event framed the visit.
  community,
}

/// The SMS opt-in consent disclosure shown beneath the **Send Code** button on
/// the phone-entry step. Renders the A2P 10DLC consent text plus the Terms of
/// Service and Privacy Policy links that RCS carrier review requires on the
/// same screen as the opt-in (#2492). The compliance body is mirrored on the
/// deployment's hosted opt-in proof; only the lead-in naming the message
/// categories varies, by [entryContext]. The links open externally so the
/// in-progress phone-entry flow isn't lost.
///
/// The two link targets are build-time config (#2953). A deployment that
/// supplies neither renders the consent text without links — which is what a
/// carrier A2P registration will fail on, so supply them before shipping SMS.
///
/// Extracted from `PhoneAuthScreen` to keep that file under the dart-size limit.
class PhoneConsentDisclosure extends StatelessWidget {
  const PhoneConsentDisclosure({
    super.key,
    required this.entryContext,
    String? termsUrl,
    String? privacyUrl,
  })  : _termsUrl = termsUrl,
        _privacyUrl = privacyUrl;

  /// The entry point that brought the user here.
  final PhoneConsentContext entryContext;

  final String? _termsUrl;
  final String? _privacyUrl;

  /// Where the Terms link points. Falls back to this build's `TERMS_URL`
  /// dart-define; empty means no link.
  String get termsUrl => _termsUrl ?? Environment.termsUrl;

  /// Where the Privacy link points. Falls back to this build's `PRIVACY_URL`
  /// dart-define; empty means no link.
  String get privacyUrl => _privacyUrl ?? Environment.privacyUrl;


  /// The message categories this entry point is opting the user into.
  String _updates(BuildContext context) => switch (entryContext) {
    PhoneConsentContext.event => context.l10n.phoneAuthConsentUpdatesEvent,
    PhoneConsentContext.request => context.l10n.phoneAuthConsentUpdatesRequest,
    PhoneConsentContext.item => context.l10n.phoneAuthConsentUpdatesItem,
    PhoneConsentContext.community =>
      context.l10n.phoneAuthConsentUpdatesCommunity,
  };

  @override
  Widget build(BuildContext context) {
    final mutedStyle = Theme.of(context).textTheme.bodySmall?.copyWith(
          color: Theme.of(context).colorScheme.onSurfaceVariant,
        );
    final linkStyle = mutedStyle?.copyWith(
      decoration: TextDecoration.underline,
    );

    // Skip a link this build has no URL for rather than rendering one that
    // goes nowhere; the separator only earns its place between two links.
    final links = <Widget>[
      if (termsUrl.isNotEmpty)
        Tappable(
          semanticsLabel: context.l10n.settingsTerms,
          isLink: true,
          semanticsIdentifier: 'consent-terms-link',
          onTap: () => _open(termsUrl),
          child: Padding(
            padding: const EdgeInsets.symmetric(vertical: 4, horizontal: 2),
            child: Text(context.l10n.settingsTerms, style: linkStyle),
          ),
        ),
      if (privacyUrl.isNotEmpty)
        Tappable(
          semanticsLabel: context.l10n.settingsPrivacyPolicy,
          isLink: true,
          semanticsIdentifier: 'consent-privacy-link',
          onTap: () => _open(privacyUrl),
          child: Padding(
            padding: const EdgeInsets.symmetric(vertical: 4, horizontal: 2),
            child: Text(
              context.l10n.settingsPrivacyPolicy,
              style: linkStyle,
            ),
          ),
        ),
    ];

    return Column(
      children: [
        Text(
          context.l10n.phoneAuthConsent(_updates(context)),
          style: mutedStyle,
          textAlign: TextAlign.center,
        ),
        if (links.isNotEmpty) ...[
          const SizedBox(height: 8),
          Wrap(
            alignment: WrapAlignment.center,
            crossAxisAlignment: WrapCrossAlignment.center,
            children: [
              links.first,
              if (links.length == 2) ...[
                Text('  •  ', style: mutedStyle),
                links.last,
              ],
            ],
          ),
        ],
      ],
    );
  }

  /// Opens a legal page externally / in a new browser tab so the in-progress
  /// phone-entry flow isn't lost.
  void _open(String url) {
    launchUrl(Uri.parse(url), mode: LaunchMode.externalApplication);
  }
}
