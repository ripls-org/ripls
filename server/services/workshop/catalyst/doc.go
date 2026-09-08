// Package catalyst is the Workshop service's catalyst-pull subpackage. It
// owns:
//
//   - Eligibility filtering: which members of a circle have taken at
//     least one originating action (proposed event, hosted instance,
//     listed offer, accepted sub-host slot) and are therefore eligible
//     to receive a catalyst-pull DM.
//   - Load detection: whether the same person has hosted the last 3+
//     instances of a recurring rhythm in the circle.
//   - Recipient rate limit: 3 catalyst-pulls in 14 days across all hosts
//     and circles.
//   - Decline cooldown: once a recipient declines, the same recurring
//     thing does not re-prompt for 30 days.
//
// The package depends only on `server/storage` and on proto models.
// Eligibility / rate-limit / cooldown rules are pure functions over
// storage so they can be unit-tested without spinning up the workshop
// service.
//
// Per docs/issues/1579-workshop-tab.md the catalyst-pull is the one
// Workshop surface that does NOT reuse the nudge data model — it
// produces a host-voiced direct message that routes through the chat
// infrastructure rather than into the existing creation modals.
package catalyst
