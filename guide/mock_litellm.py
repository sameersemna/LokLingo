import json
from http.server import BaseHTTPRequestHandler, HTTPServer


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path not in ("/models", "/v1/models"):
            self.send_response(404)
            self.end_headers()
            return

        self._json(
            200,
            {
                "data": [
                    {
                        "id": "mock-model",
                        "object": "model",
                        "created": 0,
                        "owned_by": "loklingo-ci",
                    }
                ]
            },
        )

    def do_POST(self):
        if self.path not in ("/chat/completions", "/v1/chat/completions"):
            self.send_response(404)
            self.end_headers()
            return

        length = int(self.headers.get("Content-Length", "0"))
        body = json.loads(self.rfile.read(length) or b"{}")
        messages = body.get("messages", [])
        prompt = messages[-1].get("content", "") if messages else ""
        translated = "Hallo Welt" if prompt.strip().lower() == "hello world" else f"DE: {prompt}"

        self._json(
            200,
            {
                "choices": [
                    {
                        "message": {
                            "role": "assistant",
                            "content": translated,
                        }
                    }
                ]
            },
        )

    def log_message(self, format, *args):
        return

    def _json(self, status, payload):
        data = json.dumps(payload).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)


if __name__ == "__main__":
    HTTPServer(("0.0.0.0", 4010), Handler).serve_forever()