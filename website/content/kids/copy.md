# Kids page copy — editable source

Consumer landing page for the school-and-team / parents segment — the person who
runs the school list, the soccer team, the block. Edit the text under each
`## key` heading, then tell Claude to inject the changes back into `index.html`.
Inline HTML tags (`<em>`, `<br>`) are preserved verbatim — leave them where they
make sense. (Per the shared design, `<em>` in headlines renders as upright
emphasis, not italic.)

Section IDs match the `data-bg="NN"` attributes in `index.html`. Feature sections
pair body copy with a phone screen from `screens/`. The closing section uses
`signup`. Dark sections pull `background/NN.jpg`.

---

## page.title

Ripls — Bring back 'be home by dinner.'

---

## 01.headline  [bg 01]

Bring back<br><em>"be home by dinner."</em>

## 01.subhead

Your kid hears it from you, not a screen — they never install the app. And you stop being the group chat.

## 01.cta

Get on the list

## 01.category

An app for the school list, the soccer team, the block

---

## 02.headline  [bg 02]

"Mom, can you text Jake's mom?" is the new "can I go play?"

## 02.body

You remember how it worked: you walked over and knocked, and Tommy came out or he didn't. If he did, you played until the streetlights came on. Nobody texted anybody.

Now your kid asks if Jake can come over Saturday, so you text Jake's mom — who doesn't reply for two days. By the time she does, the soccer schedule has shifted and the original Saturday is shot, so you shuffle yours, your partner's, and the team's. Practice carpool is still unsorted at 7pm Sunday, the team chain has 41 unread messages, and your kid is in the next room asking if it's worked out yet.

Your kids don't have phones, and you don't want them to. But the system coordinating their lives is built for adults — and you're the one running it.

---

## 03.headline  [bg 03, tldr]

Kids ask. Parents approve. Saturday <em>happens</em>.

## 03.body

Your kid says they want to play.

One tap floats it to the right family.

They tap yes.

The kids hear from <em>you</em>, not a screen.

---

## 09.headline  [bg 09, feature — The Send]

Tell it who, when, and <em>where</em>.

## 09.body

<em>"Jake at our house Saturday 10–12, lunch included, walking distance."</em> Out comes a real request — time, place, drop-off and pickup details, what they'll eat, who's home — sent only to Jake's parents. They tap yes, and your kids find out from you ten minutes later. No 41-message group chat, no two-day text wait.

You'll run a circle for each context — your soccer team, your block, your daughter's classroom — and a request only goes to the right one: the team carpool stays in the team, and a Saturday over at Maya's stays on the block.

## 09.media-aria

A playdate ask being generated live — time, place, who's home, lunch — filling in from a single sentence.

---

## 10.headline  [bg 10, feature flip — The Reply Loop]

Stop wondering<br>if Jake's <em>actually</em> coming.

## 10.body

Yeses, noes, time changes, who's driving, who's hosting after — all in one place. Anyone who hasn't replied gets a soft second nudge automatically, and a Saturday-morning reminder turns the maybes into yeses. You glance once and know: three kids are coming, the carpool is sorted, the snack family is set.

## 10.media-aria

A consolidated reply view for the U-10 team's Saturday game — yeses, who's driving, who's bringing snack — with a soft second nudge to anyone who hasn't replied.

---

## 11.headline  [bg 11, feature — Claim list]

Who's hosting Saturday. Who's driving to practice. Who's bringing the orange slices.

## 11.body

List what's needed — Saturday's snack family for the U-10s, Tuesday's practice carpool, the host for the next sleepover, who's grabbing the away-game uniforms from the coach. Parents tap what they can cover, and the list fills in as they see it — no more 7pm scramble in a 41-message thread, no more <em>"I think Maya's mom said she'd drive but I'm not sure."</em>

## 11.media-aria

A 'Still needed' list for the soccer team's Saturday — snack family, two carpool seats, host for after — with 'I got this' buttons.

---

## 06.headline  [bg 06, feature flip — Borrow and lend]

The cleats your kid outgrew. The trampoline. The Tuesday <em>ride to practice</em>.

## 06.body

The same circle that runs Saturday is also where the gear moves — one tap to ask, one tap to offer, pickup time and porch address baked in. The size-4 cleats your kid outgrew in March, the trampoline that's suddenly getting used three afternoons a week, the Tuesday ride to practice when both your kids are on the team, the sleepover sleeping bag, last year's Halloween costume, the bike one size up.

