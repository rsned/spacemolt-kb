---
title: Test Station Guide
summary: A fixture guide for the generator tests.
intro: |
  Why build one: no rent. Stock [[flex_polymer]] first.
steps:
  - title: Found the station
    facts:
      - {label: Founding fee, value: "5,000,000 cr", credits: 5000000, basis: "measured 2026-10-25"}
    notes: |
      Gets its own POI.
  - title: Locker Bay
    facility: locker_bay
    notes: |
      Mentions [[nope]] which is not a real id.
  - title: Recycler
    facility: atmospheric_recycler
  - title: Power hog
    facility: power_hog
  - title: Power
    facility: stellar_collector_grid
    alternatives: [bunker_fed_reactor]
    measured:
      date: "2026-10-26"
      source: "storage diff"
      upkeep_per_cycle: {circuit_board: 2}
  - title: Fuel
    notes: |
      Not yet charted.
---
## Reference

Some reference text.
