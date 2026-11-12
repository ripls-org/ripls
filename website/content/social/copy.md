# Social page copy — editable source

Investor / funder landing page for the "social-health believer" segment —
loneliness-epidemic angels, social-connection program foundations, social-health
/ loneliness-as-mortality funds, and civic / social-capital foundations. Edit the
text under each `## key` heading, then tell Claude to inject the changes back into
`index.html`. Inline HTML tags (`<em>`, `<br>`, `<strong>`) are preserved verbatim.

Voice: first-person plural — both founders (Thomas and Bryan) speaking. Keys are
semantic; the note in brackets says which background each section uses. Dark
sections pull `background/NN.jpg`; light sections carry no background.

Two sections carry inline SVG figures (the measurement line chart and the team
timeline). The figure graphics — including any text labels inside the SVG — live
in `index.html` only; this file keys only the visible caption text
(`.figure-note`). The timeline figure has no caption, so it has no key here.

Reading order: hero → gap → product → measurement → money (light) →
big-picture → team → signup (light).

---

## page.title

Ripls — Rebuild the thing we lost

---

## hero.headline  [bg 01]

Everyone agrees loneliness is the crisis.<br><em>Almost no one is building the cure.</em>

## hero.subhead

There's a whole industry naming the problem. We'd rather hand people the thing that gets them back in a room together — and measure it working.

## hero.cta

Start a conversation

## hero.category

A funding case for people who already treat connection as a matter of health

---

## gap.headline  [bg 02]

Diagnosis is crowded. <em>The cure is empty.</em>

## gap.body

You've read the same advisories we have — loneliness set next to smoking as a health risk, half of American adults with three close friends or fewer, teen loneliness climbing for a decade straight. None of it is news to you, so we'll skip the part where we scare you with it and get to the part almost nobody is doing anything about.

There are conferences, task forces, white papers, and a hundred apps that will track your mood and gently remind you you're lonely — and almost none of it ends with two people in the same room. Connection comes from somebody picking a date, sending the text, and chasing the maybes, and that somebody is usually nobody. That unglamorous coordinating job is the whole ballgame. Ripls does it for the person who always gets stuck with it, so the gathering actually happens — this week, and the week after that.

---

## product.headline  [bg 09, feature]

Less romantic than a movement. <em>A sentence and a Saturday.</em>

## product.body

Someone types <em>"Sunday dinner, my place, 6pm,"</em> and out comes a real invite — who's in, who's bringing the salad, a reminder the night before so it doesn't dissolve back into a group chat. The same people lend each other a folding table, a crib, a casserole dish, a ride. Small stuff — which is what a community is actually made of.

It's a working app, in private beta with a small set of real groups in a handful of towns. We're early, and the signal we watch is the human one: do the same people keep gathering, week after week. And because every invite reaches people who aren't on Ripls yet — they can join without downloading anything — nobody gets left out for being last to sign up. That repeat, and that reach, are what this round is built to prove.

## product.media-aria

A plan being generated live — title, time, place, and details filling in from a single sentence.

---

## measurement.headline  [bg 10]

Connection you can <em>actually measure.</em>

## measurement.body

Here's what makes this fundable and not just nice: every gathering, every favor, every loan throws off a signal, and we turn those signals into the measures the research has cared about for thirty years — the texture of real social capital, not vanity numbers. We've tracked them from day one, so as the beta grows you'll be able to watch a neighborhood's connection thicken month over month. So will a funder who wants to know their money moved a number that means something.

And these aren't private dashboards. The numbers belong to the group, and the group sees them — the way a team sees who showed up — so getting together becomes a team sport. The same social pull that gets you to the gym because a friend's already there is what gets the dinner to actually happen. The measure does the nudging; the people do the rest.

## measurement.figure-note

The number your dollars move — illustrative shape, tracked from day one in beta groups.

## measurement.metric.presence

Hours of real, in-person time with real people. The dose, basically — the thing every screen has been quietly taking.

## measurement.metric.reciprocity

Whether help flows both ways. A place where people both ask and give is the resilient kind, so we track the balance, not just the giving.

## measurement.metric.bridging

How often someone links two circles that would never otherwise meet. Putnam's bridging capital — finally something you can see.

---

## money.headline  [light]

It pays for itself, <em>so the impact compounds.</em>

## money.body

Most things in this space live grant to grant, and the good they do stalls the second the funding does. Ripls is built to stand on its own: when a group moves money through it — twelve neighbors putting in $40 each to rebuild a shared fence — we keep a small slice like any payment processor, and the clubs, leagues, and congregations that run this every week pay a flat monthly rate for the heavier tools. There are no ads, and no selling anyone's data.

So your dollars don't get spent and vanish. They buy a thing that keeps working, and keeps connecting people, long after the check clears.

---

## big-picture.headline  [bg 07]

Bowling alone was the warning. <em>This is where we bowl together again.</em>

## big-picture.body

For the first time the problem has a government advisory, a news cycle, and real money lining up behind it. The pandemic taught a whole generation what it feels like to lose their people, and AI is about to pull even more of ordinary life onto a screen and away from a kitchen table. The will to fix this is finally here — and the shelf where the solution should be is basically empty.

Putnam told us what we were losing twenty-five years ago, and we lost it anyway — the standing night, the neighbor you'd actually call, the quiet certainty that someone would show up if things went sideways. None of that comes back through a policy or a feed; it comes back one gathering at a time, in real places, between real people. Get there first with something that works, and you help set the standard for what the cure even looks like.

---

## team.headline  [bg 05]

Two people who helped build the problem, <em>now building the way out.</em>

## team.lead

We've spent our careers getting people to open an app. We'd like to spend the rest of them getting people to put one down.

## team.thomas.name

Thomas Escobar

## team.thomas.bio

Ran product as a director on Google Maps and led its biggest expansion in a decade; before that, helped push Google Drive past a billion users. And before Google, started a company in Beijing that built the world's largest open-government database.

## team.bryan.name

Bryan Klingner

## team.bryan.bio

Fifteen years at Google building computer vision for Street View and maps, most recently as a principal engineer. With a CS PhD from Berkeley, he's the one who takes a nasty research problem and turns it into something that actually ships.

## team.round

We're not going to dress a beta up as a breakout. Ripls is a working app in private beta, pre-traction by design, built on our own dime — no salaries, about <strong>$8K</strong> of actual cash spent. We're raising <strong>$1M</strong>, with the first <strong>$100K</strong> already committed on a SAFE, on the same terms as the rest, by a former Google director who worked alongside us and watched us build. The round buys roughly ten months: more communities, our first non-founder hire, and the compute the one-sentence product runs on — enough to show that connection compounds, that the social-capital numbers climb underneath it, and that the model starts carrying its own weight, so the connection outlasts the funding that started it.

---

## signup.headline  [light]

Want the deck?

## signup.subhead

We're keeping the list short — foundations and funders who already know connection is a health issue, and want to back the thing that rebuilds it, with the data to prove their dollars moved it. Tell us what pulls you in, and we'll send the deck and find a time.

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

The crisis. The measurement model. The product. The person who always organizes, in your own life.

## signup.button

Send me the deck

## signup.form-success

Got it. We'll be in touch soon with the deck and a time to talk.
