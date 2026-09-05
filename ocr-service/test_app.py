"""
OCR service unit + integration tests.

All heavy dependencies (PaddleOCR, pdf2image, numpy) are patched at the
module level so the test suite runs without GPU, Poppler, or model weights.

Run:
    python -m pytest test_app.py -v
"""
from __future__ import annotations

import base64
import json
import io
import os
import sys
import tempfile
import textwrap
import types
import unittest
import unittest.mock as mock

# ---------------------------------------------------------------------------
# Stub out heavy C extensions BEFORE app.py is imported so we don't need
# PaddleOCR, paddlepaddle, numpy, or pdf2image installed in the test env.
# ---------------------------------------------------------------------------

def _make_stub(name: str) -> types.ModuleType:
    mod = types.ModuleType(name)
    sys.modules[name] = mod
    return mod

# numpy stub
_np = _make_stub("numpy")
_np.array = lambda x, **kw: x  # type: ignore[attr-defined]

# paddleocr stub
_paddleocr = _make_stub("paddleocr")
_paddleocr.PaddleOCR = object  # type: ignore[attr-defined]

# pdf2image stub (real functions replaced per-test)
_pdf2image = _make_stub("pdf2image")
_pdf2image.convert_from_path = None  # type: ignore[attr-defined]
_pdf2image.pdfinfo_from_path = None  # type: ignore[attr-defined]

# Pillow: needs Image.Image to exist for isinstance checks
import PIL.Image  # noqa: E402 – PIL IS installed (Pillow dependency)

# Now import the app safely
import importlib  # noqa: E402

import app as ocr_app  # noqa: E402

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _fake_pil_image() -> PIL.Image.Image:
    return PIL.Image.new("RGB", (100, 50), color=(200, 200, 200))


def _make_ocr_result(texts: list[str], confidence: float = 0.9) -> list[list[tuple]]:
    """Fake PaddleOCR result structure: [page[line(bbox, (text, conf))]]."""
    lines = [
        ([[0, 0], [10, 0], [10, 10], [0, 10]], (t, confidence))
        for t in texts
    ]
    return [lines]


def _write_temp_pdf(content: bytes = b"%PDF-1.4\n%%EOF\n") -> str:
    fd, path = tempfile.mkstemp(suffix=".pdf")
    with os.fdopen(fd, "wb") as f:
        f.write(content)
    return path


# ---------------------------------------------------------------------------
# Tests: _parse_lang / _parse_dpi helpers
# ---------------------------------------------------------------------------

class TestParseLang(unittest.TestCase):
    def test_valid_code(self):
        self.assertEqual(ocr_app._parse_lang("de"), "de")

    def test_empty_string_defaults_auto(self):
        self.assertEqual(ocr_app._parse_lang(""), "auto")

    def test_none_defaults_auto(self):
        self.assertEqual(ocr_app._parse_lang(None), "auto")

    def test_whitespace_defaults_auto(self):
        self.assertEqual(ocr_app._parse_lang("   "), "auto")

    def test_strips_whitespace(self):
        self.assertEqual(ocr_app._parse_lang("  en  "), "en")


class TestParseDpi(unittest.TestCase):
    def test_default_none(self):
        self.assertEqual(ocr_app._parse_dpi(None), 200)

    def test_valid_int(self):
        self.assertEqual(ocr_app._parse_dpi(150), 150)

    def test_valid_string(self):
        self.assertEqual(ocr_app._parse_dpi("300"), 300)

    def test_below_min_raises(self):
        from fastapi import HTTPException
        with self.assertRaises(HTTPException) as ctx:
            ocr_app._parse_dpi(50)
        self.assertEqual(ctx.exception.status_code, 422)

    def test_above_max_clamps(self):
        result = ocr_app._parse_dpi(401)
        self.assertEqual(result, ocr_app.MAX_PDF_DPI)

    def test_non_numeric_raises(self):
        from fastapi import HTTPException
        with self.assertRaises(HTTPException) as ctx:
            ocr_app._parse_dpi("bad")
        self.assertEqual(ctx.exception.status_code, 400)


# ---------------------------------------------------------------------------
# Tests: _summarise
# ---------------------------------------------------------------------------

class TestSummarise(unittest.TestCase):
    def _make_blocks(self, entries: list[tuple[str, float]]) -> list:
        return [
            ocr_app.TextBlock(
                text=t,
                confidence=c,
                bbox=[0.0, 0.0, 1.0, 1.0],
            )
            for t, c in entries
        ]

    def test_empty_blocks(self):
        text, conf = ocr_app._summarise([])
        self.assertEqual(text, "")
        self.assertEqual(conf, 0.0)

    def test_single_block(self):
        text, conf = ocr_app._summarise(self._make_blocks([("hello", 0.9)]))
        self.assertEqual(text, "hello")
        self.assertAlmostEqual(conf, 0.9, places=4)

    def test_multiple_blocks_joined_by_newline(self):
        text, conf = ocr_app._summarise(
            self._make_blocks([("line1", 0.8), ("line2", 1.0)])
        )
        self.assertEqual(text, "line1\nline2")
        self.assertAlmostEqual(conf, 0.9, places=4)


