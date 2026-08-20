# Holotable P1b — playback-at-scale findings

Measured 2026-08-20 against the two shipped fixtures — `a2619bbe…` (Node
Beta, 42 participants, 30 ticks) and a synthetic stress fixture,
`ffffffffffffffffffffffffffffffff…`, built by `scripts/make-stress-replay.js`
from Node Beta: **420 hulls, 600 ticks, 44.7MB**. Neither shipped fixture
reaches the scale a real battle can (373 participants, 264 ticks); the
synthetic fixture goes past that on both axes so the render loop is measured
somewhere real battles can't quite reach either. A third fixture, exported
later the same day directly from the live game
(`c79f7810a59437b029a6168526782fe4…`, 373 participants, 264 ticks, 22MB),
closes that gap — see "Real large battle" below.

## Screenshots

- `img/holotable-p1b/stress-opening-frame.jpg` — the stress fixture's opening
  frame (paused, `1 / 600`), four sides at 105 ships each.
- `img/holotable-p1b/stress-tick3.jpg` and `stress-tick15.jpg` — the same
  fixture, tick 1000003 vs. tick 1000015 (12 ticks apart) during 1×
  playback. Used below to confirm ships are actually moving, not just
  re-rendering in place.
- `img/holotable-p1b/node-beta-arrival-fade-mid.jpg` — Node Beta mid-transition
  between frame 7 and frame 8 (tick 1615392, readout `7 / 30`, `0.25×`), the
  SIDE 2 arrival referenced below.
- `img/holotable-p1b/node-beta-arrival-fade-after.jpg` — the same arrival a
  couple of ticks further on (tick 1615395), for contrast.

### How to see this yourself

```
node scripts/make-stress-replay.js
go run ./cmd/generate-battle-holotable --replay data/battles/ffffffffffffffffffffffffffffffff.json
cd kb && python3 -m http.server 8099
```

Then open, in a browser:

- `http://localhost:8099/battles/ffffffffffffffffffffffffffffffff.html?bench=300` — 420 participants, ms/frame
- `http://localhost:8099/battles/a2619bbe328676445828b4e1007fe9aa.html?bench=300` — 42 participants, ms/frame
- `http://localhost:8099/battles/ffffffffffffffffffffffffffffffff.html` — play at 1× and 4×
- `http://localhost:8099/battles/a2619bbe328676445828b4e1007fe9aa.html?tick=1615392` — the arrival-fade frame

A methodology note on how these numbers were actually collected: the
automated Chrome session used for this task reports `document.hidden ===
true` for its own tab regardless of clicks or focus, which is Chrome's
normal background-tab policy for `requestAnimationFrame` — it throttles or
suspends the callback entirely. The `?bench=N` figures below are unaffected
(`bench` calls `render()` directly in a `for` loop, no `requestAnimationFrame`
involved). For the live-playback checks (pacing, the arrival fade), a
temporary one-line monkeypatch of `window.requestAnimationFrame` to
`setTimeout(cb, 16)` was needed to get the loop to actually run in that
sandboxed tab; this changes *when* callbacks fire, not what `advance()` and
`render()` compute from real elapsed time, so the tick-pacing numbers below
are real wall-clock measurements, not synthetic ones. A normal foreground
browser tab does not need this workaround — noted here so the numbers below
are reproducible without it.

The tick-pacing windows themselves (the "~4.0s" figures below) were bounded
by a fixed-duration `wait` tool call followed by a separate state read, not
by a single continuous instrumented timer — so they carry a few hundred
milliseconds of unaccounted tool round-trip latency on top of the nominal
wait duration. That slack is the most likely explanation for the 1× figure
below reading faster than the 500ms nominal (the true elapsed window was
probably closer to 4.3–4.5s, not exactly 4.0s) rather than any actual
speed-up in playback.

## Findings

**Q1 — ms/frame at 420 participants and at 42, and what that extrapolates to.**

| fixture | participants | ms/frame (n=300) | fps ceiling |
|---|---|---|---|
| stress | 420 | **114.75** | 9 |
| Node Beta | 42 | **4.88** | 205 |

