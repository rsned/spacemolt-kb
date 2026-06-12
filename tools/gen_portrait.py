#!/usr/bin/env python3
"""Passenger portrait generator for the Spacemolt KB.

Implements the SMKB_PORTRAIT_CMD contract used by cmd/generate-factions-kb:

  - prompt: read from $PORTRAIT_PROMPT (falls back to stdin)
  - output: written as a PNG to $PORTRAIT_OUT
  - seed:   taken from $PORTRAIT_SEED for deterministic regeneration

Backend is HuggingFace `diffusers` on the GPU, selectable via PORTRAIT_BACKEND:

  flux  (default) — FLUX.1-schnell, NF4 transformer + 8-bit T5, resident on the
                    GPU (~14 GB VRAM, ~12 s/image at 4 steps). T5 reads the WHOLE
                    prompt, so the full bio counts with no token truncation. This
                    is the production backend.
  sdxl            — SDXL-Turbo, CLIP-only (77-token window). Faster/smaller but
                    cannot comprehend a long prompt past ~the first sentence
                    (CLIP's effective window), so appearance/bio detail is lost.

SDXL long-prompt handling (only relevant to PORTRAIT_BACKEND=sdxl): CLIP
truncates at 77 tokens. The prompt leads with the style/framing/appearance cue so
truncation keeps the portrait-defining part; PORTRAIT_COMPEL=1 chunks past 77
tokens via `compel`. FLUX needs none of this — T5 handles long prompts natively.

Wire it up with:

  export SMKB_PORTRAIT_CMD='~/sd-venv/bin/python ~/spacemolt/kb/tools/gen_portrait.py'

WARM-DAEMON MODE (default)
  The KB invokes this command once per passenger, which would otherwise reload
  the model (~10-15 s) every time. To avoid that, the first invocation spawns a
  background daemon that loads the model once and listens on a Unix socket;
  every later invocation is a thin client that ships the request over the socket
  and returns once the PNG exists. The daemon self-exits after PORTRAIT_IDLE
  seconds of inactivity, so it does not linger after a run.

  Set PORTRAIT_NO_DAEMON=1 to force a single in-process generation instead
  (simpler, but reloads the model every call).

  Run `... gen_portrait.py --daemon` to start the server in the foreground.

Tunable via environment variables (all optional):
  PORTRAIT_BACKEND  flux | sdxl            (default: flux)
  PORTRAIT_MODEL    HF model id            (default: per-backend, see below)
  PORTRAIT_STEPS    inference steps        (default: flux 4, sdxl 12)
  PORTRAIT_GUIDANCE classifier-free guide  (default: flux 0, sdxl 3.5)
  PORTRAIT_SIZE     square pixel size      (default: flux 1024, sdxl 512)
  PORTRAIT_NEGATIVE negative prompt        (default: an illustration-excluding
                                            list; only takes effect when
                                            guidance > 1)
  PORTRAIT_SOCK     daemon socket path     (default: $TMPDIR/smkb_portrait.sock)
  PORTRAIT_IDLE     daemon idle timeout s  (default: 300)
  PORTRAIT_NO_DAEMON  set to 1 to disable the daemon and generate in-process

PHOTOREAL DEFAULTS: pure SDXL-Turbo (guidance_scale=0, 4 steps) renders fast but
its style is a per-seed coin flip between painterly/comic and photoreal — a bare
negative prompt has no effect at guidance 0. The defaults here instead run Turbo
with light classifier-free guidance (3.5), a few more steps (12), and a default
negative prompt that excludes illustration styles, which pins a consistent
cinematic-photoreal look while staying fast (~2 s/image warm). Set
PORTRAIT_GUIDANCE=0 to restore the classic fast-but-inconsistent Turbo behavior.
"""

import json
import os
import socket
import subprocess
import sys
import tempfile
import time

# Default negative prompt: excludes the illustration/comic styles that Turbo
# otherwise drifts into. Only effective at guidance > 1 (see PHOTOREAL DEFAULTS).
DEFAULT_NEGATIVE = (
    # illustration / non-photographic styles
    "illustration, painting, drawing, comic book, cartoon, anime, sketch, "
    "cel shaded, flat colors, 2d, line art, thick outlines, ink lines, "
    "concept art, sci-fi book cover, poster, splash art, video game screenshot, "
    # framing: keep it a head-and-shoulders portrait, not a full-body scene
    "full body, full shot, wide shot, cowboy shot, distant figure, "
    "landscape, scenery, space scene, "
    # discourage cheesecake-armor renders of combat characters — but only with
    # GENDER-NEUTRAL terms. Strongly female-coded negatives (cleavage, bikini,
    # lingerie, pin-up) masculinize female subjects at guidance > 1, flipping a
    # woman fighter into a man, so they are deliberately excluded here.
    "revealing outfit, skimpy clothing, fantasy armor"
)