class TestOCRImageParsingCompatibility(unittest.TestCase):
    def test_ocr_pil_image_parses_v3_dict_shape(self):
        fake_ocr = mock.MagicMock()
        fake_ocr.ocr = mock.MagicMock(return_value=[{
            "rec_texts": ["日本語", "English"],
            "rec_scores": [0.99, 0.85],
            "dt_polys": [
                [[10, 10], [40, 10], [40, 30], [10, 30]],
                [[50, 20], [100, 20], [100, 40], [50, 40]],
            ],
        }])
        with mock.patch.object(ocr_app, "get_ocr", return_value=fake_ocr):
            blocks = ocr_app._ocr_pil_image(_fake_pil_image(), "auto")

        self.assertEqual(len(blocks), 2)
        self.assertEqual(blocks[0].text, "日本語")
        self.assertEqual(blocks[1].text, "English")
        self.assertEqual(blocks[0].bbox, [10.0, 10.0, 40.0, 30.0])
        self.assertEqual(blocks[0].reading_order, 1)
        self.assertEqual(blocks[1].reading_order, 2)

    def test_ocr_pil_image_falls_back_on_low_confidence(self):
        fake_ocr = mock.MagicMock()
        fake_ocr.ocr = mock.MagicMock(side_effect=[
            _make_ocr_result(["uncertain"], confidence=0.25),
            _make_ocr_result(["rescued"], confidence=0.95),
        ])
        with mock.patch.object(ocr_app, "get_ocr", return_value=fake_ocr):
            blocks = ocr_app._ocr_pil_image(_fake_pil_image(), "auto", source_dpi=200, min_confidence=0.8)

        self.assertEqual(len(blocks), 1)
        self.assertEqual(blocks[0].text, "rescued")
        self.assertEqual(blocks[0].reading_order, 1)

    def test_ocr_image_can_export_json_and_debug_overlay(self):
        fake_ocr = mock.MagicMock()
        fake_ocr.ocr = mock.MagicMock(return_value=_make_ocr_result(["debuggable"], confidence=0.92))
        buffer = io.BytesIO()
        _fake_pil_image().save(buffer, format="PNG")

        with tempfile.TemporaryDirectory() as temp_dir:
            image_debug_dir = os.path.join(temp_dir, "debug")
            json_path = os.path.join(temp_dir, "ocr.json")
            req = ocr_app.OCRImageRequest(
                image_b64=base64.b64encode(buffer.getvalue()).decode("ascii"),
                lang="auto",
                min_confidence=0.8,
                export_json_path=json_path,
                debug_output_dir=image_debug_dir,
            )
            with mock.patch.object(ocr_app, "get_ocr", return_value=fake_ocr):
                resp = ocr_app.ocr_image(req)

            self.assertTrue(os.path.exists(resp.exported_json_path or ""))
            self.assertTrue(os.path.exists(resp.debug_image_path or ""))
            with open(resp.exported_json_path or "", "r", encoding="utf-8") as f:
                payload = json.load(f)
            self.assertIn("debuggable", payload["text"])


# ---------------------------------------------------------------------------
# Tests: _ocr_pdf_file — page-by-page processing
# ---------------------------------------------------------------------------

