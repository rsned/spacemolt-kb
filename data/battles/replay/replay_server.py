#!/usr/bin/env python3
"""Local game-API stand-in so the battle cinematic can replay saved battles.

Serves /api/battle/summary and /api/battle/log for any battle exported under
data/battles/ (top level, wildlife/, duels/), converted by export_to_api.py.
Every other request -- and any battle we don't hold -- is passed through to
the real server once, without retries (its rate limiter blocks the IP).

    python3 replay_server.py [--port 8090]
    # fork .env.local: NEXT_PUBLIC_GAMESERVER_URL=http://localhost:8090
"""
import argparse
import json
import sys
import urllib.error
import urllib.request
from datetime import datetime, timezone
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import parse_qs, urlsplit

sys.path.insert(0, str(Path(__file__).resolve().parent))
import export_to_api as e2a  # noqa: E402

BATTLES = Path(__file__).resolve().parent.parent
UPSTREAM = "https://game.spacemolt.com"
_cache = {}


def local_battle(battle_id):
    if battle_id in _cache:
        return _cache[battle_id]
    if not battle_id or not all(c in "0123456789abcdef" for c in battle_id):
        return None
    for folder in (BATTLES, BATTLES / "wildlife", BATTLES / "duels"):
        path = folder / f"{battle_id}.json"
        if path.exists():
            saved = datetime.fromtimestamp(path.stat().st_mtime, timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
            _cache[battle_id] = e2a.convert(json.loads(path.read_text()), ended_at=saved)
            return _cache[battle_id]
    return None


class Handler(BaseHTTPRequestHandler):
    def _cors(self):
        self.send_header("Access-Control-Allow-Origin", "*")
        self.send_header("Access-Control-Allow-Headers", "*")
        self.send_header("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")

    def _json(self, body, status=200):
        data = json.dumps(body).encode()
        self.send_response(status)
        self._cors()
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_OPTIONS(self):
        self.send_response(204)
        self._cors()
        self.end_headers()

    def do_GET(self):
        url = urlsplit(self.path)
        query = {k: v[0] for k, v in parse_qs(url.query).items()}
        battle = local_battle(query.get("battle_id", "")) if url.path in ("/api/battle/summary", "/api/battle/log") else None
        if battle and url.path == "/api/battle/summary":
            return self._json(battle[0])
        if battle and url.path == "/api/battle/log":
            tick_start = int(query.get("tick_start") or 0)
            limit = max(1, min(1000, int(query.get("limit") or 200)))
            return self._json(e2a.log_page(battle[1], battle[0]["battle_id"], tick_start, limit))
        self._passthrough()

    def _passthrough(self):
        request = urllib.request.Request(UPSTREAM + self.path, headers={"Accept": self.headers.get("Accept", "*/*")})
        try:
            upstream = urllib.request.urlopen(request, timeout=30)
        except urllib.error.HTTPError as error:
            upstream = error
        except (urllib.error.URLError, TimeoutError) as error:
            return self._json({"error": "upstream_unreachable", "message": str(error)}, 502)
        with upstream:
            self.send_response(upstream.status if hasattr(upstream, "status") else upstream.code)
            self._cors()
            content_type = upstream.headers.get("Content-Type", "application/octet-stream")
            self.send_header("Content-Type", content_type)
            if content_type.startswith("text/event-stream"):
                # Server-sent events: relay as they arrive; ends when either side closes.
                self.send_header("Cache-Control", "no-cache")
                self.end_headers()
                try:
                    while chunk := upstream.read1(4096):
                        self.wfile.write(chunk)
                        self.wfile.flush()
                except (BrokenPipeError, ConnectionResetError):
                    pass
                return
            body = upstream.read()
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

    def log_message(self, fmt, *args):
        sys.stderr.write(f"[replay] {self.command} {self.path[:120]} {args[1] if len(args) > 1 else ''}\n")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--port", type=int, default=8090)
    args = parser.parse_args()
    server = ThreadingHTTPServer(("127.0.0.1", args.port), Handler)
    print(f"replay server on http://localhost:{args.port} (local battles from {BATTLES}, else {UPSTREAM})", flush=True)
    server.serve_forever()


if __name__ == "__main__":
    main()
