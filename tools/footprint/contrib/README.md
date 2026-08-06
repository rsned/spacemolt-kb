# tools/footprint/contrib

Working scripts from the footprint-recovery effort (2026-07/08) that are worth
keeping runnable but are not part of the 7-stage pipeline proper. They were
born in a session scratchpad and got promoted here after a tmp cleanup nearly
lost them. All run under `~/moge-venv/bin/python` (same env as the pipeline);
paths are repo-relative, outputs land in the invoking directory.

| script | what it does |
|---|---|
| `contact_sheet.py` | THE review UI: 267-ship HTML sheet — hero thumb, view-axes triad, pipeline/mesh/tier panels, click-to-pick best panel, 12-way bow-direction clock, localStorage + Export (`footprint_annotations.json` to ~/Downloads). Squared mesh panels via `profile.snap_plateaus`. Needs hero art in `~/Downloads/chromakeys` + `~/Downloads`. |
| `apply_bow_orientations.py` | Applies user bow clicks (`data/footprints/user_annotations_*.json`) to store profiles nose-first (`orientation: bow_t0`), updating profile.json AND the embedded copies in report.json. |
| `resolve_all_upright.py` | Full-catalog mirror-plane re-solve from stage 1-3 artifacts (no MoGe re-run needed when mattes are unchanged); replicates the phase-A gates, calls production phase B, merges into report.json with a `.pre_upright` backup. |
| `square_demo.py` | Before/after eyeball demo for the plateau-snap squaring (now delegates to `profile.snap_plateaus`; approved 2026-08-06). |
| `upright_prior.py` | Prototype that validated the upright-art prior before it was productionized into `mirror.solve_from_view`. Historical. |
| `rerun_unkeyable.py` | Status-targeted batch re-run (used for the failed_unkeyable wave after the hue-key fix). |
| `key_diag.py` | Border hue/sat/key-fraction diagnostics that proved the 35 "unkeyable" images were shadowed magenta, not scenes. |
| `hue_key_proto.py` | Prototype of the shadow-tolerant hue key (now in `matte.py`). Historical. |
| `aspect_correlation.py` | Recovered aspect vs pkg/shipglyph aspect scatter/stats. |
| `view_directions.py` | Dumps per-ship solved viewing directions from camera.json. |

Gotchas (hard-won): report.json EMBEDS full profiles — any profile.json rewrite
must update it in lockstep. Don't regenerate the contact sheet while a batch is
rewriting cloud_resolved.npz (EOFError race). Contact-sheet localStorage keys:
`footprint_best_picks`, `footprint_bow_dirs` — Chrome extensions cannot read
file:// localStorage; the Export button is the only way out.