class TestOcrPdfFile(unittest.TestCase):
    """Patches pdf2image functions and _ocr_pil_image to avoid real deps."""

    def setUp(self):
        self.tmp_pdf = _write_temp_pdf()

    def tearDown(self):
        if os.path.exists(self.tmp_pdf):
            os.remove(self.tmp_pdf)

    def _patch_infofn(self, page_count: int):
        return mock.patch.object(
            ocr_app,
            "pdfinfo_from_path",
            return_value={"Pages": str(page_count)},
        )

    def _patch_convert(self, images_per_call: list[PIL.Image.Image]):
        """Return exactly one rendered page path per convert_from_path call."""
        self.rendered_paths: list[str] = []

        def capture_convert(*args, **kwargs):
            render_dir = kwargs["output_folder"]
            page_num = kwargs["first_page"]
            image = images_per_call[page_num - 1]
            path = os.path.join(render_dir, f"page-{page_num}.png")
            image.save(path)
            self.rendered_paths.append(path)
            return [path]

        return mock.patch.object(
            ocr_app,
            "convert_from_path",
            side_effect=capture_convert,
        )

    def _patch_ocr(self, texts_per_page: list[list[str]]):
        page_results = [_make_ocr_result(texts) for texts in texts_per_page]
        call_iter = iter(page_results)
        fake_ocr_instance = mock.MagicMock()
        fake_ocr_instance.ocr = mock.MagicMock(side_effect=lambda *a, **kw: next(call_iter))
        return mock.patch.object(ocr_app, "get_ocr", return_value=fake_ocr_instance)

    # ---- basic single-page ----

    def test_single_page_text_and_response_shape(self):
        img = _fake_pil_image()
        with self._patch_infofn(1), \
             self._patch_convert([img]), \
             self._patch_ocr([["Hello world"]]):
            result = ocr_app._ocr_pdf_file(self.tmp_pdf, "auto", 200)

        self.assertEqual(len(result.pages), 1)
        self.assertIn("Hello world", result.text)
        self.assertEqual(result.pages[0].page_number, 1)
        self.assertGreater(result.pages[0].confidence, 0.0)

    # ---- multi-page joined by newline ----

    def test_multi_page_text_joined(self):
        imgs = [_fake_pil_image() for _ in range(3)]
        with self._patch_infofn(3), \
             self._patch_convert(imgs), \
             self._patch_ocr([["Page one"], ["Page two"], ["Page three"]]):
            result = ocr_app._ocr_pdf_file(self.tmp_pdf, "auto", 200)

        self.assertEqual(len(result.pages), 3)
        self.assertEqual(result.text, "Page one\nPage two\nPage three")

    # ---- pages processed one at a time (first_page == last_page) ----

    def test_convert_called_page_by_page(self):
        imgs = [_fake_pil_image() for _ in range(3)]
        convert_calls: list[dict] = []

        def capture_convert(*args, **kwargs):
            convert_calls.append(kwargs)
            page_num = kwargs["first_page"]
            path = os.path.join(kwargs["output_folder"], f"page-{page_num}.png")
            imgs[page_num - 1].save(path)
            return [path]

        with self._patch_infofn(3), \
             mock.patch.object(ocr_app, "convert_from_path", side_effect=capture_convert), \
             self._patch_ocr([["A"], ["B"], ["C"]]):
            ocr_app._ocr_pdf_file(self.tmp_pdf, "auto", 200)

        self.assertEqual(len(convert_calls), 3)
        for i, call in enumerate(convert_calls, start=1):
            self.assertEqual(call["first_page"], i, f"first_page mismatch on iteration {i}")
            self.assertEqual(call["last_page"], i, f"last_page mismatch on iteration {i}")

    # ---- render files cleaned up after each page ----

    def test_rendered_page_files_cleaned_up_after_each_page(self):
        imgs = [_fake_pil_image() for _ in range(2)]

        with self._patch_infofn(2), \
             self._patch_convert(imgs), \
             self._patch_ocr([["X"], ["Y"]]):
            ocr_app._ocr_pdf_file(self.tmp_pdf, "auto", 200)

        self.assertEqual(len(self.rendered_paths), 2)
        for path in self.rendered_paths:
            self.assertFalse(os.path.exists(path), f"expected render path to be cleaned up: {path}")

    # ---- confidence average ----

    def test_confidence_average_across_pages(self):
        imgs = [_fake_pil_image(), _fake_pil_image()]
        # Page 1: one block at conf 1.0 ; Page 2: one block at conf 0.0
        p1 = [[([[0,0],[1,0],[1,1],[0,1]], ("A", 1.0))]]
        p2 = [[([[0,0],[1,0],[1,1],[0,1]], ("B", 0.0))]]
        fake_ocr = mock.MagicMock()
        results = iter([p1, p2])
        fake_ocr.ocr = mock.MagicMock(side_effect=lambda *a, **kw: next(results))

        with self._patch_infofn(2), \
             self._patch_convert(imgs), \
             mock.patch.object(ocr_app, "get_ocr", return_value=fake_ocr):
            result = ocr_app._ocr_pdf_file(self.tmp_pdf, "auto", 200, min_confidence=0.0)

        self.assertAlmostEqual(result.confidence, 0.5, places=2)

    # ---- empty page doesn't break the run ----

    def test_empty_page_produces_empty_text(self):
        img = _fake_pil_image()
        with self._patch_infofn(1), \
             self._patch_convert([img]), \
             self._patch_ocr([[]]):   # OCR returns no blocks
            result = ocr_app._ocr_pdf_file(self.tmp_pdf, "auto", 200)

        self.assertEqual(result.text, "")
        self.assertEqual(result.confidence, 0.0)
        self.assertEqual(len(result.pages), 1)

    # ---- invalid PDF raises 422 ----

    def test_invalid_pdf_metadata_raises_422(self):
        from fastapi import HTTPException
        with mock.patch.object(ocr_app, "pdfinfo_from_path", side_effect=Exception("bad pdf")):
            with self.assertRaises(HTTPException) as ctx:
                ocr_app._ocr_pdf_file(self.tmp_pdf, "auto", 200)
        self.assertEqual(ctx.exception.status_code, 422)

    def test_page_limit_raises_413(self):
        from fastapi import HTTPException

        with self._patch_infofn(201):
            with self.assertRaises(HTTPException) as ctx:
                ocr_app._ocr_pdf_file(self.tmp_pdf, "auto", 200, max_pages=200)

        self.assertEqual(ctx.exception.status_code, 413)
        self.assertIn("max page count", ctx.exception.detail)


# ---------------------------------------------------------------------------
# Tests: _write_upload_to_temp_pdf — temp file management
# ---------------------------------------------------------------------------

class TestWriteUploadToTempPdf(unittest.TestCase):
    def _make_upload(self, content: bytes, filename: str = "test.pdf"):
        upload = mock.MagicMock()
        upload.filename = filename
        upload.file = io.BytesIO(content)
        return upload

    def test_creates_temp_file_with_pdf_suffix(self):
        upload = self._make_upload(b"%PDF-1.4")
        path = ocr_app._write_upload_to_temp_pdf(upload)
        try:
            self.assertTrue(path.endswith(".pdf"))
            self.assertTrue(os.path.exists(path))
            with open(path, "rb") as f:
                self.assertEqual(f.read(), b"%PDF-1.4")
        finally:
            os.remove(path)

    def test_temp_file_removed_is_callers_responsibility(self):
        upload = self._make_upload(b"%PDF-1.4")
        path = ocr_app._write_upload_to_temp_pdf(upload)
        self.assertTrue(os.path.exists(path))
        os.remove(path)
        self.assertFalse(os.path.exists(path))

    def test_preserves_original_extension(self):
        upload = self._make_upload(b"%PDF-1.7", filename="document.PDF")
        path = ocr_app._write_upload_to_temp_pdf(upload)
        try:
            self.assertTrue(path.lower().endswith(".pdf"))
        finally:
            os.remove(path)

    def test_defaults_to_pdf_suffix_when_no_extension(self):
        upload = self._make_upload(b"%PDF-1.4", filename="noext")
        path = ocr_app._write_upload_to_temp_pdf(upload)
        try:
            self.assertTrue(path.endswith(".pdf"))
        finally:
            os.remove(path)

    def test_size_limit_raises_413_and_cleans_temp_file(self):
        from fastapi import HTTPException

        upload = self._make_upload(b"x" * 11)
        real_named_temporary_file = tempfile.NamedTemporaryFile

        with tempfile.TemporaryDirectory() as temp_dir:
            def named_temp_file_in_dir(*args, **kwargs):
                return real_named_temporary_file(*args, dir=temp_dir, **kwargs)

            with mock.patch.object(
                ocr_app.tempfile,
                "NamedTemporaryFile",
                side_effect=named_temp_file_in_dir,
            ):
                with self.assertRaises(HTTPException) as ctx:
                    ocr_app._write_upload_to_temp_pdf(upload, max_bytes=10)

            self.assertEqual(ctx.exception.status_code, 413)
            self.assertEqual(os.listdir(temp_dir), [])