10× the participants costs **23.5×** the render time (114.75 / 4.88), not
10× — render cost is superlinear in hull count. `render()` is one
`drawImage` of the baked static layer plus `drawShips()`, which is a single
pass over `frame.ships` with no visible O(n²) loop in `holotable.js` (checked
directly — the ship-iterating loops are single `for` passes, not nested).
The likely explanation is Canvas 2D overdraw rather than draw-call count:
the stress fixture's four side clusters are visibly dense and heavily
overlapping (see `stress-opening-frame.jpg`), so antialiased fill/stroke
cost scales with *touched pixels*, not just shape count, and 420 overlapping
hulls touch a given pixel far more than 10× as often as 42 spread-out ones
do. This is a plausible explanation, not a confirmed one — profiling which
specific draw call dominates is follow-up work, not something this task
measured.

At the real ceiling (373 participants — numerically closer to 420 than to
42, off by 47 rather than 331; an earlier draft of this line had that
backwards), naive linear extrapolation from the 42-participant figure would
suggest ~43 ms/frame; the observed superlinear trend means the real number
is probably higher than that, plausibly closer to 60–90 ms/frame, though
this wasn't measured directly (no real 373-participant fixture had been
exported yet at the time this paragraph was written — see the brief's own
note on why: 24MB and a live run to get one). **It has since been measured;
see "Real large battle" below — 88.47 ms/frame, inside the predicted range
and near its top.**

Per the brief's guidance (comfortable under 33ms, needs reporting over
100ms): **114.75 ms/frame is over the 100ms line and is being reported, not
silently accepted.** At `MS_PER_TICK = 500` (1×), this still doesn't cost a
dropped tick — one render fits in about a quarter of a tick's screen time
(114.75 / 500 = 23%) — but it removes essentially all headroom at higher
speed multipliers (Q3 below).

**Q2 — the real rail-line-per-tick count on Node Beta (Task 4 Step 5).**

Already measured, reused here rather than re-derived: **840 raw chatter
entries → 620 rail lines over 30 ticks = 20.7 lines/tick** (201 grouped
reasons + 405 zone moves + 14 kills = 620). Grouping cut the raw chatter
from 28/tick to 6.7/tick as designed — a 21.3-line/tick reduction. Zone
moves, deliberately never grouped, add 13.5/tick — about twice the 6.7/tick
the grouped chatter itself now contributes, making zone moves the rail's
largest single source. Grouping did its job on the chatter it targeted; it
left the rail's total volume (20.7/tick) dominated by the moves it was
never meant to touch. This task did not re-measure it, but it bears
repeating here rather than treated as settled: whether 20.7 lines a tick is
actually readable during real-time playback is still an open judgment call,
not a solved problem.

**Q3 — does interpolation read as motion or as sliding?**

At 1× on the stress fixture, ticks advanced in step with real elapsed time:
9 ticks in ~4.0s of wall clock (≈444 ms/tick against a 500 ms nominal — the
render cost of 114.75 ms/frame fits comfortably inside a 500 ms tick, with
roughly 4× headroom). The 444ms figure reads faster than nominal, which is
measurement slack rather than a real speed-up — see the methodology note
above on why the "~4.0s" window is approximate, not an exact instrumented
interval. Screenshots 12 ticks apart
(`stress-tick3.jpg`/`stress-tick15.jpg`) show the four clusters visibly
denser and more contracted at tick 15 than at tick 3, consistent with the
fixture's designed 40-tick inward/outward cycle — the state is genuinely
advancing frame to frame, not stalled or repeating.

At 4× (125 ms/tick nominal), the same measurement gave 32 ticks in ~4.0s
(≈125 ms/tick — pacing held), but only because the render cost (114.75 ms)
leaves just 10.25 ms of slack against the 125 ms tick budget — about 8.2%
headroom. On the
actual hardware this was measured on, playback kept pace; on any slower
machine, or under any other load on the render thread, 4× at 420
participants is close enough to its own budget that a stutter — a visibly
dropped or delayed frame — is a real possibility, not a hypothetical one.
This is a genuine limit, not a comfortable one: **1× reads as motion at
stress scale; 4× is riding the edge of its own frame budget and could read
as a stutter under any less favorable conditions than this measurement's.**

This task did not get a continuous, real-time, eye-watched confirmation of
"smooth" at 420 participants — the tab-visibility workaround above
produces individual stills at controlled intervals, not a video a human
eye judges the way Task 7's live check did. For the underlying interpolation
mechanism itself (`interpolateFrame`, linear lerp, no easing), Task 7's own
Step 6 check on real (not synthetic) Node Beta data, watched live at normal
speed, already reported "hulls slide rather than jump between ticks — PASS."
Nothing in this task's numbers contradicts that; the new information here is
that render cost, not the interpolation math, is what's tight at scale.

