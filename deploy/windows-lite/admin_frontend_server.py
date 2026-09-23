import argparse
import http.client
import mimetypes
import os
import traceback
from http.server import ThreadingHTTPServer, BaseHTTPRequestHandler
from pathlib import Path
from urllib.parse import urlsplit


HOP_BY_HOP_HEADERS = {
    "connection",
    "keep-alive",
    "proxy-authenticate",
    "proxy-authorization",
    "te",
    "trailers",
    "transfer-encoding",
    "upgrade",
}

PROXY_PREFIXES = (
    "/api/",
    "/v1/",
    "/antigravity",
    "/gemini",
    "/claude",
    "/anthropic",
    "/openai",
)


class AdminFrontendHandler(BaseHTTPRequestHandler):
    frontend_dir: Path
    upstream_host: str
    upstream_port: int

    server_version = "Sub2APIAdminFrontend/1.0"

    def do_GET(self):
        if self.should_proxy():
            return self.proxy_request()
        return self.serve_static()

    def do_HEAD(self):
        if self.should_proxy():
            return self.proxy_request()
        return self.serve_static(head_only=True)

    def do_POST(self):
        return self.proxy_request()

    def do_PUT(self):
        return self.proxy_request()

    def do_PATCH(self):
        return self.proxy_request()

    def do_DELETE(self):
        return self.proxy_request()

    def do_OPTIONS(self):
        return self.proxy_request()

    def should_proxy(self):
        path = urlsplit(self.path).path
        return path == "/health" or path.startswith(PROXY_PREFIXES)

    def serve_static(self, head_only=False):
        path = urlsplit(self.path).path
        if path == "/" or not path:
            rel_path = "index.html"
        else:
            rel_path = path.lstrip("/")

        target = (self.frontend_dir / rel_path).resolve()
        root = self.frontend_dir.resolve()

        if not str(target).startswith(str(root)) or not target.exists() or target.is_dir():
            target = root / "index.html"

        content_type = mimetypes.guess_type(str(target))[0] or "application/octet-stream"
        data = target.read_bytes()
        self.send_response(200)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(data)))
        self.send_header("Cache-Control", "no-cache" if target.name == "index.html" else "public, max-age=31536000")
        self.end_headers()
        if not head_only:
            self.wfile.write(data)

    def proxy_request(self):
        body_len = int(self.headers.get("Content-Length", "0") or "0")
        body = self.rfile.read(body_len) if body_len else None

        headers = {
            key: value
            for key, value in self.headers.items()
            if key.lower() not in HOP_BY_HOP_HEADERS and key.lower() != "host"
        }
        headers["Host"] = f"{self.upstream_host}:{self.upstream_port}"

        conn = http.client.HTTPConnection(self.upstream_host, self.upstream_port, timeout=120)
        try:
            conn.request(self.command, self.path, body=body, headers=headers)
            resp = conn.getresponse()
            payload = resp.read()

            self.send_response(resp.status, resp.reason)
            for key, value in resp.getheaders():
                if key.lower() not in HOP_BY_HOP_HEADERS:
                    self.send_header(key, value)
            self.end_headers()
            if self.command != "HEAD":
                self.wfile.write(payload)
        finally:
            conn.close()

    def log_message(self, fmt, *args):
        print("%s - - [%s] %s" % (self.client_address[0], self.log_date_time_string(), fmt % args), flush=True)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=18080)
    parser.add_argument("--upstream-host", default="127.0.0.1")
    parser.add_argument("--upstream-port", type=int, default=8080)
    parser.add_argument("--frontend-dir", default=str(Path(__file__).with_name("admin_frontend")))
    args = parser.parse_args()

    frontend_dir = Path(args.frontend_dir)
    if not (frontend_dir / "index.html").exists():
        raise SystemExit(f"index.html not found in {frontend_dir}")

    AdminFrontendHandler.frontend_dir = frontend_dir
    AdminFrontendHandler.upstream_host = args.upstream_host
    AdminFrontendHandler.upstream_port = args.upstream_port

    server = ThreadingHTTPServer((args.host, args.port), AdminFrontendHandler)
    print(f"Serving Sub2API admin frontend at http://{args.host}:{args.port}", flush=True)
    print(f"Proxying API requests to http://{args.upstream_host}:{args.upstream_port}", flush=True)
    server.serve_forever()


if __name__ == "__main__":
    try:
        main()
    except Exception:
        log_path = Path(__file__).with_name("logs") / "admin_frontend.crash.log"
        log_path.parent.mkdir(parents=True, exist_ok=True)
        log_path.write_text(traceback.format_exc(), encoding="utf-8")
        raise
