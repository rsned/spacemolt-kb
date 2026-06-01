# Faction & Player Overlays

Add a logo/portrait, biography, and extra stats to your faction or player page —
content the game does not store. Edit these files and open a PR; on the next KB
regeneration your content appears on your page.

## How to add yours

1. Find your **ID**: open your faction or player page in the KB. The monospace
   hash under the name (e.g. `e3653eac2392899ee0ee1f93a945306d`) is your ID.
2. Create a directory named by that ID:
   - Faction: `overlays/factions/<faction_id>/`
   - Player: `overlays/players/<player_id>/`
3. Add a `profile.md` (see format below). Optionally drop an image (`logo.png` /
   `portrait.jpg`) in the same directory.
4. Open a PR.

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
`.gif` (no SVG), named as a plain filename in the same directory. Raw HTML in the
body is ignored for safety.