class TestResolveSharedPdfPath(unittest.TestCase):
    def test_requires_shared_storage_enabled(self):
        with mock.patch.object(ocr_app, "OCR_SHARED_STORAGE_DIR", ""):
            with self.assertRaises(Exception) as ctx:
                ocr_app._resolve_shared_pdf_path("/tmp/loklingo/sample.pdf")
        self.assertEqual(ctx.exception.status_code, 400)

    def test_rejects_path_outside_shared_root(self):
        with tempfile.TemporaryDirectory() as shared_dir, tempfile.NamedTemporaryFile(suffix=".pdf") as temp_pdf:
            with mock.patch.object(ocr_app, "OCR_SHARED_STORAGE_DIR", os.path.realpath(shared_dir)):
                with self.assertRaises(Exception) as ctx:
                    ocr_app._resolve_shared_pdf_path(temp_pdf.name)
        self.assertEqual(ctx.exception.status_code, 400)

    def test_resolves_valid_path_inside_shared_root(self):
        with tempfile.TemporaryDirectory() as shared_dir:
            file_path = os.path.join(shared_dir, "sample.pdf")
            with open(file_path, "wb") as f:
                f.write(b"%PDF-1.4")
            with mock.patch.object(ocr_app, "OCR_SHARED_STORAGE_DIR", os.path.realpath(shared_dir)):
                resolved = ocr_app._resolve_shared_pdf_path(file_path)
        self.assertEqual(resolved, os.path.realpath(file_path))


# ---------------------------------------------------------------------------
# Integration: FastAPI /ocr/pdf endpoint via httpx TestClient
# ---------------------------------------------------------------------------

