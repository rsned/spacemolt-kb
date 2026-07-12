# Resources: "Station in System" Column

**Date:** 2026-07-03
**Status:** Approved

## Goal

On the Resources page (`kb/resources/index.html`), let readers see at a glance whether
each deposit's system contains a station (dock/sell/refuel point).

The Resources area is a single generated file: a summary header + TOC ("main page")
followed by one section per resource, each with a deposits table. Adding the column to
the per-resource deposit tables covers both views.

## Design

### Data (`cmd/generate-items-kb/resources.go`)

- Add `StationInSystem bool` to `ResourceEntry`.
- In `loadResourceEntries`, extend the SELECT with:
  `EXISTS(SELECT 1 FROM pois sp WHERE sp.system_id = s.id AND sp.type = 'station')`
  and scan it into the new field. Stations are POIs with `type = 'station'` in the
  knowledge DB (43 at time of writing).

### Presentation (template in the same file)

- New **Station** column immediately after **System ID** (it is a system-level
  attribute, so it sits with the system columns).
- Cell rendering, mirroring the existing Hidden column pattern:
  - has station: `<span class="badge badge-green">✓</span>`
  - no station: `<span class="text-muted">—</span>`
- Tables are already `class="sortable"`; no JS changes needed.

### Regeneration scoping (`cmd/generate-items-kb/main.go`)

- Add a `-resources-only` flag (mirrors `-systems-only`) that opens the knowledge DB
  and calls only `writeResourcePages`, so this change — and future resource-page
  regens — don't churn the whole site.
- Regenerate with `go run ./cmd/generate-items-kb -resources-only` and commit the
  updated `kb/resources/index.html` alongside the code.

## Out of Scope

- Station names/counts in the column (user chose the simple ✓/— badge).
- Any change to item detail pages, system pages, or the TOC.

## Verification

- `go build ./...`, `go test ./...`, `golangci-lint` clean.
- Regenerated `kb/resources/index.html` has the Station header in every deposit
  table, ✓ rows only for systems that actually contain a station POI (spot-check
  against `SELECT DISTINCT system_id FROM pois WHERE type='station'`), and — rows
  elsewhere.
