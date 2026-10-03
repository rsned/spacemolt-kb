#!/usr/bin/env python3
"""Export an Earth plate reconstruction as a tectonics-lab keyframe bundle.

Reads the Merdith et al. (2021) / Müller et al. (2022) full-plate model with
pyGPlates, resolves the plate polygons at each time step, rasterises them
onto the cube-sphere grid used by pkg/tectonics, and writes the same
frames/<step>/{thickness,plate,feature}.png + manifest.json layout that
`tectonics-lab run` produces, so the viewer and the stats script treat Earth
like any other bundle.

  thickness.png  0.78 inside a reconstructed continent polygon, 0.22 elsewhere
  plate.png      dense plate index (first-appearance order, wraps at 255)
  feature.png    boundary type in the top 2 bits (1 ridge/rift, 2 subduction,
                 3 transform/fault), age bits 0

Run with the gplates venv:
  ~/gplates-venv/bin/python scripts/earth_plates_export.py \
      --model data/tectonics/earth-src/Muller_etal_2022_SE_1Ga_Opt_PlateMotionModel_v1.1 \
      --out data/tectonics/earth-nnr --face 256 --start 1000 --end 0 --step 5

See docs/BIBLIOGRAPHY.md for the data citations.
"""
import argparse
import json
import math
import os
import sys
import time

import numpy as np
import pygplates
from PIL import Image

RADIUS_KM = 6371.0
TOPOLOGY_FILES = [
    "1000-410-Topologies_Merdith_et_al.gpml",
    "1000-410-Convergence_Merdith_et_al.gpml",
    "1000-410-Divergence_Merdith_et_al.gpml",
    "1000-410-Transforms_Merdith_et_al.gpml",
    "410-250_plate_boundaries_Merdith_et_al.gpml",
    "250-0_plate_boundaries_Merdith_et_al.gpml",
    "TopologyBuildingBlocks_Merdith_et_al.gpml",
]
CONTINENTS_FILE = "shapes_continents_Merdith_et_al.gpml"
ROTATION_FILES = {
    "nnr": "optimisation/no_net_rotation_model.rot",
    "mantle": "optimisation/1000_0_rotfile_Merdith_et_al_optimised.rot",
    "paleomag": "1000_0_rotfile_Merdith_et_al.rot",
}

FEAT_NONE, FEAT_DIVERGENT, FEAT_CONVERGENT, FEAT_TRANSFORM = 0, 1, 2, 3
FEATURE_KIND = {
    "gpml:MidOceanRidge": FEAT_DIVERGENT,
    "gpml:ContinentalRift": FEAT_DIVERGENT,
    "gpml:SubductionZone": FEAT_CONVERGENT,
    "gpml:OrogenicBelt": FEAT_CONVERGENT,
    "gpml:Transform": FEAT_TRANSFORM,
    "gpml:Fault": FEAT_TRANSFORM,
    "gpml:FractureZone": FEAT_TRANSFORM,
}
CONTINENT_THICKNESS, OCEAN_THICKNESS = 0.78, 0.22
MAJOR_COUNT = 8

# Cube-sphere layout, mirroring pkg/planetgen/cubemap (DirToFaceUV, FaceUVToDir,
# crossCells). Faces: 0 +X, 1 -X, 2 +Y, 3 -Y, 4 +Z, 5 -Z.
CROSS_CELLS = [(2, 1), (0, 1), (1, 0), (1, 2), (1, 1), (3, 1)]


def face_uv_to_dir(face, u, v):
    sc, tc = 2 * u - 1, 2 * v - 1
    if face == 0:
        x, y, z = np.ones_like(sc), -tc, -sc
    elif face == 1:
        x, y, z = -np.ones_like(sc), -tc, sc
    elif face == 2:
        x, y, z = sc, np.ones_like(sc), tc
    elif face == 3:
        x, y, z = sc, -np.ones_like(sc), -tc
    elif face == 4:
        x, y, z = sc, -tc, np.ones_like(sc)
    else:
        x, y, z = -sc, -tc, -np.ones_like(sc)
    n = np.sqrt(x * x + y * y + z * z)
    return x / n, y / n, z / n


