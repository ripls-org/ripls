# services/web

The `web` package serves dynamic HTML pages with server-side rendering. Its primary use is invitation/share-link previews: when a `/go/{code}` URL is opened in a browser or social crawler, this service renders an Open Graph–tagged page so the link unfurls correctly in messaging apps and on social platforms.

The router branches on which `oneof` variant the looked-up `ShareLink` row has set:

| `ShareLink.target` | Template | Notes |
|---|---|---|
| `community_invite_id` | `templates/community.html` | Phone-first community landing (#2875): hero + member count + inviter row + single "Join the group" CTA linking to `/group/{communityId}?intent=join&…`. Replaced the install-only card that was the last landing still gating a recipient on an app-store round trip. |
| `experience_id` | `templates/event.html` | Full-bleed hero with serif title overlay, meta-pill (date · time · place), host row, attendee cluster, sticky three-pill RSVP CTA bar. The "I'm in"/"Maybe" CTAs hand off to the Flutter web phone-first RSVP (#2492). |
| `gear_id`, `transfer_id` | `templates/gear.html` | Phone-first gear landing (#2492 WEB-4): hero + single primary CTA ("Ask to borrow" for a loan, "I want this" for a giveaway) linking to `/item/{gearId}?intent=interest&…`. A transfer target resolves to the gear behind it. |
| `request_id` | `templates/request.html` | Phone-first request landing (#2492 WEB-3): hero + "Offer to help" CTA linking to `/need/{requestId}?intent=offer&…`. |
| invalid / revoked / not found | `templates/invite.html` | Uniform error surface across all share-link variants — since #2875 this is *all* `invite.html` is. |

## Key files

- `service.go` — `Service` struct, `New(...)`, and `HandleInvitePage(...)`. The latter looks up the short code and dispatches to the appropriate template path.
- `item_preview.go` — shared item-preview helpers (`fetchGearPreview`, `fetchRequestPreview`, `getMediaImageURL`, `firstName`) that read item data from the share-link row's typed target. Consumed by the gear and request landing pages (`gear_page.go` / `request_page.go`). Legacy `?gear_id=`-style query-param invite URLs are no longer honored (#2562).
- `event_page.go` — `handleEventLanding(...)` and supporting fetchers (`fetchPlaceName`, `fetchAttendeeRSVPs`, `fetchUsersByIDs`, `fetchMediaByIDs`). Builds `eventPageData` for `event.html`. Uses `storage.GetByIDs` for batched user and media reads so render stays bounded at ≤7 storage queries regardless of attendee count. Invalid event states (soft-deleted, cancelled, completed, missing community) render the existing `invite.html` error page so the visitor-facing surface is uniform.
- `web_strings.go` — the per-page localized static-label builders (`buildEventStrings`, `buildItemStrings`, `buildInviteStrings`, shared capacity/open-in-app chrome). Copy that's derived from item/event data (headlines, roles, CTAs, OG/title text) is built alongside that data in the `*_page.go` files; the fixed chrome lives here so the templates carry no inline English (#2090).
- `event_when.go` — `formatEventWhen(...)`: assembles the event's date/time labels from the `web.when.*` catalog (Go's `time.Format` is not locale-aware, so weekday/month names and the per-locale date ordering come from `l10n`; a 12-hour clock with a localized AM/PM marker).
- `event_page_test.go` — golden-file tests for the event landing: normal render, at-capacity, empty attendees, missing hero, very-long names, host-also-RSVPed, invalid states (cancelled / completed / soft-deleted), and a query-budget assertion via `storage.AssertMaxQueries`.
- `gear_page.go` / `request_page.go` — `handleGearLanding(...)` / `handleRequestLanding(...)` and `build*PageData` for the phone-first item landings (#2492 WEB-3/WEB-4). Both reuse the `item_preview.go` resolvers for the item data and `resolveLandingHero` (in `gear_page.go`) for the item→community→logo hero fallback. The single primary CTA threads `intent=interest|offer` + the share code into the Flutter web route so the action auto-fires after phone verification. Unavailable items (terminal state, soft-deleted, missing/deleted community) render the shared `invite.html` error page.
- `gear_page_test.go` / `request_page_test.go` — tests for loan/giveaway/transfer/request renders, terminal-state and deleted-community unavailable pages, at-capacity, and CTA-href/intent assertions.
- `community_page.go` — `handleCommunityLanding(...)` + `buildCommunityPageData` for the phone-first community landing (#2875), the plain "join this group" invite. Reuses `resolveLandingHero` for the community-photo→logo fallback and threads `intent=join` + the share code into `/group/{communityId}`. The page deliberately renders a member **count** and the inviter's first name only — never the roster (`docs/invitations.md`); `truncateRunes` caps the group description. Missing/soft-deleted community or missing inviter render the shared `invite.html` error page.
- `community_page_test.go` — tests for the valid render (CTA href, cache headers, first-name-only), the no-member-names privacy invariant, at-capacity, nameless ad-hoc communities, the three unavailable states, description truncation, and `truncateRunes` itself.
- `templates/invite.html` — embedded Go HTML template for the shared share-link **error** surface. Every valid link renders its own landing, so this page only ever says "that link didn't work" and offers the two recovery paths a stranded recipient has: install the app (with the Play Install Referrer carrying the code, #1099) or re-tap / hand-enter the code. Styling lives in `website/content/css/{shared,deeplink,tokens}.css`.
- `templates/community.html` — embedded Go HTML template for the phone-first community landing. Reuses `event.css`'s `event-*` classes like the gear/request templates.
- `templates/event.html` — embedded Go HTML template for the SSR event landing. Styling lives in `website/content/css/{event,tokens}.css`.
- `templates/gear.html` / `templates/request.html` — embedded Go HTML templates for the phone-first gear and request landings. They reuse `event.css`'s `event-*` design-system classes (hero, CTA bar).

## Design tokens & styling

All templates share the Ripls app's visual language (sage primary, Libre Baskerville serif, OS-aware dark mode) via `website/content/css/tokens.css` — design tokens hand-mirrored from `app/lib/core/theme/app_colors.dart` and `app_theme.dart`. The four landings (`event.html`, `gear.html`, `request.html`, `community.html`) share `event.css`; the `invite.html` error surface uses `shared.css + deeplink.css`. All CSS files are embedded via `website/embed.go`. See `docs/issues/2049-web-event-landing.md` for the design history.

## Localization (#2090)

All landing templates render their visitor-facing copy through the server `l10n` catalog (`web.*`/`common.*` keys, en+es) in the visitor's `Accept-Language` — the `/go/` route is wrapped in `middleware.AcceptLanguage`, and each handler builds **one** `l10n.Localizer` per request (`requestLocalizer`) and threads it through the `build*` helpers. Copy is resolved in Go and passed to the templates as finished strings (the templates stay free of inline English); word-order-sensitive and pluralized copy uses parameterized keys / `Localizer.TCount`, never string concatenation. Each render sets `Vary: Accept-Language` (so a shared cache keys on locale, not just URL) and stamps `<html lang>`. The `check_no_inline_user_strings` lint gate scans the handler files for regressions. See [`docs/server/l10n.md`](../../../docs/server/l10n.md).

## Logging

All handlers begin with `logging.LoggerWithContext(ctx).With("operation", ..., "short_code", logging.MaskToken(shortCode), ...)`. Levels:

- `Info` — normal render, invalid-state render (soft-deleted / cancelled / completed events), at-capacity render. These are user-facing states, not failures.
- `Warn` — partial fetch failures with graceful degradation (missing hero media → community thumb fallback, missing host user → render with placeholder name).
- `Error` — storage failures that prevent rendering. The response is a generic 500; raw storage error text never reaches the wire.

## Twilio SMS webhooks (non-HTML)

Two public `POST` endpoints receive Twilio callbacks. Both signature-verify the
`X-Twilio-Signature` (HMAC-SHA1 via `sms.ValidateSignature`) against
`https://<hostname><path>` using the Twilio auth token wired by
`SetSMSAuthToken` (empty token disables verification — dev/local only), reject a
bad signature with `403` before any side effect, and mask the recipient number
(`logging.MaskPhone`) — never logging a message body. See
[`docs/sms_notifications.md`](../../../docs/sms_notifications.md).

- `sms_webhook.go` — `HandleSMSWebhook`, `POST /sms/webhook`. Inbound
  STOP/HELP/START opt-out sync: persist-only mirror into the `smsoptout` store,
  acks a bare `<Response/>` (Twilio's opt-out management owns the reply).
- `sms_status.go` — `HandleSMSStatusCallback`, `POST /sms/status` (#2569).
  Outbound delivery status callback: observability-only. Logs `MessageStatus`
  (with the Twilio `ErrorCode` on failure) — ERROR for `failed`/`undelivered`,
  INFO for `delivered`, DEBUG for intermediate — and acks `204`. No state change;
  no reply.

## When to add code here vs. elsewhere

Server-rendered public HTML pages belong here. RPC-based (Connect/gRPC) endpoints belong in the appropriate service sub-package — `server/services/community/share_links.go` for share-link creation/lookup RPCs, future `server/services/community/share_links.go` additions for RSVP wiring (#2050).
