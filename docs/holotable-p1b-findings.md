# Holotable P1b — playback-at-scale findings

Measured 2026-08-20 against the two shipped fixtures — `a2619bbe…` (Node
Beta, 42 participants, 30 ticks) and a synthetic stress fixture,
`ffffffffffffffffffffffffffffffff…`, built by `scripts/make-stress-replay.js`
from Node Beta: **420 hulls, 600 ticks, 44.7MB**. Neither shipped fixture
reaches the scale a real battle can (373 participants, 264 ticks); the
synthetic fixture goes past that on both axes so the render loop is measured
somewhere real battles can't quite reach either.

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

At the real ceiling (373 participants, closer to 42 than to 420), naive
linear extrapolation from the 42-participant figure would suggest ~43
ms/frame; the observed superlinear trend means the real number is probably
higher than that, plausibly closer to 60–90 ms/frame, though this wasn't
measured directly (no real 373-participant fixture was exported for this
task — see the brief's own note on why: 24MB and a live run to get one).

Per the brief's guidance (comfortable under 33ms, needs reporting over
100ms): **114.75 ms/frame is over the 100ms line and is being reported, not
silently accepted.** At `MS_PER_TICK = 500` (1×), this still doesn't cost a
dropped tick — one render fits in a fifth of a tick's screen time — but it
removes essentially all headroom at higher speed multipliers (Q3 below).

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

## Summary

| check | result |
|---|---|
| ms/frame, 420 participants | 114.75 ms (9 fps ceiling) — over the 100ms reporting line |
| ms/frame, 42 participants | 4.88 ms (205 fps ceiling) |
| scaling factor | 23.5× time for 10× participants — superlinear, likely overdraw |
| rail lines/tick, Node Beta | 20.7 (from Task 4) — still an open readability question |
| 1× pacing at 420 participants | holds, ~4× headroom |
| 4× pacing at 420 participants | holds on this hardware, ~8.2% headroom — fragile |
| interpolation quality (mechanism) | linear lerp, no popping — confirmed by Task 7's live check |
| frame-8 arrival fade | confirmed again, unchanged |
| scrub at stress scale | immediate (13.4ms), fix from Task 7 holds at 10× scale |

The headline result is not unqualified good news: the renderer **works**
at 420 participants and 600 ticks, but it does so with most of its margin
already spent. 1× is comfortable. 4× is not comfortable, it's *currently
adequate* — a distinction worth keeping in mind before anyone builds a
higher-speed mode or a busier real battle on top of this without first
profiling where the superlinear render cost actually goes.
