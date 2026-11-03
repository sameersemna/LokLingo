"""Contract tests for the GPU sidecar HTTP shape (no model required)."""

import base64
import io
import os
import unittest

os.environ.setdefault("LOAD_MODEL_ON_STARTUP", "false")

from fastapi.testclient import TestClient
from PIL import Image

import server


class FakeEngine:
    model = object()
    load_error = None

    def infer(self, request):
        self.last_request = request
        return "<PAGE>\n<|det|>text [0,0,999,999]<|/det|>Hello", 7


class TestContextLength(unittest.TestCase):
    def test_v1_context_is_capped_by_output_budget(self):
        self.assertEqual(server._v1_context_length(1, 4096), 4864)
        self.assertEqual(server._v1_context_length(5, 20480), 22272)
        self.assertEqual(server._v1_context_length(200, 20000), 32768)


class TestServerContract(unittest.TestCase):
    def setUp(self):
        self.fake = FakeEngine()
        self.original = server.engine
        server.engine = self.fake
        self.client = TestClient(server.app)

    def tearDown(self):
        server.engine = self.original

    def _png_b64(self):
        buffer = io.BytesIO()
        Image.new("RGB", (10, 10), "white").save(buffer, format="PNG")
        return base64.b64encode(buffer.getvalue()).decode("ascii")

    def test_health(self):
        response = self.client.get("/health")
        self.assertEqual(response.status_code, 200)
        self.assertTrue(response.json()["loaded"])
        self.assertEqual(response.json()["backend"], "transformers")

    def test_models(self):
        response = self.client.get("/v1/models")
        self.assertEqual(response.status_code, 200)
        self.assertEqual(response.json()["data"][0]["id"], server.MODEL_ID)

    def test_infer(self):
        response = self.client.post(
            "/infer",
            json={
                "pages": [{"page_number": 1, "image_b64": self._png_b64()}],
                "prompt": "document parsing.",
                "crop_mode": True,
            },
        )
        self.assertEqual(response.status_code, 200)
        self.assertEqual(response.json()["text"], "<PAGE>\n<|det|>text [0,0,999,999]<|/det|>Hello")
        self.assertEqual(response.json()["token_count"], 7)



if __name__ == "__main__":
    unittest.main()