class TestOcrPdfEndpoint(unittest.TestCase):
    """Full request cycle through the FastAPI app, PDF logic still mocked."""

    @classmethod
    def setUpClass(cls):
        try:
            from fastapi.testclient import TestClient
            cls.client = TestClient(ocr_app.app, raise_server_exceptions=True)
        except ImportError:
            raise unittest.SkipTest("fastapi[testclient] not available")

    def _post_pdf(self, content: bytes = b"%PDF-1.4\n%%EOF\n",
                  lang: str = "auto", dpi: str = "200"):
        return self.client.post(
            "/ocr/pdf",
            files={"file": ("sample.pdf", content, "application/pdf")},
            data={"lang": lang, "dpi": dpi},
        )

    def _render_page_path(self, temp_dir: str, image: PIL.Image.Image, page_number: int = 1) -> str:
        path = os.path.join(temp_dir, f"page-{page_number}.png")
        image.save(path)
        return path

    def test_missing_file_field_returns_400(self):
        # Send as multipart (files= dict forces multipart) but omit the 'file' field
        resp = self.client.post(
            "/ocr/pdf",
            files={"other": ("x.txt", b"x", "text/plain")},
            data={"lang": "auto", "dpi": "200"},
        )
        self.assertEqual(resp.status_code, 400)

    def test_wrong_content_type_returns_415(self):
        import json
        resp = self.client.post(
            "/ocr/pdf",
            content=json.dumps({"pdf_b64": "abc"}),
            headers={"Content-Type": "text/plain"},
        )
        self.assertEqual(resp.status_code, 415)

    def test_successful_pdf_returns_200_with_expected_fields(self):
        p1 = [[([[0,0],[1,0],[1,1],[0,1]], ("Integration test", 0.95))]]
        fake_ocr = mock.MagicMock()
        fake_ocr.ocr = mock.MagicMock(return_value=p1)

        with tempfile.TemporaryDirectory() as temp_dir:
            page_path = self._render_page_path(temp_dir, _fake_pil_image())
            with mock.patch.object(ocr_app, "pdfinfo_from_path", return_value={"Pages": "1"}), \
                 mock.patch.object(ocr_app, "convert_from_path", return_value=[page_path]), \
                 mock.patch.object(ocr_app, "get_ocr", return_value=fake_ocr):
                resp = self._post_pdf()

        self.assertEqual(resp.status_code, 200)
        body = resp.json()
        self.assertIn("text", body)
        self.assertIn("confidence", body)
        self.assertIn("pages", body)
        self.assertIn("Integration test", body["text"])

    def test_invalid_dpi_returns_422(self):
        resp = self._post_pdf(dpi="9999")
        self.assertEqual(resp.status_code, 422)

    def test_temp_file_deleted_after_request(self):
        created: list[str] = []
        original_write = ocr_app._write_upload_to_temp_pdf

        def tracking_write(upload):
            path = original_write(upload)
            created.append(path)
            return path

        fake_ocr = mock.MagicMock()
        fake_ocr.ocr = mock.MagicMock(return_value=[[]])

        with tempfile.TemporaryDirectory() as temp_dir:
            page_path = self._render_page_path(temp_dir, _fake_pil_image())
            with mock.patch.object(ocr_app, "_write_upload_to_temp_pdf", side_effect=tracking_write), \
                 mock.patch.object(ocr_app, "pdfinfo_from_path", return_value={"Pages": "1"}), \
                 mock.patch.object(ocr_app, "convert_from_path", return_value=[page_path]), \
                 mock.patch.object(ocr_app, "get_ocr", return_value=fake_ocr):
                self._post_pdf()

        self.assertTrue(len(created) == 1, "Expected exactly one temp file created")
        self.assertFalse(
            os.path.exists(created[0]),
            f"Temp file {created[0]} was NOT cleaned up after the request",
        )

    def test_file_size_limit_returns_413(self):
        with mock.patch.object(ocr_app, "MAX_PDF_UPLOAD_BYTES", 10):
            resp = self._post_pdf(content=b"x" * 11)

        self.assertEqual(resp.status_code, 413)
        self.assertIn("max size", resp.json()["detail"])

    def test_page_limit_returns_413(self):
        with mock.patch.object(ocr_app, "MAX_PDF_PAGES", 200), \
             mock.patch.object(ocr_app, "pdfinfo_from_path", return_value={"Pages": "201"}):
            resp = self._post_pdf()

        self.assertEqual(resp.status_code, 413)
        self.assertIn("max page count", resp.json()["detail"])

    def test_shared_file_path_json_returns_200(self):
        with tempfile.TemporaryDirectory() as shared_dir:
            file_path = os.path.join(shared_dir, "sample.pdf")
            with open(file_path, "wb") as f:
                f.write(b"%PDF-1.4")

            fake_ocr = mock.MagicMock()
            fake_ocr.ocr = mock.MagicMock(return_value=[[([[0,0],[1,0],[1,1],[0,1]], ("Shared path", 0.95))]])

            with tempfile.TemporaryDirectory() as render_dir:
                page_path = self._render_page_path(render_dir, _fake_pil_image())
                with mock.patch.object(ocr_app, "OCR_SHARED_STORAGE_DIR", os.path.realpath(shared_dir)), \
                     mock.patch.object(ocr_app, "pdfinfo_from_path", return_value={"Pages": "1"}), \
                     mock.patch.object(ocr_app, "convert_from_path", return_value=[page_path]), \
                     mock.patch.object(ocr_app, "get_ocr", return_value=fake_ocr):
                    resp = self.client.post(
                        "/ocr/pdf",
                        json={"file_path": file_path, "lang": "en", "dpi": 200},
                    )

        self.assertEqual(resp.status_code, 200)
        self.assertIn("Shared path", resp.json()["text"])

    def test_shared_file_path_outside_root_returns_400(self):
        with tempfile.TemporaryDirectory() as shared_dir, tempfile.NamedTemporaryFile(suffix=".pdf") as temp_pdf:
            with mock.patch.object(ocr_app, "OCR_SHARED_STORAGE_DIR", os.path.realpath(shared_dir)):
                resp = self.client.post(
                    "/ocr/pdf",
                    json={"file_path": temp_pdf.name, "lang": "en", "dpi": 200},
                )

        self.assertEqual(resp.status_code, 400)

    def test_json_base64_pdf_returns_200(self):
        fake_ocr = mock.MagicMock()
        fake_ocr.ocr = mock.MagicMock(return_value=[[([[0,0],[1,0],[1,1],[0,1]], ("Base64 path", 0.9))]])

        with tempfile.TemporaryDirectory() as temp_dir:
            page_path = self._render_page_path(temp_dir, _fake_pil_image())
            with mock.patch.object(ocr_app, "pdfinfo_from_path", return_value={"Pages": "1"}), \
                 mock.patch.object(ocr_app, "convert_from_path", return_value=[page_path]), \
                 mock.patch.object(ocr_app, "get_ocr", return_value=fake_ocr):
                resp = self.client.post(
                    "/ocr/pdf",
                    json={"pdf_b64": "JVBERi0xLjQKJSVFT0YK", "lang": "en", "dpi": 200},
                )

        self.assertEqual(resp.status_code, 200)
        self.assertIn("Base64 path", resp.json()["text"])


# ---------------------------------------------------------------------------
# Tests: /health endpoint enrichment
# ---------------------------------------------------------------------------

