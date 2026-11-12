# Builders page copy — editable source

Investor landing page for the "disillusioned technologist" segment — angels who
made money in tech but are uneasy about its impact, and humane-tech / attention-
economy funds. Edit the text under each `## key` heading, then tell Claude to
inject the changes back into `index.html`. Inline HTML tags (`<em>`, `<br>`,
`<strong>`, `&nbsp;`) are preserved verbatim — leave them where they make sense.

Voice: first-person plural — both founders (Thomas and Bryan) speaking, and
addressing the reader as a fellow builder ("you included"). Keys are semantic;
the note in brackets says which background each section uses. Dark sections pull
`background/NN.jpg`; light sections carry no background.

Two slides (north-star and founders+round) include inline SVG figures — a
Presence line chart and a viral-coefficient (k) curve. The figure graphics live
in `index.html` only; this file keys just their user-visible caption text
(the `.figure-note` keys).

Reading order: hero → bet → product → north-star → money (light) → why-now →
founders+round → signup (light).

---

## page.title

Ripls — Help build the way out

---

## hero.headline  [bg 01]

You helped build the attention economy.<br><em>Help build the way out.</em>

## hero.subhead

The same skills that built tech addiction, aimed at the opposite outcome — people off their phones, in the same room, glad they showed up.

## hero.cta

Start a conversation

## hero.category

An investment thesis for technologists who want a different legacy

---

## bet.headline  [bg 02]

What if the algorithm's only job <em>was to get you out of the house?</em>

## bet.body

Between us — you included — we've spent our careers building things that are very good at holding attention, and they worked. The graphs went up and to the right for fifteen years, and somewhere in there the average person stopped having anyone to call on a Tuesday. Loneliness now carries a health risk on the order of smoking, and the tools we built to connect everyone turned out to be great at holding attention and lousy at the rest of it.

But every trick the last decade worked out — the nudge that gets you to finish, the default that thinks for you, the one tap that replaces the five-message thread — works just as well pointed at real life. That's Ripls: it does the tedious part of getting people together, the part that always lands on the one friend who organizes everything, and then it gets out of the way. <strong>No feed, no ads, no data sold</strong> — not as a values statement, but as the architecture. The app is winning the moment you've set it down and you're standing in somebody's driveway.

---

## product.headline  [bg 09, feature]

You type a sentence, and <em>the app does the rest.</em>

## product.body

<em>"Fence build Saturday, my place, bring a chop saw."</em> Out comes a real invite — time, place, who's bringing what, a reminder the night before so nobody flakes. The same handful of people lend each other the truck and the ladder, so no one buys a tool they'll use twice. And every invite lands with someone who may not be on Ripls at all — they can tap in and show up without downloading a thing, which means each event, each ask, each loan quietly hands the product to the next person.

It's a working app, in private beta with a small set of real groups in a handful of towns. We're early, and we'll say so plainly: it's too soon to wave a retention chart around. What we watch is whether the same people come back to see each other again — and once they do, whether they bring others with them. That second question is the one this round is built to answer.

## product.media-aria

A plan being generated live — title, time, place, gear list filling in from a single sentence.

---

## north-star.headline  [bg 10]

We don't count minutes on the screen. <em>We count Presence.</em>

## north-star.body

Every company you've ever backed measured the same thing in the end — time on the app. Ripls is built to move the opposite number: Presence, the hours people spend in the same room as people they care about. We've measured it from day one, even while it's small. It's the number we optimize, the one we'll report to you every month, and the one we'd put on a billboard — most apps would be too embarrassed.

And these aren't private vanity stats. The numbers belong to the group, and the group sees them — the way a CrossFit class clocks who showed up — so getting out becomes a team sport. The same pull that gets you to the gym because a friend's already there is what gets the crew together on Saturday. The metric does the nudging; your friends do the rest.

## north-star.figure-note

The curve we're paid to move — illustrative shape, reported monthly from beta groups.

## north-star.metric.presence

Hours in the same room as people you care about. The number we chase.

## north-star.metric.commonwealth

What moves through a community when people share, lend, and show up for each other.

## north-star.metric.stewardship

What borrowing instead of buying keeps out of the landfill, and out of the air.

---

## money.headline  [light]

The part VCs always ask about. <em>Here's the whole answer.</em>

## money.body

When a group moves money through Ripls — twelve neighbors putting in $40 each to rebuild a shared fence — we keep a small slice of what changes hands, the way any payment processor does, and the clubs, leagues, and teams that run this stuff every week pay a flat monthly rate for the heavier tools. There are no ads, ever; nobody's attention is the product.

Which means the more people actually lean on each other, the better the business works. For once the spreadsheet and the mission pull the same way — and nobody running an ad model can copy that without torching their own.

---

## why-now.headline  [bg 07]

The backlash is here. <em>The alternative isn't.</em>

## why-now.body

The mood has turned. The Surgeon General has called loneliness an epidemic, the kids who grew up on these apps are the ones deleting them, and AI is about to walk into a lot of jobs asking the question nobody says out loud — what are people for, if not work. There's real hunger for something else, and almost nothing real to hand people.

You already know how to make a thing billions of people open every morning; the question left is what it does to them once they do. The most important thing tech can build now is the thing that gives people back to each other — and the people best placed to build it are the ones who built the mess, because we know where everything's buried. Put your weight behind the version where the day ends with someone you love across an actual table. That's the one worth having your name on.

---

## founders.headline  [bg 05]

Two people who helped build the problem, <em>now building the way out.</em>

## founders.lead

We've spent our careers getting people to open an app. We'd like to spend the rest of them getting people to put one down.

## founders.thomas.name

Thomas Escobar

## founders.thomas.bio

Ran product as a director on Google Maps and led its biggest expansion in a decade; before that, helped push Google Drive past a billion users. And before Google, started a company in Beijing that built the world's largest open-government database.

## founders.bryan.name

Bryan Klingner

## founders.bryan.bio

Fifteen years at Google building computer vision for Street View and maps, most recently as a principal engineer. With a CS PhD from Berkeley, he's the one who takes a nasty research problem and turns it into something that actually ships.

## round.body

We're not going to dress a beta up as a breakout. Ripls is a working app in private beta, pre-traction by design, built on our own dime — no salaries, about <strong>$8K</strong> of actual cash spent, and the sentence-to-invite magic already works. We're raising <strong>$1M</strong>, with the first <strong>$100K</strong> already committed on a SAFE, at the same terms as the rest, by a former Google director who worked alongside us and watched us build. Spreading is built into the product rather than bolted on — every event, ask, and loan reaches people who aren't on Ripls yet, no download needed — and the round buys roughly ten months of go-to-market, our first non-founder hire, and compute to drive the viral coefficient to <strong>k&nbsp;=&nbsp;0.4</strong>, with a credible path to <strong>1.0</strong>, the point where growth sustains itself.

## round.figure-note

For every ten groups on Ripls at k = 0.4, four more arrive on their own — no ads pushing them.

---

## signup.headline  [light]

Want the deck?

## signup.subhead

We're keeping the list short — angels and funds who already get why this has to exist, and would rather their name sit on this than on the next thing built to keep people staring down. Tell us what pulls you in, and we'll send the deck and find a time.

## signup.email-label

Your email

## signup.email-placeholder

you@firm.com

## signup.name-label

Your name (optional)

## signup.name-placeholder

Your name

## signup.textarea-label

What pulls you in?

## signup.textarea-placeholder

The thesis. The legacy. The product. The friend who organizes everything, in your own life.

## signup.button

Send me the deck

## signup.form-success

Got it. We'll be in touch soon with the deck and a time to talk.
