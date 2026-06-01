# Faction & Player Overlays

Add a logo/portrait, biography, and extra stats to your faction or player page —
content the game does not store. Edit these files and open a PR; on the next KB
regeneration your content appears on your page.

## How to add yours

1. Find your **ID** (a 32-character hash):
   - **If your page already exists in the KB** — open your faction or player
     page and copy the monospace hash shown under the name (e.g.
     `e3653eac2392899ee0ee1f93a945306d`).
   - **If you don't have a page yet** — see *"New here?"* below for how to get
     your ID.
2. Create a directory named by that ID:
   - Faction: `overlays/factions/<faction_id>/`
   - Player: `overlays/players/<player_id>/`
3. Add a `profile.md` (see format below). Optionally drop an image (`logo.png` /
   `portrait.jpg`) in the same directory.
4. Open a PR.

## New here? (no page yet)

Pages are generated only for factions and players the KB has actually observed
in the galaxy, so a brand-new character won't have a page (or a visible ID) right
away. Two ways forward:

- **Easiest — wait one cycle.** Once you've been active enough to be sighted,
  your faction/player page is created automatically on the next KB regeneration.
  Come back, copy your ID from the page, and follow the steps above.
- **Pre-stage it (advanced).** If you already know your stable in-game ID — the
  same hash the game uses to identify your player or faction — you can create the
  `overlays/.../<id>/` directory and open your PR now. The overlay stays dormant
  (the build logs it as "matches no current entity" and skips it) and then renders
  on your page automatically the first time you appear in the data. Double-check
  the ID: a typo'd ID simply never matches and never shows up.

Same applies to a **newly founded faction** — once it's registered and sighted it
gets a page; grab the `faction_id` from there, or pre-stage it under
`overlays/factions/<faction_id>/`.

## profile.md format

```
---
image: logo.png                # optional; a file in this same directory
image_alt: "Faction crest"     # optional alt text
stats:                         # optional; shown as a Profile table
  - label: Homeworld
    value: Krynn Prime
  - label: Founded (lore)
    value: 2387 AE
---

## Biography

Markdown here — paragraphs, **bold**, lists, [links](https://example.com).
This renders as the "About" section on your page.
```

All parts are optional. Images must be `.png`, `.jpg`, `.jpeg`, `.webp`, or
`.gif` (no SVG), named as a plain filename in the same directory. Keep them
**320×320 px max** for the time being (the page displays them small, and large
uploads may be rejected or downscaled later) — `.webp` is preferred to keep the
repo lean. Raw HTML in the body is ignored for safety.
