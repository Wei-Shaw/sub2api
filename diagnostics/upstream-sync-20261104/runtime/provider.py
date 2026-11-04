from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json

last = {}


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def send(self, value, status=200):
        raw = json.dumps(value).encode()
        self.send_response(status)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Content-Length', str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def do_GET(self):
        self.send(last if self.path == '/fixture/last' else {'data': []})

    def do_POST(self):
        global last
        body = json.loads(self.rfile.read(int(self.headers.get('Content-Length', 0))) or '{}')
        # These are fixture labels, never real account credentials.
        label = self.headers.get('Authorization', '').removeprefix('Bearer ')
        last = {'path': self.path, 'label': label, 'body': body}
        if str(body.get('model', '')).startswith('kimi-'):
            return self.send({'error': {'message': "You've reached your concurrent request limit. Please wait for your ongoing requests to finish and try again."}}, 403)
        if '/images/' in self.path:
            return self.send({'created': 1793779200, 'data': [{'b64_json': 'eA=='}, {'b64_json': 'eA=='}]})
        response = {'id': 'resp-fixture', 'object': 'response', 'status': 'completed',
                    'model': body.get('model'), 'instructions': body.get('instructions'),
                    'output': [{'type': 'message', 'role': 'assistant', 'content': [{'type': 'output_text', 'text': 'ok'}]}],
                    'usage': {'input_tokens': 1000, 'output_tokens': 100, 'total_tokens': 1100}}
        if not body.get('stream'):
            return self.send(response)
        self.send_response(200)
        self.send_header('Content-Type', 'text/event-stream')
        self.end_headers()
        for event in [{'type': 'response.created', 'response': {**response, 'output': [], 'status': 'in_progress'}},
                      {'type': 'response.output_text.delta', 'delta': 'ok'},
                      {'type': 'response.completed', 'response': response}]:
            self.wfile.write(('data: ' + json.dumps(event) + '\n\n').encode())
        self.wfile.write(b'data: [DONE]\n\n')
        self.wfile.flush()


ThreadingHTTPServer(('0.0.0.0', 8000), Handler).serve_forever()