class TestHealthEndpoint(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        try:
            from fastapi.testclient import TestClient
            cls.client = TestClient(ocr_app.app)
        except ImportError:
            raise unittest.SkipTest("fastapi[testclient] not available")

    def test_health_returns_200(self):
        resp = self.client.get("/health")
        self.assertEqual(resp.status_code, 200)

    def test_health_contains_required_fields(self):
        resp = self.client.get("/health")
        body = resp.json()
        self.assertEqual(body["status"], "ok")
        self.assertEqual(body["service"], "loklingo-ocr")
        self.assertIn("gpu_available", body)
        self.assertIn("models_warmed", body)
        self.assertIn("models_warmed_count", body)

    def test_health_models_warmed_reflects_cache(self):
        """Warm a fake model into the cache; health should reflect it."""
        with mock.patch.dict(ocr_app._ocr_cache, {"ch": object()}):
            resp = self.client.get("/health")
        body = resp.json()
        self.assertIn("ch", body["models_warmed"])
        self.assertGreaterEqual(body["models_warmed_count"], 1)

    def test_health_gpu_available_is_bool(self):
        resp = self.client.get("/health")
        self.assertIsInstance(resp.json()["gpu_available"], bool)


# ---------------------------------------------------------------------------
# Tests: RTL reading order
# ---------------------------------------------------------------------------

class TestRTLReadingOrder(unittest.TestCase):
    def _block(self, text: str, x1: float, y1: float, x2: float, y2: float):
        return ocr_app.TextBlock(text=text, confidence=0.9, bbox=[x1, y1, x2, y2])

    def test_ltr_columns_left_to_right(self):
        # Two columns: left column block and right column block
        blocks = [
            self._block("right-col", 500, 10, 590, 30),
            self._block("left-col", 10, 10, 90, 30),
        ]
        ordered = ocr_app._assign_reading_order_with_layout(blocks, image_width=600, image_height=100, lang="en")
        self.assertEqual(ordered[0].text, "left-col")
        self.assertEqual(ordered[1].text, "right-col")

    def test_rtl_columns_right_to_left(self):
        blocks = [
            self._block("right-col", 500, 10, 590, 30),
            self._block("left-col", 10, 10, 90, 30),
        ]
        ordered = ocr_app._assign_reading_order_with_layout(blocks, image_width=600, image_height=100, lang="ar")
        self.assertEqual(ordered[0].text, "right-col")
        self.assertEqual(ordered[1].text, "left-col")

    def test_rtl_within_row_right_to_left(self):
        # Same visual row (overlapping y-spans), two blocks side by side
        blocks = [
            self._block("left-word", 10, 10, 90, 30),
            self._block("right-word", 110, 12, 190, 32),
        ]
        ordered = ocr_app._assign_reading_order_with_layout(blocks, image_width=200, image_height=100, lang="ur")
        self.assertEqual(ordered[0].text, "right-word")
        self.assertEqual(ordered[1].text, "left-word")

    def test_ltr_within_row_unchanged(self):
        blocks = [
            self._block("left-word", 10, 10, 90, 30),
            self._block("right-word", 110, 12, 190, 32),
        ]
        ordered = ocr_app._assign_reading_order_with_layout(blocks, image_width=200, image_height=100, lang="de")
        self.assertEqual(ordered[0].text, "left-word")
        self.assertEqual(ordered[1].text, "right-word")

    def test_is_rtl_lang(self):
        self.assertTrue(ocr_app._is_rtl_lang("ar"))
        self.assertTrue(ocr_app._is_rtl_lang("UR"))
        self.assertFalse(ocr_app._is_rtl_lang("de"))
        self.assertFalse(ocr_app._is_rtl_lang(""))


# ---------------------------------------------------------------------------
# Tests: LLM correction similarity guard + language gate
# ---------------------------------------------------------------------------

class _FakeCorrectionProvider:
    """Provider stub returning canned corrected lines."""

    def __init__(self, lines: list[str]):
        self._lines = lines

    def name(self) -> str:
        return "fake"

    def check_model_available(self, model: str, timeout_sec: int) -> tuple[bool, str]:
        return True, ""

    def correct_chunk(self, *, model, prompt, image, timeout_sec, max_retries):
        return self._lines, 0, None


class TestCorrectionSimilarityGuard(unittest.TestCase):
    def _blocks(self, texts: list[str], confidence: float = 0.5):
        return [
            ocr_app.TextBlock(text=t, confidence=confidence, bbox=[0.0, 0.0, 10.0, 10.0])
            for t in texts
        ]

    def _run_correction(self, raw: list[str], corrected: list[str]):
        provider = _FakeCorrectionProvider(corrected)
        with mock.patch.object(ocr_app, "OCR_CORRECTION_ENABLED", True), \
             mock.patch.object(ocr_app, "_get_correction_provider", return_value=provider), \
             mock.patch.object(ocr_app, "_check_model_available_cached", return_value=(True, "")):
            return ocr_app._apply_llm_correction(
                image=_fake_pil_image(),
                blocks=self._blocks(raw),
                lang="de",
                mode="ocr_only",
                enabled_override=None,
                model_override=None,
            )

    def test_hallucinated_line_rejected(self):
        raw = ["Rechnung Nr. 1245"]
        hallucinated = ["Completely different sentence with no overlap"]
        blocks, stats, _ = self._run_correction(raw, hallucinated)
        self.assertEqual(blocks[0].text, raw[0])
        self.assertEqual(stats.rejected_lines, 1)
        self.assertFalse(stats.applied)

    def test_plausible_correction_accepted(self):
        raw = ["Rechnung Nr. I245"]  # OCR confuses 1/I
        fixed = ["Rechnung Nr. 1245"]
        blocks, stats, _ = self._run_correction(raw, fixed)
        self.assertEqual(blocks[0].text, fixed[0])
        self.assertEqual(stats.rejected_lines, 0)
        self.assertTrue(stats.applied)

    def test_correction_language_gate(self):
        self.assertTrue(ocr_app._should_apply_correction(lang="de", mode="ocr_only", enabled_override=None) or not ocr_app.OCR_CORRECTION_ENABLED)
        with mock.patch.object(ocr_app, "OCR_CORRECTION_ENABLED", True), \
             mock.patch.object(ocr_app, "OCR_CORRECTION_LANGS", {"de", "auto", "ar"}):
            self.assertTrue(ocr_app._should_apply_correction(lang="ar", mode="ocr_only", enabled_override=None))
            self.assertFalse(ocr_app._should_apply_correction(lang="fr", mode="ocr_only", enabled_override=None))
            self.assertFalse(ocr_app._should_apply_correction(lang="ar", mode="fast", enabled_override=None))

    def test_correction_stats_include_rejected_lines(self):
        stats = ocr_app.CorrectionStats(rejected_lines=3)
        payload = ocr_app._dict_from_stats(stats)
        self.assertEqual(payload["rejected_lines"], 3)


# ---------------------------------------------------------------------------
# Tests: VLM OCR fallback
# ---------------------------------------------------------------------------

class TestVLMFallback(unittest.TestCase):
    def test_gate_requires_enabled(self):
        with mock.patch.object(ocr_app, "OCR_VLM_FALLBACK_ENABLED", False):
            self.assertFalse(ocr_app._should_vlm_fallback("ar", 0.1))

    def test_gate_requires_eligible_lang(self):
        with mock.patch.object(ocr_app, "OCR_VLM_FALLBACK_ENABLED", True), \
             mock.patch.object(ocr_app, "OCR_VLM_LANGS", {"ar", "ur"}):
            self.assertFalse(ocr_app._should_vlm_fallback("de", 0.1))
            self.assertTrue(ocr_app._should_vlm_fallback("ar", 0.1))

    def test_gate_requires_low_confidence(self):
        with mock.patch.object(ocr_app, "OCR_VLM_FALLBACK_ENABLED", True), \
             mock.patch.object(ocr_app, "OCR_VLM_CONFIDENCE_THRESHOLD", 0.72):
            self.assertFalse(ocr_app._should_vlm_fallback("ar", 0.9))
            self.assertTrue(ocr_app._should_vlm_fallback("ar", 0.5))

    def test_vlm_text_to_blocks_shape_and_rtl_order(self):
        image = _fake_pil_image()
        blocks = ocr_app._vlm_text_to_blocks("سطر أول\nسطر ثان", image, "ar")
        self.assertEqual(len(blocks), 2)
        self.assertEqual(blocks[0].text, "سطر أول")
        self.assertEqual(blocks[0].reading_order, 1)
        self.assertAlmostEqual(blocks[0].confidence, ocr_app.OCR_VLM_BLOCK_CONFIDENCE, places=4)
        self.assertEqual(blocks[0].bbox[2], float(image.width))

    def test_vlm_text_to_blocks_empty(self):
        self.assertEqual(ocr_app._vlm_text_to_blocks("  \n  ", _fake_pil_image(), "ar"), [])

    def test_fallback_replaces_low_confidence_paddle(self):
        fake_ocr = mock.MagicMock()
        fake_ocr.ocr = mock.MagicMock(return_value=_make_ocr_result(["garbled"], confidence=0.3))
        with mock.patch.object(ocr_app, "get_ocr", return_value=fake_ocr), \
             mock.patch.object(ocr_app, "OCR_VLM_FALLBACK_ENABLED", True), \
             mock.patch.object(ocr_app, "OCR_VLM_LANGS", {"ar"}), \
             mock.patch.object(ocr_app, "_vlm_ocr_image", return_value="نص عربي سليم"):
            blocks = ocr_app._ocr_pil_image(_fake_pil_image(), "ar", min_confidence=0.8)
        self.assertEqual(len(blocks), 1)
        self.assertEqual(blocks[0].text, "نص عربي سليم")

    def test_fallback_failure_keeps_paddle_result(self):
        fake_ocr = mock.MagicMock()
        fake_ocr.ocr = mock.MagicMock(return_value=_make_ocr_result(["garbled"], confidence=0.3))
        with mock.patch.object(ocr_app, "get_ocr", return_value=fake_ocr), \
             mock.patch.object(ocr_app, "OCR_VLM_FALLBACK_ENABLED", True), \
             mock.patch.object(ocr_app, "OCR_VLM_LANGS", {"ar"}), \
             mock.patch.object(ocr_app, "_vlm_ocr_image", return_value=None):
            blocks = ocr_app._ocr_pil_image(_fake_pil_image(), "ar", min_confidence=0.8)
        self.assertEqual(len(blocks), 1)
        self.assertEqual(blocks[0].text, "garbled")

    def test_fallback_not_triggered_for_ineligible_lang(self):
        fake_ocr = mock.MagicMock()
        fake_ocr.ocr = mock.MagicMock(return_value=_make_ocr_result(["unsicher"], confidence=0.3))
        with mock.patch.object(ocr_app, "get_ocr", return_value=fake_ocr), \
             mock.patch.object(ocr_app, "OCR_VLM_FALLBACK_ENABLED", True), \
             mock.patch.object(ocr_app, "OCR_VLM_LANGS", {"ar"}), \
             mock.patch.object(ocr_app, "_vlm_ocr_image", return_value="should not be used") as vlm_mock:
            blocks = ocr_app._ocr_pil_image(_fake_pil_image(), "de", min_confidence=0.8)
        vlm_mock.assert_not_called()
        self.assertEqual(blocks[0].text, "unsicher")

    def test_health_includes_vlm_fields(self):
        try:
            from fastapi.testclient import TestClient
        except ImportError:
            self.skipTest("fastapi TestClient not available")
        client = TestClient(ocr_app.app)
        body = client.get("/health").json()
        self.assertIn("ocr_vlm_fallback_enabled", body)
        self.assertIn("ocr_vlm_model", body)
        self.assertIn("ocr_vlm_langs", body)


# ---------------------------------------------------------------------------
# Tests: Surya engine + ensemble merge
# ---------------------------------------------------------------------------

class TestSuryaFallback(unittest.TestCase):
    def test_gate(self):
        with mock.patch.object(ocr_app, "OCR_SURYA_ENABLED", False):
            self.assertFalse(ocr_app._should_surya_fallback("ar", 0.1))
        with mock.patch.object(ocr_app, "OCR_SURYA_ENABLED", True), \
             mock.patch.object(ocr_app, "OCR_SURYA_LANGS", {"ar", "hi"}), \
             mock.patch.object(ocr_app, "OCR_SURYA_CONFIDENCE_THRESHOLD", 0.72):
            self.assertTrue(ocr_app._should_surya_fallback("ar", 0.5))
            self.assertFalse(ocr_app._should_surya_fallback("de", 0.5))
            self.assertFalse(ocr_app._should_surya_fallback("ar", 0.9))

    def test_surya_replaces_low_confidence_paddle(self):
        fake_ocr = mock.MagicMock()
        fake_ocr.ocr = mock.MagicMock(return_value=_make_ocr_result(["garbled"], confidence=0.3))
        surya_blocks = [ocr_app.TextBlock(text="نص سليم", confidence=0.9, bbox=[0.0, 0.0, 100.0, 50.0])]
        with mock.patch.object(ocr_app, "get_ocr", return_value=fake_ocr), \
             mock.patch.object(ocr_app, "OCR_SURYA_ENABLED", True), \
             mock.patch.object(ocr_app, "OCR_SURYA_LANGS", {"ar"}), \
             mock.patch.object(ocr_app, "OCR_VLM_FALLBACK_ENABLED", False), \
             mock.patch.object(ocr_app, "OCR_ENSEMBLE_ENABLED", False), \
             mock.patch.object(ocr_app, "_surya_ocr_image", return_value=surya_blocks):
            blocks = ocr_app._ocr_pil_image(_fake_pil_image(), "ar", min_confidence=0.8)
        self.assertEqual(blocks[0].text, "نص سليم")

    def test_surya_failure_falls_through_to_paddle(self):
        fake_ocr = mock.MagicMock()
        fake_ocr.ocr = mock.MagicMock(return_value=_make_ocr_result(["garbled"], confidence=0.3))
        with mock.patch.object(ocr_app, "get_ocr", return_value=fake_ocr), \
             mock.patch.object(ocr_app, "OCR_SURYA_ENABLED", True), \
             mock.patch.object(ocr_app, "OCR_SURYA_LANGS", {"ar"}), \
             mock.patch.object(ocr_app, "OCR_VLM_FALLBACK_ENABLED", False), \
             mock.patch.object(ocr_app, "_surya_ocr_image", return_value=None):
            blocks = ocr_app._ocr_pil_image(_fake_pil_image(), "ar", min_confidence=0.8)
        self.assertEqual(blocks[0].text, "garbled")


class TestEnsembleMerge(unittest.TestCase):
    def _block(self, text, conf, bbox):
        return ocr_app.TextBlock(text=text, confidence=conf, bbox=bbox)

    def test_iou(self):
        a = [0.0, 0.0, 10.0, 10.0]
        b = [5.0, 0.0, 15.0, 10.0]
        self.assertAlmostEqual(ocr_app._bbox_iou(a, b), 1 / 3, places=3)
        self.assertEqual(ocr_app._bbox_iou(a, [20.0, 0.0, 30.0, 10.0]), 0.0)

    def test_higher_confidence_wins_on_overlap(self):
        paddle = [self._block("paddle-text", 0.5, [0, 0, 100, 20])]
        surya = [self._block("surya-text", 0.9, [5, 2, 95, 18])]
        merged = ocr_app._ensemble_merge(paddle, surya)
        self.assertEqual(len(merged), 1)
        self.assertEqual(merged[0].text, "surya-text")
        self.assertEqual(merged[0].reading_order, 1)

    def test_paddle_kept_when_more_confident(self):
        paddle = [self._block("paddle-text", 0.95, [0, 0, 100, 20])]
        surya = [self._block("surya-text", 0.6, [5, 2, 95, 18])]
        merged = ocr_app._ensemble_merge(paddle, surya)
        self.assertEqual(merged[0].text, "paddle-text")

    def test_unmatched_blocks_kept(self):
        paddle = [self._block("p1", 0.9, [0, 0, 100, 20])]
        surya = [self._block("s1", 0.9, [0, 100, 100, 120])]
        merged = ocr_app._ensemble_merge(paddle, surya)
        self.assertEqual(len(merged), 2)
        self.assertEqual([b.reading_order for b in merged], [1, 2])


# ---------------------------------------------------------------------------
# Tests: eval harness metrics
# ---------------------------------------------------------------------------

class TestEvalMetrics(unittest.TestCase):
    def test_cer_wer_perfect(self):
        import eval_ocr
        self.assertEqual(eval_ocr.cer("مرحبا بالعالم", "مرحبا بالعالم"), 0.0)
        self.assertEqual(eval_ocr.wer("مرحبا بالعالم", "مرحبا بالعالم"), 0.0)

    def test_cer_single_substitution(self):
        import eval_ocr
        self.assertAlmostEqual(eval_ocr.cer("abcde", "abXde"), 0.2, places=4)

    def test_wer_single_substitution(self):
        import eval_ocr
        self.assertAlmostEqual(eval_ocr.wer("one two three four", "one two X four"), 0.25, places=4)

    def test_empty_reference(self):
        import eval_ocr
        self.assertEqual(eval_ocr.cer("", ""), 0.0)
        self.assertEqual(eval_ocr.cer("", "extra"), 1.0)

    def test_normalization_collapses_whitespace(self):
        import eval_ocr
        self.assertEqual(eval_ocr.cer("a  b\n c", "a b c"), 0.0)


if __name__ == "__main__":
    unittest.main()