# ---------------------------------------------------------------------------
# Request assembly (shared by client and in-process paths)
# ---------------------------------------------------------------------------

def _read_prompt() -> str:
    prompt = os.environ.get("PORTRAIT_PROMPT", "").strip()
    if not prompt:
        prompt = sys.stdin.read().strip()
    if not prompt:
        sys.exit("gen_portrait: empty prompt (set $PORTRAIT_PROMPT or pipe on stdin)")
    return prompt


def _backend() -> str:
    return os.environ.get("PORTRAIT_BACKEND", "flux").strip().lower()


def _request_from_env() -> dict:
    out_path = os.environ.get("PORTRAIT_OUT", "").strip()
    if not out_path:
        sys.exit("gen_portrait: $PORTRAIT_OUT is required")
    # Per-backend defaults: FLUX is guidance-distilled (0), 4 steps, native 1024;
    # SDXL-Turbo runs light guidance (3.5), more steps (12), 512.
    if _backend() == "flux":
        steps_d, guide_d, size_d = "4", "0", "1024"
    else:
        steps_d, guide_d, size_d = "12", "3.5", "512"
    return {
        "prompt": _read_prompt(),
        "negative": os.environ.get("PORTRAIT_NEGATIVE", DEFAULT_NEGATIVE).strip(),
        "out": os.path.abspath(out_path),
        "seed": int(os.environ.get("PORTRAIT_SEED", "0") or "0"),
        "steps": int(os.environ.get("PORTRAIT_STEPS", steps_d)),
        "guidance": float(os.environ.get("PORTRAIT_GUIDANCE", guide_d)),
        "size": int(os.environ.get("PORTRAIT_SIZE", size_d)),
    }


def _sock_path() -> str:
    explicit = os.environ.get("PORTRAIT_SOCK")
    if explicit:
        return explicit
    # Key the socket by backend so a flux and an sdxl daemon never collide. For
    # sdxl, also key by compel: it is loaded once at daemon start and cannot be
    # toggled per request, so a compel-on request must not be served by a warm
    # compel-off daemon (which would truncate long prompts at CLIP's 77 tokens).
    backend = _backend()
    suffix = backend
    if backend == "sdxl":
        suffix += "_c" + ("1" if os.environ.get("PORTRAIT_COMPEL") == "1" else "0")
    return os.path.join(tempfile.gettempdir(), f"smkb_portrait_{suffix}.sock")


# ---------------------------------------------------------------------------
# Model backend
# ---------------------------------------------------------------------------

def _pad_to_same_length(torch, a, b):
    """Zero-pad the shorter of two (batch, seq, dim) embedding tensors along the
    sequence axis so both share a length (required when pairing prompt and
    negative-prompt embeddings)."""
    la, lb = a.shape[1], b.shape[1]
    if la == lb:
        return a, b
    if la < lb:
        a, b = b, a  # make `a` the longer; swap back on return
    pad = torch.zeros(b.shape[0], a.shape[1] - b.shape[1], b.shape[2],
                      device=b.device, dtype=b.dtype)
    b = torch.cat([b, pad], dim=1)
    return (b, a) if la < lb else (a, b)