Most kid stuff fits for a season and then sits in a closet for five years. In a working circle it keeps moving instead, and nobody buys ten of the same thing.

## 06.media-alt

A map of the circle's offers — outgrown cleats, the trampoline, Tuesday-practice rides, sleeping bags — with one-tap ask and offer buttons.

---

## 13.headline  [bg 13, feature — Momentum]

Turn one playdate into a <em>regular Saturday</em>.

## 13.body

After Saturday wraps, a little note asks the host parent — was that good? want to make it a thing? — and a pre-drafted next one shows up, same kids, same time, ready to send. You approve it rather than recreate it, and the practice carpool that worked once becomes the carpool that runs the season.

And when you've been the host for four Saturdays in a row, the system finds the next family up and sends a one-tap <em>"want to take next Saturday?"</em> Somebody steps up, because nobody had to remember to ask.

## 13.media-alt

The app tapping a parent with a one-tap prompt to host the next Saturday, while the burned-out host parent gets a 'still on?' pulse.

---

## re-engage.headline  [light feature flip — no background]

A note for the family who's <em>missing it</em>.

## re-engage.body

If you've been running Saturday mornings and you've noticed the Reyes haven't been around, the app drafts the note you'd send if you weren't doing dinner — <em>"haven't seen you all since [the last thing we did together] — picnic Sunday, want in?"</em> — using your last shared event. You read, you approve, you send. Parent to parent.

The gentle eye on attendance is opt-in, only on the host side, and only at the family level — never the kid level. Most families come back the first time you reach out; people drift long before they decide they're out, and the note gets there before the door closes.

## re-engage.media-alt

A drafted note to a family whose kid hasn't been at the last three Saturdays — with a soft 'is this slipping?' signal up top. (Currently reuses nudges.gif until a bespoke re-engage screen exists.)

---

## 04.headline  [bg 04 — Decentralization]

Five kids, an afternoon, and <em>nobody had a phone</em>.

## 04.body

A Saturday in October: your kid wants Jake over, you tap, Jake's parent taps, and by 10am Jake's at the door. By 11 they've roped in Maya from down the street, and by noon they're at the playground with two more, a soccer ball, and a snack you didn't have to pack. They come home when they're hungry.

It used to happen this way — the kids had agency, the parents had a Saturday morning, and the phones stayed in the kitchen. The choreography came back the moment the parent-side logistics got out of the way.

---

## 05.headline  [bg 05 — Trust & safety]

Your kid's data is yours. <em>And the kid never logs in.</em>

## 05.body

Your kid's name, address, school, and schedule are visible only to the families you've added to the circle. We don't show ads, we don't sell anything, and there's no kid-facing product designed to make children want screens — the interface belongs to the parents, and your kid never installs it, signs up, or looks at it. Your circle is the parents you invited, full stop.

Built by parents — moms and dads who've taken turns being the team-text manager — who don't want their own kids on phones either. We made the thing we wished existed when our 9-year-old started asking for Snapchat, Roblox, and an iPhone in the same week.

---

## 07.headline  [bg 07 — Floor and ceiling]

One Saturday is a Saturday. <em>Fifty</em> Saturdays is a childhood.

## 07.body

Anybody can pull off one Saturday. The work is the next forty-nine — the standing pickup at the field, the U-10 carpool that runs the whole season, the after-school crew that walks home together, the block where the 8-year-olds know the 11-year-olds and the 11-year-olds keep half an eye on them.

That's where childhood stops being a series of supervised activities and starts being something your kid will remember without prompting at twenty-five. That's how they get to know what a friend group is, before any of them have phones.

---

## signup.quote

Tried it with our soccer team. Carpool finally works (took 6 weeks but it works). My 10yo has 3 Saturday friends now and roams the block in a pack on weekends. Hasn't asked for a phone in 2 months. Was a daily question before.

## signup.attribution

— Mike R., Madison WI

## signup.headline

We're in private beta. Want in?

## signup.subhead

We're starting with school lists, soccer teams, and blocks already trying to keep something alive — a Saturday-morning crew, a practice carpool, an after-school walk-home pack. We hand-pick the first circles each week.

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

Your school list. Your soccer team. Your block. The Saturday-morning crew.

## signup.button

Get me on the list

## signup.form-success

You're on the list! We hand-pick the first circles each week, so we'll be in touch soon.
