#!/usr/bin/env python3
"""Serve the built dist with SPA fallback AND proxy /api, /v1, /setup to the backend on :8080."""
import http.server, urllib.request, urllib.error, os

DIST = "/Users/mac/sub2api-src/backend/internal/web/dist"
UPSTREAM = "http://localhost:8080"
PROXY_PREFIXES = ("/api/", "/v1/", "/setup")

class Handler(http.server.SimpleHTTPRequestHandler):
    def __init__(self, *a, **kw):
        super().__init__(*a, directory=DIST, **kw)

    def _is_proxy(self):
        path = self.path.split("?", 1)[0]
        return any(path.startswith(p) for p in PROXY_PREFIXES)

    def _proxy(self):
        length = int(self.headers.get("Content-Length", 0) or 0)
        body = self.rfile.read(length) if length else None
        headers = {k: v for k, v in self.headers.items() if k.lower() not in ("host", "content-length")}
        req = urllib.request.Request(UPSTREAM + self.path, data=body, method=self.command, headers=headers)
        try:
            resp = urllib.request.urlopen(req)
            data = resp.read()
            self.send_response(resp.status)
            for k, v in resp.headers.items():
                if k.lower() not in ("transfer-encoding", "content-encoding", "connection"):
                    self.send_header(k, v)
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)
        except urllib.error.HTTPError as e:
            data = e.read()
            self.send_response(e.code)
            for k, v in e.headers.items():
                if k.lower() not in ("transfer-encoding", "content-encoding", "connection"):
                    self.send_header(k, v)
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)
        except Exception as e:
            self.send_response(502); self.end_headers(); self.wfile.write(str(e).encode())

    def do_GET(self):
        if self._is_proxy():
            return self._proxy()
        rel = self.path.lstrip("/").split("?")[0].split("#")[0]
        target = os.path.join(DIST, rel)
        if rel == "" or not os.path.exists(target) or os.path.isdir(target):
            self.path = "/index.html"
        return super().do_GET()

    def do_POST(self): self._proxy()
    def do_PUT(self): self._proxy()
    def do_DELETE(self): self._proxy()
    def do_PATCH(self): self._proxy()

if __name__ == "__main__":
    http.server.ThreadingHTTPServer(("0.0.0.0", 3001), Handler).serve_forever()