**Q4 — does the 24-hull arrival at Node Beta frame 8 still read as arrival
with the fade?**

Yes, confirmed again here (`node-beta-arrival-fade-mid.jpg`, tick 1615392,
`0.25×`, mid-transition between frame 7 and frame 8): the newly-arriving
SIDE 2 ships are visibly fainter/lower-opacity than the established hulls
around them, consistent with `alpha: f` from `interpolateFrame`. This
matches Task 7's own Step 6 check (its item 4, reported PASS) — this task's
contribution is confirming the same mechanism still holds when exercised
from a scale-testing context, not a new result.

**Anything on the Task 7 Step 6 list that failed.**

Item 5 (scrubbing) originally failed in Task 7's own check: `setPlaying(false)`
called `syncControls()`, which overwrote `els.scrub.value` with the *current*
frame index before `seek()` read it, so a scrub was silently a no-op. Task 7
fixed this (commit `290c3560c`, "scrub reads the slider before pausing")
before this task began. Re-verified here at stress scale: setting
`els.scrub.value = '500'` and dispatching `input` moved the player straight
to `501 / 600` (tick `1000500`) in 13.4ms of synchronous JS time — scrubbing
is immediate, including on the 600-tick fixture, and the fix holds at 10×
the participant count it was fixed against.

No other item on that list failed in this task's testing. Items 1, 3, 6 and
7 were not the focus of this task (they're page-shell, rail-formatting, and
resize concerns already covered by Task 7) and nothing here contradicts
Task 7's PASS on them.

## Real large battle

Everything above used the two shipped fixtures — Node Beta (real but small,
42 participants) and the synthetic stress fixture (large but fabricated,
420 participants). Neither is what a busy real battle actually produces.
This section exports and measures one: `c79f7810a59437b029a6168526782fe4`,
a 373-participant, 264-tick battle in system GSC-0008.

**Export.** `craftsman-boss` was picked as the export agent: cross-checking
`ps aux | grep -E 'bin/worker|play_as'` against `data/agents/` found it
absent from the 160 running `--agent` processes, i.e. idle. The export
succeeded on the **first attempt**, no retries, no `session_replaced`:

```
bin/battle-export --agent craftsman-boss --battle c79f7810a59437b029a6168526782fe4 --limit 10 \
  --out data/battles/c79f7810a59437b029a6168526782fe4.json
```

86.8s wall clock, 27 ticks-of-10 fetch batches over the WebSocket. Output:
**22,180,165 bytes** (21.2 MiB / "22MB", in the ballpark of the brief's
~24MB estimate). `generate-battle-holotable` reported **373 participants,
10 ship classes, 0 without art, 0 frame-ambiguous, 0 with contract
problems** — every hull in a real 373-way battle resolved to known art on
the first pass, which the 42- and 420-participant fixtures already implied
but didn't prove at this scale.

**Load time (separate from frame cost, as the brief asked).** Measured two
ways on the local `python3 -m http.server 8099`, both from a fresh
`fetch()` with `cache: 'no-store'` and a cache-busting query string so
neither result is a disk-cache hit:

| stage | time |
|---|---|
| network transfer, body only (`PerformanceResourceTiming.duration`) | ~119 ms |
| `fetch()` header round-trip only | ~4 ms |
| `res.json()` — body download + `JSON.parse` of 22MB combined | ~254–258 ms |

The page's own `fetchJSON` calls `res.json()` directly, so **~258ms is the
realistic total** from request to a parsed, usable replay object on this
hardware. That's over half a tick (`MS_PER_TICK = 500`) spent just getting
the data in, before a single frame is drawn — on a page that otherwise
targets sub-tick render times, a quarter-second blocking load is a real cost,
just a one-time one rather than a per-frame one. **This number is
loopback-only** (client and `http.server` on the same machine, zero network
latency) — it says nothing about how a 22MB fetch behaves over a real
network path, which this task did not attempt to measure.

**ms/frame.** `?bench=300` reported:

```
bench: 300 frames, 88.47 ms/frame (11 fps ceiling), 373 participants
```