def pixel_dirs(S):
    """Unit direction of every pixel centre, flat index i = face*S*S + py*S + px."""
    px = (np.arange(S) + 0.5) / S
    u, v = np.meshgrid(px, px)  # u varies along columns (px), v along rows (py)
    xs, ys, zs = [], [], []
    for face in range(6):
        x, y, z = face_uv_to_dir(face, u.ravel(), v.ravel())
        xs.append(x)
        ys.append(y)
        zs.append(z)
    return np.concatenate(xs), np.concatenate(ys), np.concatenate(zs)


def dir_to_face_pixel(x, y, z, S):
    """Vectorised DirToFacePixel: returns flat indices."""
    ax, ay, az = np.abs(x), np.abs(y), np.abs(z)
    face = np.zeros_like(x, dtype=np.int64)
    sc = np.zeros_like(x)
    tc = np.zeros_like(x)
    ma = np.zeros_like(x)
    mx = (ax >= ay) & (ax >= az)
    my = ~mx & (ay >= az)
    mz = ~mx & ~my
    pos = x >= 0
    face[mx & pos], face[mx & ~pos] = 0, 1
    sc[mx & pos], tc[mx & pos] = -z[mx & pos], -y[mx & pos]
    sc[mx & ~pos], tc[mx & ~pos] = z[mx & ~pos], -y[mx & ~pos]
    ma[mx] = ax[mx]
    pos = y >= 0
    face[my & pos], face[my & ~pos] = 2, 3
    sc[my & pos], tc[my & pos] = x[my & pos], z[my & pos]
    sc[my & ~pos], tc[my & ~pos] = x[my & ~pos], -z[my & ~pos]
    ma[my] = ay[my]
    pos = z >= 0
    face[mz & pos], face[mz & ~pos] = 4, 5
    sc[mz & pos], tc[mz & pos] = x[mz & pos], -y[mz & pos]
    sc[mz & ~pos], tc[mz & ~pos] = -x[mz & ~pos], -y[mz & ~pos]
    ma[mz] = az[mz]
    u = 0.5 * (sc / ma + 1)
    v = 0.5 * (tc / ma + 1)
    pxi = np.clip(np.floor(u * S).astype(np.int64), 0, S - 1)
    pyi = np.clip(np.floor(v * S).astype(np.int64), 0, S - 1)
    return face * S * S + pyi * S + pxi


def xyz_to_latlon(x, y, z):
    """pyGPlates uses a geocentric frame with +Z through the north pole and
    +X through (lat 0, lon 0); our cube-sphere is the same frame."""
    lat = np.degrees(np.arcsin(np.clip(z, -1, 1)))
    lon = np.degrees(np.arctan2(y, x))
    return lat, lon


def write_cross(path, values, S):
    """values: uint8 array of 6*S*S, written as a 4S×3S RGBA cross (R=G=B=value for
    thickness is handled by the caller passing an RGB triple builder)."""
    img = np.zeros((3 * S, 4 * S, 4), dtype=np.uint8)
    img[..., 3] = 255
    for face in range(6):
        col, row = CROSS_CELLS[face]
        cell = values[face * S * S:(face + 1) * S * S].reshape(S, S)
        img[row * S:(row + 1) * S, col * S:(col + 1) * S, 0] = cell
    return img


def save_png(path, img):
    Image.fromarray(img, "RGBA").save(path, optimize=False, compress_level=6)


