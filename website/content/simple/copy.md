# Simple page copy — editable source

Consumer landing page for the simpler-living / less-consumerist segment — people
who want their day-to-day life to lean on borrowing instead of buying, doing
instead of owning, experiences instead of stuff, and the people nearby instead of
a store or a screen. Edit the text under each `## key` heading, then tell Claude
to inject the changes back into `index.html` (and the standalone
`host-journey-simple.html` test build). Inline HTML tags (`<em>`, `<br>`)
are preserved verbatim — leave them where they make sense. (Per the shared
design, `<em>` in headlines renders as upright emphasis, not italic.)

Voice & framing: thrift, anti-waste, less clutter, more time, borrow-don't-buy,
local community, a simpler and lighter life. We deliberately avoid politically
charged terms ("green," "environmental," "sustainable," "climate," "carbon,"
"consumerism" as a slogan, "activism") so the page reads the same to a frugal,
waste-nothing traditionalist and to a minimalist stepping away from buying. The
appeal is universal: own less, use what you have, lean on neighbors, spend time
not money. Local and personal, not global or political. Continuity and the
cup-of-sugar neighborhood, not crisis.

Section IDs match the `data-bg="NN"` attributes in `index.html`. Feature sections
pair body copy with a phone screen from `screens/`. The closing section uses
`signup`. Dark sections pull `background/NN.jpg`; light sections carry none.

NOTE on imagery: backgrounds are curated per section to the borrow-don't-buy /
neighborhood-sharing message and load automatically by `data-bg` number — no
regeneration needed when you swap a file. Photographer credits live on the five
editorial sections (01, 02, 03, 04, 07) inside `index.html` and the standalone
test build; the feature sections (06, 09, 10, 11, 13) are credited in
`background/CREDITS.md` and `background/attributions.json`. If you replace an
image, update those credits to match.

---

## page.title

Ripls — Less stuff. More life.

---

## 01.headline  [bg 01]

Less stuff. <em>More life.</em>

## 01.subhead

Borrow instead of buy, do instead of own, and lean on the people nearby. Ripls makes the simpler life the easy one.

## 01.cta

Get on the list

## 01.category

An app for the tool shed, the hand-me-down chain, the supper club

---

## 02.headline  [bg 02]

We filled our homes with things and somehow have less to show for it.

## 02.body

Look around a normal house. A drill used for fourteen minutes in its whole life. A garage you can't park in. Closets packed, a storage unit across town, a credit-card balance to match — and a calendar with more errands in it than people. Somewhere along the way we were sold the idea that the good life is something you buy, one purchase at a time.

Most of it we barely use. And most of what we're actually missing — a hand when we need one, people close enough to call, an afternoon that isn't about a screen or a store — was never for sale in the first place.

---

## 03.headline  [bg 03, tldr]

Ask the people nearby. <em>Skip the store.</em>

## 03.body

You need something, or you want to do something.

You write one line.

The people nearby lend, share, or show up.

You spent <em>time</em> instead of money.

---

## 09.headline  [bg 09, feature — The Send]

Write one sentence.<br>The app runs the rest.

## 09.body

Type <em>"anyone have a pressure washer I can borrow this weekend?"</em> or <em>"pizza and a fire pit Friday — who's in?"</em> and out comes a real request or a real invite, with the what, when, and where filled in — no store run, no event-planning rabbit hole.

One tap sends it, only to your circle. From there the app tracks the replies, nudges the quiet ones, and keeps the chat on topic for the details.

## 09.media-aria

A plan being generated live — what's needed, when, where — filling in from a single sentence.

---

## 10.headline  [bg 10, feature flip — The Reply Loop]

Stop wondering<br>if anyone said <em>yes</em>.

## 10.body

Who's lending, who's coming, who's bringing what — all in one place. Anyone who hasn't replied gets a soft second nudge automatically, and day-before reminders go out so plans actually happen instead of fizzling. You glance once and know it's handled.

## 10.media-aria

A consolidated reply view — who's in, what they're bringing — with a soft second nudge going out to anyone who hasn't replied.

---

## 11.headline  [bg 11, feature — Claim list]

Who's bringing the ladder. Who's got the extra chairs. Who's making the chili.

## 11.body

List what's needed — the folding tables, a cooler, the tent, somebody's truck for the dump run, a side dish, the camp stove — and everybody steps up with what they've got. Whether it's a backyard dinner, a moving day, or a weekend project, the list fills in where everyone can see it, with no ten-message thread sorting out who's bringing what.

## 11.media-aria

A Plan tab with a 'Still needed' list — tables, a cooler, a truck, a side dish — and 'I got this' buttons.

---

## 06.headline  [bg 06, feature flip — Borrow, don't buy]

