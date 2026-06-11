#!/usr/bin/env python3
"""Passenger portrait generator for the Spacemolt KB.

Implements the SMKB_PORTRAIT_CMD contract used by cmd/generate-factions-kb:

  - prompt: read from $PORTRAIT_PROMPT (falls back to stdin)
  - output: written as a PNG to $PORTRAIT_OUT
  - seed:   taken from $PORTRAIT_SEED for deterministic regeneration

Backend is HuggingFace `diffusers` running Stable Diffusion on the GPU.
Defaults to SDXL-Turbo (fast, 1-4 steps, fits a 16 GB card comfortably).

Long prompts: when `compel` is installed and the model is SDXL, prompts longer
than CLIP's 77-token window are chunked and concatenated rather than truncated,
so the whole prompt counts. Set PORTRAIT_NO_COMPEL=1 to disable and fall back to
plain (truncated) prompts.

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
  PORTRAIT_MODEL    HF model id            (default: stabilityai/sdxl-turbo)
  PORTRAIT_STEPS    inference steps        (default: 4)
  PORTRAIT_GUIDANCE classifier-free guide  (default: 0.0 — correct for Turbo)
  PORTRAIT_SIZE     square pixel size      (default: 512)
  PORTRAIT_SOCK     daemon socket path     (default: $TMPDIR/smkb_portrait.sock)
  PORTRAIT_IDLE     daemon idle timeout s  (default: 300)
  PORTRAIT_NO_DAEMON  set to 1 to disable the daemon and generate in-process
"""

import json
import os
import socket
import subprocess
import sys
import tempfile
import time


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


def _request_from_env() -> dict:
    out_path = os.environ.get("PORTRAIT_OUT", "").strip()
    if not out_path:
        sys.exit("gen_portrait: $PORTRAIT_OUT is required")
    return {
        "prompt": _read_prompt(),
        "out": os.path.abspath(out_path),
        "seed": int(os.environ.get("PORTRAIT_SEED", "0") or "0"),
        "steps": int(os.environ.get("PORTRAIT_STEPS", "4")),
        "guidance": float(os.environ.get("PORTRAIT_GUIDANCE", "0.0")),
        "size": int(os.environ.get("PORTRAIT_SIZE", "512")),
    }


def _sock_path() -> str:
    return os.environ.get(
        "PORTRAIT_SOCK", os.path.join(tempfile.gettempdir(), "smkb_portrait.sock")
    )


# ---------------------------------------------------------------------------
# Model backend
# ---------------------------------------------------------------------------

class Generator:
    """Loads the diffusion pipeline once and renders requests."""

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
        chunked and concatenated instead of truncated. Returns None (falling back
        to plain prompts) for non-SDXL models, if compel is unavailable, or when
        PORTRAIT_NO_COMPEL is set."""
        if os.environ.get("PORTRAIT_NO_COMPEL") == "1":
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
        kwargs = dict(
            num_inference_steps=int(req["steps"]),
            guidance_scale=float(req["guidance"]),
            height=int(req["size"]),
            width=int(req["size"]),
            generator=generator,
        )
        if self.compel is not None:
            embeds, pooled = self.compel(req["prompt"])
            kwargs["prompt_embeds"] = embeds
            kwargs["pooled_prompt_embeds"] = pooled
        else:
            kwargs["prompt"] = req["prompt"]
        image = self.pipe(**kwargs).images[0]
        os.makedirs(os.path.dirname(req["out"]), exist_ok=True)
        image.save(req["out"])


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
    gen = Generator()
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


def _serve_one(conn: socket.socket, gen: Generator) -> None:
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
        Generator().render(req)
        print(f"gen_portrait: wrote {req['out']} (seed={req['seed']})", file=sys.stderr)
        return
    run_client(req)


if __name__ == "__main__":
    main()