def partition_pixels(partitioner, lats, lons, lookup):
    """Return a plate id (or -1) per pixel using partition_geometry on one
    MultiPointOnSphere; `lookup` maps rounded (lat, lon) -> pixel index."""
    labels = np.full(len(lats), -1, dtype=np.int64)
    mp = pygplates.MultiPointOnSphere(list(zip(lats.tolist(), lons.tolist())))
    partitioned = []
    partitioner.partition_geometry(mp, partitioned)
    for plate, geoms in partitioned:
        pid = plate.get_feature().get_reconstruction_plate_id()
        for g in geoms:
            for pt in g.get_points():
                lat, lon = pt.to_lat_lon()
                labels[lookup[(round(lat, 5), round(lon, 5))]] = pid
    return labels


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--model", required=True, help="unzipped Müller 2022 model directory")
    ap.add_argument("--out", required=True, help="bundle directory to write")
    ap.add_argument("--face", type=int, default=256)
    ap.add_argument("--start", type=float, default=1000, help="oldest time, Ma")
    ap.add_argument("--end", type=float, default=0, help="youngest time, Ma")
    ap.add_argument("--step", type=float, default=5, help="Myr per frame")
    ap.add_argument("--frame", default="nnr", choices=sorted(ROTATION_FILES), help="absolute reference frame")
    ap.add_argument("--limit", type=int, default=0, help="only export the first N frames (testing)")
    args = ap.parse_args()

    S = args.face
    N = 6 * S * S
    model = args.model
    rot = pygplates.RotationModel(os.path.join(model, ROTATION_FILES[args.frame]))
    topo = [os.path.join(model, f) for f in TOPOLOGY_FILES]
    continents = pygplates.FeatureCollection(os.path.join(model, CONTINENTS_FILE))

    x, y, z = pixel_dirs(S)
    lats, lons = xyz_to_latlon(x, y, z)
    lookup = {(round(float(la), 5), round(float(lo), 5)): i for i, (la, lo) in enumerate(zip(lats, lons))}
    if len(lookup) != N:
        sys.exit(f"pixel lat/lon lookup collided: {len(lookup)} of {N}")

    times = []
    t = args.start
    while t >= args.end - 1e-9:
        times.append(round(t, 6))
        t -= args.step
    if args.limit:
        times = times[:args.limit]

    os.makedirs(os.path.join(args.out, "frames"), exist_ok=True)
    dense = {}  # gplates plate id -> dense index
    frames = []
    pixel_area = 1.0 / N
    t_begin = time.time()
    for step, t in enumerate(times):
        t0 = time.time()
        resolved = []
        pygplates.resolve_topologies(topo, rot, resolved, t)
        boundaries = [r for r in resolved if isinstance(r, pygplates.ResolvedTopologicalBoundary)]
        partitioner = pygplates.PlatePartitioner(boundaries, rot)
        labels = partition_pixels(partitioner, lats, lons, lookup)

        # continents -> thickness
        recon = []
        pygplates.reconstruct(continents, rot, recon, t)
        cont_polys = [r.get_reconstructed_geometry() for r in recon
                      if isinstance(r.get_reconstructed_geometry(), pygplates.PolygonOnSphere)]
        cont_part = pygplates.PlatePartitioner(
            [pygplates.Feature.create_reconstructable_feature(pygplates.FeatureType.gpml_unclassified_feature, p, reconstruction_plate_id=1)
             for p in cont_polys], rot, reconstruction_time=0)
        cont_labels = partition_pixels(cont_part, lats, lons, lookup) if cont_polys else np.full(N, -1)
        thickness = np.where(cont_labels >= 0, CONTINENT_THICKNESS, OCEAN_THICKNESS)

        # unlabelled pixels (gaps between topologies): nearest labelled neighbour by
        # repeated dilation over the flat index order is not seam-aware, so just
        # fill with the most common label (rare; report the count).
        gaps = int((labels < 0).sum())
        if gaps:
            vals, counts = np.unique(labels[labels >= 0], return_counts=True)
            labels[labels < 0] = vals[np.argmax(counts)]

        # dense plate index, stable across frames
        for pid in np.unique(labels):
            pid = int(pid)
            if pid not in dense:
                dense[pid] = len(dense) % 255
        dense_labels = np.vectorize(lambda p: dense[int(p)])(labels).astype(np.uint8)

        # boundary features
        feature = np.zeros(N, dtype=np.uint8)
        spacing = math.radians(90.0 / S) * 0.5
        for b in boundaries:
            for seg in b.get_boundary_sub_segments():
                kind = FEATURE_KIND.get(str(seg.get_feature().get_feature_type()), FEAT_NONE)
                if kind == FEAT_NONE:
                    continue
                geom = seg.get_resolved_geometry()
                try:
                    pts = geom.to_tessellated(spacing).get_points()
                except Exception:
                    pts = geom.get_points()
                arr = np.array([p.to_xyz() for p in pts])
                if len(arr) == 0:
                    continue
                idx = dir_to_face_pixel(arr[:, 0], arr[:, 1], arr[:, 2], S)
                feature[idx] = np.maximum(feature[idx], kind << 6)

        # plate rows
        rows = []
        areas = {}
        for b in boundaries:
            pid = b.get_feature().get_reconstruction_plate_id()
            areas[pid] = areas.get(pid, 0) + float((labels == pid).sum()) * pixel_area
        ranked = sorted(areas, key=lambda p: -areas[p])
        for b in boundaries:
            pid = b.get_feature().get_reconstruction_plate_id()
            if areas.get(pid, 0) == 0:
                continue
            c = b.get_resolved_boundary().get_boundary_centroid().to_xyz()
            stage = rot.get_rotation(t, pid, t + 1.0)  # moves the plate from t+1 to t
            lat, lon, ang = stage.get_lat_lon_euler_pole_and_angle_degrees()
            if ang < 0:
                lat, lon, ang = -lat, (lon + 180) % 360 - 180, -ang
            pole = pygplates.PointOnSphere(lat, lon).to_xyz()
            speed = math.radians(ang) * RADIUS_KM * 1e5 / 1e6  # cm/yr at 90° from the pole
            rows.append({"id": dense[pid], "gplates_id": pid, "name": b.get_feature().get_name(),
                         "centroid": list(c), "pole": list(pole), "speed_cm_yr": speed,
                         "area": areas[pid], "major": ranked.index(pid) < MAJOR_COUNT, "retired": False})
        rows.sort(key=lambda r: r["id"])

        rel = os.path.join("frames", f"{step:04d}")
        fdir = os.path.join(args.out, rel)
        os.makedirs(fdir, exist_ok=True)
        th8 = np.round(thickness * 255).astype(np.uint8)
        img = write_cross(None, th8, S)
        img[..., 1] = img[..., 0]
        img[..., 2] = img[..., 0]
        save_png(os.path.join(fdir, "thickness.png"), img)
        save_png(os.path.join(fdir, "plate.png"), write_cross(None, dense_labels, S))
        save_png(os.path.join(fdir, "feature.png"), write_cross(None, feature, S))
        frames.append({"step": step, "myr": args.start - t, "ma": t, "dir": rel, "plates": rows})
        print(f"frame {step:3d} t={t:7.1f} Ma  plates={len(areas)} gaps={gaps} {time.time()-t0:5.1f}s", flush=True)

    manifest = {
        "version": 1, "seed": 0, "planet": "earth", "archetype": "earth",
        "params": {"Archetype": "earth", "Face": S, "Steps": len(times) - 1, "KeyframeEvery": 1,
                   "TimelineMyr": args.start - args.end, "RadiusKm": RADIUS_KM},
        "face": S, "steps": len(times) - 1, "myr_per_step": args.step,
        "source": {"model": "Merdith et al. 2021 via Müller et al. 2022 v1.1", "frame": args.frame,
                   "licence": "CC-BY 4.0", "see": "docs/BIBLIOGRAPHY.md"},
        "plate_ids": {str(v): k for k, v in dense.items()},
        "frames": frames,
    }
    with open(os.path.join(args.out, "manifest.json"), "w") as f:
        json.dump(manifest, f, indent=1)
    print(f"wrote {len(frames)} frames to {args.out} in {time.time()-t_begin:.0f}s")


if __name__ == "__main__":
    main()