**88.47 ms/frame**, between the two synthetic-fixture bookends (4.88ms at
42 participants, 114.75ms at 420) and, notably, inside the 60–90ms range Q1
predicted before any real large fixture existed — near the top of that
range, not the middle. Naive linear extrapolation from the 42-participant
figure predicts 373/42 × 4.88 = 43.3 ms; the real figure is **2.04× that**,
consistent with the superlinear (overdraw-driven) scaling already
documented, not a new mechanism. Unlike the synthetic figure, 88.47 ms/frame
stays **under** the brief's 100ms reporting line, though not by a
comfortable margin — at `MS_PER_TICK = 500` it consumes about 18% of a
tick's budget at 1×, and would be tight at 4× (125ms budget) the same way
the 420-participant figure is, just slightly less so (70.8% vs 91.8% of
budget consumed).

Independent re-measurement note: a second `?bench=300` run, in a fresh
Chrome session run by review rather than this task, read **94.54 ms/frame**
against this section's 88.47 ms — about **7% apart**, same conclusion
(under the 100ms line, same order of magnitude). `bench()` is not a
zero-variance measurement; treat any single ms/frame figure in this doc as
accurate to roughly ±5-10%, not as an exact constant, and expect a
different machine or a different run to land a few ms either side of the
number quoted.

**Shape the synthetic fixture doesn't have.** The stress fixture is four
deliberately balanced, evenly-spaced clusters. The real battle is nothing
like that:

- **Wildly asymmetric sides**: side 1 has 12 participants, side 2 has 361
  (a 30:1 ratio) — not the synthetic fixture's even split. Side 1 won
  anyway (`outcome: "victory"`, `winning_side: 1`).
- **A station is a combat participant.** `Nyx Nexus Station` (kind:
  `"station"`) appears in `participants` with `max_hull: 120000`,
  `max_shield: 30000` — two orders of magnitude past any ship's hull pool
  — and an empty `ship_class` string. The generator's "0 without art" means
  the pipeline already special-cases stations rather than needing hull art
  for it; nothing here caught it unhandled, but it's the first real
  confirmation that the station-as-participant path the P1a spec called
  out is exercised by an actual station, not just a synthetic stand-in.
- **One hull class dominates**: 340 of 373 participants (91%) share a
  single `ship_class` ("shard"); the other 9 classes cover the remaining
  33. The synthetic fixture's four clusters don't model this kind of
  skew.
- **Narrow angular clustering, not spread quadrants.** The two sides'
  `bearing_mean` values (199.3° and 11.2°) sit roughly opposite each other
  but both are tight, not spread across a quadrant each — on the table this
  reads as a thin diagonal wedge of overlapping hulls rather than the
  stress fixture's four separated rings (visible directly at
  `?tick=busiest`, frame 22/264, tick 1463786 — screenshot not saved for
  this task, but reproducible with the command above).
- **Per-participant death detail** (`destroyed_at_tick`, `killed_by`) is
  populated throughout — 85 kills recorded in this battle alone — giving
  the rail genuinely varied chatter rather than the synthetic fixture's
  scripted 40-tick cycle.

`?tick=busiest` reached a valid frame (22/264) on this fixture with no
console errors, confirming that entry point generalizes past the two
fixtures it was built against, not just P1a's original screenshot target.

**Live playback watch — could not be observed.** The bench figure above
answers what one frame costs, not what the battle looks like in motion, so
a genuine playback watch at 1× and 4× was attempted on
`c79f7810a59437b029a6168526782fe4.html` (served the same way as
`?bench=300`). It failed to produce an observation, and the failure mode
is worth recording precisely rather than papered over:

- `document.hidden` read `true` for this tab throughout, including after
  clicking directly on the page (`hasFocus()` went `true`, `hidden` stayed
  `true`) — the same Chrome background-tab-group policy Task 8 hit.
- Clicking the play control (found via its accessible name, "Play / pause
  (space)", after two coordinate-based clicks missed — the automation
  viewport is 2177×1254 but screenshots render at ~1461×842, so
  eyeballed pixel coordinates land wrong without a ref) did flip the button
  to `⏸`, confirming the player's internal `playing` state was genuinely
  `true`, not that the click itself failed.
- With `playing === true` at 1×, a real 8-second wall-clock wait
  (`computer` tool `wait`, not a scripted delay) produced **zero** tick
  advancement: readout stayed `1 / 264`, tick stayed `1463765`.
- Switching `speed` to `4` and waiting a further real 8 seconds (16s total
  with the transport reporting "playing") still produced **zero**
  advancement.