Why own the ladder you'll use <em>twice a year</em>?

## 06.body

One tap to ask, one tap to offer — the drill, the ladder, the carpet cleaner, the canoe, the punch bowl, the camping gear, the kid's bike between sizes. Most of what we own sits idle ninety percent of its life; in a real neighborhood, all of it moves.

The drill that lived in your basement now keeps four homes in working order, and the truck that hauls your mulch hauls everyone's. Nobody buys what someone two doors down already has — and the closets, the garage, and the landfill all get a little lighter.

## 06.media-alt

A map of the circle's stuff — drills, ladders, canoes, camping gear — with one-tap borrow and lend buttons.

---

## 13.headline  [bg 13, feature — Momentum]

One good night becomes a <em>standing thing</em>.

## 13.body

After the dinner or the project wraps, a short recap goes out the next day, and one more tap makes it a regular thing — the monthly potluck, the Sunday repair café, the seasonal swap. You approve it; you don't recreate it.

And when you've been the one hosting for a few months straight, the app finds the next person up and sends a one-tap <em>"want to take the next one?"</em> Somebody steps up, and nobody has to remember to ask.

## 13.media-alt

The app tapping a circle member with a one-tap prompt to host the next gathering, while the burned-out host gets a 'still on?' pulse.

---

## re-engage.headline  [light feature flip — no background]

A note for the one who's <em>fallen out of the loop</em>.

## re-engage.body

If you've been keeping the circle going and you've noticed someone hasn't been around in a while, the app drafts the note you'd send if you had the time — <em>"haven't seen you since the spring swap — potluck Sunday, come by"</em> — built from the last thing you did together. You read it, change a word, and send it, person to person.

The gentle eye on who's drifting is opt-in, and only on the organizer's side. Most people come back the first time someone reaches out; they drift long before they decide they're done, and the note gets there before the door closes.

## re-engage.media-alt

A drafted note to someone who's missed the last few gatherings — with a soft 'is this slipping?' signal up top. (Currently reuses nudges.gif until a bespoke re-engage screen exists.)

---

## 04.headline  [bg 04 — The way it used to work]

A borrowed ladder, a shared dinner, and a street that <em>knows each other again</em>.

## 04.body

A Tuesday in spring: someone in the circle needs a ladder to clear the gutters, and two doors down one's been hanging in a garage all year. A tap, and it's on the porch by noon. That weekend the same handful of people end up around a fire pit nobody had to buy — folding chairs from three houses, a cooler from a fourth, kids running between yards.

It used to work this way without an app — you knew your neighbors, you borrowed the cup of sugar, you didn't buy what the block already had. The muscle's still there. It came back the moment somebody made it easy to ask.

---

## 05.headline  [light — Trust & privacy, no background]

Your circle is private. <em>Period.</em>

## 05.body

Who's in your circle, what you've borrowed, where you live, the plans you've made — all of it is visible to the people you invited and invisible to everyone else. We don't show ads, we don't sell data, and we don't recommend you to strangers or turn your life into a feed. This isn't a marketplace, and it isn't social media.

Built by people who got tired of owning more and knowing fewer — who wanted the cup-of-sugar neighborhood back without the group text that loses the thread. We made the thing we wanted: a closed circle that runs quiet enough that the only thing you notice is that life got simpler.

---

## 07.headline  [bg 07 — Floor and ceiling]

One borrowed drill is a favor. <em>A hundred</em> is a different way of living.

## 07.body

Anybody can borrow a ladder once. The shift is when it becomes how you live — the swap that's run six seasons, the tool shelf the whole street shares, the standing dinner that means you actually know the people around you.

That's when "buy it, use it once, store it forever" stops being the default, and a simpler, lighter, more connected life takes its place — not because anyone gave anything up, but because it turned out to be the better one.

---

## signup.quote

Started just trying not to buy a tile saw for one bathroom. Borrowed it from a guy on my street instead. Six months later we've got a little tool list going and a monthly dinner. Bought way less, see people way more.

## signup.attribution

— Dana M., Portland ME

## signup.headline

We're in private beta. Want in?

## signup.subhead

We're starting with circles already trying to keep something alive — a tool-sharing group, a swap, a standing dinner, a hand-me-down chain, a block that wants to know each other again. We hand-pick the first ones each week.

## signup.email-label

Your email

## signup.email-placeholder

your@email.com

## signup.name-label

Your name (optional)

## signup.name-placeholder

Your name

## signup.textarea-label

What are you trying to keep alive?

## signup.textarea-placeholder

A tool-sharing group. A swap. A standing dinner. The block that's currently just a group text.

## signup.button

Get me on the list

## signup.form-success

You're on the list! We hand-pick the first circles each week, so we'll be in touch soon.