class SDXLGenerator:
    """Loads the SDXL-Turbo pipeline once and renders requests (CLIP-only)."""

    def __init__(self):
        import torch
        from diffusers import AutoPipelineForText2Image

        if not torch.cuda.is_available():
            sys.exit("gen_portrait: CUDA GPU not available — check the torch install")

        self.torch = torch
        model = os.environ.get("PORTRAIT_MODEL", "stabilityai/sdxl-turbo")
        self.pipe = AutoPipelineForText2Image.from_pretrained(
            model, torch_dtype=torch.float16, variant="fp16"
        ).to("cuda")
        self.pipe.set_progress_bar_config(disable=True)
        self.compel = self._make_compel()

    def _make_compel(self):
        """Build a Compel encoder so prompts beyond CLIP's 77-token window are
        chunked and concatenated instead of truncated. Compel is OPT-IN
        (PORTRAIT_COMPEL=1): for portraits the trailing bio text tends to pull the
        model into wide scenic sci-fi illustration, so by default we let CLIP
        truncate to the leading style/framing/archetype cue (which now leads the
        prompt). Returns None when not opted in, for non-SDXL models, or if compel
        is unavailable."""
        if os.environ.get("PORTRAIT_COMPEL") != "1":
            return None
        # SDXL has two tokenizers/encoders and needs pooled embeds; bail otherwise.
        if not getattr(self.pipe, "tokenizer_2", None):
            return None
        try:
            from compel import Compel, ReturnedEmbeddingsType
        except ImportError:
            print("gen_portrait: compel not installed, using truncated prompts",
                  file=sys.stderr)
            return None
        return Compel(
            tokenizer=[self.pipe.tokenizer, self.pipe.tokenizer_2],
            text_encoder=[self.pipe.text_encoder, self.pipe.text_encoder_2],
            returned_embeddings_type=ReturnedEmbeddingsType.PENULTIMATE_HIDDEN_STATES_NON_NORMALIZED,
            requires_pooled=[False, True],
            truncate_long_prompts=False,
        )

    def render(self, req: dict) -> None:
        generator = self.torch.Generator(device="cuda").manual_seed(int(req["seed"]))
        negative = req.get("negative", "")
        kwargs = dict(
            num_inference_steps=int(req["steps"]),
            guidance_scale=float(req["guidance"]),
            height=int(req["size"]),
            width=int(req["size"]),
            generator=generator,
        )
        if self.compel is not None:
            embeds, pooled = self.compel(req["prompt"])
            if negative:
                neg_embeds, neg_pooled = self.compel(negative)
                # Conditioning and its negative must share a sequence length. compel's
                # own padding helper is broken for the multi-encoder SDXL wrapper, so
                # zero-pad the shorter of the two along the token axis here.
                embeds, neg_embeds = _pad_to_same_length(self.torch, embeds, neg_embeds)
                kwargs["negative_prompt_embeds"] = neg_embeds
                kwargs["negative_pooled_prompt_embeds"] = neg_pooled
            kwargs["prompt_embeds"] = embeds
            kwargs["pooled_prompt_embeds"] = pooled
        else:
            kwargs["prompt"] = req["prompt"]
            if negative:
                kwargs["negative_prompt"] = negative
        image = self.pipe(**kwargs).images[0]
        os.makedirs(os.path.dirname(req["out"]), exist_ok=True)
        image.save(req["out"])


class FluxGenerator:
    """FLUX.1-schnell resident on the GPU: NF4 transformer + 8-bit T5 (~14 GB
    VRAM). T5 reads the full prompt, so the whole bio is comprehended with no
    CLIP-style truncation. Guidance-distilled (guidance 0) with no negative
    prompt — the SDXL negative is ignored here."""

    def __init__(self):
        import torch
        from diffusers import FluxPipeline
        from transformers import T5EncoderModel, BitsAndBytesConfig

        if not torch.cuda.is_available():
            sys.exit("gen_portrait: CUDA GPU not available — check the torch install")

        self.torch = torch
        model = os.environ.get("PORTRAIT_MODEL", "magespace/FLUX.1-schnell-bnb-nf4")
        # Quantize T5 to 8-bit so the whole pipeline stays resident in 16 GB VRAM.
        t5 = T5EncoderModel.from_pretrained(
            model, subfolder="text_encoder_2",
            quantization_config=BitsAndBytesConfig(load_in_8bit=True),
            torch_dtype=torch.bfloat16,
        )
        self.pipe = FluxPipeline.from_pretrained(
            model, text_encoder_2=t5, torch_dtype=torch.bfloat16
        )
        # The NF4 transformer and 8-bit T5 are already placed on the GPU by
        # bitsandbytes; move the remaining (CPU-loaded) components onto it too.
        # We deliberately do NOT enable cpu-offload: it parks bf16 weights in
        # system RAM and thrashed swap on a 32 GB box. Resident is faster + safe.
        self.pipe.text_encoder.to("cuda")
        self.pipe.vae.to("cuda")
        self.pipe.set_progress_bar_config(disable=True)
        self.max_seq = int(os.environ.get("PORTRAIT_T5_TOKENS", "256"))

    def render(self, req: dict) -> None:
        generator = self.torch.Generator("cpu").manual_seed(int(req["seed"]))
        image = self.pipe(
            req["prompt"],
            guidance_scale=float(req.get("guidance", 0.0)),
            num_inference_steps=int(req["steps"]),
            max_sequence_length=self.max_seq,
            height=int(req["size"]),
            width=int(req["size"]),
            generator=generator,
        ).images[0]
        os.makedirs(os.path.dirname(req["out"]), exist_ok=True)
        image.save(req["out"])