This is a full stall, not a slowdown — `requestAnimationFrame` appears to
be suspended entirely for a hidden tab in this environment, not merely
throttled to a low rate the way some browsers throttle background tabs.
Per this task's explicit instruction, no synthetic pacing loop (e.g. the
`requestAnimationFrame`→`setTimeout` monkeypatch used elsewhere in this doc
for the stress fixture) was substituted to manufacture a playback
observation — that technique measures a proxy loop's timing, not whether
the real page plays smoothly in a real foreground tab, and mislabeling it
as "watched it play" would be worse than admitting the gap. **Net: whether
hulls slide or jump, whether the rail keeps pace, and whether anything
visibly stutters during real playback of the 373-participant battle remains
unverified by this task.** It needs a human eyeballing a real foreground
browser tab (or an automation environment that doesn't background its
tabs), neither of which was available here.

This limitation is not new to this fixture — it's the same one the
existing methodology note above (under "How to see this yourself")
already documents for the stress fixture, worked around there with the
labeled synthetic-loop technique. What's new here is that, on this task's
explicit instruction, no such workaround was used to paper over the gap
for the real-battle fixture, so the gap is reported as a gap rather than
closed with a proxy.

No other disappointments on this fixture beyond the load-time number
above, the live-playback gap just described, and the two documentation
corrections made in the course of writing this section (the "fifth of a
tick" → "quarter of a tick" wording, and "closer to 42 than to 420" → the
numerically correct opposite, both above). The frame cost landed inside
the predicted range, the export needed no retries, and the station and
hull-art paths held up against data no synthetic fixture had modeled.

## Summary

| check | result |
|---|---|
| ms/frame, 420 participants (synthetic) | 114.75 ms (9 fps ceiling) — over the 100ms reporting line |
| ms/frame, 373 participants (real battle) | 88.47 ms (11 fps ceiling) — under the line, 2.04× naive linear extrapolation; independent re-run read 94.54 ms (~7% apart, same conclusion) |
| ms/frame, 42 participants (Node Beta) | 4.88 ms (205 fps ceiling) |
| real-battle live playback (1× and 4×) | **not observed** — tab stayed `document.hidden` even focused; 0 tick advance over 8s real wait at each speed with `playing===true` — see "Live playback watch" above |
| scaling factor | 23.5× time for 10× participants (42→420) — superlinear, likely overdraw |
| real-battle export | craftsman-boss, first attempt, no retries; 22,180,165 bytes |
| real-battle load time (22MB, localhost) | ~119ms network + ~254-258ms incl. JSON.parse ≈ 258ms total, not measured over a real network |
| rail lines/tick, Node Beta | 20.7 (from Task 4) — still an open readability question |
| 1× pacing at 420 participants | holds, ~4× headroom |
| 4× pacing at 420 participants | holds on this hardware, ~8.2% headroom — fragile |
| interpolation quality (mechanism) | linear lerp, no popping — confirmed by Task 7's live check |
| frame-8 arrival fade | confirmed again, unchanged |
| scrub at stress scale | immediate (13.4ms), fix from Task 7 holds at 10× scale |
| real-battle data shape | 30:1 side split, station-as-participant (120k hull), 91% one hull class, narrow angular clustering — none modeled by the synthetic fixture |

The headline result is not unqualified good news, and it now rests on two
fixtures rather than one. At the synthetic stress scale (420 participants,
600 ticks) the renderer **works** but with most of its margin already
spent: 1× is comfortable, 4× is not comfortable, it's *currently adequate*.
At the scale a real battle actually produced (373 participants), the
number is better than the synthetic figure predicted worst-case — 88.47ms
(±~7% by independent re-measurement), under the 100ms reporting line,
versus the synthetic fixture's 114.75ms over it — so the real ceiling this
renderer needs to survive is, on the one battle measured, less punishing
than the stress test alone would suggest. But frame cost is only half of
what "acceptance" was supposed to establish: this task could not verify
that the real battle actually plays smoothly in a real browser tab (see
"Live playback watch" above — the automation tab stayed backgrounded and
`requestAnimationFrame` never fired, at either 1× or 4×, over 16 real
seconds of "playing" state). The renderer's per-frame cost is measured and
comfortable at real-battle scale; whether that cost translates into smooth
motion, a keeping-pace rail, and a responsive transport during actual
playback is still open, and closing it needs a human watching a real
foreground tab — not something this task's environment could produce.