def make_generator():
    """Construct the generator for the selected PORTRAIT_BACKEND."""
    return FluxGenerator() if _backend() == "flux" else SDXLGenerator()


# ---------------------------------------------------------------------------
# Daemon: load once, serve newline-delimited JSON over a Unix socket
# ---------------------------------------------------------------------------

def run_daemon() -> None:
    sock_path = _sock_path()
    idle = float(os.environ.get("PORTRAIT_IDLE", "300"))

    srv = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    try:
        srv.bind(sock_path)
    except OSError:
        # Another daemon already owns the socket; nothing to do.
        print("gen_portrait: daemon already running", file=sys.stderr)
        return
    srv.listen(8)
    print(f"gen_portrait: daemon listening on {sock_path}", file=sys.stderr)

    # Bind the socket before the (slow, possibly downloading) model load so
    # clients can connect immediately; their request simply blocks until ready.
    gen = make_generator()
    print("gen_portrait: model loaded, ready", file=sys.stderr)

    try:
        srv.settimeout(idle)
        while True:
            try:
                conn, _ = srv.accept()
            except socket.timeout:
                print("gen_portrait: idle timeout, daemon exiting", file=sys.stderr)
                break
            with conn:
                _serve_one(conn, gen)
    finally:
        srv.close()
        try:
            os.unlink(sock_path)
        except OSError:
            pass


def _serve_one(conn: socket.socket, gen) -> None:
    buf = b""
    while not buf.endswith(b"\n"):
        chunk = conn.recv(65536)
        if not chunk:
            return
        buf += chunk
    try:
        req = json.loads(buf.decode("utf-8"))
        gen.render(req)
        resp = {"ok": True}
        print(f"gen_portrait: wrote {req['out']} (seed={req['seed']})", file=sys.stderr)
    except Exception as exc:  # report failure to the client, keep serving
        resp = {"ok": False, "error": str(exc)}
        print(f"gen_portrait: error: {exc}", file=sys.stderr)
    conn.sendall((json.dumps(resp) + "\n").encode("utf-8"))


# ---------------------------------------------------------------------------
# Client: connect to (auto-spawning) daemon, ship one request
# ---------------------------------------------------------------------------

def _connect(sock_path: str, timeout: float) -> socket.socket | None:
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        try:
            s.connect(sock_path)
            return s
        except OSError:
            s.close()
            time.sleep(0.2)
    return None


def _spawn_daemon() -> None:
    log_path = os.path.join(tempfile.gettempdir(), "smkb_portrait_daemon.log")
    log = open(log_path, "ab")  # noqa: SIM115 — handed to the detached child
    subprocess.Popen(
        [sys.executable, os.path.abspath(__file__), "--daemon"],
        stdin=subprocess.DEVNULL,
        stdout=log,
        stderr=log,
        start_new_session=True,
    )


def run_client(req: dict) -> None:
    sock_path = _sock_path()
    # Fast path: an existing warm daemon.
    conn = _connect(sock_path, timeout=1.0)
    if conn is None:
        _spawn_daemon()
        # Generous wait: first launch may download model weights.
        conn = _connect(sock_path, timeout=900.0)
    if conn is None:
        sys.exit("gen_portrait: could not reach portrait daemon")

    with conn:
        conn.sendall((json.dumps(req) + "\n").encode("utf-8"))
        conn.shutdown(socket.SHUT_WR)
        buf = b""
        while not buf.endswith(b"\n"):
            chunk = conn.recv(65536)
            if not chunk:
                sys.exit("gen_portrait: daemon closed connection before responding")
            buf += chunk

    resp = json.loads(buf.decode("utf-8"))
    if not resp.get("ok"):
        sys.exit(f"gen_portrait: {resp.get('error', 'unknown daemon error')}")
    print(f"gen_portrait: wrote {req['out']} (seed={req['seed']})", file=sys.stderr)


# ---------------------------------------------------------------------------

def main() -> None:
    if "--daemon" in sys.argv[1:]:
        run_daemon()
        return

    req = _request_from_env()
    if os.environ.get("PORTRAIT_NO_DAEMON") == "1":
        make_generator().render(req)
        print(f"gen_portrait: wrote {req['out']} (seed={req['seed']})", file=sys.stderr)
        return
    run_client(req)


if __name__ == "__main__":
    main()
